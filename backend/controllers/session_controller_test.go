package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"
	"meerkat/users"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type sessionEnv struct {
	db  *gorm.DB
	r   *gin.Engine
	cfg *config.Config
}

// newSessionEnv wires the real login, logout and session routes behind the real auth middleware.
func newSessionEnv(t *testing.T) *sessionEnv {
	t.Helper()
	db, router := setupRouter() // seeds one user ("tester") that the stub middleware logs in
	cfg := &config.Config{JWTSecretKey: "session-test-secret-0123456789abcdef", EncryptionKey: "+WawZKvzbWZ2/2AjCy61aKmgjZub4jqXuGWNXY4eJUc=", JWTExpiryHours: 24}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("db", db); c.Set("cfg", *cfg); c.Next() })
	p := r.Group("/", middleware.AuthMiddleware(cfg))
	users.New(users.Deps{DB: db, Config: cfg}).Register(api.New(r.Group(""), r), r, p)
	p.GET("/me", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	_ = router
	return &sessionEnv{db: db, r: r, cfg: cfg}
}

func (e *sessionEnv) user(t *testing.T, name string) models.User {
	t.Helper()
	hash, _ := services.HashPassword(strongPassword)
	u := models.User{Username: name, Email: name + "@example.com", Password: hash}
	require.NoError(t, e.db.Create(&u).Error)
	return u
}

func (e *sessionEnv) do(method, path string, token string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TestBrowser/1.0")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	return w
}

func (e *sessionEnv) login(t *testing.T, name string) string {
	t.Helper()
	w := e.do("POST", "/login", "", map[string]string{"identifier": name, "password": strongPassword})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return cookie(w, "auth_token").Value
}

func (e *sessionEnv) sessions(t *testing.T, token string) []map[string]any {
	t.Helper()
	w := e.do("GET", "/sessions", token, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out struct{ Sessions []map[string]any }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out.Sessions
}

func (e *sessionEnv) events(event string) []models.AuditLog {
	var out []models.AuditLog
	e.db.Where("event = ?", event).Order("id").Find(&out)
	return out
}

func TestLoginCreatesAListedSessionAndAnAuditEntry(t *testing.T) {
	e := newSessionEnv(t)
	u := e.user(t, "alice")
	token := e.login(t, "alice")

	list := e.sessions(t, token)
	require.Len(t, list, 1)
	assert.Equal(t, true, list[0]["current"])
	assert.Equal(t, "TestBrowser/1.0", list[0]["user_agent"])
	assert.NotEmpty(t, list[0]["id"])
	assert.NotContains(t, list[0], "UserID")

	logins := e.events(models.AuditLogin)
	require.Len(t, logins, 1)
	assert.Equal(t, u.ID, *logins[0].UserID)
	assert.Equal(t, "alice", logins[0].Username)
	assert.Equal(t, "TestBrowser/1.0", logins[0].UserAgent)
}

func TestFailedLoginsAreAuditedWithoutWhatWasTyped(t *testing.T) {
	e := newSessionEnv(t)
	u := e.user(t, "alice")

	w := e.do("POST", "/login", "", map[string]string{"identifier": "alice", "password": "wrong-password-1"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w = e.do("POST", "/login", "", map[string]string{"identifier": "nobody-secret-typed", "password": "wrong-password-1"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	failed := e.events(models.AuditLoginFailed)
	require.Len(t, failed, 2)
	assert.Equal(t, "reason:wrong_password", failed[0].Detail)
	assert.Equal(t, "reason:unknown_account", failed[1].Detail)
	require.NotNil(t, failed[0].UserID)
	assert.Equal(t, u.ID, *failed[0].UserID)
	assert.Nil(t, failed[1].UserID, "unknown account")
	for _, f := range failed {
		assert.NotContains(t, f.Username+f.Detail, "nobody-secret-typed")
		assert.NotContains(t, f.Username+f.Detail, "wrong-password-1")
	}
	var sessions int64
	e.db.Model(&models.Session{}).Count(&sessions)
	assert.Zero(t, sessions)
}

func TestRevokingASessionLogsThatBrowserOutAndOnlyThat(t *testing.T) {
	e := newSessionEnv(t)
	e.user(t, "alice")
	laptop := e.login(t, "alice")
	phone := e.login(t, "alice")

	list := e.sessions(t, laptop)
	require.Len(t, list, 2)
	var phoneID string
	for _, s := range list {
		if s["current"] == false {
			phoneID = s["id"].(string)
		}
	}
	require.NotEmpty(t, phoneID)

	w := e.do("DELETE", "/sessions/"+phoneID, laptop, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, http.StatusUnauthorized, e.do("GET", "/me", phone, nil).Code, "the phone is out")
	assert.Equal(t, http.StatusOK, e.do("GET", "/me", laptop, nil).Code, "the laptop is not")
	assert.Len(t, e.sessions(t, laptop), 1)
	assert.Len(t, e.events(models.AuditSessionRevoked), 1)

	// Revoking it again, or something that is no session id, is a 404
	assert.Equal(t, http.StatusNotFound, e.do("DELETE", "/sessions/"+phoneID, laptop, nil).Code)
	assert.Equal(t, http.StatusNotFound, e.do("DELETE", "/sessions/not-an-id", laptop, nil).Code)
}

func TestSessionsAreIsolatedBetweenUsers(t *testing.T) {
	e := newSessionEnv(t)
	e.user(t, "alice")
	e.user(t, "bob")
	alice := e.login(t, "alice")
	bob := e.login(t, "bob")

	aliceID := e.sessions(t, alice)[0]["id"].(string)
	assert.Len(t, e.sessions(t, bob), 1)
	assert.NotEqual(t, aliceID, e.sessions(t, bob)[0]["id"])

	assert.Equal(t, http.StatusNotFound, e.do("DELETE", "/sessions/"+aliceID, bob, nil).Code, "bob cannot revoke alice's session")
	assert.Equal(t, http.StatusOK, e.do("GET", "/me", alice, nil).Code)

	// Signing alice out everywhere leaves bob alone
	assert.Equal(t, http.StatusOK, e.do("POST", "/sessions/revoke-all", alice, nil).Code)
	assert.Equal(t, http.StatusOK, e.do("GET", "/me", bob, nil).Code)
}

func TestSignOutEverywhere(t *testing.T) {
	e := newSessionEnv(t)
	e.user(t, "alice")
	first := e.login(t, "alice")
	second := e.login(t, "alice")

	w := e.do("POST", "/sessions/revoke-all", first, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, -1, cookie(w, "auth_token").MaxAge, "the cookie is cleared")
	assert.Equal(t, http.StatusUnauthorized, e.do("GET", "/me", first, nil).Code)
	assert.Equal(t, http.StatusUnauthorized, e.do("GET", "/me", second, nil).Code)
	assert.Len(t, e.events(models.AuditSignOutEverywhere), 1)

	// Logging in again works and starts from a clean list
	third := e.login(t, "alice")
	assert.Equal(t, http.StatusOK, e.do("GET", "/me", third, nil).Code)
	assert.Len(t, e.sessions(t, third), 1)
}

func TestLogoutRevokesTheSession(t *testing.T) {
	e := newSessionEnv(t)
	e.user(t, "alice")
	token := e.login(t, "alice")
	assert.Equal(t, http.StatusOK, e.do("POST", "/logout", token, nil).Code)
	assert.Equal(t, http.StatusUnauthorized, e.do("GET", "/me", token, nil).Code, "a stolen copy of the cookie is dead")
	assert.Len(t, e.events(models.AuditLogout), 1)
	// Logging out without a (valid) cookie still works
	assert.Equal(t, http.StatusOK, e.do("POST", "/logout", "", nil).Code)
	assert.Equal(t, http.StatusOK, e.do("POST", "/logout", "garbage", nil).Code)
}

func TestChangingThePasswordSignsOutTheOtherBrowsers(t *testing.T) {
	e := newSessionEnv(t)
	e.user(t, "alice")
	laptop := e.login(t, "alice")
	phone := e.login(t, "alice")

	w := e.do("POST", "/users/change-password", laptop, map[string]string{"current_password": strongPassword, "new_password": strongPasswordAlt})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Equal(t, http.StatusUnauthorized, e.do("GET", "/me", phone, nil).Code, "the other browser is out")
	assert.Equal(t, http.StatusUnauthorized, e.do("GET", "/me", laptop, nil).Code, "the old cookie is dead")
	fresh := cookie(w, "auth_token")
	require.NotNil(t, fresh, "this browser gets a new session")
	assert.Equal(t, http.StatusOK, e.do("GET", "/me", fresh.Value, nil).Code)
	assert.Len(t, e.sessions(t, fresh.Value), 1)
	assert.Len(t, e.events(models.AuditPasswordChanged), 1)
}

func TestPasswordResetSignsOutEverywhere(t *testing.T) {
	e := newSessionEnv(t)
	u := e.user(t, "alice")
	token := e.login(t, "alice")
	hash := services.HashPasswordResetToken("reset-token-value")
	exp := time.Now().Add(time.Hour)
	require.NoError(t, e.db.Model(&u).Updates(map[string]any{"password_reset_token_hash": hash, "password_reset_expires_at": exp}).Error)

	w := e.do("POST", "/password-reset/confirm", "", map[string]string{"token": "reset-token-value", "password": strongPasswordAlt})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, http.StatusUnauthorized, e.do("GET", "/me", token, nil).Code)
	assert.Len(t, e.events(models.AuditPasswordReset), 1)
}

func TestDeletingAUserRemovesTheirSessionsButKeepsTheAuditTrail(t *testing.T) {
	db, router := setupRouter()
	require.NoError(t, db.AutoMigrate(&models.ApiToken{}, &models.IcsFeed{}, &models.CardDAVConnection{}, &models.CalendarSubscription{}, &models.CardDAVContactLink{}, &models.CalendarEventLink{}))
	var admin models.User
	require.NoError(t, db.First(&admin).Error)
	require.NoError(t, db.Model(&admin).Update("is_admin", true).Error)
	victim := models.User{Username: "victim", Email: "v@example.com", Password: "x"}
	require.NoError(t, db.Create(&victim).Error)
	require.NoError(t, db.Create(&models.Session{UserID: victim.ID, PublicID: "v1", ExpiresAt: time.Now().Add(time.Hour)}).Error)
	services.WriteAudit(db, models.AuditLog{UserID: &victim.ID, Username: "victim", Event: models.AuditLogin})
	mountAdmin(router, db)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("DELETE", fmt.Sprintf("/admin/users/%d", victim.ID), nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var sessions int64
	db.Model(&models.Session{}).Where("user_id = ?", victim.ID).Count(&sessions)
	assert.Zero(t, sessions)
	var logs []models.AuditLog
	db.Order("id").Find(&logs)
	require.Len(t, logs, 2)
	assert.Equal(t, "victim", logs[0].Username, "the old entry keeps the name")
	assert.Equal(t, models.AuditAdminUserDeleted, logs[1].Event)
	assert.Equal(t, fmt.Sprintf("user:victim#%d", victim.ID), logs[1].Detail)
	assert.Equal(t, admin.ID, *logs[1].UserID)
}

// The new password and the end of the sessions are one change: when the sessions cannot be
// revoked, the request fails and the old password still works.
func TestPasswordResetIsAllOrNothing(t *testing.T) {
	e := newSessionEnv(t)
	u := e.user(t, "alice")
	hash := services.HashPasswordResetToken("reset-token-value")
	require.NoError(t, e.db.Model(&u).Updates(map[string]any{"password_reset_token_hash": hash, "password_reset_expires_at": time.Now().Add(time.Hour)}).Error)
	require.NoError(t, e.db.Exec("DROP TABLE sessions").Error)

	w := e.do("POST", "/password-reset/confirm", "", map[string]string{"token": "reset-token-value", "password": strongPasswordAlt})
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())

	var after models.User
	require.NoError(t, e.db.First(&after, u.ID).Error)
	assert.Equal(t, u.Password, after.Password, "the password did not change")
	assert.NotNil(t, after.PasswordResetTokenHash, "the reset token is still usable")
}

func TestAdminUserUpdateAuditDetailIsStructured(t *testing.T) {
	db, router := setupRouter()
	require.NoError(t, db.AutoMigrate(&models.AuditLog{}, &models.Session{}))
	var admin models.User
	require.NoError(t, db.First(&admin).Error)
	require.NoError(t, db.Model(&admin).Update("is_admin", true).Error)
	target := models.User{Username: "target", Email: "t@example.com", Password: "x"}
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&models.Session{UserID: target.ID, PublicID: "t1", ExpiresAt: time.Now().Add(time.Hour)}).Error)
	mountAdmin(router, db)

	body := `{"email":"new@example.com","password":"` + strongPasswordAlt + `"}`
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/admin/users/%d", target.ID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var entry models.AuditLog
	require.NoError(t, db.Where("event = ?", models.AuditAdminUserUpdated).First(&entry).Error)
	assert.Equal(t, fmt.Sprintf("user:target#%d;changed:email,password", target.ID), entry.Detail)
	assert.NotContains(t, entry.Detail, strongPasswordAlt)
	live, _ := services.ListSessions(db, target.ID, time.Now())
	assert.Empty(t, live, "a password set by an admin ends the sessions")
}
