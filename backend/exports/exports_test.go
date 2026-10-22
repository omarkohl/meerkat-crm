package exports_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"meerkat/clock"
	"meerkat/config"
	"meerkat/exports"
	"meerkat/middleware"
	"meerkat/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var now = time.Date(2026, 3, 10, 15, 30, 0, 0, time.UTC)

type env struct {
	db   *gorm.DB
	a, b models.User
	asA  *gin.Engine
	asB  *gin.Engine
	anon *gin.Engine
}

func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Relationship{}, &models.Activity{}, &models.Note{},
		&models.Reminder{}, &models.AuditLog{}))
	e := &env{db: db}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x", CustomFieldNames: []string{"Pet"}}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)
	mount := func(userID uint) *gin.Engine {
		r := gin.New()
		r.Use(middleware.ClockMiddleware(clock.NewFake(now)))
		authenticated := r.Group("")
		if userID != 0 {
			authenticated.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
		}
		exports.New(exports.Deps{DB: db, Config: &config.Config{ProfilePhotoDir: t.TempDir()}}).Register(authenticated)
		return r
	}
	e.asA, e.asB, e.anon = mount(e.a.ID), mount(e.b.ID), mount(0)
	return e
}

func get(r http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}

func (e *env) seed(t *testing.T) {
	t.Helper()
	endDay := now.AddDate(0, 0, 3)
	alice := models.Contact{UserID: e.a.ID, Firstname: "Alice", Lastname: "Johnson", Email: "alice@example.com", Circles: []string{"Friends", "Work"},
		CustomFields: map[string]string{"Pet": "cat"}}
	require.NoError(t, e.db.Create(&alice).Error)
	require.NoError(t, e.db.Create(&models.Contact{UserID: e.a.ID, Firstname: "Bob", Lastname: "Smith"}).Error)
	require.NoError(t, e.db.Create(&models.Relationship{UserID: e.a.ID, ContactID: alice.ID, Name: "Charlie", Type: "Friend"}).Error)
	require.NoError(t, e.db.Create(&models.Activity{UserID: e.a.ID, Title: "Coffee Meeting", Description: "Catch up", Location: "Local Cafe",
		Date: now, EndDate: &endDay, Contacts: []models.Contact{alice}}).Error)
	require.NoError(t, e.db.Create(&models.Note{UserID: e.a.ID, ContactID: &alice.ID, Content: "Follow up about the project", Date: now}).Error)
	require.NoError(t, e.db.Create(&models.Reminder{UserID: e.a.ID, ContactID: &alice.ID, Message: "Birthday reminder", RemindAt: now, Recurrence: "yearly"}).Error)
}

// @016-US1
func TestCSVExportContainsEverySectionOfTheUsersData(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	w := get(e.asA, "/export")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "text/csv; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "attachment; filename=meerkat-export-2026-03-10.csv", w.Header().Get("Content-Disposition"))
	body := w.Body.String()
	for _, want := range []string{
		"=== CONTACTS ===", "Alice", "alice@example.com", "Friends; Work", ",Pet", ",cat", "Bob",
		"=== RELATIONSHIPS ===", "Charlie", "Friend", "Alice Johnson",
		"=== ACTIVITIES ===", "Coffee Meeting", "Local Cafe", ",End Date,", "2026-03-13T",
		"=== NOTES ===", "Follow up about the project",
		"=== REMINDERS ===", "Birthday reminder", "yearly",
	} {
		assert.Contains(t, body, want)
	}
}

func TestCSVExportOfAnEmptyAccountStillHasTheSections(t *testing.T) {
	e := newEnv(t)
	body := get(e.asB, "/export").Body.String()
	for _, s := range []string{"=== CONTACTS ===", "=== RELATIONSHIPS ===", "=== ACTIVITIES ===", "=== NOTES ===", "=== REMINDERS ==="} {
		assert.Contains(t, body, s)
	}
}

func TestExportsNeverContainAnotherUsersData(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	require.NoError(t, e.db.Create(&models.Contact{UserID: e.b.ID, Firstname: "OtherUserContact"}).Error)
	for _, path := range []string{"/export", "/export/vcf"} {
		body := get(e.asA, path).Body.String()
		assert.Contains(t, body, "Alice", path)
		assert.NotContains(t, body, "OtherUserContact", path)
		other := get(e.asB, path).Body.String()
		assert.Contains(t, other, "OtherUserContact", path)
		assert.NotContains(t, other, "Alice", path)
	}
}

// @016-US1
func TestVCardExportListsEveryContact(t *testing.T) {
	e := newEnv(t)
	e.seed(t)
	w := get(e.asA, "/export/vcf")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "text/vcard; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "attachment; filename=meerkat-contacts-2026-03-10.vcf", w.Header().Get("Content-Disposition"))
	assert.Equal(t, 2, strings.Count(w.Body.String(), "BEGIN:VCARD"))
	assert.Contains(t, w.Body.String(), "Alice")
}

func TestExportsAreAuditedWithTheirFormat(t *testing.T) {
	e := newEnv(t)
	get(e.asA, "/export")
	get(e.asA, "/export/vcf")
	var got []models.AuditLog
	require.NoError(t, e.db.Order("id").Find(&got).Error)
	require.Len(t, got, 2)
	assert.Equal(t, []string{"format:csv", "format:vcf"}, []string{got[0].Detail, got[1].Detail})
	assert.Equal(t, models.AuditExport, got[0].Event)
	assert.Equal(t, "a", got[0].Username)
	require.NotNil(t, got[0].UserID)
	assert.Equal(t, e.a.ID, *got[0].UserID)
}

func TestExportsNeedAUser(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{"/export", "/export/vcf"} {
		assert.Equal(t, http.StatusUnauthorized, get(e.anon, path).Code, path)
	}
	var n int64
	e.db.Model(&models.AuditLog{}).Count(&n)
	assert.Zero(t, n)
}
