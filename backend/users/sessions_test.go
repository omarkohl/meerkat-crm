package users_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"meerkat/models"
	"meerkat/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

const (
	oldPassword = "correct horse battery staple"
	newPassword = "purple monkey dishwasher engine"
)

func (e *env) session(t *testing.T, user models.User, id string, ageDays int) models.Session {
	t.Helper()
	s := models.Session{
		UserID: user.ID, PublicID: id, UserAgent: "agent-" + id[:4], IP: "10.0.0.1",
		LastSeenAt: now, ExpiresAt: now.AddDate(0, 0, 7), CreatedAt: now.AddDate(0, 0, -ageDays),
	}
	require.NoError(t, e.db.Create(&s).Error)
	return s
}

func (e *env) withPassword(t *testing.T, user *models.User, plain string) {
	t.Helper()
	hash, err := services.HashPassword(plain)
	require.NoError(t, err)
	require.NoError(t, e.db.Model(user).Updates(map[string]any{"password": hash}).Error)
	user.Password = hash
}

func (e *env) auditEvents(t *testing.T) []models.AuditLog {
	t.Helper()
	var entries []models.AuditLog
	require.NoError(t, e.db.Order("id").Find(&entries).Error)
	return entries
}

func (e *env) tokenVersion(t *testing.T, id uint) int {
	t.Helper()
	var v int
	require.NoError(t, e.db.Raw("SELECT token_version FROM users WHERE id = ?", id).Scan(&v).Error)
	return v
}

func (e *env) liveSessions(t *testing.T, id uint) int64 {
	t.Helper()
	var n int64
	e.db.Model(&models.Session{}).Where("user_id = ? AND revoked_at IS NULL", id).Count(&n)
	return n
}

const (
	sid1 = "11111111111111111111111111111111"
	sid2 = "22222222222222222222222222222222"
	sid3 = "33333333333333333333333333333333"
)

func authCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" {
			return c
		}
	}
	return nil
}

func TestSessionsAreListedNewestFirstWithTheCurrentOneMarked(t *testing.T) {
	e := newEnv(t)
	e.session(t, e.a, sid1, 3)
	e.session(t, e.a, sid2, 1)
	e.session(t, e.b, sid3, 0)
	expired := e.session(t, e.a, "44444444444444444444444444444444", 0)
	require.NoError(t, e.db.Model(&expired).Update("expires_at", now.Add(-time.Hour)).Error)
	revoked := e.session(t, e.a, "55555555555555555555555555555555", 0)
	require.NoError(t, e.db.Model(&revoked).Update("revoked_at", now).Error)

	w := send(e.engine(&e.a, map[string]any{"sessionID": sid1}), http.MethodGet, "/sessions", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Sessions []map[string]any
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Sessions, 2)
	assert.Equal(t, sid2, body.Sessions[0]["id"])
	assert.Equal(t, false, body.Sessions[0]["current"])
	assert.Equal(t, sid1, body.Sessions[1]["id"])
	assert.Equal(t, true, body.Sessions[1]["current"])
	for _, key := range []string{"user_agent", "ip", "last_seen_at", "expires_at", "created_at"} {
		assert.Contains(t, body.Sessions[0], key)
	}
	assert.NotContains(t, body.Sessions[0], "UserID")
}

func TestNoSessionsIsAnEmptyList(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, `[]`, raw(t, send(e.asA, http.MethodGet, "/sessions", ""), "sessions"))
}

func TestRevokingASessionSignsItOutAndIsAudited(t *testing.T) {
	e := newEnv(t)
	e.session(t, e.a, sid1, 1)
	e.session(t, e.a, sid2, 0)

	w := send(e.engine(&e.a, map[string]any{"sessionID": sid1}), http.MethodDelete, "/sessions/"+sid2, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Session revoked"}`, w.Body.String())
	assert.Nil(t, authCookie(w), "another session: this browser stays signed in")
	assert.EqualValues(t, 1, e.liveSessions(t, e.a.ID))
	entries := e.auditEvents(t)
	require.Len(t, entries, 1)
	assert.Equal(t, models.AuditSessionRevoked, entries[0].Event)
	assert.Equal(t, "a", entries[0].Username)
}

func TestRevokingTheCurrentSessionClearsTheCookie(t *testing.T) {
	e := newEnv(t)
	e.session(t, e.a, sid1, 1)

	w := send(e.engine(&e.a, map[string]any{"sessionID": sid1}), http.MethodDelete, "/sessions/"+sid1, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	cookie := authCookie(w)
	require.NotNil(t, cookie)
	assert.Empty(t, cookie.Value)
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	assert.Equal(t, "/", cookie.Path)
	assert.Less(t, cookie.MaxAge, 0)
}

// @003-US1
func TestOnlyALiveSessionOfTheActorCanBeRevoked(t *testing.T) {
	e := newEnv(t)
	e.session(t, e.a, sid1, 1)
	theirs := e.session(t, e.b, sid3, 1)
	gone := e.session(t, e.a, sid2, 1)
	require.NoError(t, e.db.Model(&gone).Update("revoked_at", now).Error)

	for _, id := range []string{sid3, sid2, "99999999999999999999999999999999", "not-a-session", "11111111111111111111111111111G"} {
		w := send(e.asA, http.MethodDelete, "/sessions/"+id, "")
		assert.Equal(t, http.StatusNotFound, w.Code, id)
		assert.Contains(t, w.Body.String(), "Session not found", id)
	}
	var stored models.Session
	require.NoError(t, e.db.First(&stored, theirs.ID).Error)
	assert.Nil(t, stored.RevokedAt)
	assert.Empty(t, e.auditEvents(t))
}

// @003-US2
func TestSigningOutEverywhereRevokesAllSessionsAndRaisesTheTokenVersion(t *testing.T) {
	e := newEnv(t)
	e.session(t, e.a, sid1, 1)
	e.session(t, e.a, sid2, 0)
	e.session(t, e.b, sid3, 0)

	w := send(e.engine(&e.a, map[string]any{"sessionID": sid1}), http.MethodPost, "/sessions/revoke-all", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Signed out everywhere"}`, w.Body.String())
	require.NotNil(t, authCookie(w))
	assert.Less(t, authCookie(w).MaxAge, 0)
	assert.Zero(t, e.liveSessions(t, e.a.ID))
	assert.EqualValues(t, 1, e.liveSessions(t, e.b.ID), "another user's sessions stay")
	assert.Equal(t, 1, e.tokenVersion(t, e.a.ID))
	assert.Zero(t, e.tokenVersion(t, e.b.ID))
	assert.Equal(t, models.AuditSignOutEverywhere, e.auditEvents(t)[0].Event)
}

func TestAnAPITokenCannotManageSessions(t *testing.T) {
	e := newEnv(t)
	e.session(t, e.a, sid1, 1)
	token := e.engine(&e.a, map[string]any{"isAPIToken": true})
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/sessions"}, {http.MethodDelete, "/sessions/" + sid1}, {http.MethodPost, "/sessions/revoke-all"},
	} {
		w := send(token, call.method, call.path, "")
		assert.Equal(t, http.StatusForbidden, w.Code, call.method+" "+call.path)
		assert.Contains(t, w.Body.String(), "API tokens cannot manage sessions")
	}
	assert.EqualValues(t, 1, e.liveSessions(t, e.a.ID))
	assert.Zero(t, e.tokenVersion(t, e.a.ID))
}

func TestChangingThePasswordSignsOtherBrowsersOutAndStartsAFreshSession(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	e.session(t, e.a, sid1, 1)
	hash := "reset-hash"
	require.NoError(t, e.db.Model(&e.a).Updates(map[string]any{"password_reset_token_hash": hash, "password_reset_expires_at": now, "password_reset_requested_at": now}).Error)

	w := send(e.engine(&e.a, map[string]any{"sessionID": sid1}), http.MethodPost, "/users/change-password",
		`{"current_password":"`+oldPassword+`","new_password":"`+newPassword+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Password updated successfully"}`, w.Body.String())

	got := e.reload(t, e.a.ID)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(got.Password), []byte(newPassword)))
	assert.Nil(t, got.PasswordResetTokenHash)
	assert.Nil(t, got.PasswordResetExpiresAt)
	assert.Nil(t, got.PasswordResetRequestedAt)
	assert.Equal(t, 1, e.tokenVersion(t, e.a.ID))

	var old models.Session
	require.NoError(t, e.db.Where("public_id = ?", sid1).First(&old).Error)
	assert.NotNil(t, old.RevokedAt, "the old session ends")
	assert.EqualValues(t, 1, e.liveSessions(t, e.a.ID), "and this browser gets a new one")
	cookie := authCookie(w)
	require.NotNil(t, cookie)
	assert.NotEmpty(t, cookie.Value)
	assert.Equal(t, 24*3600, cookie.MaxAge)
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, models.AuditPasswordChanged, e.auditEvents(t)[0].Event)
}

func TestChangePasswordRefusesWhatIsWrongAndChangesNothing(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	before := e.reload(t, e.a.ID).Password
	post := func(body string) *httptest.ResponseRecorder {
		return send(e.asA, http.MethodPost, "/users/change-password", body)
	}

	w := post(`{"current_password":"wrong password here","new_password":"` + newPassword + `"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Current password is incorrect")

	w = post(`{"current_password":"` + oldPassword + `","new_password":"` + oldPassword + `"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "New password must differ from current password")

	w = post(`{"current_password":"` + oldPassword + `","new_password":"short"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	w = post(`{"current_password":"` + oldPassword + `"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	w = post(`{"new_password":"` + newPassword + `"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	long := strings.Repeat("a long and winding passphrase ", 3)
	w = post(`{"current_password":"` + oldPassword + `","new_password":"` + long + `"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "VALIDATION_ERROR")
	assert.Contains(t, w.Body.String(), "password must not exceed 72 characters")

	assert.Equal(t, before, e.reload(t, e.a.ID).Password)
	assert.Zero(t, e.tokenVersion(t, e.a.ID))
	assert.Empty(t, e.auditEvents(t))
}

func TestPasswordChangesAreRefusedInDemoModeAndToAPITokens(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	body := `{"current_password":"` + oldPassword + `","new_password":"` + newPassword + `"}`

	token := e.engine(&e.a, map[string]any{"isAPIToken": true})
	assert.Equal(t, http.StatusUnauthorized, send(token, http.MethodPost, "/users/change-password", body).Code, "an API token carries no username")
	assert.Equal(t, http.StatusUnauthorized, send(token, http.MethodPatch, "/users/language", `{"language":"de"}`).Code)
	assert.Equal(t, http.StatusUnauthorized, send(token, http.MethodPatch, "/users/date-format", `{"date_format":"us"}`).Code)
	assert.Equal(t, http.StatusUnauthorized, send(e.anon, http.MethodPost, "/users/change-password", body).Code)

	t.Setenv("DEMO_MODE", "true")
	w := send(e.asA, http.MethodPost, "/users/change-password", body)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Password changes are disabled in demo mode")
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(e.reload(t, e.a.ID).Password), []byte(oldPassword)))
}

func TestChangingOnePasswordLeavesTheOtherUsersPasswordAndSessionsAlone(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	e.withPassword(t, &e.b, oldPassword)
	e.session(t, e.a, sid1, 1)
	e.session(t, e.b, sid2, 1)
	before := e.reload(t, e.b.ID)

	w := send(e.engine(&e.a, map[string]any{"sessionID": sid1}), http.MethodPost, "/users/change-password",
		`{"current_password":"`+oldPassword+`","new_password":"`+newPassword+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	after := e.reload(t, e.b.ID)
	assert.Equal(t, before.Password, after.Password)
	assert.Equal(t, 0, e.tokenVersion(t, e.b.ID))
	assert.EqualValues(t, 1, e.liveSessions(t, e.b.ID), "the other user stays signed in")
}
