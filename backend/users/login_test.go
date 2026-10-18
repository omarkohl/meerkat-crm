package users_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"meerkat/clock"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (e *env) loginAs(t *testing.T, identifier string) *httptest.ResponseRecorder {
	t.Helper()
	return send(e.anon, http.MethodPost, "/login", `{"identifier":"`+identifier+`","password":"`+oldPassword+`"}`)
}

// clearLimiter forgets the failed attempts counted for identifiers; the limiter is global.
func clearLimiter(t *testing.T, identifiers ...string) {
	t.Helper()
	reset := func() {
		for _, id := range identifiers {
			middleware.GetAccountRateLimiter().RecordSuccessfulLogin(id)
		}
	}
	reset()
	t.Cleanup(reset)
}

func jsonOf(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m), w.Body.String())
	return m
}

// @001-US2
func TestLoggingInByUsernameEmailOrLegacyEmailFieldSetsTheCookieAndAnswersWithPreferences(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	require.NoError(t, e.db.Model(&e.a).Updates(map[string]any{"language": "de", "date_format": "iso"}).Error)
	clearLimiter(t, "a", "a@example.com")

	for name, body := range map[string]string{
		"username":     `{"identifier":"A","password":"` + oldPassword + `"}`,
		"email":        `{"identifier":"A@Example.com","password":"` + oldPassword + `"}`,
		"legacy field": `{"email":"a@example.com","password":"` + oldPassword + `"}`,
	} {
		w := send(e.anon, http.MethodPost, "/login", body)
		require.Equal(t, http.StatusOK, w.Code, name+": "+w.Body.String())
		assert.JSONEq(t, `{"language":"de","date_format":"iso"}`, w.Body.String(), name)
		require.NotNil(t, authCookie(w), name)
	}
	assert.EqualValues(t, 3, e.liveSessions(t, e.a.ID))
}

func TestTheLoginCookieAndTokenCarryTheSessionAndTheConfiguredAttributes(t *testing.T) {
	e := newEnv(t)
	e.cfg.CookieDomain, e.cfg.CookieSecure = "example.org", true
	e.withPassword(t, &e.a, oldPassword)
	clearLimiter(t, "a")

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"identifier":"a","password":"`+oldPassword+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TestBrowser/1.0")
	req.RemoteAddr = "203.0.113.5:5555"
	w := httptest.NewRecorder()
	e.engine(nil, nil).ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	cookie := authCookie(w)
	require.NotNil(t, cookie)
	assert.True(t, cookie.HttpOnly)
	assert.True(t, cookie.Secure)
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	assert.Equal(t, "/", cookie.Path)
	assert.Equal(t, "example.org", cookie.Domain)
	assert.Equal(t, 24*3600, cookie.MaxAge)
	assert.NotContains(t, w.Body.String(), cookie.Value, "the token is only in the cookie")

	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(cookie.Value, claims, func(*jwt.Token) (any, error) { return []byte(e.cfg.JWTSecretKey), nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithTimeFunc(func() time.Time { return now }))
	require.NoError(t, err)
	assert.EqualValues(t, e.a.ID, claims["user_id"])
	assert.Equal(t, "a", claims["username"])
	assert.EqualValues(t, 0, claims["tv"])
	assert.EqualValues(t, now.Add(24*time.Hour).Unix(), claims["exp"])
	assert.NotContains(t, claims, "is_admin")

	var session models.Session
	require.NoError(t, e.db.First(&session).Error)
	assert.Equal(t, claims["sid"], session.PublicID)
	assert.Equal(t, "TestBrowser/1.0", session.UserAgent)
	assert.Equal(t, "203.0.113.5", session.IP)
	assert.True(t, session.ExpiresAt.Equal(now.Add(24*time.Hour)))
	entries := e.auditEvents(t)
	require.Len(t, entries, 1)
	assert.Equal(t, models.AuditLogin, entries[0].Event)
	assert.Equal(t, &e.a.ID, entries[0].UserID)
}

func TestAWrongPasswordAndAnUnknownAccountAnswerAlikeAndAreAuditedWithoutWhatWasTyped(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	clearLimiter(t, "a", "ghost")

	wrong := send(e.anon, http.MethodPost, "/login", `{"identifier":"a","password":"not the password"}`)
	unknown := send(e.anon, http.MethodPost, "/login", `{"identifier":"ghost","password":"not the password"}`)

	for _, w := range []*httptest.ResponseRecorder{wrong, unknown} {
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		body := jsonOf(t, w)["error"].(map[string]any)
		assert.Equal(t, "INVALID_CREDENTIALS", body["code"])
		assert.Equal(t, "Invalid email or password", body["message"])
	}
	assert.Nil(t, authCookie(wrong))
	assert.Zero(t, e.liveSessions(t, e.a.ID))

	entries := e.auditEvents(t)
	require.Len(t, entries, 2)
	assert.Equal(t, models.AuditLoginFailed, entries[0].Event)
	assert.Equal(t, "reason:wrong_password", entries[0].Detail)
	assert.Equal(t, &e.a.ID, entries[0].UserID)
	assert.Equal(t, "reason:unknown_account", entries[1].Detail)
	assert.Nil(t, entries[1].UserID, "an unknown account is not named: what was typed may be a password")
	assert.Empty(t, entries[1].Username)
}

func TestLoginNeedsAnIdentifierAndAPassword(t *testing.T) {
	e := newEnv(t)
	for name, body := range map[string]string{
		"nothing": `{}`, "no password": `{"identifier":"a"}`, "empty password": `{"identifier":"a","password":""}`, "no identifier": `{"password":"x"}`,
	} {
		w := send(e.anon, http.MethodPost, "/login", body)
		assert.Equal(t, http.StatusBadRequest, w.Code, name)
		assert.Contains(t, w.Body.String(), "MISSING_FIELD", name)
	}
	assert.Contains(t, send(e.anon, http.MethodPost, "/login", `{"password":"x"}`).Body.String(), "identifier")
	assert.Equal(t, http.StatusBadRequest, send(e.anon, http.MethodPost, "/login", `{"identifier":`).Code)
}

func TestWrongPasswordsLockTheIdentifierEvenForTheRightPasswordAndUnknownNamesCountToo(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	clearLimiter(t, "a", "a@example.com", "ghost")

	var last *httptest.ResponseRecorder
	for i := 0; i < middleware.MaxLoginAttempts; i++ {
		last = send(e.anon, http.MethodPost, "/login", `{"identifier":"a","password":"wrong"}`)
	}
	assert.Equal(t, http.StatusTooManyRequests, last.Code, "the attempt that locks answers 429")
	body := jsonOf(t, last)
	assert.Equal(t, "Account temporarily locked", body["error"])
	assert.Equal(t, "Too many failed login attempts. Please try again later.", body["message"])
	assert.Greater(t, body["retry_after"], float64(0))
	at, err := time.Parse(time.RFC3339, body["retry_after_at"].(string))
	require.NoError(t, err)
	assert.True(t, at.After(now), "retry_after_at counts from the request clock")
	assert.Equal(t, "reason:wrong_password_locked", e.auditEvents(t)[middleware.MaxLoginAttempts-1].Detail)

	assert.Equal(t, http.StatusTooManyRequests, e.loginAs(t, "a").Code, "locked: the right password is not even looked at")
	assert.Equal(t, http.StatusOK, e.loginAs(t, "a@example.com").Code, "the lock is by what was typed as the identifier")

	for i := 0; i < middleware.MaxLoginAttempts; i++ {
		last = send(e.anon, http.MethodPost, "/login", `{"identifier":"ghost","password":"wrong"}`)
	}
	assert.Equal(t, http.StatusUnauthorized, last.Code, "the attempt that locks an unknown name still looks like any other")
	assert.Equal(t, http.StatusTooManyRequests, send(e.anon, http.MethodPost, "/login", `{"identifier":"ghost","password":"wrong"}`).Code,
		"unknown names are counted like known ones")
}

func TestASuccessfulLoginForgetsEarlierWrongPasswords(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	clearLimiter(t, "a")
	for i := 0; i < middleware.MaxLoginAttempts-1; i++ {
		send(e.anon, http.MethodPost, "/login", `{"identifier":"a","password":"wrong"}`)
	}
	require.Equal(t, http.StatusOK, e.loginAs(t, "a").Code)
	for i := 0; i < middleware.MaxLoginAttempts-1; i++ {
		assert.Equal(t, http.StatusUnauthorized, send(e.anon, http.MethodPost, "/login", `{"identifier":"a","password":"wrong"}`).Code)
	}
}

func TestAnAccountWithTwoFactorAsksForTheCodeAndNeedsItToLogIn(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	clearLimiter(t, "a", "a@example.com")
	resetLockout(t, e.a.ID)
	secret, codes := e.enrol(t, e.asA, oldPassword)
	sessions := e.liveSessions(t, e.a.ID)
	login := func(extra string) *httptest.ResponseRecorder {
		return send(e.anon, http.MethodPost, "/login", `{"identifier":"a","password":"`+oldPassword+`"`+extra+`}`)
	}

	w := login("")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"totp_required":true}`, w.Body.String())
	assert.Nil(t, authCookie(w))
	assert.Equal(t, sessions, e.liveSessions(t, e.a.ID), "asking for the code starts no session")

	w = login(`,"totp_code":"000000"`)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "INVALID_TOTP_CODE", jsonOf(t, w)["error"].(map[string]any)["code"])
	assert.Equal(t, "reason:wrong_2fa_code", e.auditEvents(t)[len(e.auditEvents(t))-1].Detail)

	code := services.TOTPCode(secret, clock.Real().Now().Add(30*time.Second))
	w = login(`,"totp_code":"` + code + `"`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotNil(t, authCookie(w))
	assert.Equal(t, http.StatusUnauthorized, login(`,"totp_code":"`+code+`"`).Code, "a code works once")

	w = login(`,"recovery_code":"` + codes[0] + `"`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, http.StatusUnauthorized, login(`,"recovery_code":"`+codes[0]+`"`).Code, "a recovery code works once")
	var kinds []string
	for _, entry := range e.auditEvents(t) {
		kinds = append(kinds, entry.Event)
	}
	assert.Contains(t, kinds, models.AuditTOTPRecoveryUsed)
}

func TestWrongSecondFactorCodesLockTheAccountAcrossIdentifiers(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	clearLimiter(t, "a", "a@example.com")
	resetLockout(t, e.a.ID)
	secret, _ := e.enrol(t, e.asA, oldPassword)

	var last *httptest.ResponseRecorder
	for i := 0; i < middleware.MaxLoginAttempts; i++ {
		who := "a"
		if i%2 == 1 {
			who = "a@example.com"
		}
		last = send(e.anon, http.MethodPost, "/login", `{"identifier":"`+who+`","password":"`+oldPassword+`","totp_code":"000000"}`)
	}
	assert.Equal(t, http.StatusTooManyRequests, last.Code)
	assert.Equal(t, "Too many failed attempts. Please try again later.", jsonOf(t, last)["message"], "the second step has its own words")
	entries := e.auditEvents(t)
	assert.Equal(t, "reason:wrong_2fa_code_locked", entries[len(entries)-1].Detail)

	good := services.TOTPCode(secret, clock.Real().Now().Add(30*time.Second))
	w := send(e.anon, http.MethodPost, "/login", `{"identifier":"a","password":"`+oldPassword+`","totp_code":"`+good+`"}`)
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "locked: even the right code is refused")
	assert.Nil(t, authCookie(w))
}

// @001-US3
func TestLoggingOutEndsTheSessionOfTheCookieAndClearsIt(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	clearLimiter(t, "a")
	e.session(t, e.b, sid3, 1)
	token := authCookie(e.loginAs(t, "a")).Value
	require.EqualValues(t, 1, e.liveSessions(t, e.a.ID))

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	w := httptest.NewRecorder()
	e.anon.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Logged out successfully"}`, w.Body.String())
	cookie := authCookie(w)
	require.NotNil(t, cookie)
	assert.Empty(t, cookie.Value)
	assert.Less(t, cookie.MaxAge, 0)
	assert.Zero(t, e.liveSessions(t, e.a.ID))
	assert.EqualValues(t, 1, e.liveSessions(t, e.b.ID))
	entries := e.auditEvents(t)
	assert.Equal(t, models.AuditLogout, entries[len(entries)-1].Event)
	assert.Equal(t, &e.a.ID, entries[len(entries)-1].UserID)
}

func TestLoggingOutWithoutAUsableCookieStillSucceeds(t *testing.T) {
	e := newEnv(t)
	e.session(t, e.a, sid1, 1)
	for name, cookie := range map[string]*http.Cookie{
		"none": nil, "empty": {Name: "auth_token", Value: ""}, "garbage": {Name: "auth_token", Value: "garbage"},
	} {
		req := httptest.NewRequest(http.MethodPost, "/logout", nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		e.anon.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, name)
		require.NotNil(t, authCookie(w), name)
	}
	assert.EqualValues(t, 1, e.liveSessions(t, e.a.ID), "nothing was revoked")
	assert.Empty(t, e.auditEvents(t))
}

func TestLoginIsRateLimitedPerClientAddress(t *testing.T) {
	e := newEnv(t)
	var last *httptest.ResponseRecorder
	for n := 0; n < 80; n++ {
		identifier := fmt.Sprintf("address-limit-%d", n)
		clearLimiter(t, identifier)
		last = sendFrom(e.anon, "198.51.100.40", http.MethodPost, "/login", `{"identifier":"`+identifier+`","password":"y"}`)
		if last.Code == http.StatusTooManyRequests {
			break
		}
		require.Equal(t, http.StatusUnauthorized, last.Code, "a new identifier each time: no lockout is involved")
	}
	require.Equal(t, http.StatusTooManyRequests, last.Code)
	assert.NotContains(t, jsonOf(t, last), "retry_after", "the address limit, not an account lock")
	assert.Equal(t, "Rate limit exceeded", jsonOf(t, last)["error"])
}
