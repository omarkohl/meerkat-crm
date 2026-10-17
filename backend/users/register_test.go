package users_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"meerkat/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func registration(extra string) string {
	return `{"username":"Newbie","email":"New@Example.com","password":"` + newPassword + `"` + extra + `}`
}

func (e *env) userByName(t *testing.T, name string) models.User {
	t.Helper()
	var u models.User
	require.NoError(t, e.db.Where("username = ?", name).First(&u).Error)
	return u
}

func TestRegisteringCreatesALowerCasedAccountWithAHashedPassword(t *testing.T) {
	e := newEnv(t)
	w := send(e.anon, http.MethodPost, "/register", registration(`,"language":"de"`))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"User registered successfully"}`, w.Body.String())

	got := e.userByName(t, "newbie")
	assert.Equal(t, "new@example.com", got.Email)
	assert.Equal(t, "de", got.Language)
	assert.False(t, got.IsAdmin, "only the first account is the administrator")
	assert.NotEqual(t, newPassword, got.Password)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(got.Password), []byte(newPassword)))
}

func TestTheLanguageDefaultsToEnglish(t *testing.T) {
	e := newEnv(t)
	require.Equal(t, http.StatusCreated, send(e.anon, http.MethodPost, "/register", registration("")).Code)
	assert.Equal(t, "en", e.userByName(t, "newbie").Language)
}

func TestTheFirstAccountOfAnInstanceIsTheAdministrator(t *testing.T) {
	e := newEnv(t)
	require.NoError(t, e.db.Exec("DELETE FROM users").Error)

	require.Equal(t, http.StatusCreated, send(e.anon, http.MethodPost, "/register", registration("")).Code)
	assert.True(t, e.userByName(t, "newbie").IsAdmin)
	second := `{"username":"second","email":"second@example.com","password":"` + newPassword + `"}`
	require.Equal(t, http.StatusCreated, send(e.anon, http.MethodPost, "/register", second).Code)
	assert.False(t, e.userByName(t, "second").IsAdmin)
}

func TestAnExistingUsernameOrEmailIsAConflict(t *testing.T) {
	e := newEnv(t)
	for name, body := range map[string]string{
		"email":    `{"username":"fresh","email":"A@EXAMPLE.COM","password":"` + newPassword + `"}`,
		"username": `{"username":"A","email":"fresh@example.com","password":"` + newPassword + `"}`,
	} {
		w := send(e.anon, http.MethodPost, "/register", body)
		assert.Equal(t, http.StatusConflict, w.Code, name+": "+w.Body.String())
		assert.Contains(t, w.Body.String(), "User already exists", name)
		assert.Contains(t, w.Body.String(), `"email"`, name)
	}
	var n int64
	e.db.Model(&models.User{}).Count(&n)
	assert.EqualValues(t, 2, n)
}

func TestRegistrationRefusesWhatIsInvalid(t *testing.T) {
	e := newEnv(t)
	long := strings.Repeat("a long and winding passphrase ", 3)
	for name, body := range map[string]string{
		"no email":      `{"username":"u1","password":"` + newPassword + `"}`,
		"bad email":     `{"username":"u1","email":"nope","password":"` + newPassword + `"}`,
		"no username":   `{"email":"u1@example.com","password":"` + newPassword + `"}`,
		"at in name":    `{"username":"u@1","email":"u1@example.com","password":"` + newPassword + `"}`,
		"long name":     `{"username":"` + strings.Repeat("u", 51) + `","email":"u1@example.com","password":"` + newPassword + `"}`,
		"weak password": `{"username":"u1","email":"u1@example.com","password":"password"}`,
		"short":         `{"username":"u1","email":"u1@example.com","password":"Ab1!"}`,
		"bad language":  `{"username":"u1","email":"u1@example.com","password":"` + newPassword + `","language":"xx"}`,
		"too long":      `{"username":"u1","email":"u1@example.com","password":"` + long + `"}`,
	} {
		w := send(e.anon, http.MethodPost, "/register", body)
		assert.Equal(t, http.StatusBadRequest, w.Code, name+": "+w.Body.String())
	}
	assert.Contains(t, send(e.anon, http.MethodPost, "/register", `{"username":"u1","email":"u1@example.com"}`).Body.String(), "VALIDATION_ERROR")
}

func TestRegistrationCanBeSwitchedOffAndAnswersInItsOwnShape(t *testing.T) {
	e := newEnv(t)
	e.cfg.RegistrationDisabled = true
	r := e.engine(nil, nil)

	w := send(r, http.MethodPost, "/register", registration(""))
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.JSONEq(t, `{"error":{"code":"registration_disabled","message":"Registration is disabled."}}`, w.Body.String())
	var n int64
	e.db.Model(&models.User{}).Where("username = ?", "newbie").Count(&n)
	assert.Zero(t, n)

	w = send(r, http.MethodPost, "/register", `{"username":"u1"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, "an invalid request is refused before the switch is looked at")
}

func TestRegistrationIsRateLimitedPerClientAddress(t *testing.T) {
	e := newEnv(t)
	var last int
	for n := 0; n < 80; n++ {
		last = sendFrom(e.anon, "198.51.100.9", http.MethodPost, "/register", `{"username":"x"}`).Code
		if last == http.StatusTooManyRequests {
			break
		}
	}
	assert.Equal(t, http.StatusTooManyRequests, last)
}

func TestTheCurrentUserIsTheCallerWithTheirPreferences(t *testing.T) {
	e := newEnv(t)
	require.NoError(t, e.db.Model(&e.a).Updates(map[string]any{"language": "fr", "is_admin": true}).Error)
	require.Equal(t, http.StatusOK, send(e.asA, http.MethodPatch, "/users/custom-fields", `{"names":["Pet"]}`).Code)

	w := send(e.asA, http.MethodGet, "/users/me", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var me map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &me))
	assert.EqualValues(t, e.a.ID, me["id"])
	assert.Equal(t, "a", me["username"])
	assert.Equal(t, "a@example.com", me["email"])
	assert.Equal(t, "fr", me["language"])
	assert.Equal(t, "eu", me["date_format"])
	assert.Equal(t, true, me["is_admin"])
	assert.Equal(t, []any{"Pet"}, me["custom_field_names"])
	assert.Nil(t, me["enabled_contact_fields"])
	for _, key := range []string{"created_at", "updated_at", "two_factor_enabled"} {
		assert.Contains(t, me, key)
	}
	assert.NotContains(t, me, "password")
	assert.NotContains(t, w.Body.String(), "totp")

	w = send(e.asB, http.MethodGet, "/users/me", "")
	assert.Contains(t, w.Body.String(), `"username":"b"`)
}

func TestTheCurrentUserIsAvailableToAnAPIToken(t *testing.T) {
	e := newEnv(t)
	w := send(e.engine(&e.a, map[string]any{"isAPIToken": true}), http.MethodGet, "/users/me", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"username":"a"`)
}

func TestTheCurrentUserNeedsALoginAndAnAccountThatExists(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, http.StatusUnauthorized, send(e.anon, http.MethodGet, "/users/me", "").Code)
	require.NoError(t, e.db.Delete(&e.a).Error)
	w := send(e.asA, http.MethodGet, "/users/me", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "User not found")
}
