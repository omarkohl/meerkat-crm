package trash_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/trash"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var now = time.Date(2026, 3, 10, 15, 30, 0, 0, time.UTC)

type env struct {
	db     *gorm.DB
	a, b   models.User
	photos string
	asA    *gin.Engine
	asB    *gin.Engine
	anon   *gin.Engine
}

func newEnv(t *testing.T, retentionDays int) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Note{}, &models.Reminder{}, &models.ReminderCompletion{},
		&models.Relationship{}, &models.Activity{}, &models.CardDAVContactLink{}, &models.Webhook{}, &models.WebhookDelivery{}))
	e := &env{db: db, photos: t.TempDir()}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)

	cfg := &config.Config{ProfilePhotoDir: e.photos, TrashRetentionDays: retentionDays, WebhookBlockPrivateURLs: true}
	mount := func(userID uint) *gin.Engine {
		r := gin.New()
		r.Use(middleware.ClockMiddleware(clock.NewFake(now)))
		authenticated := r.Group("")
		if userID != 0 {
			authenticated.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
		}
		trash.New(trash.Deps{DB: db, Config: cfg}).Register(api.New(r.Group(""), r), authenticated)
		return r
	}
	e.asA, e.asB, e.anon = mount(e.a.ID), mount(e.b.ID), mount(0)
	return e
}

func (e *env) trashed(t *testing.T, owner uint, name string, deletedAt time.Time) models.Contact {
	t.Helper()
	c := models.Contact{UserID: owner, Firstname: name, Email: name + "@example.com", PhotoThumbnailFile: "t_thumb.jpg"}
	require.NoError(t, e.db.Create(&c).Error)
	require.NoError(t, e.db.Model(&c).UpdateColumn("deleted_at", deletedAt).Error)
	return c
}

func do(r http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m), w.Body.String())
	return m
}

func sid(id uint) string { return strconv.Itoa(int(id)) }

func TestListShowsTheActorsDeletedContactsNewestFirstWithTheirPurgeDate(t *testing.T) {
	e := newEnv(t, 7)
	old := e.trashed(t, e.a.ID, "Old", now.AddDate(0, 0, -3))
	recent := e.trashed(t, e.a.ID, "Recent", now.AddDate(0, 0, -1))
	e.trashed(t, e.b.ID, "Foreign", now)
	live := models.Contact{UserID: e.a.ID, Firstname: "Live"}
	require.NoError(t, e.db.Create(&live).Error)

	w := do(e.asA, http.MethodGet, "/trash/contacts")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.EqualValues(t, 7, body["retention_days"])
	list := body["contacts"].([]any)
	require.Len(t, list, 2)
	first := list[0].(map[string]any)
	assert.EqualValues(t, recent.ID, first["id"])
	assert.Equal(t, "Recent", first["firstname"])
	assert.Equal(t, "Recent@example.com", first["email"])
	assert.Contains(t, first["photo_thumbnail"], "thumbnail=true&v=t")
	assert.Equal(t, now.AddDate(0, 0, -1).Format(time.RFC3339), first["deleted_at"])
	assert.Equal(t, now.AddDate(0, 0, 6).Format(time.RFC3339), first["purge_at"])
	assert.EqualValues(t, old.ID, list[1].(map[string]any)["id"])
}

func TestAnEmptyTrashIsAnEmptyListAndTheRetentionDefaultsTo30Days(t *testing.T) {
	e := newEnv(t, 0)
	body := decode(t, do(e.asA, http.MethodGet, "/trash/contacts"))
	assert.Equal(t, []any{}, body["contacts"])
	assert.EqualValues(t, 30, body["retention_days"])
}

// @008-US1
func TestRestoreBringsTheContactBackAndAnnouncesIt(t *testing.T) {
	e := newEnv(t, 30)
	c := e.trashed(t, e.a.ID, "Alice", now.AddDate(0, 0, -1))
	hook := models.Webhook{UserID: e.a.ID, Name: "h", URL: "http://127.0.0.1/hook", Events: []string{"contact.restored"}, Secret: "s", IsActive: true}
	require.NoError(t, e.db.Create(&hook).Error)

	w := do(e.asA, http.MethodPost, "/trash/contacts/"+sid(c.ID)+"/restore")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Contact restored", body["message"])
	assert.EqualValues(t, c.ID, body["id"])

	var live int64
	e.db.Model(&models.Contact{}).Where("id = ?", c.ID).Count(&live)
	assert.EqualValues(t, 1, live)
	var d models.WebhookDelivery
	require.Eventually(t, func() bool { return e.db.Where("event_type = ?", "contact.restored").First(&d).Error == nil }, 3*time.Second, 20*time.Millisecond)
	var payload struct{ Data map[string]any }
	require.NoError(t, json.Unmarshal([]byte(d.Payload), &payload))
	assert.Equal(t, "Alice", payload.Data["firstname"])
	assert.EqualValues(t, c.ID, payload.Data["ID"])

	assert.Equal(t, http.StatusNotFound, do(e.asA, http.MethodPost, "/trash/contacts/"+sid(c.ID)+"/restore").Code, "a second restore finds nothing")
}

// @008-US2
func TestPurgeDeletesOneTrashedContactAndItsFiles(t *testing.T) {
	e := newEnv(t, 30)
	c := e.trashed(t, e.a.ID, "Alice", now.AddDate(0, 0, -1))
	other := e.trashed(t, e.a.ID, "Bob", now.AddDate(0, 0, -1))
	require.NoError(t, os.WriteFile(filepath.Join(e.photos, "t_thumb.jpg"), []byte("x"), 0o600))

	w := do(e.asA, http.MethodDelete, "/trash/contacts/"+sid(c.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Contact deleted permanently"}`, w.Body.String())

	var gone, kept int64
	e.db.Unscoped().Model(&models.Contact{}).Where("id = ?", c.ID).Count(&gone)
	e.db.Unscoped().Model(&models.Contact{}).Where("id = ?", other.ID).Count(&kept)
	assert.Zero(t, gone)
	assert.EqualValues(t, 1, kept)
	_, err := os.Stat(filepath.Join(e.photos, "t_thumb.jpg"))
	assert.True(t, os.IsNotExist(err))
	assert.Equal(t, http.StatusNotFound, do(e.asA, http.MethodDelete, "/trash/contacts/"+sid(c.ID)).Code)
}

func TestPurgeAndRestoreRefuseLiveForeignAndMalformedContacts(t *testing.T) {
	e := newEnv(t, 30)
	live := models.Contact{UserID: e.a.ID, Firstname: "Live"}
	require.NoError(t, e.db.Create(&live).Error)
	theirs := e.trashed(t, e.b.ID, "Theirs", now.AddDate(0, 0, -1))

	for _, id := range []string{sid(live.ID), sid(theirs.ID), "9999", "abc", "0"} {
		w := do(e.asA, http.MethodDelete, "/trash/contacts/"+id)
		assert.Equal(t, http.StatusNotFound, w.Code, "purge "+id)
		assert.Equal(t, id, decode(t, w)["error"].(map[string]any)["details"].(map[string]any)["id"])
		assert.Equal(t, http.StatusNotFound, do(e.asA, http.MethodPost, "/trash/contacts/"+id+"/restore").Code, "restore "+id)
	}
	var kept int64
	e.db.Unscoped().Model(&models.Contact{}).Where("id IN ?", []uint{live.ID, theirs.ID}).Count(&kept)
	assert.EqualValues(t, 2, kept)
}

func TestEmptyingTheTrashPurgesOnlyTheActorsDeletedContacts(t *testing.T) {
	e := newEnv(t, 30)
	e.trashed(t, e.a.ID, "One", now)
	e.trashed(t, e.a.ID, "Two", now)
	theirs := e.trashed(t, e.b.ID, "Theirs", now)
	live := models.Contact{UserID: e.a.ID, Firstname: "Live"}
	require.NoError(t, e.db.Create(&live).Error)

	w := do(e.asA, http.MethodDelete, "/trash/contacts")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Trash emptied", body["message"])
	assert.EqualValues(t, 2, body["purged"])
	var kept int64
	e.db.Unscoped().Model(&models.Contact{}).Where("id IN ?", []uint{live.ID, theirs.ID}).Count(&kept)
	assert.EqualValues(t, 2, kept)
	assert.EqualValues(t, 0, decode(t, do(e.asA, http.MethodDelete, "/trash/contacts"))["purged"])
}

func TestEverythingRefusesWithoutAnAuthenticatedUser(t *testing.T) {
	e := newEnv(t, 30)
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/trash/contacts"}, {http.MethodDelete, "/trash/contacts"},
		{http.MethodPost, "/trash/contacts/1/restore"}, {http.MethodDelete, "/trash/contacts/1"},
	} {
		assert.Equal(t, http.StatusUnauthorized, do(e.anon, call.method, call.path).Code, call.method+" "+call.path)
	}
}
