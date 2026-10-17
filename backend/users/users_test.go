package users_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"
	"meerkat/users"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var now = time.Date(2026, 3, 10, 15, 30, 0, 0, time.UTC)

type env struct {
	db   *gorm.DB
	cfg  *config.Config
	oidc *services.OIDCProvider
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
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Session{}, &models.AuditLog{}, &models.RecoveryCode{}, &models.EmailOutbox{}))
	e := &env{db: db}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)

	e.cfg = &config.Config{JWTSecretKey: "test-secret-test-secret-test-secret", JWTExpiryHours: 24}
	e.asA, e.asB, e.anon = e.engine(&e.a, nil), e.engine(&e.b, nil), e.engine(nil, nil)
	return e
}

// engine mounts the module the way the router does: behind a middleware that sets what the
// authentication middleware sets. extras are further context values (isAPIToken, sessionID).
func (e *env) engine(user *models.User, extras map[string]any) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ClockMiddleware(clock.NewFake(now)))
	authenticated := r.Group("")
	if user != nil {
		authenticated.Use(func(c *gin.Context) {
			c.Set("userID", user.ID)
			if _, apiToken := extras["isAPIToken"]; !apiToken {
				c.Set("username", user.Username)
			}
			for k, v := range extras {
				c.Set(k, v)
			}
			c.Next()
		})
	}
	users.New(users.Deps{DB: e.db, Config: e.cfg, OIDC: e.oidc}).Register(api.New(r.Group(""), r), r.Group(""), authenticated)
	return r
}

func (e *env) reload(t *testing.T, id uint) models.User {
	t.Helper()
	var u models.User
	require.NoError(t, e.db.First(&u, id).Error)
	return u
}

var nextClient atomic.Uint32

func newClientAddress() string {
	n := nextClient.Add(1)
	return fmt.Sprintf("10.%d.%d.%d", (n>>16)&255, (n>>8)&255, n&255)
}

// send makes a request from a client address of its own: the public routes are rate limited
// per address, by a limiter shared by the whole process.
func send(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = newClientAddress() + ":4000"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func raw(t *testing.T, w *httptest.ResponseRecorder, field string) string {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m), w.Body.String())
	return string(m[field])
}

func message(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var b struct{ Error struct{ Message string } }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b), w.Body.String())
	return b.Error.Message
}

func TestLanguageIsChangedForTheActorOnly(t *testing.T) {
	e := newEnv(t)
	w := send(e.asA, http.MethodPatch, "/users/language", `{"language":"de"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Language updated successfully","language":"de"}`, w.Body.String())
	assert.Equal(t, "de", e.reload(t, e.a.ID).Language)
	assert.Equal(t, "en", e.reload(t, e.b.ID).Language)
}

func TestUnsupportedLanguagesAreRefusedWithTheListOfSupportedOnes(t *testing.T) {
	e := newEnv(t)
	for _, body := range []string{`{"language":"xx"}`, `{"language":""}`, `{}`, `{"language":"DE"}`, `{"language":"de-DE"}`} {
		w := send(e.asA, http.MethodPatch, "/users/language", body)
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
		assert.Equal(t, "Invalid value for field 'language'", message(t, w), body)
		assert.Contains(t, w.Body.String(), "Unsupported language. Supported: en, de, it, es, fr", body)
	}
	assert.Equal(t, "en", e.reload(t, e.a.ID).Language)
}

func TestAMalformedBodyIsABadRequest(t *testing.T) {
	e := newEnv(t)
	w := send(e.asA, http.MethodPatch, "/users/language", `{"language":`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, "Invalid value for field 'request body'", message(t, w))
	assert.Contains(t, w.Body.String(), "validation failed")
	w = send(e.asA, http.MethodPatch, "/users/language", ``)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "request body is required")
}

func TestDateFormatIsChangedForTheActorOnly(t *testing.T) {
	e := newEnv(t)
	w := send(e.asA, http.MethodPatch, "/users/date-format", `{"date_format":"iso"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Date format updated successfully","date_format":"iso"}`, w.Body.String())
	assert.Equal(t, "iso", e.reload(t, e.a.ID).DateFormat)
	assert.Equal(t, "eu", e.reload(t, e.b.ID).DateFormat)

	w = send(e.asA, http.MethodPatch, "/users/date-format", `{"date_format":"dmy"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Unsupported date format. Supported: eu, us, iso")
	assert.Equal(t, "iso", e.reload(t, e.a.ID).DateFormat)
}

func TestCustomFieldNamesDefaultToAnEmptyListAndAreKeptPerUser(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, `[]`, raw(t, send(e.asA, http.MethodGet, "/users/custom-fields", ""), "custom_field_names"))

	w := send(e.asA, http.MethodPatch, "/users/custom-fields", `{"names":["Pet","Car"]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Custom field names updated successfully","custom_field_names":["Pet","Car"]}`, w.Body.String())
	assert.JSONEq(t, `["Pet","Car"]`, raw(t, send(e.asA, http.MethodGet, "/users/custom-fields", ""), "custom_field_names"))
	assert.Equal(t, `[]`, raw(t, send(e.asB, http.MethodGet, "/users/custom-fields", ""), "custom_field_names"))

	w = send(e.asA, http.MethodPatch, "/users/custom-fields", `{"names":["`+strings.Repeat("x", 101)+`"]}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.JSONEq(t, `["Pet","Car"]`, raw(t, send(e.asA, http.MethodGet, "/users/custom-fields", ""), "custom_field_names"))
}

// Never configured is null (the client applies its defaults); configured empty is [].
func TestEnabledContactFieldsKeepNullApartFromEmpty(t *testing.T) {
	e := newEnv(t)
	get := func(r http.Handler) string {
		w := send(r, http.MethodGet, "/users/enabled-contact-fields", "")
		require.Equal(t, http.StatusOK, w.Code)
		return raw(t, w, "enabled_contact_fields")
	}
	assert.Equal(t, "null", get(e.asA))

	w := send(e.asA, http.MethodPatch, "/users/enabled-contact-fields", `{"fields":["emails","phones"]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Enabled contact fields updated successfully","enabled_contact_fields":["emails","phones"]}`, w.Body.String())
	assert.JSONEq(t, `["emails","phones"]`, get(e.asA))
	assert.Equal(t, "null", get(e.asB), "another user's setting is theirs")

	require.Equal(t, http.StatusOK, send(e.asA, http.MethodPatch, "/users/enabled-contact-fields", `{"fields":[]}`).Code)
	assert.Equal(t, "[]", get(e.asA))

	w = send(e.asA, http.MethodPatch, "/users/enabled-contact-fields", `{"fields":["`+strings.Repeat("x", 51)+`"]}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestChangingPreferencesLeavesThePasswordAndOtherSettingsAlone(t *testing.T) {
	e := newEnv(t)
	require.NoError(t, e.db.Model(&e.a).Updates(map[string]any{"password": "hash", "language": "fr"}).Error)

	require.Equal(t, http.StatusOK, send(e.asA, http.MethodPatch, "/users/date-format", `{"date_format":"us"}`).Code)
	require.Equal(t, http.StatusOK, send(e.asA, http.MethodPatch, "/users/custom-fields", `{"names":["Pet"]}`).Code)

	got := e.reload(t, e.a.ID)
	assert.Equal(t, "hash", got.Password)
	assert.Equal(t, "fr", got.Language)
	assert.Equal(t, "us", got.DateFormat)
}

func TestEverythingRefusesWithoutAnAuthenticatedUser(t *testing.T) {
	e := newEnv(t)
	for _, call := range []struct{ method, path, body string }{
		{http.MethodPatch, "/users/language", `{"language":"de"}`}, {http.MethodPatch, "/users/date-format", `{"date_format":"us"}`},
		{http.MethodGet, "/users/custom-fields", ""}, {http.MethodPatch, "/users/custom-fields", `{"names":[]}`},
		{http.MethodGet, "/users/enabled-contact-fields", ""}, {http.MethodPatch, "/users/enabled-contact-fields", `{"fields":[]}`},
	} {
		assert.Equal(t, http.StatusUnauthorized, send(e.anon, call.method, call.path, call.body).Code, call.method+" "+call.path)
	}
}
