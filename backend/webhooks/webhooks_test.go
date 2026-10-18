package webhooks_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/webhooks"

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
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Webhook{}, &models.WebhookDelivery{}))
	e := &env{db: db}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)
	e.asA, e.asB, e.anon = e.engine(e.a.ID), e.engine(e.b.ID), e.engine(0)
	return e
}

func (e *env) engine(userID uint) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ClockMiddleware(clock.NewFake(now)))
	authenticated := r.Group("")
	if userID != 0 {
		authenticated.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
	}
	webhooks.New(webhooks.Deps{DB: e.db, Config: &config.Config{}}).Register(api.New(r.Group(""), r), authenticated)
	return r
}

func do(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const validBody = `{"name":"My Hook","url":"https://example.com/hook","events":["contact.created","note.created"],"is_active":true}`

func (e *env) seed(t *testing.T, owner uint, url string) models.Webhook {
	t.Helper()
	wh := models.Webhook{UserID: owner, Name: "Test Hook", URL: url, Events: []string{"contact.created"}, Secret: "testsecret", IsActive: true}
	require.NoError(t, e.db.Create(&wh).Error)
	return wh
}

func path(id uint, suffix string) string { return fmt.Sprintf("/webhooks/%d%s", id, suffix) }

func TestCreateWebhookShowsTheSecretOnceAndKeepsItOutOfEveryOtherResponse(t *testing.T) {
	e := newEnv(t)
	w := do(e.asA, "POST", "/webhooks", validBody)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		ID     uint     `json:"id"`
		Name   string   `json:"name"`
		URL    string   `json:"url"`
		Events []string `json:"events"`
		Secret string   `json:"secret"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, "My Hook", created.Name)
	assert.Len(t, created.Events, 2)
	assert.Len(t, created.Secret, 43)

	var stored models.Webhook
	require.NoError(t, e.db.First(&stored, created.ID).Error)
	assert.Equal(t, created.Secret, stored.Secret)
	assert.Equal(t, e.a.ID, stored.UserID)
	for _, r := range []*httptest.ResponseRecorder{do(e.asA, "GET", "/webhooks", ""), do(e.asA, "GET", path(created.ID, ""), "")} {
		assert.NotContains(t, r.Body.String(), created.Secret)
	}
}

func TestCreateWebhookValidatesTheBody(t *testing.T) {
	e := newEnv(t)
	for name, body := range map[string]string{
		"no name":       `{"url":"https://example.com","events":["contact.created"]}`,
		"bad url":       `{"name":"x","url":"ftp://example.com","events":["contact.created"]}`,
		"no events":     `{"name":"x","url":"https://example.com","events":[]}`,
		"unknown event": `{"name":"x","url":"https://example.com","events":["contact.exploded"]}`,
	} {
		assert.Equal(t, http.StatusBadRequest, do(e.asA, "POST", "/webhooks", body).Code, name)
	}
}

func TestEveryEventTheCRMPublishesCanBeSubscribed(t *testing.T) {
	e := newEnv(t)
	body := `{"name":"all","url":"https://example.com","events":["contact.created","contact.updated","contact.deleted","contact.restored",` +
		`"note.created","note.updated","note.deleted","activity.created","activity.updated","activity.deleted","reminder.triggered","birthday.occurred"]}`
	assert.Equal(t, http.StatusCreated, do(e.asA, "POST", "/webhooks", body).Code)
}

func TestAUserCanHaveTwentyWebhooks(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 20; i++ {
		e.seed(t, e.a.ID, fmt.Sprintf("https://example.com/%d", i))
	}
	assert.Equal(t, http.StatusConflict, do(e.asA, "POST", "/webhooks", validBody).Code)
	assert.Equal(t, http.StatusCreated, do(e.asB, "POST", "/webhooks", validBody).Code)
}

func TestListWebhooksIsScopedNewestFirstAndAnArray(t *testing.T) {
	e := newEnv(t)
	assert.JSONEq(t, `{"webhooks":[]}`, do(e.asA, "GET", "/webhooks", "").Body.String())
	first := e.seed(t, e.a.ID, "https://example.com/a")
	second := e.seed(t, e.a.ID, "https://example.com/b")
	require.NoError(t, e.db.Model(&second).UpdateColumn("created_at", first.CreatedAt.Add(time.Hour)).Error)
	e.seed(t, e.b.ID, "https://example.com/other")

	var body struct{ Webhooks []struct{ ID uint } }
	require.NoError(t, json.Unmarshal(do(e.asA, "GET", "/webhooks", "").Body.Bytes(), &body))
	require.Len(t, body.Webhooks, 2)
	assert.Equal(t, second.ID, body.Webhooks[0].ID)
}

func TestGetWebhook(t *testing.T) {
	e := newEnv(t)
	wh := e.seed(t, e.a.ID, "https://example.com/hook")
	w := do(e.asA, "GET", path(wh.ID, ""), "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"name":"Test Hook"`)
	assert.Equal(t, http.StatusNotFound, do(e.asA, "GET", "/webhooks/99999", "").Code)
	assert.Equal(t, http.StatusBadRequest, do(e.asA, "GET", "/webhooks/abc", "").Code)
}

func TestUpdateWebhookKeepsTheSecret(t *testing.T) {
	e := newEnv(t)
	wh := e.seed(t, e.a.ID, "https://example.com/hook")
	w := do(e.asA, "PUT", path(wh.ID, ""), `{"name":"Renamed","url":"https://example.com/new","events":["activity.created"],"is_active":false}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"is_active":false`)

	var stored models.Webhook
	require.NoError(t, e.db.First(&stored, wh.ID).Error)
	assert.Equal(t, "Renamed", stored.Name)
	assert.Equal(t, "https://example.com/new", stored.URL)
	assert.Equal(t, []string{"activity.created"}, stored.Events)
	assert.False(t, stored.IsActive)
	assert.Equal(t, "testsecret", stored.Secret)
}

func TestDeleteWebhookSoftDeletes(t *testing.T) {
	e := newEnv(t)
	wh := e.seed(t, e.a.ID, "https://example.com/hook")
	w := do(e.asA, "DELETE", path(wh.ID, ""), "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"message":"Webhook deleted"}`, w.Body.String())
	var deleted models.Webhook
	require.NoError(t, e.db.Unscoped().First(&deleted, wh.ID).Error)
	assert.True(t, deleted.DeletedAt.Valid)
	assert.Equal(t, http.StatusNotFound, do(e.asA, "GET", path(wh.ID, ""), "").Code)
}

// @017-US2
func TestTestWebhookSendsASignedTestPayloadNow(t *testing.T) {
	received := make(chan struct{}, 2)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.NotEmpty(t, r.Header.Get("X-Webhook-Signature"))
		assert.Equal(t, "test", r.Header.Get("X-Meerkat-Event"))
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	e := newEnv(t)
	wh := e.seed(t, e.a.ID, target.URL)

	w := do(e.asA, "POST", path(wh.ID, "/test"), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Len(t, received, 1)
	var body struct{ Delivery struct{ Status string } }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "delivered", body.Delivery.Status)
}

func TestWebhookDeliveriesAreListedNewestFirstForTheOwnerOnly(t *testing.T) {
	e := newEnv(t)
	wh := e.seed(t, e.a.ID, "https://example.com/hook")
	code := 200
	require.NoError(t, e.db.Create(&models.WebhookDelivery{WebhookID: wh.ID, EventType: "contact.created", Payload: "{}", StatusCode: &code, Attempts: 1}).Error)
	require.NoError(t, e.db.Create(&models.WebhookDelivery{WebhookID: wh.ID, EventType: "note.created", Payload: "{}", StatusCode: &code, Attempts: 1}).Error)

	var body struct {
		Deliveries []struct {
			EventType string `json:"event_type"`
		}
	}
	w := do(e.asA, "GET", path(wh.ID, "/deliveries"), "")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Deliveries, 2)
	assert.Equal(t, "note.created", body.Deliveries[0].EventType)
	assert.Equal(t, http.StatusNotFound, do(e.asB, "GET", path(wh.ID, "/deliveries"), "").Code)
}

func TestAnotherUsersWebhookIs404ForEveryOperationAndStaysUnchanged(t *testing.T) {
	e := newEnv(t)
	wh := e.seed(t, e.a.ID, "https://example.com/hook")
	for _, c := range [][3]string{
		{"GET", path(wh.ID, ""), ""}, {"PUT", path(wh.ID, ""), validBody}, {"DELETE", path(wh.ID, ""), ""},
		{"POST", path(wh.ID, "/test"), ""}, {"GET", path(wh.ID, "/deliveries"), ""},
	} {
		assert.Equal(t, http.StatusNotFound, do(e.asB, c[0], c[1], c[2]).Code, c[0]+" "+c[1])
	}
	var stored models.Webhook
	require.NoError(t, e.db.First(&stored, wh.ID).Error)
	assert.Equal(t, "Test Hook", stored.Name)
	var n int64
	e.db.Model(&models.WebhookDelivery{}).Count(&n)
	assert.Zero(t, n, "no test delivery for a foreign webhook")
}

func TestWebhookEndpointsNeedAUser(t *testing.T) {
	e := newEnv(t)
	for _, c := range [][2]string{
		{"GET", "/webhooks"}, {"POST", "/webhooks"}, {"GET", "/webhooks/1"}, {"PUT", "/webhooks/1"},
		{"DELETE", "/webhooks/1"}, {"POST", "/webhooks/1/test"}, {"GET", "/webhooks/1/deliveries"},
	} {
		assert.Equal(t, http.StatusUnauthorized, do(e.anon, c[0], c[1], validBody).Code, c[0]+" "+c[1])
	}
}
