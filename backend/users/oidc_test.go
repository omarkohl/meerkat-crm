package users_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"meerkat/clock"
	"meerkat/config"
	"meerkat/models"
	"meerkat/services"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeIdP is a minimal OpenID provider: discovery, JWKS and a token endpoint that hands out an
// ID token with the claims set by the test.
type fakeIdP struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	claims jwt.MapClaims
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	idp := &fakeIdP{key: key}
	mux := http.NewServeMux()
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": idp.srv.URL, "authorization_endpoint": idp.srv.URL + "/authorize",
			"token_endpoint": idp.srv.URL + "/token", "jwks_uri": idp.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, idp.claims)
		tok.Header["kid"] = "k1"
		signed, err := tok.SignedString(key)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": signed})
	})
	return idp
}

func (idp *fakeIdP) idClaims(nonce, sub, email string) jwt.MapClaims {
	at := clock.Real().Now()
	return jwt.MapClaims{
		"iss": idp.srv.URL, "aud": "client", "sub": sub, "nonce": nonce, "email": email,
		"email_verified": true, "name": "Test User", "preferred_username": "Tester.Two",
		"exp": at.Add(time.Hour).Unix(), "iat": at.Unix(),
	}
}

// sso connects the module to the fake provider and returns an engine without a login.
func (e *env) sso(t *testing.T, idp *fakeIdP, mutate func(*config.Config)) http.Handler {
	t.Helper()
	e.cfg.OIDC = config.OIDCConfig{
		Enabled: true, ProviderURL: idp.srv.URL, ClientID: "client", ClientSecret: "s",
		RedirectURL: "http://app/cb", AllowAutoProvision: true,
	}
	if mutate != nil {
		mutate(e.cfg)
	}
	provider, err := services.InitOIDCProvider(context.Background(), e.cfg)
	require.NoError(t, err)
	e.oidc = provider
	return e.engine(nil, nil)
}

const callback = "/auth/oidc/callback"

func getFrom(r http.Handler, ip, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.RemoteAddr = ip + ":4000"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// visit makes the request from an address of its own, as the public routes are rate limited.
func visit(r http.Handler, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return getFrom(r, newClientAddress(), target, cookies...)
}

func cookieOf(w *httptest.ResponseRecorder, name string) *http.Cookie {
	var found *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			found = c
		}
	}
	return found
}

func state(s, n string) []*http.Cookie {
	return []*http.Cookie{{Name: "oidc_state", Value: s}, {Name: "oidc_nonce", Value: n}}
}

func TestSingleSignOnLoginSendsTheBrowserToTheProviderWithFreshStateAndNonce(t *testing.T) {
	e := newEnv(t)
	e.cfg.CookieDomain, e.cfg.CookieSecure = "example.org", true
	r := e.sso(t, newFakeIdP(t), nil)

	w := visit(r, "/auth/oidc/login")
	require.Equal(t, http.StatusFound, w.Code)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/authorize", loc.Path)
	assert.Equal(t, "client", loc.Query().Get("client_id"))
	assert.Equal(t, "code", loc.Query().Get("response_type"))
	assert.Equal(t, "http://app/cb", loc.Query().Get("redirect_uri"))
	assert.Contains(t, loc.Query().Get("scope"), "openid")

	st, nonce := cookieOf(w, "oidc_state"), cookieOf(w, "oidc_nonce")
	require.NotNil(t, st)
	require.NotNil(t, nonce)
	assert.Equal(t, st.Value, loc.Query().Get("state"))
	assert.Equal(t, nonce.Value, loc.Query().Get("nonce"))
	assert.Len(t, st.Value, 64)
	assert.NotEqual(t, st.Value, nonce.Value)
	for _, c := range []*http.Cookie{st, nonce} {
		assert.True(t, c.HttpOnly)
		assert.True(t, c.Secure)
		assert.Equal(t, "example.org", c.Domain)
		assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
		assert.Equal(t, "/api/v1/auth/oidc/callback", c.Path, "only the callback receives them")
		assert.Equal(t, 600, c.MaxAge)
	}
	assert.NotEqual(t, st.Value, cookieOf(visit(r, "/auth/oidc/login"), "oidc_state").Value, "every attempt is fresh")
}

func TestSingleSignOnStatusNamesTheProviderWhenConfigured(t *testing.T) {
	e := newEnv(t)
	idp := newFakeIdP(t)
	w := visit(e.sso(t, idp, nil), "/auth/oidc/config")
	require.Equal(t, http.StatusOK, w.Code)
	body := jsonOf(t, w)
	assert.Equal(t, true, body["enabled"])
	assert.Equal(t, services.ProviderName(idp.srv.URL), body["provider_name"])

	off := newEnv(t)
	w = visit(off.engine(nil, nil), "/auth/oidc/config")
	assert.JSONEq(t, `{"enabled":false}`, w.Body.String())
}

func TestAConfiguredProviderThatWasNotReachedLeavesOnlyTheStatus(t *testing.T) {
	e := newEnv(t)
	e.cfg.OIDC.Enabled = true
	r := e.engine(nil, nil)
	assert.Contains(t, visit(r, "/auth/oidc/config").Body.String(), `"enabled":true`)
	assert.Equal(t, http.StatusNotFound, visit(r, "/auth/oidc/login").Code)
	assert.Equal(t, http.StatusNotFound, visit(r, callback+"?state=s&code=c", state("s", "n")...).Code)
}

func TestWithoutSingleSignOnTheRoutesDoNotExist(t *testing.T) {
	e := newEnv(t)
	r := e.engine(nil, nil)
	assert.Equal(t, http.StatusNotFound, visit(r, "/auth/oidc/login").Code)
	assert.Equal(t, http.StatusNotFound, visit(r, callback+"?state=s&code=c", state("s", "n")...).Code)
}

func TestACallbackThatCannotBeTrustedEndsInTheLoginPageWithNoSession(t *testing.T) {
	e := newEnv(t)
	r := e.sso(t, newFakeIdP(t), nil)
	st, no := state("state-1", "nonce-1")[0], state("state-1", "nonce-1")[1]

	cases := map[string]*httptest.ResponseRecorder{
		"no cookies":        visit(r, callback+"?state=state-1&code=c"),
		"no nonce cookie":   visit(r, callback+"?state=state-1&code=c", st),
		"no state":          visit(r, callback+"?code=c", st, no),
		"state mismatch":    visit(r, callback+"?state=other&code=c", st, no),
		"missing code":      visit(r, callback+"?state=state-1", st, no),
		"empty state match": visit(r, callback+"?state=&code=c", &http.Cookie{Name: "oidc_state", Value: ""}, no),
	}
	for name, w := range cases {
		assert.Equal(t, http.StatusFound, w.Code, name)
		assert.Equal(t, "/login?error=oidc_error", w.Header().Get("Location"), name)
		assert.Nil(t, cookieOf(w, "auth_token"), name)
		assert.Negative(t, cookieOf(w, "oidc_state").MaxAge, name+": the attempt is used up")
	}
	assert.Empty(t, e.auditEvents(t))

	w := visit(r, callback+"?error=access_denied", st, no)
	assert.Equal(t, "/login?error=oidc_denied", w.Header().Get("Location"))
	assert.Nil(t, cookieOf(w, "oidc_state"), "a provider error ends the attempt before the cookies are touched")
}

func TestASuccessfulCallbackProvisionsTheAccountStartsASessionAndRedirectsHome(t *testing.T) {
	e := newEnv(t)
	e.cfg.CookieDomain, e.cfg.CookieSecure = "example.org", true
	idp := newFakeIdP(t)
	idp.claims = idp.idClaims("nonce-1", "sub-42", "New.Person@Example.com")
	r := e.sso(t, idp, nil)

	w := visit(r, callback+"?state=state-1&code=c&redirect=https://evil.example", state("state-1", "nonce-1")...)
	require.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/", w.Header().Get("Location"), "no part of the request chooses where the browser goes")
	auth := cookieOf(w, "auth_token")
	require.NotNil(t, auth)
	assert.True(t, auth.HttpOnly)
	assert.True(t, auth.Secure)
	assert.Equal(t, "example.org", auth.Domain)
	assert.Equal(t, "/", auth.Path)
	assert.Equal(t, http.SameSiteLaxMode, auth.SameSite)
	assert.Equal(t, 24*3600, auth.MaxAge)
	assert.Negative(t, cookieOf(w, "oidc_state").MaxAge)
	assert.Negative(t, cookieOf(w, "oidc_nonce").MaxAge)

	var u models.User
	require.NoError(t, e.db.Where("email = ?", "new.person@example.com").First(&u).Error)
	assert.Equal(t, "sub-42", *u.OIDCSubject)
	assert.Equal(t, idp.srv.URL, *u.OIDCProvider)
	assert.Equal(t, "tester_two", u.Username)
	assert.Empty(t, u.Password, "single sign-on accounts have no password")
	assert.False(t, u.IsAdmin)

	entries := e.auditEvents(t)
	require.Len(t, entries, 1)
	assert.Equal(t, models.AuditLogin, entries[0].Event)
	assert.Equal(t, "method:oidc", entries[0].Detail)
	assert.EqualValues(t, 1, e.liveSessions(t, u.ID))

	again := visit(r, callback+"?state=s2&code=c", state("s2", "nonce-1")...)
	assert.Equal(t, "/", again.Header().Get("Location"))
	var n int64
	e.db.Model(&models.User{}).Where("oidc_subject = ?", "sub-42").Count(&n)
	assert.EqualValues(t, 1, n, "the second sign-in finds the account by its subject")

	w = send(e.anon, http.MethodPost, "/login", `{"identifier":"tester_two","password":"anything at all"}`)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "there is no password to guess")
}

func TestAVerifiedEmailLinksTheIdentityToTheExistingAccountAndAnUnverifiedOneDoesNot(t *testing.T) {
	e := newEnv(t)
	idp := newFakeIdP(t)
	idp.claims = idp.idClaims("n", "sub-a", "a@example.com")
	r := e.sso(t, idp, nil)

	idp.claims["email_verified"] = false
	w := visit(r, callback+"?state=s&code=c", state("s", "n")...)
	assert.Equal(t, "/login?error=oidc_error", w.Header().Get("Location"), "no takeover through an unverified email")
	assert.Nil(t, e.reload(t, e.a.ID).OIDCSubject)

	idp.claims["email_verified"] = true
	w = visit(r, callback+"?state=s&code=c", state("s", "n")...)
	assert.Equal(t, "/", w.Header().Get("Location"))
	got := e.reload(t, e.a.ID)
	require.NotNil(t, got.OIDCSubject)
	assert.Equal(t, "sub-a", *got.OIDCSubject)
	assert.Nil(t, e.reload(t, e.b.ID).OIDCSubject)
}

func TestTrustingTheProvidersEmailLinksAnUnverifiedOne(t *testing.T) {
	e := newEnv(t)
	idp := newFakeIdP(t)
	idp.claims = idp.idClaims("n", "sub-a", "a@example.com")
	idp.claims["email_verified"] = false
	r := e.sso(t, idp, func(c *config.Config) { c.OIDC.TrustEmail = true })

	w := visit(r, callback+"?state=s&code=c", state("s", "n")...)
	assert.Equal(t, "/", w.Header().Get("Location"))
	assert.NotNil(t, e.reload(t, e.a.ID).OIDCSubject)
}

func TestWithoutAutoProvisioningAStrangerIsToldThereIsNoAccount(t *testing.T) {
	e := newEnv(t)
	idp := newFakeIdP(t)
	idp.claims = idp.idClaims("n", "sub", "stranger@example.com")
	r := e.sso(t, idp, func(c *config.Config) { c.OIDC.AllowAutoProvision = false })

	w := visit(r, callback+"?state=s&code=c", state("s", "n")...)
	assert.Equal(t, "/login?error=oidc_no_account", w.Header().Get("Location"))
	assert.Nil(t, cookieOf(w, "auth_token"))
	var n int64
	e.db.Model(&models.User{}).Where("email = ?", "stranger@example.com").Count(&n)
	assert.Zero(t, n)
}

func TestTheNonceAndTheTokenAreChecked(t *testing.T) {
	e := newEnv(t)
	idp := newFakeIdP(t)
	r := e.sso(t, idp, nil)
	fails := func(name string) {
		w := visit(r, callback+"?state=s&code=c", state("s", "n")...)
		assert.Equal(t, "/login?error=oidc_error", w.Header().Get("Location"), name)
		assert.Nil(t, cookieOf(w, "auth_token"), name)
	}

	idp.claims = idp.idClaims("someone-elses-nonce", "sub", "x@example.com")
	fails("nonce mismatch")
	idp.claims = idp.idClaims("n", "sub", "x@example.com")
	idp.claims["aud"] = "someone-else"
	fails("other audience")
	idp.claims = idp.idClaims("n", "sub", "x@example.com")
	idp.claims["exp"] = clock.Real().Now().Add(-time.Hour).Unix()
	fails("expired")
	var n int64
	e.db.Model(&models.User{}).Where("email = ?", "x@example.com").Count(&n)
	assert.Zero(t, n, "no account is created for a refused sign-in")
}

func TestSingleSignOnIsTheSecondFactorSoTwoFactorIsNotAsked(t *testing.T) {
	e := newEnv(t)
	idp := newFakeIdP(t)
	idp.claims = idp.idClaims("n", "sub-2fa", "sso2fa@example.com")
	r := e.sso(t, idp, nil)
	require.Equal(t, http.StatusFound, visit(r, callback+"?state=s&code=c", state("s", "n")...).Code)
	require.NoError(t, e.db.Model(&models.User{}).Where("email = ?", "sso2fa@example.com").
		Updates(map[string]any{"totp_enabled": true, "totp_secret": "x"}).Error)

	w := visit(r, callback+"?state=s&code=c", state("s", "n")...)
	assert.Equal(t, "/", w.Header().Get("Location"))
	assert.NotNil(t, cookieOf(w, "auth_token"))
}

func TestTheSingleSignOnRoutesAreRateLimitedPerAddress(t *testing.T) {
	e := newEnv(t)
	r := e.sso(t, newFakeIdP(t), nil)
	for target, ip := range map[string]string{"/auth/oidc/login": "198.51.100.61", callback + "?error=x": "198.51.100.60"} {
		var last int
		for n := 0; n < 80; n++ {
			last = getFrom(r, ip, target).Code
			if last == http.StatusTooManyRequests {
				break
			}
		}
		assert.Equal(t, http.StatusTooManyRequests, last, target)
	}
}
