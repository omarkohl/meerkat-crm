package tokens_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/i18n"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/tokens"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var now = time.Date(2026, 3, 10, 15, 30, 0, 0, time.UTC)

type env struct {
	db   *gorm.DB
	a, b models.User
	asA  *gin.Engine
	asB  *gin.Engine
	anon *gin.Engine
}

func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Reminder{}, &models.ApiToken{}, &models.IcsFeed{}, &models.AuditLog{}))
	e := &env{db: db}
	e.a = models.User{Username: "plain", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "other", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)
	e.asA, e.asB, e.anon = e.engine(e.a.ID), e.engine(e.b.ID), e.engine(0)
	return e
}

// engine mounts the module as the router does. The user, when there is one, is in the
// context of every route, the public feed route too: it must ignore it.
func (e *env) engine(userID uint) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ClockMiddleware(clock.NewFake(now)))
	if userID != 0 {
		r.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
	}
	authenticated := r.Group("")
	tokens.New(tokens.Deps{DB: e.db}).Register(api.New(r.Group(""), r), r.Group(""), authenticated)
	return r
}

func do(r http.Handler, method, path, body string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type tokenJSON struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	Token      string     `json:"token"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

func createToken(t *testing.T, r http.Handler, name string) tokenJSON {
	t.Helper()
	w := do(r, "POST", "/api-tokens", fmt.Sprintf(`{"name":%q}`, name))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var tok tokenJSON
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tok))
	return tok
}

func listTokens(t *testing.T, r http.Handler) []tokenJSON {
	t.Helper()
	w := do(r, "GET", "/api-tokens", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct{ Tokens []tokenJSON }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Tokens
}

func TestAPITokensStartEmptyAsAList(t *testing.T) {
	e := newEnv(t)
	w := do(e.asA, "GET", "/api-tokens", "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"tokens":[]}`, w.Body.String())
}

func TestCreateAPITokenShowsThePlaintextOnceAndStoresOnlyTheHash(t *testing.T) {
	e := newEnv(t)
	tok := createToken(t, e.asA, "ci-token")
	assert.Equal(t, "ci-token", tok.Name)
	assert.NotZero(t, tok.ID)
	assert.True(t, strings.HasPrefix(tok.Token, "meerkat_"))
	assert.Nil(t, tok.LastUsedAt)
	assert.Nil(t, tok.RevokedAt)

	var stored models.ApiToken
	require.NoError(t, e.db.First(&stored, tok.ID).Error)
	assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(tok.Token))), stored.TokenHash)
	assert.Equal(t, e.a.ID, stored.UserID)

	for _, listed := range listTokens(t, e.asA) {
		assert.Empty(t, listed.Token)
	}
	assert.NotContains(t, do(e.asA, "GET", "/api-tokens", "").Body.String(), tok.Token)
}

func TestCreateAPITokenValidatesTheName(t *testing.T) {
	e := newEnv(t)
	for name, body := range map[string]string{
		"empty":   `{"name":""}`,
		"missing": `{}`,
		"long":    `{"name":"` + strings.Repeat("x", 101) + `"}`,
	} {
		assert.Equal(t, http.StatusBadRequest, do(e.asA, "POST", "/api-tokens", body).Code, name)
	}
}

func TestListAPITokensIsScopedNewestFirstAndIncludesRevoked(t *testing.T) {
	e := newEnv(t)
	createToken(t, e.asA, "first")
	second := createToken(t, e.asA, "second")
	createToken(t, e.asB, "of-b")
	require.Equal(t, http.StatusOK, do(e.asA, "DELETE", fmt.Sprintf("/api-tokens/%d", second.ID), "").Code)

	got := listTokens(t, e.asA)
	require.Len(t, got, 2)
	assert.Equal(t, "second", got[0].Name)
	assert.NotNil(t, got[0].RevokedAt)
	assert.Equal(t, "first", got[1].Name)
	assert.Nil(t, got[1].RevokedAt)
}

func TestRevokeAPITokenStampsTheRequestTime(t *testing.T) {
	e := newEnv(t)
	tok := createToken(t, e.asA, "to-revoke")
	w := do(e.asA, "DELETE", fmt.Sprintf("/api-tokens/%d", tok.ID), "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"message":"Token revoked successfully"}`, w.Body.String())

	var stored models.ApiToken
	require.NoError(t, e.db.First(&stored, tok.ID).Error)
	require.NotNil(t, stored.RevokedAt)
	assert.True(t, now.Equal(*stored.RevokedAt))
}

func TestRevokeAPITokenOfAnotherUserIs404AndChangesNothing(t *testing.T) {
	e := newEnv(t)
	tok := createToken(t, e.asB, "victim")
	assert.Equal(t, http.StatusNotFound, do(e.asA, "DELETE", fmt.Sprintf("/api-tokens/%d", tok.ID), "").Code)
	assert.Equal(t, http.StatusNotFound, do(e.asA, "DELETE", "/api-tokens/9999", "").Code)
	assert.Nil(t, listTokens(t, e.asB)[0].RevokedAt)
}

func TestRevokeAPITokenWithAMalformedIDIs400(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, http.StatusBadRequest, do(e.asA, "DELETE", "/api-tokens/not-a-number", "").Code)
}

func TestTokenAndFeedManagementNeedsAUser(t *testing.T) {
	e := newEnv(t)
	for _, c := range [][2]string{
		{"GET", "/api-tokens"}, {"POST", "/api-tokens"}, {"DELETE", "/api-tokens/1"},
		{"GET", "/ics-feed"}, {"POST", "/ics-feed"}, {"DELETE", "/ics-feed"},
	} {
		assert.Equal(t, http.StatusUnauthorized, do(e.anon, c[0], c[1], `{"name":"x"}`).Code, c[0]+" "+c[1])
	}
}

type feedJSON struct {
	Enabled       bool       `json:"enabled"`
	CreatedAt     *time.Time `json:"created_at"`
	LastFetchedAt *time.Time `json:"last_fetched_at"`
	Token         string     `json:"token"`
}

func createFeed(t *testing.T, r http.Handler) feedJSON {
	t.Helper()
	w := do(r, "POST", "/ics-feed", "")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var f feedJSON
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &f))
	return f
}

func feedStatus(t *testing.T, r http.Handler) feedJSON {
	t.Helper()
	w := do(r, "GET", "/ics-feed", "")
	require.Equal(t, http.StatusOK, w.Code)
	var f feedJSON
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &f))
	return f
}

func storedFeed(t *testing.T, e *env, userID uint) models.IcsFeed {
	t.Helper()
	var f models.IcsFeed
	require.NoError(t, e.db.Where("user_id = ?", userID).First(&f).Error)
	return f
}

func TestFeedStatusIsDisabledUntilCreated(t *testing.T) {
	e := newEnv(t)
	st := feedStatus(t, e.asA)
	assert.False(t, st.Enabled)
	assert.Nil(t, st.CreatedAt)
	assert.Contains(t, do(e.asA, "GET", "/ics-feed", "").Body.String(), `"last_fetched_at":null`)
}

func TestCreateFeedReturnsTheTokenOnceAndStoresOnlyTheHash(t *testing.T) {
	e := newEnv(t)
	f := createFeed(t, e.asA)
	assert.True(t, f.Enabled)
	assert.NotNil(t, f.CreatedAt)
	assert.Len(t, f.Token, 43)
	assert.Nil(t, f.LastFetchedAt)

	assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(f.Token))), storedFeed(t, e, e.a.ID).TokenHash)
	w := do(e.asA, "GET", "/ics-feed", "")
	assert.NotContains(t, w.Body.String(), f.Token)
	assert.Contains(t, w.Body.String(), `"enabled":true`)
}

func TestValidTokenServesTheCalendarWithoutALogin(t *testing.T) {
	e := newEnv(t)
	require.NoError(t, e.db.Create(&models.Contact{UserID: e.a.ID, Firstname: "Ada", Birthday: "1815-12-10"}).Error)
	f := createFeed(t, e.asA)

	w := do(e.anon, "GET", "/ics/"+f.Token+".ics", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/calendar; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Cache-Control"), "private")
	assert.Equal(t, "noindex", w.Header().Get("X-Robots-Tag"))
	assert.Equal(t, "no-referrer", w.Header().Get("Referrer-Policy"))
	assert.Equal(t, `inline; filename="meerkat.ics"`, w.Header().Get("Content-Disposition"))
	assert.True(t, strings.HasPrefix(w.Body.String(), "BEGIN:VCALENDAR\r\n"))
	assert.Contains(t, w.Body.String(), "Ada")

	cached := do(e.anon, "GET", "/ics/"+f.Token+".ics", "", "If-None-Match", w.Header().Get("ETag"))
	assert.Equal(t, http.StatusNotModified, cached.Code)
	assert.Empty(t, cached.Body.String())
}

func TestBadFeedTokensAre404AndNeverCached(t *testing.T) {
	e := newEnv(t)
	f := createFeed(t, e.asA)
	for name, path := range map[string]string{
		"unknown":        "/ics/" + strings.Repeat("A", 43) + ".ics",
		"too short":      "/ics/abc.ics",
		"bad charset":    "/ics/" + strings.Repeat("!", 43) + ".ics",
		"missing suffix": "/ics/" + f.Token,
		"wrong suffix":   "/ics/" + f.Token + ".txt",
		"tampered":       "/ics/" + f.Token[:42] + map[bool]string{true: "B", false: "A"}[f.Token[42] == 'A'] + ".ics",
	} {
		w := do(e.anon, "GET", path, "")
		assert.Equal(t, http.StatusNotFound, w.Code, name)
		assert.NotContains(t, w.Body.String(), "BEGIN:VCALENDAR", name)
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"), name)
	}
}

func TestFeedAlwaysServesTheOwnersDataNotTheSessionUsers(t *testing.T) {
	e := newEnv(t)
	require.NoError(t, e.db.Create(&models.Contact{UserID: e.b.ID, Firstname: "Bertrand", Birthday: "1991-03-04"}).Error)
	require.NoError(t, e.db.Create(&models.Contact{UserID: e.a.ID, Firstname: "Alice", Birthday: "1990-01-02"}).Error)
	f := createFeed(t, e.asA)

	w := do(e.asB, "GET", "/ics/"+f.Token+".ics", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Alice")
	assert.NotContains(t, w.Body.String(), "Bertrand")
}

func TestFeedsAreIsolatedBetweenUsers(t *testing.T) {
	e := newEnv(t)
	ca := models.Contact{UserID: e.a.ID, Firstname: "Alice", Birthday: "1990-01-02"}
	cb := models.Contact{UserID: e.b.ID, Firstname: "Bertrand", Birthday: "1991-03-04"}
	require.NoError(t, e.db.Create(&ca).Error)
	require.NoError(t, e.db.Create(&cb).Error)
	require.NoError(t, e.db.Create(&models.Reminder{UserID: e.a.ID, ContactID: &ca.ID, Message: "reminder-of-a", RemindAt: now, Recurrence: "once"}).Error)
	require.NoError(t, e.db.Create(&models.Reminder{UserID: e.b.ID, ContactID: &cb.ID, Message: "reminder-of-b", RemindAt: now, Recurrence: "once"}).Error)
	tokA, tokB := createFeed(t, e.asA).Token, createFeed(t, e.asB).Token
	assert.NotEqual(t, tokA, tokB)

	wa, wb := do(e.anon, "GET", "/ics/"+tokA+".ics", ""), do(e.anon, "GET", "/ics/"+tokB+".ics", "")
	require.Equal(t, http.StatusOK, wa.Code)
	require.Equal(t, http.StatusOK, wb.Code)
	assert.Contains(t, wa.Body.String(), "reminder-of-a")
	assert.NotContains(t, wa.Body.String(), "Bertrand")
	assert.NotContains(t, wa.Body.String(), "reminder-of-b")
	assert.Contains(t, wb.Body.String(), "reminder-of-b")
	assert.NotContains(t, wb.Body.String(), "Alice")

	assert.Equal(t, http.StatusOK, do(e.asB, "DELETE", "/ics-feed", "").Code)
	assert.Equal(t, http.StatusNotFound, do(e.anon, "GET", "/ics/"+tokB+".ics", "").Code)
	assert.Equal(t, http.StatusOK, do(e.anon, "GET", "/ics/"+tokA+".ics", "").Code)
}

func TestRegeneratingTheFeedInvalidatesTheOldURLAndKeepsOneRow(t *testing.T) {
	e := newEnv(t)
	first, second := createFeed(t, e.asA), createFeed(t, e.asA)
	assert.NotEqual(t, first.Token, second.Token)
	assert.Equal(t, http.StatusNotFound, do(e.anon, "GET", "/ics/"+first.Token+".ics", "").Code)
	assert.Equal(t, http.StatusOK, do(e.anon, "GET", "/ics/"+second.Token+".ics", "").Code)
	var n int64
	e.db.Model(&models.IcsFeed{}).Where("user_id = ?", e.a.ID).Count(&n)
	assert.EqualValues(t, 1, n)
}

func TestRevokingTheFeedIsIdempotent(t *testing.T) {
	e := newEnv(t)
	f := createFeed(t, e.asA)
	w := do(e.asA, "DELETE", "/ics-feed", "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"message":"Calendar feed revoked successfully"}`, w.Body.String())
	assert.Equal(t, http.StatusNotFound, do(e.anon, "GET", "/ics/"+f.Token+".ics", "").Code)
	assert.Equal(t, http.StatusOK, do(e.asA, "DELETE", "/ics-feed", "").Code)
	assert.False(t, feedStatus(t, e.asA).Enabled)
}

func TestFeedContentCannotInjectCalendarLines(t *testing.T) {
	e := newEnv(t)
	c := models.Contact{UserID: e.a.ID, Firstname: "Eve", Birthday: "1990-01-02"}
	require.NoError(t, e.db.Create(&c).Error)
	require.NoError(t, e.db.Create(&models.Reminder{UserID: e.a.ID, ContactID: &c.ID, Message: "x\r\nBEGIN:VEVENT\r\nSUMMARY:injected", RemindAt: now, Recurrence: "once"}).Error)
	f := createFeed(t, e.asA)
	events := 0
	for _, l := range strings.Split(do(e.anon, "GET", "/ics/"+f.Token+".ics", "").Body.String(), "\r\n") {
		if l == "BEGIN:VEVENT" {
			events++
		}
		assert.NotEqual(t, "SUMMARY:injected", l)
	}
	assert.Equal(t, 2, events)
}

func TestFeedOfADeletedOwnerIs404(t *testing.T) {
	e := newEnv(t)
	f := createFeed(t, e.asB)
	require.Equal(t, http.StatusOK, do(e.anon, "GET", "/ics/"+f.Token+".ics", "").Code)
	require.NoError(t, e.db.Delete(&e.b).Error)
	assert.Equal(t, http.StatusNotFound, do(e.anon, "GET", "/ics/"+f.Token+".ics", "").Code)
}

func TestCreatingTheFeedRetriesOnAUniqueConflict(t *testing.T) {
	e := newEnv(t)
	first := true
	require.NoError(t, e.db.Callback().Create().Before("gorm:create").Register("test:ics-conflict", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*models.IcsFeed); ok && first {
			first = false
			_ = tx.AddError(fmt.Errorf("UNIQUE constraint failed: ics_feeds.user_id"))
		}
	}))
	f := createFeed(t, e.asA)
	assert.False(t, first, "conflict path was exercised")
	assert.Equal(t, http.StatusOK, do(e.anon, "GET", "/ics/"+f.Token+".ics", "").Code)
}

func TestFetchesAreRecordedAtMostOncePerMinute(t *testing.T) {
	e := newEnv(t)
	f := createFeed(t, e.asA)
	url := "/ics/" + f.Token + ".ics"
	assert.Nil(t, f.LastFetchedAt)

	require.Equal(t, http.StatusOK, do(e.anon, "GET", url, "").Code)
	first := storedFeed(t, e, e.a.ID).LastFetchedAt
	require.NotNil(t, first)
	assert.True(t, now.Equal(*first))
	require.NotNil(t, feedStatus(t, e.asA).LastFetchedAt)

	require.Equal(t, http.StatusOK, do(e.anon, "GET", url, "").Code)
	assert.True(t, first.Equal(*storedFeed(t, e, e.a.ID).LastFetchedAt), "no write inside the window")

	old := now.Add(-2 * time.Minute)
	require.NoError(t, e.db.Model(&models.IcsFeed{}).Where("user_id = ?", e.a.ID).Update("last_fetched_at", old).Error)
	etag := do(e.anon, "GET", url, "").Header().Get("ETag")
	assert.True(t, now.Equal(*storedFeed(t, e, e.a.ID).LastFetchedAt))

	require.NoError(t, e.db.Model(&models.IcsFeed{}).Where("user_id = ?", e.a.ID).Update("last_fetched_at", old).Error)
	require.Equal(t, http.StatusNotModified, do(e.anon, "GET", url, "", "If-None-Match", etag).Code)
	assert.True(t, now.Equal(*storedFeed(t, e, e.a.ID).LastFetchedAt), "a 304 counts as a fetch")
}

func TestOtherUsersFetchesDoNotShowAndRegeneratingResetsThem(t *testing.T) {
	e := newEnv(t)
	a := createFeed(t, e.asA)
	createFeed(t, e.asB)
	require.Equal(t, http.StatusOK, do(e.anon, "GET", "/ics/"+a.Token+".ics", "").Code)
	assert.NotNil(t, feedStatus(t, e.asA).LastFetchedAt)
	assert.Nil(t, feedStatus(t, e.asB).LastFetchedAt)
	createFeed(t, e.asA)
	assert.Nil(t, feedStatus(t, e.asA).LastFetchedAt)
}

func TestAFailedBuildIsNotRecordedAsAFetch(t *testing.T) {
	e := newEnv(t)
	f := createFeed(t, e.asA)
	require.NoError(t, e.db.Callback().Query().Before("gorm:query").Register("test:ics-build-fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "contacts" {
			_ = tx.AddError(fmt.Errorf("boom"))
		}
	}))
	assert.Equal(t, http.StatusInternalServerError, do(e.anon, "GET", "/ics/"+f.Token+".ics", "").Code)
	assert.Nil(t, storedFeed(t, e, e.a.ID).LastFetchedAt)
}

func TestSecurityRelevantActionsAreAudited(t *testing.T) {
	e := newEnv(t)
	tok := createToken(t, e.asA, "ci;prod")
	require.Equal(t, http.StatusOK, do(e.asA, "DELETE", fmt.Sprintf("/api-tokens/%d", tok.ID), "").Code)
	createFeed(t, e.asA)
	require.Equal(t, http.StatusOK, do(e.asA, "DELETE", "/ics-feed", "").Code)

	var got []models.AuditLog
	require.NoError(t, e.db.Order("id").Find(&got).Error)
	var events []string
	for _, g := range got {
		events = append(events, g.Event)
		require.NotNil(t, g.UserID)
		assert.Equal(t, e.a.ID, *g.UserID)
		assert.Equal(t, "plain", g.Username)
		assert.NotContains(t, g.Detail, "meerkat_")
	}
	assert.Equal(t, []string{models.AuditAPITokenCreated, models.AuditAPITokenRevoked, models.AuditICSFeedCreated, models.AuditICSFeedRevoked}, events)
	assert.Equal(t, "token:ci,prod", got[0].Detail)
	assert.Equal(t, "token:ci,prod", got[1].Detail)
}
