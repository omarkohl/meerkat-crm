package admin_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"meerkat/clock"
	"meerkat/config"
	"meerkat/models"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const strongPassword = "TrulySecurePassphrase99#"

func doJSON(r http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, target, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func uid(id uint) string { return strconv.Itoa(int(id)) }

func (e *env) asAdmin() *gin.Engine { return e.as(e.admin.ID, e.config()) }

func TestUserEndpointsRefuseAPlainUser(t *testing.T) {
	e := newEnv(t)
	id := uid(e.admin.ID)
	r := e.as(e.plain.ID, e.config())
	for _, tc := range []struct{ method, path string }{
		{"GET", "/admin/users"}, {"GET", "/admin/users/" + id}, {"PATCH", "/admin/users/" + id}, {"DELETE", "/admin/users/" + id},
		{"POST", "/admin/users/" + id + "/2fa/reset"}, {"POST", "/admin/trigger-reminders"},
	} {
		w := doJSON(r, tc.method, tc.path, map[string]any{"is_admin": true})
		assert.Equal(t, http.StatusForbidden, w.Code, tc.method+" "+tc.path)
	}
	var reloaded models.User
	require.NoError(t, e.db.First(&reloaded, e.plain.ID).Error)
	assert.False(t, reloaded.IsAdmin, "a plain user cannot promote anyone")
}

func TestListingUsersPagesAndHidesSecrets(t *testing.T) {
	e := newEnv(t)
	r := e.asAdmin()
	w := doJSON(r, "GET", "/admin/users?limit=1", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, strings.ToLower(w.Body.String()), "password")
	body := decode(t, w)
	assert.EqualValues(t, 2, body["total"])
	assert.EqualValues(t, 2, body["total_pages"])
	assert.Len(t, body["users"], 1)
	assert.Equal(t, "root", body["users"].([]any)[0].(map[string]any)["username"])
	assert.EqualValues(t, 1, decode(t, doJSON(r, "GET", "/admin/users?page=abc&limit=-4", nil))["page"])

	assert.Equal(t, http.StatusNotFound, doJSON(r, "GET", "/admin/users/9999", nil).Code)
	assert.Equal(t, http.StatusBadRequest, doJSON(r, "GET", "/admin/users/abc", nil).Code)
	got := decode(t, doJSON(r, "GET", "/admin/users/"+uid(e.plain.ID), nil))
	assert.Equal(t, "plain", got["username"])
	assert.Equal(t, false, got["two_factor_enabled"])
}

func TestUpdatingUsersKeepsAnAdminAndNormalisesNames(t *testing.T) {
	e := newEnv(t)
	r := e.asAdmin()
	second := models.User{Username: "second", Email: "s@example.com", Password: "x", IsAdmin: true}
	require.NoError(t, e.db.Create(&second).Error)
	path := func(id uint) string { return "/admin/users/" + uid(id) }

	assert.Equal(t, http.StatusForbidden, doJSON(r, "PATCH", path(e.admin.ID), map[string]any{"is_admin": false}).Code, "own admin status stays")
	w := doJSON(r, "PATCH", path(second.ID), map[string]any{"is_admin": false})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, http.StatusForbidden, doJSON(r, "DELETE", path(e.admin.ID), nil).Code, "deleting yourself is refused")

	w = doJSON(r, "PATCH", path(e.plain.ID), map[string]any{"username": "NewName", "email": "New@Example.COM"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var u models.User
	require.NoError(t, e.db.First(&u, e.plain.ID).Error)
	assert.Equal(t, "newname", u.Username)
	assert.Equal(t, "new@example.com", u.Email)

	require.NoError(t, e.db.Exec("CREATE UNIQUE INDEX idx_users_username_t ON users(username)").Error)
	assert.Equal(t, http.StatusConflict, doJSON(r, "PATCH", path(e.plain.ID), map[string]any{"username": "root"}).Code)

	assert.Equal(t, http.StatusBadRequest, doJSON(r, "PATCH", path(e.plain.ID), map[string]any{"password": "short"}).Code)
	w = doJSON(r, "PATCH", path(e.plain.ID), map[string]any{"password": "A-Str0ng-Passw0rd!x", "email": nil})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, e.db.First(&u, e.plain.ID).Error)
	assert.NotContains(t, u.Password, "Passw0rd")
	assert.NotEqual(t, "x", u.Password)
	assert.Equal(t, "new@example.com", u.Email, "a null leaves the field alone")
}

func TestUpdateAuditDetailIsStructuredAndAPasswordEndsTheSessions(t *testing.T) {
	e := newEnv(t)
	target := models.User{Username: "target", Email: "t@example.com", Password: "x"}
	require.NoError(t, e.db.Create(&target).Error)
	require.NoError(t, e.db.Create(&models.Session{UserID: target.ID, PublicID: "t1", ExpiresAt: now.Add(time.Hour)}).Error)

	w := doJSON(e.asAdmin(), "PATCH", "/admin/users/"+uid(target.ID), map[string]any{"email": "new@example.com", "password": strongPassword})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var entry models.AuditLog
	require.NoError(t, e.db.Where("event = ?", models.AuditAdminUserUpdated).First(&entry).Error)
	assert.Equal(t, fmt.Sprintf("user:target#%d;changed:email,password", target.ID), entry.Detail)
	assert.NotContains(t, entry.Detail, strongPassword)
	assert.Equal(t, e.admin.ID, *entry.UserID)
	live, _ := services.ListSessions(e.db, target.ID, now)
	assert.Empty(t, live, "a password set by an admin ends the sessions")
}

func TestDeletingAUserRemovesTheirDataButKeepsTheAuditTrailAndOtherUsers(t *testing.T) {
	e := newEnv(t)
	victim := models.User{Username: "victim", Email: "victim@example.com", Password: "x"}
	bystander := models.User{Username: "bystander", Email: "by@example.com", Password: "x"}
	require.NoError(t, e.db.Create(&victim).Error)
	require.NoError(t, e.db.Create(&bystander).Error)
	db := e.db
	for _, u := range []models.User{victim, bystander} {
		require.NoError(t, db.Create(&models.ApiToken{UserID: u.ID, Name: "t", TokenHash: "h" + u.Username}).Error)
		require.NoError(t, db.Create(&models.IcsFeed{UserID: u.ID, TokenHash: "f" + u.Username, CreatedAt: now}).Error)
		require.NoError(t, db.Create(&models.CardDAVConnection{UserID: u.ID}).Error)
		require.NoError(t, db.Create(&models.CalendarSubscription{UserID: u.ID}).Error)
		wh := models.Webhook{UserID: u.ID}
		require.NoError(t, db.Create(&wh).Error)
		require.NoError(t, db.Create(&models.WebhookDelivery{WebhookID: wh.ID}).Error)
		id := u.ID
		require.NoError(t, db.Create(&models.EmailOutbox{UserID: &id, ToAddr: u.Email, Subject: "s", HTML: "h", Status: models.OutboxPending, NextAttemptAt: now}).Error)
		require.NoError(t, db.Create(&models.Session{UserID: u.ID, PublicID: "s" + u.Username, ExpiresAt: now.Add(time.Hour)}).Error)
	}
	var victimWebhook models.Webhook
	require.NoError(t, db.Where("user_id = ?", victim.ID).First(&victimWebhook).Error)
	services.WriteAudit(db, models.AuditLog{UserID: &victim.ID, Username: "victim", Event: models.AuditLogin})

	w := doJSON(e.asAdmin(), "DELETE", "/admin/users/"+uid(victim.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "User deleted successfully", decode(t, w)["message"])

	count := func(model any, where string, args ...any) int64 {
		var n int64
		db.Unscoped().Model(model).Where(where, args...).Count(&n)
		return n
	}
	for _, m := range []any{&models.ApiToken{}, &models.IcsFeed{}, &models.CardDAVConnection{}, &models.CalendarSubscription{}, &models.Webhook{}, &models.EmailOutbox{}, &models.Session{}} {
		assert.Zero(t, count(m, "user_id = ?", victim.ID), "%T must be gone", m)
		assert.EqualValues(t, 1, count(m, "user_id = ?", bystander.ID), "%T of the bystander stays", m)
	}
	assert.Zero(t, count(&models.WebhookDelivery{}, "webhook_id = ?", victimWebhook.ID))
	assert.EqualValues(t, 1, count(&models.WebhookDelivery{}, "1 = 1"), "only the bystander's delivery remains")

	var logs []models.AuditLog
	db.Order("id").Find(&logs)
	require.Len(t, logs, 2)
	assert.Equal(t, "victim", logs[0].Username, "the old entry keeps the name")
	assert.Equal(t, models.AuditAdminUserDeleted, logs[1].Event)
	assert.Equal(t, fmt.Sprintf("user:victim#%d", victim.ID), logs[1].Detail)
	assert.Equal(t, e.admin.ID, *logs[1].UserID)
}

func TestADeletedOIDCUserReleasesTheirIdentity(t *testing.T) {
	e := newEnv(t)
	db := e.db
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_users_oidc_subject ON users(oidc_subject, oidc_provider) WHERE oidc_subject IS NOT NULL").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_users_username_t ON users(username)").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_users_email_t ON users(email)").Error)

	cfg := &config.Config{}
	cfg.OIDC.AllowAutoProvision = true
	claims := &services.OIDCClaims{Subject: "sub-1", Provider: "https://idp", Email: "bob@example.com", EmailVerified: true, PreferredUsername: "bob"}
	first, err := services.FindOrProvisionUser(db, claims, cfg)
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.Contact{UserID: first.ID, Firstname: "Old"}).Error)

	require.Equal(t, http.StatusOK, doJSON(e.asAdmin(), "DELETE", "/admin/users/"+uid(first.ID), nil).Code)

	second, err := services.FindOrProvisionUser(db, claims, cfg)
	require.NoError(t, err, "logging in again must create a new account")
	assert.NotEqual(t, first.ID, second.ID)
	assert.Equal(t, "bob@example.com", second.Email)
	var n int64
	db.Model(&models.Contact{}).Where("user_id = ?", first.ID).Count(&n)
	assert.Zero(t, n, "the old data stays deleted")
	db.Model(&models.User{}).Where("id = ?", first.ID).Count(&n)
	assert.Zero(t, n, "the old account stays soft-deleted")
}

func TestResettingTwoFactorNeedsItToBeOn(t *testing.T) {
	e := newEnv(t)
	r := e.asAdmin()
	assert.Equal(t, http.StatusNotFound, doJSON(r, "POST", "/admin/users/99999/2fa/reset", nil).Code)
	assert.Equal(t, http.StatusConflict, doJSON(r, "POST", "/admin/users/"+uid(e.plain.ID)+"/2fa/reset", nil).Code)

	require.NoError(t, e.db.Model(&e.plain).Updates(map[string]any{"totp_enabled": true, "totp_secret": "s"}).Error)
	require.NoError(t, e.db.Create(&models.Session{UserID: e.plain.ID, PublicID: "p1", ExpiresAt: now.Add(time.Hour)}).Error)
	w := doJSON(r, "POST", "/admin/users/"+uid(e.plain.ID)+"/2fa/reset", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var u models.User
	require.NoError(t, e.db.First(&u, e.plain.ID).Error)
	assert.False(t, u.TOTPEnabled)
	live, _ := services.ListSessions(e.db, e.plain.ID, now)
	assert.Empty(t, live)
	var entry models.AuditLog
	require.NoError(t, e.db.Where("event = ?", models.AuditTOTPAdminReset).First(&entry).Error)
	assert.Equal(t, fmt.Sprintf("user:plain#%d", e.plain.ID), entry.Detail)
	listed := decode(t, doJSON(r, "GET", "/admin/users", nil))["users"].([]any)
	assert.Equal(t, false, listed[1].(map[string]any)["two_factor_enabled"], "the list shows who still has it")
}

func TestTriggeringRemindersRunsTheJobWithTheRequestClockAndAuditsIt(t *testing.T) {
	e := newEnv(t)
	var got clock.Clock
	e.reminders = func(c clock.Clock) error { got = c; return nil }
	w := doJSON(e.asAdmin(), "POST", "/admin/trigger-reminders", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "Reminder emails sent successfully", decode(t, w)["message"])
	require.NotNil(t, got)
	assert.Equal(t, now, got.Now())
	var n int64
	e.db.Model(&models.AuditLog{}).Where("event = ?", models.AuditAdminReminders).Count(&n)
	assert.EqualValues(t, 1, n)

	e.reminders = func(clock.Clock) error { return errors.New("smtp down") }
	w = doJSON(e.asAdmin(), "POST", "/admin/trigger-reminders", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "smtp down")
	e.db.Model(&models.AuditLog{}).Where("event = ?", models.AuditAdminReminders).Count(&n)
	assert.EqualValues(t, 1, n, "a failed run is not audited")
}
