package contactsync_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/contactsync"
	"meerkat/contactsync/internal/domain"
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
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(4)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.CardDAVConnection{}, &models.CardDAVContactLink{}, &models.Webhook{}, &models.WebhookDelivery{}))
	e := &env{db: db}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)
	module := contactsync.New(contactsync.Deps{DB: db, Config: &config.Config{JWTSecretKey: secret}})
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

func (e *env) stored(t *testing.T, owner uint) models.CardDAVConnection {
	t.Helper()
	var c models.CardDAVConnection
	require.NoError(t, e.db.Where("user_id = ?", owner).First(&c).Error)
	return c
}

func TestNoConnectionIs404UntilOneIsSaved(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, http.StatusNotFound, do(e.asA, "GET", "/carddav/connection", nil).Code)
	assert.Equal(t, http.StatusCreated, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "https://dav.example.com", "address_book_path": "/a/"}).Code)
	assert.Equal(t, http.StatusOK, do(e.asA, "GET", "/carddav/connection", nil).Code)
	assert.Equal(t, http.StatusNotFound, do(e.asB, "GET", "/carddav/connection", nil).Code)
	assert.Equal(t, http.StatusOK, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "https://dav.example.com", "address_book_path": "/a/"}).Code, "a second save replaces it")
	var n int64
	e.db.Model(&models.CardDAVConnection{}).Count(&n)
	assert.EqualValues(t, 1, n)
}

func TestPasswordHandling(t *testing.T) {
	e := newEnv(t)
	w := do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "https://user:inline@dav.example.com/dav", "address_book_path": "/books/a/", "username": "u", "password": "hunter2"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "hunter2")
	assert.NotContains(t, w.Body.String(), "inline", "credentials in the URL are dropped")

	conn := e.stored(t, e.a.ID)
	assert.NotContains(t, conn.BaseURL, "inline")
	plain, err := services.DecryptCredential(secret, conn.PasswordEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "hunter2", plain)
	assert.Equal(t, models.CardDAVDirectionTwoWay, conn.Direction)
	assert.True(t, conn.SyncEnabled)

	got := do(e.asA, "GET", "/carddav/connection", nil)
	assert.Contains(t, got.Body.String(), `"has_password":true`)
	assert.Contains(t, got.Body.String(), `"last_sync_stats":null`)
	assert.NotContains(t, got.Body.String(), conn.PasswordEncrypted)

	assert.Equal(t, http.StatusBadRequest, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "https://dav.example.com", "address_book_path": "books/a"}).Code, "relative path")
	assert.Equal(t, http.StatusBadRequest, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "file:///etc", "address_book_path": "/x"}).Code, "scheme")
	assert.Equal(t, http.StatusBadRequest, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "https://dav.example.com", "address_book_path": "/x", "direction": "sideways"}).Code, "direction")

	require.Equal(t, http.StatusOK, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "https://dav.example.com/other", "address_book_path": "/books/b/", "username": "u"}).Code)
	assert.NotEmpty(t, e.stored(t, e.a.ID).PasswordEncrypted, "same server keeps the password")

	for _, target := range []string{"https://evil.example.net/dav", "http://dav.example.com/dav", "https://dav.example.com:8443/dav"} {
		w := do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": target, "address_book_path": "/books/b/", "username": "u"})
		require.Equal(t, http.StatusBadRequest, w.Code, target)
		assert.Contains(t, w.Body.String(), "password required")
		conn = e.stored(t, e.a.ID)
		assert.NotEmpty(t, conn.PasswordEncrypted)
		assert.Equal(t, "https://dav.example.com/other", conn.BaseURL)
	}
	require.Equal(t, http.StatusOK, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "https://evil.example.net/dav", "address_book_path": "/books/b/", "password": "new"}).Code)
	require.Equal(t, http.StatusOK, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "https://evil.example.net/dav", "address_book_path": "/books/b/", "clear_password": true}).Code)
	assert.Empty(t, e.stored(t, e.a.ID).PasswordEncrypted)
}

func TestSwitchingAddressBookDropsLinksAndSyncState(t *testing.T) {
	e := newEnv(t)
	require.Equal(t, http.StatusCreated, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "https://dav.example.com", "address_book_path": "/a/", "password": "p"}).Code)
	conn := e.stored(t, e.a.ID)
	require.NoError(t, e.db.Model(&conn).Updates(map[string]any{"sync_token": "tok", "last_sync_status": "success", "last_sync_stats": `{"skipped":2}`}).Error)
	require.NoError(t, e.db.Create(&models.CardDAVContactLink{ConnectionID: conn.ID, UserID: e.a.ID, ContactID: 1, RemoteUID: "u1", RemotePath: "/a/1.vcf"}).Error)

	same := do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "https://dav.example.com", "address_book_path": "/a/", "address_book_name": "Renamed"})
	require.Equal(t, http.StatusOK, same.Code)
	assert.Contains(t, same.Body.String(), `"skipped":2`)
	var n int64
	e.db.Model(&models.CardDAVContactLink{}).Where("connection_id = ?", conn.ID).Count(&n)
	assert.EqualValues(t, 1, n, "the same address book keeps its links")

	require.Equal(t, http.StatusOK, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "https://dav.example.com", "address_book_path": "/b/"}).Code)
	e.db.Model(&models.CardDAVContactLink{}).Where("connection_id = ?", conn.ID).Count(&n)
	assert.Zero(t, n)
	conn = e.stored(t, e.a.ID)
	assert.Empty(t, conn.SyncToken)
	assert.Empty(t, conn.LastSyncStatus)
	assert.Empty(t, conn.LastSyncStats)
}

func TestDeleteRemovesTheConnectionAndLinksButNotContacts(t *testing.T) {
	e := newEnv(t)
	require.Equal(t, http.StatusCreated, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": "https://dav.example.com", "address_book_path": "/a/"}).Code)
	conn := e.stored(t, e.a.ID)
	contact := models.Contact{UserID: e.a.ID, Firstname: "Ada"}
	require.NoError(t, e.db.Create(&contact).Error)
	require.NoError(t, e.db.Create(&models.CardDAVContactLink{ConnectionID: conn.ID, UserID: e.a.ID, ContactID: contact.ID, RemoteUID: "u1", RemotePath: "/a/1.vcf"}).Error)

	assert.Equal(t, http.StatusNotFound, do(e.asB, "DELETE", "/carddav/connection", nil).Code)
	w := do(e.asA, "DELETE", "/carddav/connection", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"message":"CardDAV connection deleted"}`, w.Body.String())
	var links, contacts int64
	e.db.Model(&models.CardDAVContactLink{}).Count(&links)
	e.db.Model(&models.Contact{}).Count(&contacts)
	assert.Zero(t, links)
	assert.EqualValues(t, 1, contacts)
	assert.Equal(t, http.StatusNotFound, do(e.asA, "DELETE", "/carddav/connection", nil).Code)
}

func TestDiscoverReusesTheStoredPasswordOnlyForTheStoredServer(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pass, _ := r.BasicAuth()
		mu.Lock()
		seen = append(seen, pass)
		mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	e := newEnv(t)
	require.Equal(t, http.StatusCreated, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": server.URL, "address_book_path": "/a/", "username": "u", "password": "stored-secret"}).Code)

	w := do(e.asA, "POST", "/carddav/discover", map[string]any{"base_url": server.URL, "username": "u"})
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "authentication failed - check username and password")
	mu.Lock()
	assert.Contains(t, seen, "stored-secret")
	seen = nil
	mu.Unlock()

	other := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	do(e.asA, "POST", "/carddav/discover", map[string]any{"base_url": other, "username": "u"})
	mu.Lock()
	assert.NotContains(t, seen, "stored-secret", "another host never receives the stored password")
	mu.Unlock()

	do(e.asB, "POST", "/carddav/discover", map[string]any{"base_url": server.URL, "username": "u"})
	mu.Lock()
	assert.NotContains(t, seen, "stored-secret", "another user never gets it either")
	seen = nil
	mu.Unlock()

	require.Equal(t, http.StatusCreated, do(e.asB, "PUT", "/carddav/connection", map[string]any{"base_url": server.URL, "address_book_path": "/b/", "username": "u", "password": "b-secret"}).Code)
	do(e.asA, "POST", "/carddav/discover", map[string]any{"base_url": server.URL, "username": "u"})
	do(e.asB, "POST", "/carddav/discover", map[string]any{"base_url": server.URL, "username": "u"})
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"stored-secret", "b-secret"}, seen, "each user's own password, never the other's")
}

func TestDiscoverRefusesBadAddressesAndPrivateOnes(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, http.StatusBadRequest, do(e.asA, "POST", "/carddav/discover", map[string]any{"base_url": "ftp://x.example"}).Code)
	assert.Equal(t, http.StatusBadRequest, do(e.asA, "POST", "/carddav/discover", map[string]any{}).Code)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	blocked := contactsync.New(contactsync.Deps{DB: e.db, Config: &config.Config{JWTSecretKey: secret, CardDAVBlockPrivateURLs: true}})
	r := gin.New()
	r.Use(middleware.ClockMiddleware(clock.NewFake(now)), func(c *gin.Context) { c.Set("userID", e.a.ID); c.Next() })
	blocked.Register(api.New(r.Group(""), r), r.Group(""))
	w := do(r, "POST", "/carddav/discover", map[string]any{"base_url": server.URL})
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "blocked private address")
}

func TestSyncStartsInTheBackgroundAndRefusesASecondRun(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	e := newEnv(t)
	assert.Equal(t, http.StatusNotFound, do(e.asA, "POST", "/carddav/connection/sync", nil).Code, "nothing to sync")
	require.Equal(t, http.StatusCreated, do(e.asA, "PUT", "/carddav/connection", map[string]any{"base_url": server.URL, "address_book_path": "/a/"}).Code)

	w := do(e.asA, "POST", "/carddav/connection/sync", nil)
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Contact sync started"}`, w.Body.String())
	<-entered
	assert.Contains(t, do(e.asA, "GET", "/carddav/connection", nil).Body.String(), `"sync_running":true`)
	assert.Equal(t, http.StatusConflict, do(e.asA, "POST", "/carddav/connection/sync", nil).Code)
	assert.Equal(t, http.StatusNotFound, do(e.asB, "POST", "/carddav/connection/sync", nil).Code)

	close(release)
	require.Eventually(t, func() bool {
		return strings.Contains(do(e.asA, "GET", "/carddav/connection", nil).Body.String(), `"sync_running":false`)
	}, 10*time.Second, 20*time.Millisecond)
}

func TestConnectionEndpointsNeedAUser(t *testing.T) {
	e := newEnv(t)
	body := map[string]any{"base_url": "https://dav.example.com", "address_book_path": "/a/"}
	for _, c := range [][2]string{{"GET", "/carddav/connection"}, {"PUT", "/carddav/connection"}, {"DELETE", "/carddav/connection"}, {"POST", "/carddav/discover"}, {"POST", "/carddav/connection/sync"}} {
		assert.Equal(t, http.StatusUnauthorized, do(e.anon, c[0], c[1], body).Code, c[0]+" "+c[1])
	}
}

func TestNormalizeBaseURLAgreesWithTheSyncServicesURLRule(t *testing.T) {
	for _, raw := range []string{"https://dav.example.com/x", " http://x.example ", "https://u:p@x.example/#f", "file:///etc", "ftp://x", "webcal://x.example", "https://", "", "not a url"} {
		_, syncErr := services.NormalizeCardDAVURL(raw)
		got, err := domain.NormalizeBaseURL(raw)
		assert.Equal(t, syncErr == nil, err == nil, raw)
		if syncErr == nil {
			parsed, _ := services.NormalizeCardDAVURL(raw)
			assert.Equal(t, parsed.String(), got, raw)
		}
	}
}
