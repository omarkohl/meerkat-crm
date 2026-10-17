package controllers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type trashEnv struct {
	db      *gorm.DB
	userA   models.User
	routerA *gin.Engine
	photos  string
}

func newTrashEnv(t *testing.T) *trashEnv {
	t.Helper()
	db, routerA := setupRouter()
	require.NoError(t, db.AutoMigrate(&models.ReminderCompletion{}))
	var a models.User
	db.First(&a)
	photos := t.TempDir()
	cfg := &config.Config{ProfilePhotoDir: photos, TrashRetentionDays: 30, WebhookBlockPrivateURLs: true}
	mountContacts(routerA, db, cfg)
	mountTrash(routerA, db, cfg)
	return &trashEnv{db: db, userA: a, routerA: routerA, photos: photos}
}

func (e *trashEnv) contact(t *testing.T, userID uint, name string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: userID, Firstname: name, Circles: []string{"Circle-" + name}}
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

type trashResponse struct {
	Contacts []struct {
		Firstname string    `json:"firstname"`
		DeletedAt time.Time `json:"deleted_at"`
		PurgeAt   time.Time `json:"purge_at"`
	} `json:"contacts"`
	RetentionDays int `json:"retention_days"`
}

func (e *trashEnv) trashed(t *testing.T, r *gin.Engine) trashResponse {
	t.Helper()
	w := doReq(r, "GET", "/trash/contacts")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp trashResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func (e *trashEnv) liveContacts(t *testing.T, r *gin.Engine) int {
	t.Helper()
	w := doReq(r, "GET", "/contacts")
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Total
}

func idPath(prefix string, id uint, suffix string) string {
	return prefix + strconv.Itoa(int(id)) + suffix
}

func TestDeleteMovesContactToTrashAndKeepsItsData(t *testing.T) {
	e := newTrashEnv(t)
	c := e.contact(t, e.userA.ID, "Alice")
	cid := c.ID
	note := models.Note{UserID: e.userA.ID, Content: "hi", Date: time.Now(), ContactID: &cid}
	require.NoError(t, e.db.Create(&note).Error)
	act := models.Activity{UserID: e.userA.ID, Title: "Lunch", Date: time.Now()}
	require.NoError(t, e.db.Create(&act).Error)
	require.NoError(t, e.db.Model(&act).Association("Contacts").Append(&c))
	require.NoError(t, os.WriteFile(filepath.Join(e.photos, "p.jpg"), []byte("x"), 0o600))
	require.NoError(t, e.db.Model(&c).Update("photo", "p.jpg").Error)

	require.Equal(t, http.StatusOK, doReq(e.routerA, "DELETE", idPath("/contacts/", c.ID, "")).Code)

	assert.Equal(t, 0, e.liveContacts(t, e.routerA))
	resp := e.trashed(t, e.routerA)
	require.Len(t, resp.Contacts, 1)
	assert.Equal(t, "Alice", resp.Contacts[0].Firstname)
	assert.Equal(t, 30, resp.RetentionDays)
	assert.WithinDuration(t, resp.Contacts[0].DeletedAt.Add(30*24*time.Hour), resp.Contacts[0].PurgeAt, time.Second)
	// The activity link and the photo wait in the trash.
	var joins int64
	require.NoError(t, e.db.Table("activity_contacts").Where("contact_id = ?", c.ID).Count(&joins).Error)
	assert.EqualValues(t, 1, joins)
	_, err := os.Stat(filepath.Join(e.photos, "p.jpg"))
	assert.NoError(t, err)
}

func TestDeletedContactIsHiddenFromCircles(t *testing.T) {
	e := newTrashEnv(t)
	c := e.contact(t, e.userA.ID, "Alice")
	e.contact(t, e.userA.ID, "Bob")
	require.Equal(t, http.StatusOK, doReq(e.routerA, "DELETE", idPath("/contacts/", c.ID, "")).Code)

	w := doReq(e.routerA, "GET", "/contacts/circles")
	var circles []string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &circles))
	assert.Equal(t, []string{"Circle-Bob"}, circles)
}

func TestRestoreFromTrashMakesContactVisibleAgain(t *testing.T) {
	e := newTrashEnv(t)
	c := e.contact(t, e.userA.ID, "Alice")
	doReq(e.routerA, "DELETE", idPath("/contacts/", c.ID, ""))

	w := doReq(e.routerA, "POST", idPath("/trash/contacts/", c.ID, "/restore"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Equal(t, 1, e.liveContacts(t, e.routerA))
	assert.Empty(t, e.trashed(t, e.routerA).Contacts)
	// A second restore finds nothing in the trash.
	assert.Equal(t, http.StatusNotFound, doReq(e.routerA, "POST", idPath("/trash/contacts/", c.ID, "/restore")).Code)
}

func TestWebhookInputAcceptsContactRestored(t *testing.T) {
	assert.Empty(t, middleware.ValidateStruct(models.WebhookInput{Name: "n", URL: "https://example.com", Events: []string{"contact.restored"}}))
}

func TestPurgeSingleContact(t *testing.T) {
	e := newTrashEnv(t)
	c1 := e.contact(t, e.userA.ID, "Alice")
	c2 := e.contact(t, e.userA.ID, "Bob")
	require.NoError(t, os.WriteFile(filepath.Join(e.photos, "p.jpg"), []byte("x"), 0o600))
	require.NoError(t, e.db.Model(&c1).Update("photo", "p.jpg").Error)
	doReq(e.routerA, "DELETE", idPath("/contacts/", c1.ID, ""))
	doReq(e.routerA, "DELETE", idPath("/contacts/", c2.ID, ""))

	require.Equal(t, http.StatusOK, doReq(e.routerA, "DELETE", idPath("/trash/contacts/", c1.ID, "")).Code)

	resp := e.trashed(t, e.routerA)
	require.Len(t, resp.Contacts, 1)
	assert.Equal(t, "Bob", resp.Contacts[0].Firstname)
	_, err := os.Stat(filepath.Join(e.photos, "p.jpg"))
	assert.True(t, os.IsNotExist(err))
	assert.Equal(t, http.StatusNotFound, doReq(e.routerA, "DELETE", idPath("/trash/contacts/", c1.ID, "")).Code)
}

func TestTrashedContactIsNotFoundBySearchAndActivities(t *testing.T) {
	e := newTrashEnv(t)
	mountActivities(e.routerA, e.db, &config.Config{})
	c := e.contact(t, e.userA.ID, "Zebulon")
	act := models.Activity{UserID: e.userA.ID, Title: "Picnic", Date: time.Now()}
	require.NoError(t, e.db.Create(&act).Error)
	require.NoError(t, e.db.Model(&act).Association("Contacts").Append(&c))

	search := func() int {
		w := doReq(e.routerA, "GET", "/activities?search=zebulon")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Activities []models.Activity `json:"activities"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return len(resp.Activities)
	}
	assert.Equal(t, 1, search())
	doReq(e.routerA, "DELETE", idPath("/contacts/", c.ID, ""))
	assert.Equal(t, 0, search(), "activity search must not match a trashed contact's name")
	doReq(e.routerA, "POST", idPath("/trash/contacts/", c.ID, "/restore"))
	assert.Equal(t, 1, search())
}
