package monicaimport_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/i18n"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/monica"
	"meerkat/monicaimport"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var now = time.Date(2026, 3, 10, 15, 30, 0, 0, time.UTC)

type env struct {
	db   *gorm.DB
	clk  *clock.Fake
	a, b models.User
	asA  *gin.Engine
	asB  *gin.Engine
	anon *gin.Engine
}

// fakeMonicaServer serves a small canned Monica v4 account.
func fakeMonicaServer(t *testing.T) *httptest.Server {
	t.Helper()

	paged := func(w http.ResponseWriter, items []map[string]any) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": items,
			"meta": map[string]int{"current_page": 1, "last_page": 1, "total": len(items)},
		})
	}

	contacts := []map[string]any{
		{
			"id": 101, "first_name": "Ada", "last_name": "Lovelace", "nickname": "",
			"gender": "Woman", "gender_type": "F", "is_partial": false, "is_starred": true,
			"description": "Mathematician", "food_preferences": "Vegetarian",
			"information": map[string]any{
				"dates": map[string]any{
					"birthdate": map[string]any{"date": "1990-12-10T00:00:00Z", "is_age_based": false, "is_year_unknown": false},
				},
				"career": map[string]any{"job": "Analyst", "company": "Engines Ltd"},
				"avatar": map[string]any{"url": nil, "source": "default"},
			},
			"tags":      []map[string]any{{"name": "friends"}},
			"addresses": []map[string]any{{"name": "Home", "street": "1 Main St", "city": "London", "province": "", "postal_code": "SW1", "country": map[string]any{"name": "UK"}}},
			"contactFields": []map[string]any{
				{"content": "ada@example.com", "contact_field_type": map[string]any{"name": "Email", "protocol": "mailto:", "type": "email"}},
			},
		},
		{
			"id": 102, "first_name": "Bob", "last_name": "Existing",
			"gender_type": "M", "is_partial": false,
			"information": map[string]any{},
		},
		{
			"id": 103, "first_name": "Stub", "last_name": "Person", "is_partial": true,
			"information": map[string]any{},
		},
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/contacts":
			paged(w, contacts)
		case r.URL.Path == "/api/contacts/101/relationships":
			paged(w, []map[string]any{{
				"relationship_type": map[string]any{"name": "daughter"},
				"contact_is":        map[string]any{"id": 101},
				"of_contact":        map[string]any{"id": 102, "first_name": "Bob", "last_name": "Existing", "complete_name": "Bob Existing"},
			}})
		case strings.HasPrefix(r.URL.Path, "/api/contacts/") && strings.HasSuffix(r.URL.Path, "/relationships"):
			paged(w, nil)
		case r.URL.Path == "/api/activities":
			paged(w, []map[string]any{
				{
					"id": 1, "summary": "Lunch", "description": "At the pub", "happened_at": "2023-05-01",
					"attendees": map[string]any{"contacts": []map[string]any{{"id": 101}, {"id": 102}}},
				},
				{
					"id": 2, "summary": "Stub only", "happened_at": "2023-05-02",
					"attendees": map[string]any{"contacts": []map[string]any{{"id": 103}}},
				},
			})
		case r.URL.Path == "/api/notes":
			paged(w, []map[string]any{
				{"body": "Loves poetry", "created_at": "2023-01-02T10:00:00Z", "contact": map[string]any{"id": 101}},
			})
		case r.URL.Path == "/api/reminders":
			future := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
			paged(w, []map[string]any{
				{"title": "Wish happy birthday", "frequency_type": "year", "frequency_number": 1, "initial_date": future, "contact": map[string]any{"id": 101}},
			})
		case r.URL.Path == "/api/calls":
			paged(w, []map[string]any{
				{"content": "Talked about work", "called_at": "2023-04-01 12:00:00", "contact": map[string]any{"id": 101}},
			})
		case r.URL.Path == "/api/tasks", r.URL.Path == "/api/gifts", r.URL.Path == "/api/debts":
			paged(w, nil)
		default:
			t.Errorf("unexpected Monica API path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	monica.DisableRateLimitForTesting()
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Activity{}, &models.Note{}, &models.Relationship{},
		&models.Reminder{}, &models.Webhook{}, &models.WebhookDelivery{}, &models.Image{}))
	e := &env{db: db, clk: clock.NewFake(now)}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)

	module := monicaimport.New(monicaimport.Deps{DB: db, Config: &config.Config{ProfilePhotoDir: t.TempDir()}})
	module.SetClock(e.clk)
	mount := func(userID uint) *gin.Engine {
		r := gin.New()
		r.Use(middleware.ClockMiddleware(e.clk))
		authenticated := r.Group("")
		if userID != 0 {
			authenticated.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
		}
		module.Register(api.New(r.Group(""), r), authenticated)
		return r
	}
	e.asA, e.asB, e.anon = mount(e.a.ID), mount(e.b.ID), mount(0)
	return e
}

const prefix = "/contacts/import/monica"

func send(r http.Handler, method, path string, payload any) *httptest.ResponseRecorder {
	var body *bytes.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	} else {
		body = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type row struct {
	RowIndex        int            `json:"row_index"`
	ParsedContact   map[string]any `json:"parsed_contact"`
	SuggestedAction string         `json:"suggested_action"`
	DuplicateMatch  map[string]any `json:"duplicate_match"`
	Related         map[string]int `json:"related"`
}

type preview struct {
	TotalRows      int   `json:"total_rows"`
	DuplicateCount int   `json:"duplicate_count"`
	Rows           []row `json:"rows"`
}

func connect(t *testing.T, r http.Handler, url string) string {
	t.Helper()
	w := send(r, "POST", prefix+"/connect", map[string]any{"base_url": url, "api_token": "good-token"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		SessionID string `json:"session_id"`
		Totals    struct{ Contacts int }
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 3, resp.Totals.Contacts)
	assert.NotEmpty(t, resp.SessionID)
	return resp.SessionID
}

func flow(t *testing.T, r http.Handler, url string) (string, preview) {
	t.Helper()
	session := connect(t, r, url)
	w := send(r, "POST", prefix+"/fetch", map[string]any{"session_id": session, "include_relationships": true, "include_extras": true})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	assert.JSONEq(t, fmt.Sprintf(`{"session_id":%q}`, session), w.Body.String())

	deadline := time.Now().Add(10 * time.Second)
	var status struct{ Phase, Error string }
	for {
		w = send(r, "GET", prefix+"/status?session_id="+session, nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
		if status.Phase == "ready" || status.Phase == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("fetch did not finish, stuck in phase %s", status.Phase)
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, "ready", status.Phase, status.Error)

	w = send(r, "GET", prefix+"/preview?session_id="+session, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var p preview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	return session, p
}

// @015-US2
func TestMonicaImportEndToEnd(t *testing.T) {
	server := fakeMonicaServer(t)
	defer server.Close()
	e := newEnv(t)
	existing := models.Contact{UserID: e.a.ID, Firstname: "Bob", Lastname: "Existing"}
	require.NoError(t, e.db.Create(&existing).Error)

	session, p := flow(t, e.asA, server.URL)

	assert.Equal(t, 2, p.TotalRows, "the partial contact is filtered out")
	assert.Equal(t, 1, p.DuplicateCount)
	rows := map[string]row{}
	for _, r := range p.Rows {
		rows[fmt.Sprint(r.ParsedContact["firstname"])] = r
	}
	adaRow, bobRow := rows["Ada"], rows["Bob"]
	assert.Equal(t, "add", adaRow.SuggestedAction)
	assert.Nil(t, adaRow.DuplicateMatch)
	assert.Equal(t, "update", bobRow.SuggestedAction)
	assert.NotNil(t, bobRow.DuplicateMatch)
	assert.Equal(t, map[string]int{"activities": 1, "notes": 1, "reminders": 1, "relationships": 1, "extra_notes": 1}, adaRow.Related)

	w := send(e.asA, "POST", prefix+"/confirm", map[string]any{"session_id": session, "actions": []map[string]any{
		{"row_index": adaRow.RowIndex, "action": "add"}, {"row_index": bobRow.RowIndex, "action": "update"},
	}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var result struct {
		Created              int `json:"created"`
		Updated              int `json:"updated"`
		ActivitiesCreated    int `json:"activities_created"`
		NotesCreated         int `json:"notes_created"`
		RemindersCreated     int `json:"reminders_created"`
		RelationshipsCreated int `json:"relationships_created"`
		PhotosQueued         int `json:"photos_queued"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 1, result.Updated)
	assert.Equal(t, 1, result.ActivitiesCreated)
	assert.Equal(t, 2, result.NotesCreated)
	assert.Equal(t, 1, result.RemindersCreated)
	assert.Equal(t, 1, result.RelationshipsCreated)
	assert.Equal(t, 0, result.PhotosQueued)

	var ada models.Contact
	require.NoError(t, e.db.Where("firstname = ?", "Ada").First(&ada).Error)
	assert.Equal(t, e.a.ID, ada.UserID)
	assert.Equal(t, "female", ada.Gender)
	assert.Equal(t, "1990-12-10", ada.Birthday)
	assert.Equal(t, "ada@example.com", ada.Email)
	assert.Equal(t, []string{"friends"}, ada.Circles)
	assert.Equal(t, "Analyst", ada.JobTitle)
	assert.Equal(t, "Engines Ltd", ada.Organization)
	assert.Equal(t, "yes", ada.CustomFields["Starred"])

	var activity models.Activity
	require.NoError(t, e.db.Preload("Contacts").Where("title = ?", "Lunch").First(&activity).Error)
	assert.Len(t, activity.Contacts, 2)

	var relationship models.Relationship
	require.NoError(t, e.db.Where("contact_id = ?", ada.ID).First(&relationship).Error)
	assert.Equal(t, "daughter", relationship.Type)
	assert.Equal(t, "Bob Existing", relationship.Name)
	require.NotNil(t, relationship.RelatedContactID)
	assert.Equal(t, existing.ID, *relationship.RelatedContactID)

	var reminders, notes int64
	e.db.Model(&models.Reminder{}).Where("contact_id = ?", ada.ID).Count(&reminders)
	e.db.Model(&models.Note{}).Where("contact_id = ?", ada.ID).Count(&notes)
	assert.EqualValues(t, 1, reminders)
	assert.EqualValues(t, 2, notes)

	var user models.User
	e.db.First(&user, e.a.ID)
	assert.Contains(t, user.CustomFieldNames, "Starred")

	var stubs int64
	e.db.Model(&models.Contact{}).Where("firstname = ?", "Stub").Count(&stubs)
	assert.Zero(t, stubs)
}

func TestMonicaImportRerunIsIdempotent(t *testing.T) {
	server := fakeMonicaServer(t)
	defer server.Close()
	e := newEnv(t)

	run := func() {
		session, p := flow(t, e.asA, server.URL)
		actions := make([]map[string]any, 0, len(p.Rows))
		for _, r := range p.Rows {
			action := r.SuggestedAction
			if action == "skip" {
				action = "add"
			}
			actions = append(actions, map[string]any{"row_index": r.RowIndex, "action": action})
		}
		w := send(e.asA, "POST", prefix+"/confirm", map[string]any{"session_id": session, "actions": actions})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	run()
	run()

	var contacts, activities, notes, reminders, relationships int64
	e.db.Model(&models.Contact{}).Count(&contacts)
	e.db.Model(&models.Activity{}).Count(&activities)
	e.db.Model(&models.Note{}).Where("content IN ?", []string{"Loves poetry"}).Count(&notes)
	e.db.Model(&models.Reminder{}).Count(&reminders)
	e.db.Model(&models.Relationship{}).Count(&relationships)
	assert.EqualValues(t, 2, contacts)
	assert.EqualValues(t, 1, activities)
	assert.EqualValues(t, 1, notes)
	assert.EqualValues(t, 1, reminders)
	assert.EqualValues(t, 1, relationships)
}

func TestConnectRejectsABadTokenAndInvalidBodies(t *testing.T) {
	server := fakeMonicaServer(t)
	defer server.Close()
	e := newEnv(t)
	w := send(e.asA, "POST", prefix+"/connect", map[string]any{"base_url": server.URL, "api_token": "wrong"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "api_token")

	assert.Equal(t, http.StatusBadRequest, send(e.asA, "POST", prefix+"/connect", map[string]any{"api_token": "x"}).Code, "no URL")
	assert.Equal(t, http.StatusBadRequest, send(e.asA, "POST", prefix+"/fetch", map[string]any{}).Code, "no session")
	assert.Equal(t, http.StatusBadRequest, send(e.asA, "POST", prefix+"/confirm", map[string]any{"session_id": "x"}).Code, "no actions")
}

func TestSessionsBelongToTheirUser(t *testing.T) {
	server := fakeMonicaServer(t)
	defer server.Close()
	e := newEnv(t)
	session := connect(t, e.asA, server.URL)

	assert.Equal(t, http.StatusNotFound, send(e.asB, "GET", prefix+"/status?session_id="+session, nil).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asB, "GET", prefix+"/preview?session_id="+session, nil).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asB, "DELETE", prefix+"/session?session_id="+session, nil).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asB, "POST", prefix+"/fetch", map[string]any{"session_id": session}).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asB, "POST", prefix+"/confirm", map[string]any{"session_id": session, "actions": []map[string]any{{"row_index": 0, "action": "add"}}}).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asA, "GET", prefix+"/status?session_id=doesnotexist", nil).Code)
	assert.Equal(t, http.StatusOK, send(e.asA, "GET", prefix+"/status?session_id="+session, nil).Code)
}

func TestPrivateAddressesAreRefusedWhenBlockedInTheConfig(t *testing.T) {
	server := fakeMonicaServer(t)
	defer server.Close()
	e := newEnv(t)
	blocked := monicaimport.New(monicaimport.Deps{DB: e.db, Config: &config.Config{MonicaBlockPrivateURLs: true}})
	r := gin.New()
	r.Use(middleware.ClockMiddleware(e.clk), func(c *gin.Context) { c.Set("userID", e.a.ID); c.Next() })
	blocked.Register(api.New(r.Group(""), r), r.Group(""))

	w := send(r, "POST", prefix+"/connect", map[string]any{"base_url": server.URL, "api_token": "good-token"})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestSessionIDIsRequiredOnTheQueryEndpoints(t *testing.T) {
	e := newEnv(t)
	for _, c := range [][2]string{{"GET", "/status"}, {"GET", "/preview"}, {"DELETE", "/session"}} {
		w := send(e.asA, c[0], prefix+c[1], nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, c[1])
		assert.Contains(t, w.Body.String(), "session_id", c[1])
	}
}

func TestCancelDropsTheSession(t *testing.T) {
	server := fakeMonicaServer(t)
	defer server.Close()
	e := newEnv(t)
	session := connect(t, e.asA, server.URL)

	w := send(e.asA, "DELETE", prefix+"/session?session_id="+session, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"cancelled"}`, w.Body.String())
	assert.Equal(t, http.StatusNotFound, send(e.asA, "GET", prefix+"/status?session_id="+session, nil).Code)
}

func TestSessionsExpireWithTheApplicationClock(t *testing.T) {
	server := fakeMonicaServer(t)
	defer server.Close()
	e := newEnv(t)
	session := connect(t, e.asA, server.URL)

	e.clk.Advance(24 * time.Hour)
	w := send(e.asA, "GET", prefix+"/status?session_id="+session, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "expired")
}

func TestMonicaImportNeedsAUser(t *testing.T) {
	e := newEnv(t)
	for _, c := range [][2]string{
		{"POST", "/connect"}, {"POST", "/fetch"}, {"GET", "/status?session_id=x"}, {"GET", "/preview?session_id=x"},
		{"POST", "/confirm"}, {"DELETE", "/session?session_id=x"},
	} {
		w := send(e.anon, c[0], prefix+c[1], map[string]any{"base_url": "http://x", "api_token": "t", "session_id": "x", "actions": []any{}})
		assert.Equal(t, http.StatusUnauthorized, w.Code, c[0]+" "+c[1])
	}
}

// avatarMonica serves two contacts whose avatar is a Monica-hosted photo: the
// first has a photo record, the second has none. While broken is set, the
// notes endpoint fails.
func avatarMonica(t *testing.T, broken *atomic.Bool) *httptest.Server {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 60, 60))))
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	contact := func(id int, first string) map[string]any {
		return map[string]any{
			"id": id, "first_name": first, "last_name": "Photo", "is_partial": false,
			"information": map[string]any{"avatar": map[string]any{"url": "https://monica.example/store/avatars/p.png", "source": "photo"}},
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		paged := func(items []map[string]any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": items, "meta": map[string]int{"current_page": 1, "last_page": 1, "total": len(items)}})
		}
		switch {
		case r.URL.Path == "/api/notes" && broken.Load():
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		case r.URL.Path == "/api/contacts":
			paged([]map[string]any{contact(201, "Ana"), contact(202, "Ben"), {"id": 203, "first_name": "", "last_name": "", "is_partial": false, "information": map[string]any{}}})
		case r.URL.Path == "/api/contacts/201/photos":
			paged([]map[string]any{{"id": 1, "new_filename": "avatars/p.png", "mime_type": "image/png", "dataUrl": dataURL}})
		default:
			paged(nil)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type monicaStatus struct {
	Phase, Error string
	Result       struct {
		Created     int `json:"created"`
		PhotosSaved int `json:"photos_saved"`
		PhotosFail  int `json:"photos_failed"`
	}
}

func waitForPhase(t *testing.T, r http.Handler, session string, phases ...string) monicaStatus {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		w := send(r, "GET", prefix+"/status?session_id="+session, nil)
		require.Equal(t, http.StatusOK, w.Code)
		var st monicaStatus
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &st))
		for _, p := range phases {
			if st.Phase == p {
				return st
			}
		}
		require.True(t, time.Now().Before(deadline), "stuck in phase %s", st.Phase)
		time.Sleep(10 * time.Millisecond)
	}
}

func startMonicaFetch(t *testing.T, r http.Handler, url string, beforeFetch func()) string {
	t.Helper()
	w := send(r, "POST", prefix+"/connect", map[string]any{"base_url": url, "api_token": "good-token"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		SessionID string `json:"session_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	beforeFetch()
	w = send(r, "POST", prefix+"/fetch", map[string]any{"session_id": resp.SessionID})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	return resp.SessionID
}

// @015-US2
func TestMonicaImportSavesAvatarsAndCountsTheOnesItCouldNotGet(t *testing.T) {
	var broken atomic.Bool
	server := avatarMonica(t, &broken)
	e := newEnv(t)
	session := startMonicaFetch(t, e.asA, server.URL, func() {})
	waitForPhase(t, e.asA, session, "ready")

	w := send(e.asA, "GET", prefix+"/preview?session_id="+session, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var p preview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	var actions []map[string]any
	for _, r := range p.Rows {
		actions = append(actions, map[string]any{"row_index": r.RowIndex, "action": "add"})
	}
	w = send(e.asA, "POST", prefix+"/confirm", map[string]any{"session_id": session, "actions": actions})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	st := waitForPhase(t, e.asA, session, "done")
	assert.Equal(t, 2, st.Result.Created)
	assert.Equal(t, 1, st.Result.PhotosSaved, "the contact with a photo record got its picture")
	assert.Equal(t, 1, st.Result.PhotosFail, "the other is counted, not hidden")
	var withPhoto, without models.Contact
	require.NoError(t, e.db.Where("firstname = ?", "Ana").First(&withPhoto).Error)
	require.NoError(t, e.db.Where("firstname = ?", "Ben").First(&without).Error)
	assert.NotEmpty(t, withPhoto.Photo)
	assert.Empty(t, without.Photo, "the contact is imported all the same")
	var total int64
	require.NoError(t, e.db.Model(&models.Contact{}).Count(&total).Error)
	assert.EqualValues(t, 2, total, "the contact without a first name was not imported")
}

// @015-US2
func TestMonicaImportShowsASafeErrorWhenTheRemoteFailsDuringTheFetch(t *testing.T) {
	var broken atomic.Bool
	server := avatarMonica(t, &broken)
	e := newEnv(t)

	session := startMonicaFetch(t, e.asA, server.URL, func() { broken.Store(true) })
	st := waitForPhase(t, e.asA, session, "failed", "ready")

	assert.Equal(t, "failed", st.Phase)
	assert.Equal(t, "The Monica instance could not be reached", st.Error)
	assert.NotContains(t, st.Error, "good-token")
	var n int64
	require.NoError(t, e.db.Model(&models.Contact{}).Count(&n).Error)
	assert.Zero(t, n, "nothing was imported")
}

// @015-US2
func TestMonicaImportNeverFetchesAnAvatarFromAnInternalAddress(t *testing.T) {
	var avatarHits atomic.Int32
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 60, 60))))
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paged := func(items []map[string]any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": items, "meta": map[string]int{"current_page": 1, "last_page": 1, "total": len(items)}})
		}
		switch r.URL.Path {
		case "/internal/avatar.png":
			avatarHits.Add(1)
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(buf.Bytes())
		case "/api/contacts":
			paged([]map[string]any{{
				"id": 301, "first_name": "Eve", "last_name": "Gravatar", "is_partial": false,
				"information": map[string]any{"avatar": map[string]any{"url": srv.URL + "/internal/avatar.png", "source": "gravatar"}},
			}})
		default:
			paged(nil)
		}
	}))
	t.Cleanup(srv.Close)
	e := newEnv(t)
	session := startMonicaFetch(t, e.asA, srv.URL, func() {})
	waitForPhase(t, e.asA, session, "ready")

	w := send(e.asA, "POST", prefix+"/confirm", map[string]any{"session_id": session, "actions": []map[string]any{{"row_index": 0, "action": "add"}}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	st := waitForPhase(t, e.asA, session, "done")
	assert.Equal(t, 1, st.Result.Created)
	assert.Equal(t, 1, st.Result.PhotosFail)
	assert.Zero(t, avatarHits.Load(), "a private Monica instance is allowed, an internal avatar URL is not")
}
