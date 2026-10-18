package calendars_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/calendars"
	"meerkat/calendars/internal/domain"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const secret = "unit-test-secret"

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
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Activity{}, &models.CalendarSubscription{}, &models.CalendarEventLink{}))
	e := &env{db: db}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)
	module := calendars.New(calendars.Deps{DB: db, Config: &config.Config{JWTSecretKey: secret}})
	mount := func(userID uint) *gin.Engine {
		r := gin.New()
		r.Use(middleware.ClockMiddleware(clock.NewFake(now)))
		authenticated := r.Group("")
		if userID != 0 {
			authenticated.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
		}
		module.Register(api.New(r.Group(""), r), authenticated)
		return r
	}
	e.asA, e.asB, e.anon = mount(e.a.ID), mount(e.b.ID), mount(0)
	return e
}

func do(r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var rdr *strings.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = strings.NewReader(string(b))
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func idPath(id uint, suffix string) string { return fmt.Sprintf("/calendars/%d%s", id, suffix) }

func (e *env) create(t *testing.T, r http.Handler, body map[string]any) uint {
	t.Helper()
	w := do(r, "POST", "/calendars", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var out struct {
		ID uint `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out.ID
}

func TestPasswordIsEncryptedAtRestAndNeverReturned(t *testing.T) {
	e := newEnv(t)
	w := do(e.asA, "POST", "/calendars", map[string]any{"name": "Work", "url": "https://cal.example.com/a.ics", "username": "u", "password": "hunter2"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "hunter2")
	assert.Contains(t, w.Body.String(), `"has_password":true`)

	var sub models.CalendarSubscription
	require.NoError(t, e.db.Where("user_id = ?", e.a.ID).First(&sub).Error)
	assert.NotContains(t, sub.PasswordEncrypted, "hunter2")
	plain, err := services.DecryptCredential(secret, sub.PasswordEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "hunter2", plain)
	assert.Equal(t, 5, sub.PastDays)
	assert.Equal(t, 10, sub.FutureDays)
	assert.True(t, sub.SyncEnabled)

	list := do(e.asA, "GET", "/calendars", nil)
	assert.NotContains(t, list.Body.String(), "hunter2")
	assert.NotContains(t, list.Body.String(), sub.PasswordEncrypted)
}

func TestListIsScopedOldestFirstAndAnArray(t *testing.T) {
	e := newEnv(t)
	assert.JSONEq(t, `{"calendars":[]}`, do(e.asA, "GET", "/calendars", nil).Body.String())
	e.create(t, e.asA, map[string]any{"name": "first", "url": "https://a.example.com/a.ics"})
	e.create(t, e.asA, map[string]any{"name": "second", "url": "https://b.example.com/a.ics"})
	e.create(t, e.asB, map[string]any{"name": "of b", "url": "https://c.example.com/a.ics"})
	var body struct{ Calendars []struct{ Name string } }
	require.NoError(t, json.Unmarshal(do(e.asA, "GET", "/calendars", nil).Body.Bytes(), &body))
	require.Len(t, body.Calendars, 2)
	assert.Equal(t, "first", body.Calendars[0].Name)
}

func TestOtherSchemesAreRefusedAndValidationRuns(t *testing.T) {
	e := newEnv(t)
	for _, u := range []string{"file:///etc/passwd", "ftp://example.com/x.ics", "gopher://x"} {
		assert.Equal(t, http.StatusBadRequest, do(e.asA, "POST", "/calendars", map[string]any{"name": "x", "url": u}).Code, u)
	}
	assert.Equal(t, http.StatusCreated, do(e.asA, "POST", "/calendars", map[string]any{"name": "x", "url": "webcal://cal.example.com/a.ics"}).Code)
	assert.Equal(t, http.StatusBadRequest, do(e.asA, "POST", "/calendars", map[string]any{"url": "https://x.example"}).Code, "no name")
	assert.Equal(t, http.StatusBadRequest, do(e.asA, "POST", "/calendars", map[string]any{"name": "x", "url": "https://x.example", "past_days": 4000}).Code)
}

func TestAUserCanHaveTenSubscriptions(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 10; i++ {
		e.create(t, e.asA, map[string]any{"name": fmt.Sprint(i), "url": fmt.Sprintf("https://c%d.example.com/a.ics", i)})
	}
	assert.Equal(t, http.StatusConflict, do(e.asA, "POST", "/calendars", map[string]any{"name": "x", "url": "https://x.example"}).Code)
	assert.Equal(t, http.StatusCreated, do(e.asB, "POST", "/calendars", map[string]any{"name": "x", "url": "https://x.example"}).Code)
}

func TestAnotherUsersSubscriptionIs404ForEveryOperation(t *testing.T) {
	e := newEnv(t)
	id := e.create(t, e.asB, map[string]any{"name": "T", "url": "https://t.example.com/x.ics"})
	body := map[string]any{"name": "mine now", "url": "https://evil.example.com/x.ics"}
	assert.Equal(t, http.StatusNotFound, do(e.asA, "PUT", idPath(id, ""), body).Code)
	assert.Equal(t, http.StatusNotFound, do(e.asA, "DELETE", idPath(id, ""), nil).Code)
	assert.Equal(t, http.StatusNotFound, do(e.asA, "POST", idPath(id, "/sync"), nil).Code)
	var reloaded models.CalendarSubscription
	require.NoError(t, e.db.First(&reloaded, id).Error)
	assert.Equal(t, "T", reloaded.Name)
	assert.Equal(t, http.StatusBadRequest, do(e.asA, "DELETE", "/calendars/abc", nil).Code)
}

func TestAStoredPasswordDoesNotFollowAUrlChangeToAnotherServer(t *testing.T) {
	e := newEnv(t)
	id := e.create(t, e.asA, map[string]any{"name": "Work", "url": "https://cal.example.com/a.ics", "username": "u", "password": "hunter2"})
	stored := func() models.CalendarSubscription {
		var sub models.CalendarSubscription
		require.NoError(t, e.db.First(&sub, id).Error)
		return sub
	}

	require.Equal(t, http.StatusOK, do(e.asA, "PUT", idPath(id, ""), map[string]any{"name": "Work", "url": "https://CAL.example.com/b.ics", "username": "u"}).Code)
	assert.NotEmpty(t, stored().PasswordEncrypted)

	for _, target := range []string{"https://attacker.example.net/b.ics", "http://cal.example.com/b.ics", "https://cal.example.com:8443/b.ics"} {
		w := do(e.asA, "PUT", idPath(id, ""), map[string]any{"name": "Work", "url": target, "username": "u"})
		require.Equal(t, http.StatusBadRequest, w.Code, target)
		assert.Contains(t, w.Body.String(), "password required")
		assert.Equal(t, "https://CAL.example.com/b.ics", stored().URL)
		assert.NotEmpty(t, stored().PasswordEncrypted)
	}
	require.Equal(t, http.StatusOK, do(e.asA, "PUT", idPath(id, ""), map[string]any{"name": "Work", "url": "https://other.example.net/b.ics", "username": "u", "password": "new"}).Code)
	plain, err := services.DecryptCredential(secret, stored().PasswordEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "new", plain)
	require.Equal(t, http.StatusOK, do(e.asA, "PUT", idPath(id, ""), map[string]any{"name": "Work", "url": "http://third.example.net/b.ics", "username": "u", "clear_password": true}).Code)
	assert.Empty(t, stored().PasswordEncrypted)
}

func TestUpdateKeepsUnsetOptionsAndChangesSetOnes(t *testing.T) {
	e := newEnv(t)
	id := e.create(t, e.asA, map[string]any{"name": "Work", "url": "https://cal.example.com/a.ics", "past_days": 7})
	w := do(e.asA, "PUT", idPath(id, ""), map[string]any{"name": "Renamed", "url": "https://cal.example.com/a.ics", "sync_enabled": false})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var sub models.CalendarSubscription
	require.NoError(t, e.db.First(&sub, id).Error)
	assert.Equal(t, "Renamed", sub.Name)
	assert.False(t, sub.SyncEnabled)
	assert.Equal(t, 7, sub.PastDays)
}

func TestDeleteRemovesEventLinksButKeepsImportedActivities(t *testing.T) {
	e := newEnv(t)
	id := e.create(t, e.asA, map[string]any{"name": "Work", "url": "https://cal.example.com/a.ics"})
	activity := models.Activity{UserID: e.a.ID, Title: "Imported", Date: now}
	require.NoError(t, e.db.Create(&activity).Error)
	require.NoError(t, e.db.Create(&models.CalendarEventLink{SubscriptionID: id, UserID: e.a.ID, UID: "u1", ActivityID: activity.ID, ContentHash: "h"}).Error)

	w := do(e.asA, "DELETE", idPath(id, ""), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"message":"Calendar subscription deleted"}`, w.Body.String())
	var links, activities int64
	e.db.Model(&models.CalendarEventLink{}).Count(&links)
	e.db.Model(&models.Activity{}).Count(&activities)
	assert.Zero(t, links)
	assert.EqualValues(t, 1, activities)
	assert.Equal(t, http.StatusNotFound, do(e.asA, "DELETE", idPath(id, ""), nil).Code)
}

const ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//EN\r\nBEGIN:VEVENT\r\nUID:e1@example.com\r\nDTSTAMP:20260301T000000Z\r\n" +
	"DTSTART:20260311T100000Z\r\nDTEND:20260311T110000Z\r\nSUMMARY:Standup\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

// @017-US3
func TestSyncImportsEventsAndRecordsTheStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(ics))
	}))
	defer server.Close()
	e := newEnv(t)
	id := e.create(t, e.asA, map[string]any{"name": "Work", "url": server.URL + "/a.ics"})

	w := do(e.asA, "POST", idPath(id, "/sync"), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out struct {
		Message string
		Created int
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "Calendar sync completed", out.Message)
	assert.Equal(t, 1, out.Created)

	var sub models.CalendarSubscription
	require.NoError(t, e.db.First(&sub, id).Error)
	assert.Equal(t, models.CalendarSyncStatusSuccess, sub.LastSyncStatus)
	require.NotNil(t, sub.LastSyncedAt)
	var activity models.Activity
	require.NoError(t, e.db.Where("user_id = ?", e.a.ID).First(&activity).Error)
	assert.Equal(t, "Standup", activity.Title)
}

func TestSyncFailuresAreWordedForTheUser(t *testing.T) {
	status := http.StatusUnauthorized
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
	defer server.Close()
	e := newEnv(t)
	id := e.create(t, e.asA, map[string]any{"name": "Work", "url": server.URL + "/a.ics"})

	w := do(e.asA, "POST", idPath(id, "/sync"), nil)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "authentication failed - check username and password")
	var sub models.CalendarSubscription
	require.NoError(t, e.db.First(&sub, id).Error)
	assert.Equal(t, models.CalendarSyncStatusError, sub.LastSyncStatus)
}

func TestSyncRefusesPrivateAddressesWhenBlocked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(ics)) }))
	defer server.Close()
	e := newEnv(t)
	blocked := calendars.New(calendars.Deps{DB: e.db, Config: &config.Config{JWTSecretKey: secret, CalDAVBlockPrivateURLs: true}})
	r := gin.New()
	r.Use(middleware.ClockMiddleware(clock.NewFake(now)), func(c *gin.Context) { c.Set("userID", e.a.ID); c.Next() })
	blocked.Register(api.New(r.Group(""), r), r.Group(""))
	id := e.create(t, r, map[string]any{"name": "Work", "url": server.URL + "/a.ics"})

	w := do(r, "POST", idPath(id, "/sync"), nil)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "blocked private address")
}

func TestSyncReportsADatabaseErrorAsOne(t *testing.T) {
	e := newEnv(t)
	id := e.create(t, e.asA, map[string]any{"name": "Work", "url": "https://cal.example.com/a.ics"})
	require.NoError(t, e.db.Callback().Query().Before("gorm:query").Register("test:fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "calendar_subscriptions" {
			_ = tx.AddError(fmt.Errorf("boom"))
		}
	}))
	w := do(e.asA, "POST", idPath(id, "/sync"), nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "DATABASE_ERROR")
}

func TestCheckURLAgreesWithTheSyncServicesURLRule(t *testing.T) {
	for _, raw := range []string{
		"https://cal.example.com/a.ics", "http://x.example", "webcal://x.example/a.ics", "WEBCAL://x.example", " https://x.example ",
		"file:///etc/passwd", "ftp://x.example/a", "gopher://x", "https://", "not a url", "", "//x.example/a.ics",
	} {
		_, syncErr := services.NormalizeCalendarURL(raw)
		assert.Equal(t, syncErr == nil, domain.CheckURL(raw) == nil, raw)
	}
}

func TestCalendarEndpointsNeedAUser(t *testing.T) {
	e := newEnv(t)
	body := map[string]any{"name": "x", "url": "https://x.example"}
	for _, c := range [][2]string{{"GET", "/calendars"}, {"POST", "/calendars"}, {"PUT", "/calendars/1"}, {"DELETE", "/calendars/1"}, {"POST", "/calendars/1/sync"}} {
		assert.Equal(t, http.StatusUnauthorized, do(e.anon, c[0], c[1], body).Code, c[0]+" "+c[1])
	}
}

func TestDeletingOneUsersSubscriptionKeepsTheOtherUsersEventLinks(t *testing.T) {
	e := newEnv(t)
	mine := e.create(t, e.asA, map[string]any{"name": "Work", "url": "https://cal.example.com/a.ics"})
	theirs := e.create(t, e.asB, map[string]any{"name": "Home", "url": "https://cal.example.com/b.ics"})
	for owner, sub := range map[uint]uint{e.a.ID: mine, e.b.ID: theirs} {
		activity := models.Activity{UserID: owner, Title: "Imported", Date: now}
		require.NoError(t, e.db.Create(&activity).Error)
		require.NoError(t, e.db.Create(&models.CalendarEventLink{SubscriptionID: sub, UserID: owner, UID: "u", ActivityID: activity.ID, ContentHash: "h"}).Error)
	}
	assert.Equal(t, http.StatusNotFound, do(e.asA, "DELETE", idPath(theirs, ""), nil).Code)
	require.Equal(t, http.StatusOK, do(e.asA, "DELETE", idPath(mine, ""), nil).Code)

	var links []models.CalendarEventLink
	require.NoError(t, e.db.Find(&links).Error)
	require.Len(t, links, 1)
	assert.Equal(t, e.b.ID, links[0].UserID, "only the other user's link is left")
}
