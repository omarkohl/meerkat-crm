package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"meerkat/config"
	"meerkat/database"
	"meerkat/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var flowRuns atomic.Int32

// flowApp calls from an address of its own: the rate limiters are global and keyed by address,
// so sharing the default test address would starve other tests.
type flowApp struct {
	router *gin.Engine
	cookie *http.Cookie
	addr   string
}

func (a *flowApp) call(method, path, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = a.addr
	if mutate != nil {
		mutate(req)
	}
	w := httptest.NewRecorder()
	a.router.ServeHTTP(w, req)
	return w
}

func (a *flowApp) asUser(method, path, body string) *httptest.ResponseRecorder {
	return a.call(method, path, body, func(r *http.Request) { r.AddCookie(a.cookie) })
}

func asToken(token string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

// @017-US1
func TestAnAPITokenWorksForDataButNotForAccountSecurityAndStopsWhenRevoked(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db, err := database.InitDB(filepath.Join(t.TempDir(), "app.db"))
	require.NoError(t, err)
	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!", JWTExpiryHours: 24, ProfilePhotoDir: t.TempDir()}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("db", db); c.Set("cfg", *cfg); c.Next() })
	RegisterRoutes(r, cfg, db, NewModules(db, cfg, nil))
	app := &flowApp{router: r, addr: fmt.Sprintf("203.0.113.%d:4000", flowRuns.Add(1))}
	t.Cleanup(func() { middleware.GetAccountRateLimiter().RecordSuccessfulLogin("tokenuser") })

	const password = "Correct-horse-1!"
	w := app.call("POST", "/api/v1/register", `{"username":"tokenuser","email":"token@example.com","password":"`+password+`"}`, nil)
	require.Less(t, w.Code, 300, w.Body.String())
	w = app.call("POST", "/api/v1/login", `{"identifier":"tokenuser","password":"`+password+`"}`, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	for _, c := range w.Result().Cookies() {
		if c.Name == "auth_token" {
			app.cookie = c
		}
	}
	require.NotNil(t, app.cookie)

	w = app.asUser("POST", "/api/v1/api-tokens", `{"name":"sync script"}`)
	require.Less(t, w.Code, 300, w.Body.String())
	var created struct {
		Token string `json:"token"`
		ID    uint   `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.True(t, strings.HasPrefix(created.Token, "meerkat_"), w.Body.String())
	token := asToken(created.Token)

	w = app.call("POST", "/api/v1/contacts", `{"firstname":"Via","lastname":"Token"}`, token)
	assert.Less(t, w.Code, 300, "a token can write data: %s", w.Body.String())
	w = app.call("GET", "/api/v1/contacts", "", token)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Via")

	require.Equal(t, http.StatusOK, app.asUser("GET", "/api/v1/admin/users", "").Code, "the first account is an administrator")
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/api/v1/users/change-password", `{"current_password":"` + password + `","new_password":"Another-horse-2!"}`, http.StatusUnauthorized},
		{"POST", "/api/v1/users/me/2fa/setup", `{"password":"` + password + `"}`, http.StatusForbidden},
		{"GET", "/api/v1/sessions", "", http.StatusForbidden},
		{"POST", "/api/v1/sessions/revoke-all", "", http.StatusForbidden},
		{"GET", "/api/v1/admin/users", "", http.StatusForbidden},
	} {
		w = app.call(tc.method, tc.path, tc.body, token)
		assert.Equal(t, tc.want, w.Code, "%s %s: %s", tc.method, tc.path, w.Body.String())
	}
	w = app.call("POST", "/api/v1/login", `{"identifier":"tokenuser","password":"`+password+`"}`, nil)
	assert.Equal(t, http.StatusOK, w.Code, "the token did not change the password")

	w = app.asUser("DELETE", "/api/v1/api-tokens/"+strconv.Itoa(int(created.ID)), "")
	require.Less(t, w.Code, 300, w.Body.String())
	w = app.call("GET", "/api/v1/contacts", "", token)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "a revoked token is refused")
	assert.Equal(t, http.StatusOK, app.asUser("GET", "/api/v1/contacts", "").Code, "the login itself still works")
}
