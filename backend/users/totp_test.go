package users_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"
	"meerkat/users"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (e *env) enrol(t *testing.T, r http.Handler, password string) (string, []string) {
	t.Helper()
	w := send(r, http.MethodPost, "/users/me/2fa/setup", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var setup struct {
		Secret     string `json:"secret"`
		OtpauthURI string `json:"otpauth_uri"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &setup))
	assert.Contains(t, setup.OtpauthURI, "otpauth://totp/Meerkat%20CRM:a?")
	code := services.TOTPCode(setup.Secret, clock.Real().Now())
	w = send(r, http.MethodPost, "/users/me/2fa/enable", `{"password":"`+password+`","code":"`+code+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var codes struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &codes))
	require.Len(t, codes.RecoveryCodes, services.RecoveryCodeCount)
	return setup.Secret, codes.RecoveryCodes
}

func resetLockout(t *testing.T, id uint) {
	t.Helper()
	middleware.GetAccountRateLimiter().RecordSuccessfulLogin(middleware.SecondFactorKey(id))
	t.Cleanup(func() { middleware.GetAccountRateLimiter().RecordSuccessfulLogin(middleware.SecondFactorKey(id)) })
}

func TestTwoFactorStartsOffAndIsAvailableToAccountsWithAPassword(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	w := send(e.asA, http.MethodGet, "/users/me/2fa", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"enabled":false,"recovery_codes_remaining":0,"available":true}`, w.Body.String())

	w = send(e.asB, http.MethodGet, "/users/me/2fa", "")
	assert.JSONEq(t, `{"enabled":false,"recovery_codes_remaining":0,"available":true}`, w.Body.String())
}

func TestSingleSignOnAccountsCannotEnrol(t *testing.T) {
	e := newEnv(t)
	require.NoError(t, e.db.Model(&e.a).Update("password", "").Error)

	assert.JSONEq(t, `{"enabled":false,"recovery_codes_remaining":0,"available":false}`, send(e.asA, http.MethodGet, "/users/me/2fa", "").Body.String())
	w := send(e.asA, http.MethodPost, "/users/me/2fa/setup", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Two-factor authentication is not available for single sign-on accounts")
	w = send(e.asA, http.MethodPost, "/users/me/2fa/enable", `{"password":"anything","code":"123456"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestEnrolmentTurnsTwoFactorOnAndSignsOtherBrowsersOut(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	resetLockout(t, e.a.ID)
	e.session(t, e.a, sid1, 1)
	e.session(t, e.a, sid2, 0)
	r := e.engine(&e.a, map[string]any{"sessionID": sid1})

	_, codes := e.enrol(t, r, oldPassword)

	st := send(r, http.MethodGet, "/users/me/2fa", "")
	assert.JSONEq(t, `{"enabled":true,"recovery_codes_remaining":10,"available":true}`, st.Body.String())
	assert.EqualValues(t, 1, e.liveSessions(t, e.a.ID), "the browser that enrolled stays")
	assert.Len(t, codes, 10)
	got := e.reload(t, e.a.ID)
	assert.NotContains(t, got.TOTPSecret, "=", "stored encrypted, never as the base32 secret")
	events := e.auditEvents(t)
	require.Len(t, events, 1)
	assert.Equal(t, models.AuditTOTPEnabled, events[0].Event)

	w := send(r, http.MethodPost, "/users/me/2fa/setup", "")
	assert.Equal(t, http.StatusConflict, w.Code, "setup is refused while enabled")
	assert.Contains(t, w.Body.String(), "Two-factor authentication is already enabled")
}

func TestEnablingNeedsThePasswordAndAValidCode(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	resetLockout(t, e.a.ID)
	w := send(e.asA, http.MethodPost, "/users/me/2fa/setup", "")
	require.Equal(t, http.StatusOK, w.Code)

	w = send(e.asA, http.MethodPost, "/users/me/2fa/enable", `{"password":"wrong","code":"000000"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Password is incorrect")
	w = send(e.asA, http.MethodPost, "/users/me/2fa/enable", `{"password":"`+oldPassword+`","code":"000000"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid verification code")
	assert.False(t, e.reload(t, e.a.ID).TOTPEnabled)
	for _, body := range []string{`{"code":"123456"}`, `{"password":"x"}`, `{"password":"x","code":"12345678901234567"}`} {
		assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPost, "/users/me/2fa/enable", body).Code, body)
	}
}

func TestDisablingAndNewRecoveryCodesNeedAnEnabledSecondFactor(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	resetLockout(t, e.a.ID)
	for _, path := range []string{"/users/me/2fa/disable", "/users/me/2fa/recovery-codes"} {
		w := send(e.asA, http.MethodPost, path, `{"password":"`+oldPassword+`","code":"123456"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code, path)
		assert.Contains(t, w.Body.String(), "Two-factor authentication is not enabled", path)
	}
}

func TestDisablingWorksWithACodeOrAOneTimeRecoveryCode(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	resetLockout(t, e.a.ID)
	secret, codes := e.enrol(t, e.asA, oldPassword)

	w := send(e.asA, http.MethodPost, "/users/me/2fa/disable", `{"password":"`+oldPassword+`","recovery_code":"WRONG-CODE"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.True(t, e.reload(t, e.a.ID).TOTPEnabled)

	w = send(e.asA, http.MethodPost, "/users/me/2fa/recovery-codes", `{"password":"`+oldPassword+`","recovery_code":"`+codes[0]+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var fresh struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &fresh))
	assert.Len(t, fresh.RecoveryCodes, 10)
	assert.NotEqual(t, codes, fresh.RecoveryCodes)
	w = send(e.asA, http.MethodPost, "/users/me/2fa/disable", `{"password":"`+oldPassword+`","recovery_code":"`+codes[1]+`"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, "the old recovery codes stopped working")

	code := services.TOTPCode(secret, clock.Real().Now().Add(30*time.Second))
	w = send(e.asA, http.MethodPost, "/users/me/2fa/disable", `{"password":"`+oldPassword+`","code":"`+code+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Two-factor authentication disabled"}`, w.Body.String())
	got := e.reload(t, e.a.ID)
	assert.False(t, got.TOTPEnabled)
	assert.Empty(t, got.TOTPSecret)

	var kinds []string
	for _, entry := range e.auditEvents(t) {
		kinds = append(kinds, entry.Event)
	}
	assert.Equal(t, []string{models.AuditTOTPEnabled, models.AuditTOTPRecoveryUsed, models.AuditTOTPRecoveryRegenerated, models.AuditTOTPDisabled}, kinds)
}

func TestWrongAttemptsLockTheSecondFactorWith429AndTheLoginShapedBody(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	resetLockout(t, e.a.ID)
	e.enrol(t, e.asA, oldPassword)

	var last int
	for i := 0; i < middleware.MaxLoginAttempts; i++ {
		w := send(e.asA, http.MethodPost, "/users/me/2fa/disable", `{"password":"wrong"}`)
		last = w.Code
		if last == http.StatusTooManyRequests {
			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "Account temporarily locked", body["error"])
			assert.Equal(t, "Too many failed attempts. Please try again later.", body["message"])
			assert.Greater(t, body["retry_after"], float64(0))
			_, err := time.Parse(time.RFC3339, body["retry_after_at"].(string))
			assert.NoError(t, err)
		}
	}
	assert.Equal(t, http.StatusTooManyRequests, last)

	w := send(e.asA, http.MethodPost, "/users/me/2fa/disable", `{"password":"`+oldPassword+`","code":"000000"}`)
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "locked: even a correct password is not looked at")
	assert.True(t, e.reload(t, e.a.ID).TOTPEnabled)
}

func TestAnAPITokenCannotManageTwoFactor(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	token := e.engine(&e.a, map[string]any{"isAPIToken": true})
	for _, call := range []struct{ method, path, body string }{
		{http.MethodGet, "/users/me/2fa", ""}, {http.MethodPost, "/users/me/2fa/setup", ""},
		{http.MethodPost, "/users/me/2fa/enable", `{"password":"x","code":"1"}`},
		{http.MethodPost, "/users/me/2fa/disable", `{"password":"x"}`},
		{http.MethodPost, "/users/me/2fa/recovery-codes", `{"password":"x"}`},
	} {
		w := send(token, call.method, call.path, call.body)
		assert.Equal(t, http.StatusForbidden, w.Code, call.method+" "+call.path)
		assert.Contains(t, w.Body.String(), "API tokens cannot manage two-factor authentication")
	}
}

func TestTwoFactorRefusesWithoutAnAuthenticatedUser(t *testing.T) {
	e := newEnv(t)
	for _, call := range []struct{ method, path, body string }{
		{http.MethodGet, "/users/me/2fa", ""}, {http.MethodPost, "/users/me/2fa/setup", ""},
		{http.MethodPost, "/users/me/2fa/enable", `{"password":"x","code":"1"}`},
		{http.MethodPost, "/users/me/2fa/disable", `{"password":"x"}`},
		{http.MethodPost, "/users/me/2fa/recovery-codes", `{"password":"x"}`},
	} {
		assert.Equal(t, http.StatusUnauthorized, send(e.anon, call.method, call.path, call.body).Code, call.method+" "+call.path)
	}
}

func TestTheSpecDocumentsTheLockoutAnswer(t *testing.T) {
	e := newEnv(t)
	r := gin.New()
	a := api.New(r.Group(""), r)
	users.New(users.Deps{DB: e.db, Config: e.cfg}).Register(a, r.Group(""), r.Group(""))

	for _, path := range []string{"/users/me/2fa/enable", "/users/me/2fa/disable", "/users/me/2fa/recovery-codes"} {
		responses := a.OpenAPI().Paths[path].Post.Responses
		assert.Contains(t, responses, "429", path)
		assert.Contains(t, responses, "default", path, "the error format is still documented")
	}
}

func TestEnablingEndsOtherSessionsAtTheApplicationTime(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	resetLockout(t, e.a.ID)
	e.session(t, e.a, sid1, 1)
	other := e.session(t, e.a, sid2, 0)

	e.enrol(t, e.engine(&e.a, map[string]any{"sessionID": sid1}), oldPassword)

	var stored models.Session
	require.NoError(t, e.db.First(&stored, other.ID).Error)
	require.NotNil(t, stored.RevokedAt)
	assert.True(t, stored.RevokedAt.Equal(now), "stamped with the request clock, which follows FAKE_NOW")
}

func TestEnablingWithoutASetupAndRefusedDialogsChangeNothing(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	resetLockout(t, e.a.ID)
	w := send(e.asA, http.MethodPost, "/users/me/2fa/enable", `{"password":"`+oldPassword+`","code":"123456"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, "no setup yet")
	assert.Contains(t, w.Body.String(), "Invalid verification code")

	secret, codes := e.enrol(t, e.asA, oldPassword)
	good := services.TOTPCode(secret, clock.Real().Now().Add(30*time.Second))
	for _, path := range []string{"/users/me/2fa/disable", "/users/me/2fa/recovery-codes"} {
		middleware.GetAccountRateLimiter().RecordSuccessfulLogin(middleware.SecondFactorKey(e.a.ID))
		w = send(e.asA, http.MethodPost, path, `{"password":"wrong","code":"`+good+`"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code, path+" with a wrong password")
		assert.Contains(t, w.Body.String(), "Password is incorrect")
		w = send(e.asA, http.MethodPost, path, `{"password":"`+oldPassword+`"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code, path+" without a code")
		w = send(e.asA, http.MethodPost, path, `{"password":"`+oldPassword+`","code":"000000"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code, path+" with a wrong code")
	}
	assert.True(t, e.reload(t, e.a.ID).TOTPEnabled)

	middleware.GetAccountRateLimiter().RecordSuccessfulLogin(middleware.SecondFactorKey(e.a.ID))
	w = send(e.asA, http.MethodPost, "/users/me/2fa/disable", `{"password":"`+oldPassword+`","recovery_code":"`+codes[0]+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var left int64
	e.db.Model(&models.RecoveryCode{}).Where("user_id = ?", e.a.ID).Count(&left)
	assert.Zero(t, left, "disabling removes the recovery codes")
}

func TestOneUsersTwoFactorCannotBeSeenOrChangedByAnotherUser(t *testing.T) {
	e := newEnv(t)
	e.withPassword(t, &e.a, oldPassword)
	e.withPassword(t, &e.b, oldPassword)
	resetLockout(t, e.a.ID)
	resetLockout(t, e.b.ID)
	secret, codes := e.enrol(t, e.asA, oldPassword)
	storedBefore := e.reload(t, e.a.ID).TOTPSecret
	var codesBefore int64
	e.db.Model(&models.RecoveryCode{}).Where("user_id = ?", e.a.ID).Count(&codesBefore)
	require.EqualValues(t, services.RecoveryCodeCount, codesBefore)

	assert.JSONEq(t, `{"enabled":false,"recovery_codes_remaining":0,"available":true}`, send(e.asB, http.MethodGet, "/users/me/2fa", "").Body.String())
	assert.GreaterOrEqual(t, send(e.asB, http.MethodPost, "/users/me/2fa/recovery-codes", `{"password":"`+oldPassword+`","code":"`+services.TOTPCode(secret, clock.Real().Now())+`"}`).Code, 400)
	assert.GreaterOrEqual(t, send(e.asB, http.MethodPost, "/users/me/2fa/disable", `{"password":"`+oldPassword+`","recovery_code":"`+codes[0]+`"}`).Code, 400)
	w := send(e.asB, http.MethodPost, "/users/me/2fa/setup", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), secret, "the other user's secret is not shown")
	assert.GreaterOrEqual(t, send(e.asB, http.MethodPost, "/users/me/2fa/enable", `{"password":"`+oldPassword+`","code":"`+services.TOTPCode(secret, clock.Real().Now())+`"}`).Code, 400,
		"a code for another user's secret does not enable it")

	assert.Equal(t, storedBefore, e.reload(t, e.a.ID).TOTPSecret)
	assert.True(t, e.reload(t, e.a.ID).TOTPEnabled)
	var codesAfter int64
	e.db.Model(&models.RecoveryCode{}).Where("user_id = ?", e.a.ID).Count(&codesAfter)
	assert.Equal(t, codesBefore, codesAfter)
	assert.False(t, e.reload(t, e.b.ID).TOTPEnabled)
}
