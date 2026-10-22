package activities_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"meerkat/activities"
	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type env struct {
	dir  string
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
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Activity{}, &models.Image{}, &models.Webhook{}, &models.WebhookDelivery{}))
	e := &env{db: db, dir: t.TempDir()}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)

	cfg := &config.Config{WebhookBlockPrivateURLs: true, ProfilePhotoDir: e.dir}
	mount := func(userID uint) *gin.Engine {
		r := gin.New()
		r.Use(middleware.ClockMiddleware(clock.Real()))
		authenticated := r.Group("")
		if userID != 0 {
			authenticated.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
		}
		activities.New(activities.Deps{DB: db, Config: cfg}).Register(api.New(r.Group(""), r), authenticated)
		return r
	}
	e.asA, e.asB, e.anon = mount(e.a.ID), mount(e.b.ID), mount(0)
	return e
}

func (e *env) contact(t *testing.T, owner uint, first, last string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: owner, Firstname: first, Lastname: last, Circles: []string{"Friends"}, Email: "private@example.com"}
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

func (e *env) activity(t *testing.T, owner uint, title string, with ...models.Contact) models.Activity {
	t.Helper()
	a := models.Activity{UserID: owner, Title: title, Date: time.Now(), Contacts: with}
	require.NoError(t, e.db.Create(&a).Error)
	return a
}

func (e *env) subscribe(t *testing.T, owner uint, events ...string) {
	t.Helper()
	require.NoError(t, e.db.Create(&models.Webhook{UserID: owner, Name: "w", URL: "http://127.0.0.1:1/", Events: events, Secret: "s", IsActive: true}).Error)
}

func (e *env) delivery(t *testing.T, event string) map[string]any {
	t.Helper()
	var d models.WebhookDelivery
	require.Eventually(t, func() bool {
		return e.db.Where("event_type = ?", event).First(&d).Error == nil
	}, 2*time.Second, 10*time.Millisecond)
	var payload struct{ Data map[string]any }
	require.NoError(t, json.Unmarshal([]byte(d.Payload), &payload))
	return payload.Data
}

func send(r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m), w.Body.String())
	return m
}

func sid(id uint) string { return strconv.Itoa(int(id)) }

func titles(body map[string]any) []string {
	var out []string
	for _, a := range body["activities"].([]any) {
		out = append(out, a.(map[string]any)["title"].(string))
	}
	return out
}

func links(t *testing.T, e *env, a models.Activity) []uint {
	t.Helper()
	var ids []uint
	require.NoError(t, e.db.Table("activity_contacts").Where("activity_id = ?", a.ID).Pluck("contact_id", &ids).Error)
	return ids
}

const day = "2026-03-01T12:00:00Z"

// @005-US2
func TestCreateLinksTheContactsAndAnnouncesTheActivity(t *testing.T) {
	e := newEnv(t)
	e.subscribe(t, e.a.ID, "activity.created")
	alice, bob := e.contact(t, e.a.ID, "Alice", "A"), e.contact(t, e.a.ID, "Bob", "B")

	w := send(e.asA, http.MethodPost, "/activities", map[string]any{
		"title": "Lunch", "description": "d", "location": "Berlin", "date": day, "contact_ids": []uint{alice.ID, bob.ID},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Activity created successfully", body["message"])
	act := body["activity"].(map[string]any)
	assert.Equal(t, "Lunch", act["title"])
	for _, key := range []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt", "date", "location", "description"} {
		assert.Contains(t, act, key)
	}
	assert.Len(t, act["contacts"], 2)

	var stored models.Activity
	require.NoError(t, e.db.First(&stored, uint(act["ID"].(float64))).Error)
	assert.Equal(t, e.a.ID, stored.UserID)
	assert.ElementsMatch(t, []uint{alice.ID, bob.ID}, links(t, e, stored))

	data := e.delivery(t, "activity.created")
	assert.Equal(t, "Lunch", data["title"])
	assert.Len(t, data["contacts"], 2)
}

func TestCreateWithoutContactsAndWithUnknownOnes(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, http.StatusOK, send(e.asA, http.MethodPost, "/activities", map[string]any{"title": "Solo", "date": day}).Code)

	foreign := e.contact(t, e.b.ID, "Mallory", "M")
	mine := e.contact(t, e.a.ID, "Alice", "A")
	for name, ids := range map[string][]uint{"foreign": {foreign.ID}, "missing": {9999}, "partly": {mine.ID, foreign.ID}, "duplicate": {mine.ID, mine.ID}} {
		w := send(e.asA, http.MethodPost, "/activities", map[string]any{"title": "x", "date": day, "contact_ids": ids})
		assert.Equal(t, http.StatusNotFound, w.Code, name+": "+w.Body.String())
	}
	var n int64
	e.db.Model(&models.Activity{}).Count(&n)
	assert.EqualValues(t, 1, n, "only the first activity was stored")
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	e := newEnv(t)
	for name, body := range map[string]map[string]any{
		"no title":         {"date": day},
		"title too long":   {"title": strings.Repeat("x", 201), "date": day},
		"no date":          {"title": "x"},
		"description long": {"title": "x", "date": day, "description": strings.Repeat("x", 2001)},
		"location long":    {"title": "x", "date": day, "location": strings.Repeat("x", 301)},
	} {
		assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPost, "/activities", body).Code, name)
	}
}

func TestGetShowsOnlyTheContactNamesOfTheActor(t *testing.T) {
	e := newEnv(t)
	alice := e.contact(t, e.a.ID, "Alice", "A")
	act := e.activity(t, e.a.ID, "Lunch", alice)

	w := send(e.asA, http.MethodGet, "/activities/"+sid(act.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Lunch", body["title"])
	contact := body["contacts"].([]any)[0].(map[string]any)
	assert.Equal(t, "Alice", contact["firstname"])
	assert.ElementsMatch(t, []string{"ID", "firstname", "lastname", "circles"}, keys(contact), "no email or other private values")

	w = send(e.asB, http.MethodGet, "/activities/"+sid(act.ID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, sid(act.ID), decode(t, w)["error"].(map[string]any)["details"].(map[string]any)["id"])
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodGet, "/activities/9999", nil).Code)
	assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodGet, "/activities/abc", nil).Code)
}

func keys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestListIsScopedPagedNewestFirstAndContactsAreOptional(t *testing.T) {
	e := newEnv(t)
	alice := e.contact(t, e.a.ID, "Alice", "A")
	old := e.activity(t, e.a.ID, "old", alice)
	require.NoError(t, e.db.Model(&old).Update("date", time.Now().AddDate(-1, 0, 0)).Error)
	e.activity(t, e.a.ID, "new")
	e.activity(t, e.b.ID, "foreign")

	body := decode(t, send(e.asA, http.MethodGet, "/activities", nil))
	assert.Equal(t, []string{"new", "old"}, titles(body))
	assert.EqualValues(t, 2, body["total"])
	assert.EqualValues(t, 25, body["limit"])
	assert.NotContains(t, body["activities"].([]any)[1], "contacts")

	body = decode(t, send(e.asA, http.MethodGet, "/activities?include=contacts&limit=1&page=2", nil))
	assert.Equal(t, []string{"old"}, titles(body))
	assert.Len(t, body["activities"].([]any)[0].(map[string]any)["contacts"], 1)

	body = decode(t, send(e.asA, http.MethodGet, "/activities?limit=1000&page=x", nil))
	assert.EqualValues(t, 100, body["limit"])
	assert.EqualValues(t, 1, body["page"])
}

func TestListSearchesTitlesAndContactNamesAndFiltersByDate(t *testing.T) {
	e := newEnv(t)
	alice := e.contact(t, e.a.ID, "Alice", "Smith")
	spring := models.Activity{UserID: e.a.ID, Title: "Picnic", Location: "Park", Date: time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), Contacts: []models.Contact{alice}}
	summer := models.Activity{UserID: e.a.ID, Title: "Swimming", Date: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)}
	require.NoError(t, e.db.Create(&spring).Error)
	require.NoError(t, e.db.Create(&summer).Error)
	list := func(q string) []string { return titles(decode(t, send(e.asA, http.MethodGet, "/activities?"+q, nil))) }

	assert.Equal(t, []string{"Swimming"}, list("search=swim"))
	assert.Equal(t, []string{"Picnic"}, list("search=park"))
	assert.Equal(t, []string{"Picnic"}, list("search=smith"), "by the name of a linked contact")
	assert.Equal(t, []string{"Swimming"}, list("fromDate=2026-06-01"))
	assert.Equal(t, []string{"Picnic"}, list("toDate=2026-03-10"), "the end date is inclusive")
	assert.Len(t, list("fromDate=garbage"), 2)
	assert.Empty(t, list("search=nobody"))

	other := e.contact(t, e.b.ID, "Mallory", "Smith")
	e.activity(t, e.b.ID, "Secret", other)
	assert.Equal(t, []string{"Picnic"}, list("search=smith"), "another user's contacts and activities do not match")
}

func TestContactActivitiesListTheContactsActivitiesOfTheActor(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.contact(t, e.a.ID, "Alice", "A"), e.contact(t, e.a.ID, "Bob", "B")
	e.activity(t, e.a.ID, "with alice", alice)
	e.activity(t, e.a.ID, "with both", alice, bob)
	e.activity(t, e.a.ID, "with bob", bob)

	w := send(e.asA, http.MethodGet, "/contacts/"+sid(alice.ID)+"/activities", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, []string{"with alice", "with both"}, titles(body))
	assert.Len(t, body["activities"].([]any)[1].(map[string]any)["contacts"], 2)

	empty := e.contact(t, e.a.ID, "Empty", "E")
	assert.Equal(t, []any{}, decode(t, send(e.asA, http.MethodGet, "/contacts/"+sid(empty.ID)+"/activities", nil))["activities"])
	w = send(e.asB, http.MethodGet, "/contacts/"+sid(alice.ID)+"/activities", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, sid(alice.ID), decode(t, w)["error"].(map[string]any)["details"].(map[string]any)["id"])
}

func TestUpdateReplacesFieldsAndContactsOnlyWhenGiven(t *testing.T) {
	e := newEnv(t)
	e.subscribe(t, e.a.ID, "activity.updated")
	alice, bob := e.contact(t, e.a.ID, "Alice", "A"), e.contact(t, e.a.ID, "Bob", "B")
	act := e.activity(t, e.a.ID, "old", alice)

	w := send(e.asA, http.MethodPut, "/activities/"+sid(act.ID), map[string]any{"title": "new", "date": day, "location": "Rome"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "new", body["title"])
	assert.Equal(t, "Rome", body["location"])
	assert.Len(t, body["contacts"], 1, "no contact_ids keeps the contacts")
	assert.Equal(t, "new", e.delivery(t, "activity.updated")["title"])

	w = send(e.asA, http.MethodPut, "/activities/"+sid(act.ID), map[string]any{"title": "new", "date": day, "contact_ids": []uint{bob.ID}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.ElementsMatch(t, []uint{bob.ID}, links(t, e, act))

	w = send(e.asA, http.MethodPut, "/activities/"+sid(act.ID), map[string]any{"title": "new", "date": day, "contact_ids": []uint{}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, links(t, e, act), "an empty list unlinks everyone")
}

func TestUpdateRefusesOtherUsersActivitiesAndContactsAndBadInput(t *testing.T) {
	e := newEnv(t)
	alice := e.contact(t, e.a.ID, "Alice", "A")
	foreign := e.contact(t, e.b.ID, "Mallory", "M")
	act := e.activity(t, e.a.ID, "mine", alice)
	path := "/activities/" + sid(act.ID)

	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodPut, path, map[string]any{"title": "hacked", "date": day}).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodPut, path, map[string]any{"title": "x", "date": day, "contact_ids": []uint{foreign.ID}}).Code)
	assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPut, path, map[string]any{"title": ""}).Code)

	var stored models.Activity
	require.NoError(t, e.db.First(&stored, act.ID).Error)
	assert.Equal(t, "mine", stored.Title)
	assert.Equal(t, []uint{alice.ID}, links(t, e, act), "a refused update leaves the contacts alone")
}

func TestDeleteRemovesTheActivityAndAnnouncesIt(t *testing.T) {
	e := newEnv(t)
	e.subscribe(t, e.a.ID, "activity.deleted")
	act := e.activity(t, e.a.ID, "bye")

	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodDelete, "/activities/"+sid(act.ID), nil).Code)
	w := send(e.asA, http.MethodDelete, "/activities/"+sid(act.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Activity deleted"}`, w.Body.String())
	var live int64
	e.db.Model(&models.Activity{}).Where("id = ?", act.ID).Count(&live)
	assert.Zero(t, live)
	assert.EqualValues(t, act.ID, e.delivery(t, "activity.deleted")["id"])
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodDelete, "/activities/"+sid(act.ID), nil).Code)
}

func TestWithoutAnAuthenticatedUserEverythingRefuses(t *testing.T) {
	e := newEnv(t)
	act := e.activity(t, e.a.ID, "mine")
	body := map[string]any{"title": "x", "date": day}
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/activities"}, {http.MethodPost, "/activities"}, {http.MethodGet, "/activities/" + sid(act.ID)},
		{http.MethodPut, "/activities/" + sid(act.ID)}, {http.MethodDelete, "/activities/" + sid(act.ID)},
		{http.MethodGet, "/contacts/1/activities"},
	} {
		assert.Equal(t, http.StatusUnauthorized, send(e.anon, call.method, call.path, body).Code, call.method+" "+call.path)
	}
}

// @005-US6
func TestAnActivityMayHaveAnEndDateThatCanBeChangedKeptAndCleared(t *testing.T) {
	e := newEnv(t)
	e.subscribe(t, e.a.ID, "activity.created")
	end := "2026-03-04T00:00:00Z"

	w := send(e.asA, http.MethodPost, "/activities", map[string]any{"title": "Trip", "date": day, "end_date": end})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	act := decode(t, w)["activity"].(map[string]any)
	assert.Equal(t, end, act["end_date"])
	assert.Equal(t, end, e.delivery(t, "activity.created")["end_date"])

	path := "/activities/" + sid(uint(act["ID"].(float64)))
	assert.Equal(t, end, decode(t, send(e.asA, http.MethodGet, path, nil))["end_date"])

	later := "2026-03-09T00:00:00Z"
	assert.Equal(t, later, decode(t, send(e.asA, http.MethodPut, path, map[string]any{"title": "Trip", "date": day, "end_date": later}))["end_date"])
	assert.Equal(t, later, decode(t, send(e.asA, http.MethodPut, path, map[string]any{"title": "Trip", "date": day}))["end_date"], "leaving it out keeps it")
	assert.Nil(t, decode(t, send(e.asA, http.MethodPut, path, map[string]any{"title": "Trip", "date": day, "end_date": nil}))["end_date"], "null clears it")

	w = send(e.asA, http.MethodPost, "/activities", map[string]any{"title": "Day", "date": day})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, decode(t, w)["activity"], "end_date")
	assert.Nil(t, decode(t, w)["activity"].(map[string]any)["end_date"])
}

// @005-US6
func TestAnEndDateBeforeTheStartIsRefused(t *testing.T) {
	e := newEnv(t)
	act := e.activity(t, e.a.ID, "Trip")
	before := "2026-02-28T00:00:00Z"

	assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPost, "/activities", map[string]any{"title": "x", "date": day, "end_date": before}).Code)
	assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPut, "/activities/"+sid(act.ID), map[string]any{"title": "x", "date": day, "end_date": before}).Code)
	sameDay := "2026-03-01T00:00:00Z"
	assert.Equal(t, http.StatusOK, send(e.asA, http.MethodPost, "/activities", map[string]any{"title": "x", "date": day, "end_date": sameDay}).Code, "the end may be the start day")
}

// @005-US6
func TestMovingTheStartPastAKeptEndDateIsRefused(t *testing.T) {
	e := newEnv(t)
	w := send(e.asA, http.MethodPost, "/activities", map[string]any{"title": "Trip", "date": day, "end_date": "2026-03-04T00:00:00Z"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	path := "/activities/" + sid(uint(decode(t, w)["activity"].(map[string]any)["ID"].(float64)))

	assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPut, path, map[string]any{"title": "Trip", "date": "2026-03-05T00:00:00Z"}).Code)
	assert.Equal(t, "2026-03-04T00:00:00Z", decode(t, send(e.asA, http.MethodGet, path, nil))["end_date"])
	assert.Equal(t, http.StatusOK, send(e.asA, http.MethodPut, path, map[string]any{"title": "Trip", "date": "2026-03-04T00:00:00Z"}).Code, "the start may move onto the end day")
}

// @005-US6
func TestListFilterKeepsActivitiesThatOverlapTheRange(t *testing.T) {
	e := newEnv(t)
	utc := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	march := utc(3, 3)
	trip := models.Activity{UserID: e.a.ID, Title: "Trip", Date: utc(2, 28), EndDate: &march}
	lunch := models.Activity{UserID: e.a.ID, Title: "Lunch", Date: utc(3, 10)}
	require.NoError(t, e.db.Create(&trip).Error)
	require.NoError(t, e.db.Create(&lunch).Error)
	list := func(q string) []string { return titles(decode(t, send(e.asA, http.MethodGet, "/activities?"+q, nil))) }

	assert.Equal(t, []string{"Trip"}, list("fromDate=2026-03-01&toDate=2026-03-02"), "a range running into the filter")
	assert.Equal(t, []string{"Trip"}, list("fromDate=2026-03-03&toDate=2026-03-05"), "the last day counts")
	assert.Equal(t, []string{"Lunch"}, list("fromDate=2026-03-04"), "a range that ended is out")
	assert.Equal(t, []string{"Trip"}, list("toDate=2026-02-28"))
}

func (e *env) image(t *testing.T, owner uint, name string) models.Image {
	t.Helper()
	img := models.Image{
		UserID: owner, File: "images/u1/" + name + "_image.webp", ThumbFile: "images/u1/" + name + "_thumb.webp",
		Mime: "image/webp", Width: 30, Height: 20, Bytes: 7,
	}
	require.NoError(t, e.db.Create(&img).Error)
	require.NoError(t, os.MkdirAll(filepath.Join(e.dir, "images", "u1"), 0o750))
	for _, f := range []string{img.File, img.ThumbFile} {
		require.NoError(t, os.WriteFile(filepath.Join(e.dir, f), []byte("x"), 0o600))
	}
	return img
}

func (e *env) has(img models.Image) bool {
	_, err := os.Stat(filepath.Join(e.dir, img.File))
	return err == nil
}

func imageIDs(body map[string]any) []float64 {
	out := []float64{}
	for _, i := range body["images"].([]any) {
		out = append(out, i.(map[string]any)["id"].(float64))
	}
	return out
}

// @023-US2
func TestAnActivityCarriesItsImagesInOrderEverywhereItIsListed(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Ann", "A")
	one, two := e.image(t, e.a.ID, "1"), e.image(t, e.a.ID, "2")

	w := send(e.asA, http.MethodPost, "/activities", map[string]any{
		"title": "Hike", "date": day, "contact_ids": []uint{c.ID}, "image_ids": []uint{two.ID, one.ID},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	created := decode(t, w)["activity"].(map[string]any)
	want := []float64{float64(two.ID), float64(one.ID)}
	assert.Equal(t, want, imageIDs(created))
	assert.Contains(t, created["images"].([]any)[0].(map[string]any)["thumb_url"], "/api/v1/images/")

	id := sid(uint(created["ID"].(float64)))
	assert.Equal(t, want, imageIDs(decode(t, send(e.asA, http.MethodGet, "/activities/"+id, nil))))
	listed := decode(t, send(e.asA, http.MethodGet, "/activities", nil))["activities"].([]any)
	assert.Equal(t, want, imageIDs(listed[0].(map[string]any)))
	forContact := decode(t, send(e.asA, http.MethodGet, "/contacts/"+sid(c.ID)+"/activities", nil))["activities"].([]any)
	assert.Equal(t, want, imageIDs(forContact[0].(map[string]any)))
}

// @023-US2
func TestUpdateKeepsActivityImagesWhenTheFieldIsLeftOutAndRemovesThemWithAnEmptyList(t *testing.T) {
	e := newEnv(t)
	img := e.image(t, e.a.ID, "1")
	w := send(e.asA, http.MethodPost, "/activities", map[string]any{"title": "Hike", "date": day, "image_ids": []uint{img.ID}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	id := sid(uint(decode(t, w)["activity"].(map[string]any)["ID"].(float64)))

	w = send(e.asA, http.MethodPut, "/activities/"+id, map[string]any{"title": "Hike 2", "date": day})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []float64{float64(img.ID)}, imageIDs(decode(t, w)))
	assert.True(t, e.has(img))

	w = send(e.asA, http.MethodPut, "/activities/"+id, map[string]any{"title": "Hike 2", "date": day, "image_ids": []uint{}})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []float64{}, imageIDs(decode(t, w)))
	assert.False(t, e.has(img), "the file goes with the row")
}

// @023-US2
func TestDeletingAnActivityDeletesItsImagesAndFiles(t *testing.T) {
	e := newEnv(t)
	img := e.image(t, e.a.ID, "1")
	w := send(e.asA, http.MethodPost, "/activities", map[string]any{"title": "Hike", "date": day, "image_ids": []uint{img.ID}})
	require.Equal(t, http.StatusOK, w.Code)
	id := sid(uint(decode(t, w)["activity"].(map[string]any)["ID"].(float64)))

	require.Equal(t, http.StatusOK, send(e.asA, http.MethodDelete, "/activities/"+id, nil).Code)
	var rows int64
	e.db.Model(&models.Image{}).Count(&rows)
	assert.Zero(t, rows)
	assert.False(t, e.has(img))
}

func TestActivityImagesOfOtherUsersOrOtherActivitiesCannotBeAttached(t *testing.T) {
	e := newEnv(t)
	theirs, mine := e.image(t, e.b.ID, "theirs"), e.image(t, e.a.ID, "mine")
	body := func(ids ...uint) map[string]any {
		return map[string]any{"title": "Hike", "date": day, "image_ids": ids}
	}
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodPost, "/activities", body(theirs.ID)).Code)
	require.Equal(t, http.StatusOK, send(e.asA, http.MethodPost, "/activities", body(mine.ID)).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodPost, "/activities", body(mine.ID)).Code, "an image already on an activity")
	var n int64
	e.db.Model(&models.Activity{}).Count(&n)
	assert.EqualValues(t, 1, n, "a refused request creates no activity")

	var many []uint
	for i := range 21 {
		many = append(many, e.image(t, e.a.ID, "m"+strconv.Itoa(i)).ID)
	}
	assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPost, "/activities", body(many...)).Code)
}
