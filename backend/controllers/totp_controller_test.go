package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTOTPEnv extends the session environment (which mounts the users module, two-factor
// included) with the admin routes.
func newTOTPEnv(t *testing.T) *sessionEnv {
	t.Helper()
	e := newSessionEnv(t)
	admin := e.r.Group("/admin", middleware.AuthMiddleware(e.cfg), middleware.AdminMiddleware())
	admin.POST("/users/:id/2fa/reset", ResetUserTOTP)
	admin.GET("/users", ListUsers)
	return e
}

func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out), string(body))
	return out
}

// enrol sets 2FA up for the logged-in user and returns the secret and the recovery codes.
func (e *sessionEnv) enrol(t *testing.T, token string) (string, []string) {
	t.Helper()
	w := e.do("POST", "/users/me/2fa/setup", token, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	setup := decode(t, w.Body.Bytes())
	secret := setup["secret"].(string)
	require.True(t, strings.HasPrefix(setup["otpauth_uri"].(string), "otpauth://totp/"))
	w = e.do("POST", "/users/me/2fa/enable", token, map[string]string{"password": strongPassword, "code": services.TOTPCode(secret, time.Now())})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.RecoveryCodes, services.RecoveryCodeCount)
	return secret, out.RecoveryCodes
}

// nextCode is a valid code for the step after the one enrolment used (accepted: one step of drift).
func nextCode(secret string) string { return services.TOTPCode(secret, time.Now().Add(30*time.Second)) }

func (e *sessionEnv) loginWith(name string, extra map[string]string) *httptest.ResponseRecorder {
	body := map[string]string{"identifier": name, "password": strongPassword}
	for k, v := range extra {
		body[k] = v
	}
	return e.do("POST", "/login", "", body)
}

func TestEnrolmentAndLoginRequireTheSecondStep(t *testing.T) {
	e := newTOTPEnv(t)
	u := e.user(t, "totp_flow")
	token := e.login(t, "totp_flow")

	// before enrolment: off
	st := decode(t, e.do("GET", "/users/me/2fa", token, nil).Body.Bytes())
	assert.Equal(t, false, st["enabled"])

	// setup alone does not turn it on; a wrong code does not either
	w := e.do("POST", "/users/me/2fa/setup", token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	secret := decode(t, w.Body.Bytes())["secret"].(string)
	w = e.do("POST", "/users/me/2fa/enable", token, map[string]string{"password": strongPassword, "code": "000000"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, http.StatusOK, e.loginWith("totp_flow", nil).Code, "still no second step")
	assert.Equal(t, false, decode(t, e.do("GET", "/users/me/2fa", token, nil).Body.Bytes())["enabled"])

	w = e.do("POST", "/users/me/2fa/enable", token, map[string]string{"password": strongPassword, "code": services.TOTPCode(secret, time.Now())})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	st = decode(t, e.do("GET", "/users/me/2fa", token, nil).Body.Bytes())
	assert.Equal(t, true, st["enabled"])
	assert.EqualValues(t, services.RecoveryCodeCount, st["recovery_codes_remaining"])
	assert.Len(t, e.events(models.AuditTOTPEnabled), 1)

	// password alone: asks for the code, sets no cookie, creates no session
	var before int64
	e.db.Model(&models.Session{}).Count(&before)
	w = e.loginWith("totp_flow", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, decode(t, w.Body.Bytes())["totp_required"])
	assert.Nil(t, cookie(w, "auth_token"))
	var after int64
	e.db.Model(&models.Session{}).Count(&after)
	assert.Equal(t, before, after)

	// wrong code
	w = e.loginWith("totp_flow", map[string]string{"totp_code": "000000"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Nil(t, cookie(w, "auth_token"))
	failed := e.events(models.AuditLoginFailed)
	require.NotEmpty(t, failed)
	assert.Equal(t, "reason:wrong_2fa_code", failed[len(failed)-1].Detail)
	assert.Equal(t, u.ID, *failed[len(failed)-1].UserID)

	// right code
	code := nextCode(secret)
	w = e.loginWith("totp_flow", map[string]string{"totp_code": code})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, cookie(w, "auth_token"))
	assert.Equal(t, http.StatusOK, e.do("GET", "/me", cookie(w, "auth_token").Value, nil).Code)

	// replay of the same code
	w = e.loginWith("totp_flow", map[string]string{"totp_code": code})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWrongPasswordNeverReachesTheSecondStep(t *testing.T) {
	e := newTOTPEnv(t)
	e.user(t, "totp_pw")
	token := e.login(t, "totp_pw")
	secret, _ := e.enrol(t, token)
	w := e.do("POST", "/login", "", map[string]string{"identifier": "totp_pw", "password": "wrong-password-1", "totp_code": nextCode(secret)})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Nil(t, cookie(w, "auth_token"))
}

func TestRecoveryCodeLogsInOnceAndIsAudited(t *testing.T) {
	e := newTOTPEnv(t)
	e.user(t, "totp_rec")
	token := e.login(t, "totp_rec")
	_, codes := e.enrol(t, token)

	w := e.loginWith("totp_rec", map[string]string{"recovery_code": strings.ToLower(codes[0])})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, cookie(w, "auth_token"))
	assert.Len(t, e.events(models.AuditTOTPRecoveryUsed), 1)

	w = e.loginWith("totp_rec", map[string]string{"recovery_code": codes[0]})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	st := decode(t, e.do("GET", "/users/me/2fa", token, nil).Body.Bytes())
	assert.EqualValues(t, services.RecoveryCodeCount-1, st["recovery_codes_remaining"])
}

func TestWrongCodesLockTheAccountAcrossIdentifiersAndPasswordOnlyRequestsDoNotResetTheCount(t *testing.T) {
	e := newTOTPEnv(t)
	lockUser := e.user(t, "totp_lock")
	token := e.login(t, "totp_lock")
	secret, _ := e.enrol(t, token)
	lim := middleware.GetAccountRateLimiter()
	t.Cleanup(func() {
		lim.RecordSuccessfulLogin("totp_lock")
		lim.RecordSuccessfulLogin("totp_lock@example.com")
		lim.RecordSuccessfulLogin(totpLimiterKey(lockUser.ID))
	})

	for i := 0; i < middleware.MaxLoginAttempts-1; i++ {
		// asking for the code (password only) between guesses must not clear the counter
		assert.Equal(t, http.StatusOK, e.loginWith("totp_lock", nil).Code)
		who := "totp_lock"
		if i%2 == 1 {
			who = "totp_lock@example.com" // another identifier for the same account
		}
		w := e.loginWith(who, map[string]string{"totp_code": "000000"})
		assert.Equal(t, http.StatusUnauthorized, w.Code, "attempt %d", i)
	}
	w := e.loginWith("totp_lock", map[string]string{"totp_code": "000000"})
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	failed := e.events(models.AuditLoginFailed)
	assert.Equal(t, "reason:wrong_2fa_code_locked", failed[len(failed)-1].Detail, "the attempt that locks is audited")
	// locked: even the right code is refused
	w = e.loginWith("totp_lock", map[string]string{"totp_code": nextCode(secret)})
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Nil(t, cookie(w, "auth_token"))
}

func TestSecretIsEncryptedAtRestAndNeverReturned(t *testing.T) {
	e := newTOTPEnv(t)
	u := e.user(t, "totp_enc")
	token := e.login(t, "totp_enc")
	secret, codes := e.enrol(t, token)

	var user models.User
	require.NoError(t, e.db.First(&user, u.ID).Error)
	assert.NotEmpty(t, user.TOTPSecret)
	assert.NotContains(t, user.TOTPSecret, secret)

	for _, path := range []string{"/users/me/2fa", "/users/me"} {
		body := e.do("GET", path, token, nil).Body.String()
		assert.NotContains(t, body, secret)
		assert.NotContains(t, strings.ToLower(body), "totp_secret")
		assert.NotContains(t, body, user.TOTPSecret)
	}
	var rows []models.RecoveryCode
	require.NoError(t, e.db.Find(&rows).Error)
	for _, r := range rows {
		for _, c := range codes {
			assert.NotContains(t, r.CodeHash, strings.ReplaceAll(c, "-", ""))
		}
	}
}

func TestTOTPIsPerUser(t *testing.T) {
	e := newTOTPEnv(t)
	e.user(t, "totp_a")
	e.user(t, "totp_b")
	tokenA := e.login(t, "totp_a")
	tokenB := e.login(t, "totp_b")
	e.enrol(t, tokenA)

	assert.Equal(t, false, decode(t, e.do("GET", "/users/me/2fa", tokenB, nil).Body.Bytes())["enabled"])
	assert.Equal(t, http.StatusOK, e.loginWith("totp_b", nil).Code)
	assert.Nil(t, decode(t, e.loginWith("totp_b", nil).Body.Bytes())["totp_required"])
	// B cannot use A's recovery codes or disable A's 2FA
	w := e.do("POST", "/users/me/2fa/disable", tokenB, map[string]string{"password": strongPassword, "code": "000000"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, true, decode(t, e.do("GET", "/users/me/2fa", tokenA, nil).Body.Bytes())["enabled"])
}

func TestAdminResetClearsTwoFactorAndSignsTheUserOut(t *testing.T) {
	e := newTOTPEnv(t)
	admin := e.user(t, "totp_admin")
	require.NoError(t, e.db.Model(&admin).Update("is_admin", true).Error)
	victim := e.user(t, "totp_victim")
	e.user(t, "totp_other")
	adminToken := e.login(t, "totp_admin")
	victimToken := e.login(t, "totp_victim")
	otherToken := e.login(t, "totp_other")
	e.enrol(t, victimToken)
	e.enrol(t, otherToken)

	// a normal user cannot
	assert.Equal(t, http.StatusForbidden, e.do("POST", "/admin/users/"+itoa(victim.ID)+"/2fa/reset", otherToken, nil).Code)
	assert.Equal(t, http.StatusNotFound, e.do("POST", "/admin/users/99999/2fa/reset", adminToken, nil).Code)

	w := e.do("POST", "/admin/users/"+itoa(victim.ID)+"/2fa/reset", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// the victim can log in with the password only, and the old session is gone
	w = e.loginWith("totp_victim", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotNil(t, cookie(w, "auth_token"))
	assert.Equal(t, http.StatusUnauthorized, e.do("GET", "/me", victimToken, nil).Code)
	entries := e.events(models.AuditTOTPAdminReset)
	require.Len(t, entries, 1)
	assert.Equal(t, "totp_admin", entries[0].Username)
	assert.Contains(t, entries[0].Detail, "totp_victim")

	// the other user is untouched
	assert.Equal(t, true, decode(t, e.do("GET", "/users/me/2fa", otherToken, nil).Body.Bytes())["enabled"])

	// the admin list tells which users have it
	list := e.do("GET", "/admin/users", adminToken, nil)
	require.Equal(t, http.StatusOK, list.Code)
	var parsed struct {
		Users []struct {
			Username         string `json:"username"`
			TwoFactorEnabled bool   `json:"two_factor_enabled"`
		} `json:"users"`
	}
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &parsed))
	got := map[string]bool{}
	for _, u := range parsed.Users {
		got[u.Username] = u.TwoFactorEnabled
	}
	assert.False(t, got["totp_victim"])
	assert.True(t, got["totp_other"])
}

func TestEnablingAndDisablingEndOtherSessionsButNotThisOne(t *testing.T) {
	e := newTOTPEnv(t)
	e.user(t, "totp_sess")
	laptop := e.login(t, "totp_sess")
	phone := e.login(t, "totp_sess")
	secret, codes := e.enrol(t, laptop)
	assert.Equal(t, http.StatusOK, e.do("GET", "/me", laptop, nil).Code)
	assert.Equal(t, http.StatusUnauthorized, e.do("GET", "/me", phone, nil).Code)

	w := e.loginWith("totp_sess", map[string]string{"totp_code": nextCode(secret)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	tablet := cookie(w, "auth_token").Value
	require.Equal(t, http.StatusOK, e.do("GET", "/me", tablet, nil).Code)

	w = e.do("POST", "/users/me/2fa/disable", laptop, map[string]string{"password": strongPassword, "recovery_code": codes[0]})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, http.StatusOK, e.do("GET", "/me", laptop, nil).Code)
	assert.Equal(t, http.StatusUnauthorized, e.do("GET", "/me", tablet, nil).Code)
}

func TestResetOfAUserWithoutTwoFactorIsRefusedAndKeepsTheirSessions(t *testing.T) {
	e := newTOTPEnv(t)
	admin := e.user(t, "totp_adm2")
	require.NoError(t, e.db.Model(&admin).Update("is_admin", true).Error)
	plain := e.user(t, "totp_plain")
	adminToken := e.login(t, "totp_adm2")
	plainToken := e.login(t, "totp_plain")
	w := e.do("POST", "/admin/users/"+itoa(plain.ID)+"/2fa/reset", adminToken, nil)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, http.StatusOK, e.do("GET", "/me", plainToken, nil).Code)
	assert.Empty(t, e.events(models.AuditTOTPAdminReset))
}
