package users_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/models"
	"meerkat/services"
	"meerkat/users"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

const resetMessage = `{"message":"If an account exists, password reset instructions were sent"}`

func (e *env) emailOn() {
	e.cfg.FrontendURL = "http://localhost:3000"
	e.cfg.UseSMTP = true
	e.cfg.SMTPHost, e.cfg.SMTPPort, e.cfg.SMTPFromEmail = "127.0.0.1", 1, "from@example.com"
}

func (e *env) withResetToken(t *testing.T, user models.User, token string, expires time.Time) {
	t.Helper()
	hash := services.HashPasswordResetToken(token)
	require.NoError(t, e.db.Model(&user).Updates(map[string]any{
		"password_reset_token_hash": hash, "password_reset_expires_at": expires, "password_reset_requested_at": now,
	}).Error)
}

func sendFrom(r http.Handler, ip, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":4000"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestARequestForAKnownAccountStoresAHashedTokenAndQueuesTheMail(t *testing.T) {
	defer services.StubEmailDelivery(func(func()) {})()
	e := newEnv(t)
	e.emailOn()
	r := e.engine(nil, nil)

	w := send(r, http.MethodPost, "/password-reset/request", `{"email":"A@Example.com"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, resetMessage, w.Body.String())

	got := e.reload(t, e.a.ID)
	require.NotNil(t, got.PasswordResetTokenHash)
	assert.Len(t, *got.PasswordResetTokenHash, 64, "only the SHA-256 of the token is stored")
	require.NotNil(t, got.PasswordResetExpiresAt)
	assert.True(t, got.PasswordResetExpiresAt.Equal(now.Add(time.Hour)))
	assert.True(t, got.PasswordResetRequestedAt.Equal(now))
	assert.Nil(t, e.reload(t, e.b.ID).PasswordResetTokenHash)

	var row models.EmailOutbox
	require.NoError(t, e.db.Where("to_addr = ?", "a@example.com").First(&row).Error)
	assert.Equal(t, &e.a.ID, row.UserID)
	assert.NotContains(t, row.HTML, *got.PasswordResetTokenHash, "the mail carries the token, not its hash")
	assert.NotContains(t, w.Body.String(), "token")
}

func TestARequestForAnUnknownAccountLooksTheSameAndStoresNothing(t *testing.T) {
	defer services.StubEmailDelivery(func(func()) {})()
	e := newEnv(t)
	e.emailOn()
	w := send(e.anon, http.MethodPost, "/password-reset/request", `{"email":"nobody@example.com"}`)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, resetMessage, w.Body.String())
	var n int64
	e.db.Model(&models.EmailOutbox{}).Count(&n)
	assert.Zero(t, n)
	e.db.Model(&models.User{}).Where("password_reset_token_hash IS NOT NULL").Count(&n)
	assert.Zero(t, n)
}

func TestWithoutAMailChannelARequestStillSucceeds(t *testing.T) {
	e := newEnv(t)
	w := send(e.anon, http.MethodPost, "/password-reset/request", `{"email":"a@example.com"}`)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotNil(t, e.reload(t, e.a.ID).PasswordResetTokenHash)
}

func TestARequestWithABadEmailIsRefused(t *testing.T) {
	e := newEnv(t)
	for _, body := range []string{`{}`, `{"email":""}`, `{"email":"not-an-email"}`} {
		assert.Equal(t, http.StatusBadRequest, send(e.anon, http.MethodPost, "/password-reset/request", body).Code, body)
	}
}

// @001-US4
func TestAConfirmedResetChangesThePasswordEndsEverySessionAndIsSingleUse(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	e.withPassword(t, &e.b, oldPassword)
	e.session(t, e.a, sid1, 1)
	e.session(t, e.b, sid3, 1)
	e.withResetToken(t, e.a, "the-reset-token-value", now.Add(time.Hour))
	body := `{"token":"the-reset-token-value","password":"` + newPassword + `"}`

	w := send(e.anon, http.MethodPost, "/password-reset/confirm", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Password reset successful"}`, w.Body.String())

	got := e.reload(t, e.a.ID)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(got.Password), []byte(newPassword)))
	assert.Nil(t, got.PasswordResetTokenHash)
	assert.Nil(t, got.PasswordResetExpiresAt)
	assert.Nil(t, got.PasswordResetRequestedAt)
	assert.Zero(t, e.liveSessions(t, e.a.ID))
	assert.Equal(t, 1, e.tokenVersion(t, e.a.ID))
	assert.EqualValues(t, 1, e.liveSessions(t, e.b.ID))
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(e.reload(t, e.b.ID).Password), []byte(oldPassword)))
	entries := e.auditEvents(t)
	require.Len(t, entries, 1)
	assert.Equal(t, models.AuditPasswordReset, entries[0].Event)
	assert.Equal(t, &e.a.ID, entries[0].UserID)

	w = send(e.anon, http.MethodPost, "/password-reset/confirm", body)
	assert.Equal(t, http.StatusBadRequest, w.Code, "the token worked once")
	assert.Contains(t, w.Body.String(), "Password reset token is invalid or expired")
}

func TestAnUnknownOrExpiredTokenIsRefusedWithTheSameWords(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	e.withResetToken(t, e.a, "an-old-reset-token-value", now.Add(-time.Minute))

	for _, token := range []string{"an-old-reset-token-value", "never-issued-token-value"} {
		w := send(e.anon, http.MethodPost, "/password-reset/confirm", `{"token":"`+token+`","password":"`+newPassword+`"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code, token)
		var body struct {
			Error struct{ Code, Message string }
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "INVALID_INPUT", body.Error.Code)
		assert.Contains(t, w.Body.String(), "Password reset token is invalid or expired")
	}
	got := e.reload(t, e.a.ID)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(got.Password), []byte(oldPassword)))
	assert.Nil(t, got.PasswordResetTokenHash, "an expired token is cleared")
	assert.Empty(t, e.auditEvents(t))
}

func TestAConfirmedResetWithAnUnacceptablePasswordChangesNothing(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	e.withResetToken(t, e.a, "the-reset-token-value", now.Add(time.Hour))
	long := strings.Repeat("a long and winding passphrase ", 3)

	for name, body := range map[string]string{
		"short token": `{"token":"short","password":"` + newPassword + `"}`,
		"weak":        `{"token":"the-reset-token-value","password":"password"}`,
		"too long":    `{"token":"the-reset-token-value","password":"` + long + `"}`,
		"no password": `{"token":"the-reset-token-value"}`,
	} {
		w := send(e.anon, http.MethodPost, "/password-reset/confirm", body)
		assert.Equal(t, http.StatusBadRequest, w.Code, name+": "+w.Body.String())
	}
	assert.NotNil(t, e.reload(t, e.a.ID).PasswordResetTokenHash, "the token survives a rejected password")
}

func TestPasswordResetsAreRefusedInDemoMode(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")
	e := newEnv(t)
	for _, call := range []struct{ path, body string }{
		{"/password-reset/request", `{"email":"a@example.com"}`},
		{"/password-reset/confirm", `{"token":"the-reset-token-value","password":"` + newPassword + `"}`},
	} {
		w := send(e.anon, http.MethodPost, call.path, call.body)
		assert.Equal(t, http.StatusForbidden, w.Code, call.path)
		assert.Contains(t, w.Body.String(), "Password changes are disabled in demo mode")
	}
}

func TestThePasswordStrengthCheckIsPublicAndNeedsAPassword(t *testing.T) {
	e := newEnv(t)
	w := send(e.anon, http.MethodPost, "/check-password-strength", `{"password":"`+newPassword+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, true, got["is_valid"])
	for _, key := range []string{"entropy", "score", "feedback", "min_entropy", "char_set_size", "length"} {
		assert.Contains(t, got, key)
	}
	assert.EqualValues(t, len(newPassword), got["length"])

	w = send(e.anon, http.MethodPost, "/check-password-strength", `{"password":"abc"}`)
	var weak map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &weak))
	assert.Equal(t, false, weak["is_valid"])

	for _, body := range []string{`{}`, `{"password":""}`} {
		w = send(e.anon, http.MethodPost, "/check-password-strength", body)
		assert.Equal(t, http.StatusBadRequest, w.Code, body)
		assert.Contains(t, w.Body.String(), "MISSING_FIELD", body)
	}
}

func TestThePublicRoutesAreRateLimitedPerClientAddress(t *testing.T) {
	e := newEnv(t)
	for i, call := range []struct{ path, body string }{
		{"/check-password-strength", `{"password":"abc"}`},
		{"/password-reset/request", `{"email":"nobody@example.com"}`},
		{"/password-reset/confirm", `{"token":"never-issued-token-value","password":"` + newPassword + `"}`},
	} {
		ip := "198.51.100." + string(rune('1'+i))
		var last int
		for n := 0; n < 80; n++ {
			last = sendFrom(e.anon, ip, http.MethodPost, call.path, call.body).Code
			if last == http.StatusTooManyRequests {
				break
			}
		}
		assert.Equal(t, http.StatusTooManyRequests, last, call.path)
	}
}

func TestTheSpecDocumentsWhatTheMiddlewareAndDemoModeAnswer(t *testing.T) {
	e := newEnv(t)
	r := gin.New()
	a := api.New(r.Group(""), r)
	users.New(users.Deps{DB: e.db, Config: e.cfg}).Register(a, r.Group(""), r.Group(""))

	for _, path := range []string{"/check-password-strength", "/password-reset/request", "/password-reset/confirm"} {
		assert.Contains(t, a.OpenAPI().Paths[path].Post.Responses, "429", path)
	}
	for _, path := range []string{"/password-reset/request", "/password-reset/confirm"} {
		assert.Contains(t, a.OpenAPI().Paths[path].Post.Responses, "403", path)
	}
}
