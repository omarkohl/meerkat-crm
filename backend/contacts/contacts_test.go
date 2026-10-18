package contacts_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/contacts"
	"meerkat/middleware"
	"meerkat/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type env struct {
	db     *gorm.DB
	a, b   models.User
	asA    *gin.Engine
	asB    *gin.Engine
	anon   *gin.Engine
	photos string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Activity{}, &models.Note{}, &models.Relationship{},
		&models.Reminder{}, &models.Webhook{}, &models.WebhookDelivery{}))
	e := &env{db: db, photos: t.TempDir()}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)

	cfg := &config.Config{ProfilePhotoDir: e.photos}
	mount := func(userID uint) *gin.Engine {
		r := gin.New()
		r.Use(middleware.ClockMiddleware(clock.Real()))
		authenticated := r.Group("")
		if userID != 0 {
			authenticated.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
		}
		contacts.New(contacts.Deps{DB: db, Config: cfg}).Register(api.New(r.Group(""), r), authenticated)
		return r
	}
	e.asA, e.asB, e.anon = mount(e.a.ID), mount(e.b.ID), mount(0)
	return e
}

func (e *env) contact(t *testing.T, owner uint, name string, circles ...string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: owner, Firstname: name, Circles: circles}
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

func do(r http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
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

func (e *env) subscribe(t *testing.T, owner uint, events ...string) {
	t.Helper()
	require.NoError(t, e.db.Create(&models.Webhook{UserID: owner, Name: "w", URL: "http://127.0.0.1:1/", Events: events, Secret: "s", IsActive: true}).Error)
}

func (e *env) deliveries(event string) func() int64 {
	return func() int64 {
		var n int64
		e.db.Model(&models.WebhookDelivery{}).Where("event_type = ?", event).Count(&n)
		return n
	}
}

func idPath(c models.Contact, suffix string) string {
	return "/contacts/" + strconv.Itoa(int(c.ID)) + suffix
}

// @004-US7
func TestArchiveHidesTheContactAndDeletesItsReminders(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	require.NoError(t, e.db.Create(&models.Reminder{UserID: e.a.ID, ContactID: &c.ID, Message: "call", RemindAt: time.Now(), Recurrence: "once"}).Error)

	w := do(e.asA, http.MethodPost, idPath(c, "/archive"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["archived"])
	assert.Equal(t, "Alice", body["firstname"])
	// The JSON names are those of the old model, not new ones.
	for _, key := range []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt", "firstname", "relationships", "custom_fields", "how_we_met"} {
		assert.Contains(t, body, key)
	}
	assert.Nil(t, body["DeletedAt"])

	var stored models.Contact
	require.NoError(t, e.db.First(&stored, c.ID).Error)
	assert.True(t, stored.Archived)
	var reminders int64
	e.db.Model(&models.Reminder{}).Where("contact_id = ?", c.ID).Count(&reminders)
	assert.Zero(t, reminders, "archiving deletes the reminders")
}

func TestUnarchive(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	require.NoError(t, e.db.Model(&c).Update("archived", true).Error)

	w := do(e.asA, http.MethodPost, idPath(c, "/unarchive"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, false, body["archived"])
	var stored models.Contact
	require.NoError(t, e.db.First(&stored, c.ID).Error)
	assert.False(t, stored.Archived)
}

// @004-US5
func TestDeleteMovesTheContactAndItsDependantsToTheTrash(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	other := e.contact(t, e.a.ID, "Bob")
	require.NoError(t, e.db.Create(&models.Note{UserID: e.a.ID, ContactID: &c.ID, Content: "n"}).Error)
	require.NoError(t, e.db.Create(&models.Reminder{UserID: e.a.ID, ContactID: &c.ID, Message: "r", RemindAt: time.Now(), Recurrence: "once"}).Error)
	require.NoError(t, e.db.Create(&models.Relationship{UserID: e.a.ID, ContactID: c.ID, RelatedContactID: &other.ID, Name: "Bob", Type: "friend"}).Error)

	w := do(e.asA, http.MethodDelete, idPath(c, ""))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Contact deleted"}`, w.Body.String())

	var live int64
	e.db.Model(&models.Contact{}).Where("id = ?", c.ID).Count(&live)
	assert.Zero(t, live)
	var trashed int64
	e.db.Unscoped().Model(&models.Contact{}).Where("id = ? AND deleted_at IS NOT NULL", c.ID).Count(&trashed)
	assert.EqualValues(t, 1, trashed, "soft-deleted, so it can be restored")
	for _, m := range []any{&models.Note{}, &models.Reminder{}, &models.Relationship{}} {
		var n int64
		e.db.Model(m).Where("contact_id = ?", c.ID).Count(&n)
		assert.Zero(t, n, "dependants are soft-deleted with the contact")
	}
	e.db.Model(&models.Contact{}).Where("id = ?", other.ID).Count(&live)
	assert.EqualValues(t, 1, live, "other contacts stay")
}

func TestDeleteKeepsPhotoFilesForTheTrash(t *testing.T) {
	e := newEnv(t)
	photo := filepath.Join(e.photos, "p.jpg")
	require.NoError(t, os.WriteFile(photo, []byte("x"), 0o600))
	c := models.Contact{UserID: e.a.ID, Firstname: "Alice", Photo: "p.jpg"}
	require.NoError(t, e.db.Create(&c).Error)

	require.Equal(t, http.StatusOK, do(e.asA, http.MethodDelete, idPath(c, "")).Code)

	_, err := os.Stat(photo)
	assert.NoError(t, err, "a restore from the trash must find the photo")
}

// @004-US8
func TestCirclesAreDistinctAndOnlyTheActorsLiveContacts(t *testing.T) {
	e := newEnv(t)
	e.contact(t, e.a.ID, "Alice", "Friends", "Family")
	e.contact(t, e.a.ID, "Bob", "Friends", "Work")
	e.contact(t, e.b.ID, "Mallory", "Secret")
	gone := e.contact(t, e.a.ID, "Gone", "Trashed")
	require.NoError(t, e.db.Delete(&gone).Error)

	w := do(e.asA, http.MethodGet, "/contacts/circles")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var names []string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &names))
	assert.ElementsMatch(t, []string{"Friends", "Family", "Work"}, names)
}

func TestCirclesWithoutContactsIsNullLikeBefore(t *testing.T) {
	e := newEnv(t)
	w := do(e.asA, http.MethodGet, "/contacts/circles")
	require.Equal(t, http.StatusOK, w.Code)
	t.Logf("empty circles body: %q", w.Body.String())
	var names []string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &names))
	assert.Empty(t, names)
}

// Isolation: another user's contact is a 404 for every operation, and is left untouched.
// @013-US1
func TestOtherUsersContactsAreNotFound(t *testing.T) {
	e := newEnv(t)
	victim := e.contact(t, e.a.ID, "Alice")

	for _, call := range []struct{ method, suffix string }{
		{http.MethodPost, "/archive"}, {http.MethodPost, "/unarchive"}, {http.MethodDelete, ""},
	} {
		w := do(e.asB, call.method, idPath(victim, call.suffix))
		assert.Equal(t, http.StatusNotFound, w.Code, call.method+call.suffix+": "+w.Body.String())
		var b struct {
			Error struct {
				Code    string
				Details map[string]any
			}
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
		assert.Equal(t, "NOT_FOUND", b.Error.Code)
		assert.Equal(t, strconv.Itoa(int(victim.ID)), b.Error.Details["id"])
	}
	var stored models.Contact
	require.NoError(t, e.db.First(&stored, victim.ID).Error)
	assert.False(t, stored.Archived, "untouched")
}

func TestMissingContactAndBadIDs(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, http.StatusNotFound, do(e.asA, http.MethodPost, "/contacts/9999/archive").Code)
	assert.Equal(t, http.StatusBadRequest, do(e.asA, http.MethodPost, "/contacts/abc/archive").Code)
	assert.Equal(t, http.StatusBadRequest, do(e.asA, http.MethodDelete, "/contacts/0").Code)
}

func TestWithoutAnAuthenticatedUserTheOperationsRefuse(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	for _, call := range []struct{ method, path string }{
		{http.MethodPost, idPath(c, "/archive")}, {http.MethodDelete, idPath(c, "")}, {http.MethodGet, "/contacts/circles"},
	} {
		assert.Equal(t, http.StatusUnauthorized, do(e.anon, call.method, call.path).Code, call.method+" "+call.path)
	}
	var stored models.Contact
	require.NoError(t, e.db.First(&stored, c.ID).Error)
	assert.False(t, stored.Archived)
}

// @004-US3
func TestCreateStoresTheContactForTheActorAndKeepsPrimaryValuesInSync(t *testing.T) {
	e := newEnv(t)
	w := send(e.asA, http.MethodPost, "/contacts", map[string]any{
		"firstname": "Alice", "lastname": "Smith", "birthday": "--12-25", "circles": []string{"Friends"},
		"emails":    []map[string]any{{"type": "home", "value": "a@example.com"}, {"value": "b@example.com"}},
		"phones":    []map[string]any{{"value": "+49 30 1234"}},
		"addresses": []map[string]any{{"street": "Main St 1", "city": "Berlin"}},
		"urls":      []map[string]any{{"value": "https://example.com"}},
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Contact created successfully", body["message"])
	contact := body["contact"].(map[string]any)
	assert.Equal(t, "Alice", contact["firstname"])
	assert.Equal(t, "a@example.com", contact["email"])
	assert.Equal(t, "+49 30 1234", contact["phone"])
	assert.Equal(t, "Main St 1, Berlin", contact["address"])
	assert.NotZero(t, contact["ID"])

	var stored models.Contact
	require.NoError(t, e.db.First(&stored, uint(contact["ID"].(float64))).Error)
	assert.Equal(t, e.a.ID, stored.UserID)
	assert.Equal(t, "a@example.com", stored.Email)
	assert.Len(t, stored.Emails, 2)
}

func TestCreateRejectsInvalidInputWithTheValidatorsMessages(t *testing.T) {
	e := newEnv(t)
	for name, body := range map[string]map[string]any{
		"no firstname": {"lastname": "Smith"},
		"bad email":    {"firstname": "A", "emails": []map[string]any{{"value": "nope"}}},
		"bad gender":   {"firstname": "A", "gender": "robot"},
		"bad birthday": {"firstname": "A", "birthday": "25-12-1990"},
		"bad url":      {"firstname": "A", "urls": []map[string]any{{"value": "javascript:alert(1)"}}},
		"circles":      {"firstname": "A", "circles": []string{"x", "x"}},
	} {
		w := send(e.asA, http.MethodPost, "/contacts", body)
		assert.Equal(t, http.StatusBadRequest, w.Code, name+": "+w.Body.String())
	}
	var n int64
	e.db.Model(&models.Contact{}).Count(&n)
	assert.Zero(t, n)
}

func TestCreateAnnouncesTheContactToWebhooks(t *testing.T) {
	e := newEnv(t)
	e.subscribe(t, e.a.ID, "contact.created")
	require.Equal(t, http.StatusCreated, send(e.asA, http.MethodPost, "/contacts", map[string]any{"firstname": "Alice"}).Code)
	assert.Eventually(t, func() bool { return e.deliveries("contact.created")() == 1 }, 2*time.Second, 10*time.Millisecond)
	var d models.WebhookDelivery
	require.NoError(t, e.db.First(&d).Error)
	assert.Contains(t, d.Payload, `"firstname":"Alice"`)
}

// @004-US4
func TestUpdateReplacesTheEditableFieldsAndKeepsThePhotoAndArchiveState(t *testing.T) {
	e := newEnv(t)
	c := models.Contact{UserID: e.a.ID, Firstname: "Alice", Lastname: "Old", Photo: "p.jpg", Archived: true, Note: "old note"}
	require.NoError(t, e.db.Create(&c).Error)

	w := send(e.asA, http.MethodPut, idPath(c, ""), map[string]any{
		"firstname": "Alicia", "emails": []map[string]any{{"value": "new@example.com"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Alicia", body["firstname"])
	assert.Equal(t, "new@example.com", body["email"])
	assert.Equal(t, "", body["lastname"], "fields left out are cleared, as before")

	var stored models.Contact
	require.NoError(t, e.db.First(&stored, c.ID).Error)
	assert.Equal(t, "Alicia", stored.Firstname)
	assert.Equal(t, "new@example.com", stored.Email)
	assert.Equal(t, "p.jpg", stored.Photo)
	assert.True(t, stored.Archived)
	assert.Empty(t, stored.Note)
}

func TestUpdateRejectsInvalidInputAndLeavesTheContact(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	w := send(e.asA, http.MethodPut, idPath(c, ""), map[string]any{"firstname": ""})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var stored models.Contact
	require.NoError(t, e.db.First(&stored, c.ID).Error)
	assert.Equal(t, "Alice", stored.Firstname)
}

func TestUpdateAnnouncesTheContactToWebhooks(t *testing.T) {
	e := newEnv(t)
	e.subscribe(t, e.a.ID, "contact.updated")
	c := e.contact(t, e.a.ID, "Alice")
	require.Equal(t, http.StatusOK, send(e.asA, http.MethodPut, idPath(c, ""), map[string]any{"firstname": "Alicia"}).Code)
	assert.Eventually(t, func() bool { return e.deliveries("contact.updated")() == 1 }, 2*time.Second, 10*time.Millisecond)
}

func TestGetReturnsTheContactWithItsRelationships(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	require.NoError(t, e.db.Create(&models.Relationship{UserID: e.a.ID, ContactID: c.ID, Name: "Bob", Type: "friend"}).Error)
	require.NoError(t, e.db.Create(&models.Relationship{UserID: e.b.ID, ContactID: c.ID, Name: "Spy", Type: "friend"}).Error)

	w := do(e.asA, http.MethodGet, idPath(c, ""))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Alice", body["firstname"])
	rels := body["relationships"].([]any)
	require.Len(t, rels, 1, "only the actor's relationships are loaded")
	assert.Equal(t, "Bob", rels[0].(map[string]any)["name"])
}

func TestGetReturnsNotesActivitiesAndRemindersOfTheActor(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	require.NoError(t, e.db.AutoMigrate(&models.Activity{}))
	require.NoError(t, e.db.Create(&models.Note{UserID: e.a.ID, ContactID: &c.ID, Content: "met", Date: time.Now()}).Error)
	require.NoError(t, e.db.Create(&models.Note{UserID: e.b.ID, ContactID: &c.ID, Content: "foreign", Date: time.Now()}).Error)
	require.NoError(t, e.db.Create(&models.Reminder{UserID: e.a.ID, ContactID: &c.ID, Message: "call", RemindAt: time.Now(), Recurrence: "once"}).Error)
	act := models.Activity{UserID: e.a.ID, Title: "Lunch", Date: time.Now(), Contacts: []models.Contact{c}}
	require.NoError(t, e.db.Create(&act).Error)

	body := decode(t, do(e.asA, http.MethodGet, idPath(c, "")))
	notes := body["notes"].([]any)
	require.Len(t, notes, 1)
	assert.Equal(t, "met", notes[0].(map[string]any)["content"])
	assert.Equal(t, "call", body["reminders"].([]any)[0].(map[string]any)["message"])
	assert.Equal(t, "Lunch", body["activities"].([]any)[0].(map[string]any)["title"])
}

func TestGetWithoutRelationshipsReturnsAnEmptyListButFieldsLeaveItNull(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	assert.Equal(t, []any{}, decode(t, do(e.asA, http.MethodGet, idPath(c, "")))["relationships"])
	assert.Nil(t, decode(t, do(e.asA, http.MethodGet, idPath(c, "?fields=firstname")))["relationships"])
}

func TestGetWithFieldsReturnsOnlyThoseValues(t *testing.T) {
	e := newEnv(t)
	c := models.Contact{UserID: e.a.ID, Firstname: "Alice", Lastname: "Smith", Email: "a@example.com"}
	require.NoError(t, e.db.Create(&c).Error)

	w := do(e.asA, http.MethodGet, idPath(c, "?fields=firstname,email,bogus"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Alice", body["firstname"])
	assert.Equal(t, "a@example.com", body["email"])
	assert.Equal(t, "", body["lastname"])
	assert.Nil(t, body["relationships"])
}

func TestGetUpdateAndCreateRefuseOtherUsersAndAnonymous(t *testing.T) {
	e := newEnv(t)
	victim := e.contact(t, e.a.ID, "Alice")

	assert.Equal(t, http.StatusNotFound, do(e.asB, http.MethodGet, idPath(victim, "")).Code)
	w := send(e.asB, http.MethodPut, idPath(victim, ""), map[string]any{"firstname": "Hacked"})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	var stored models.Contact
	require.NoError(t, e.db.First(&stored, victim.ID).Error)
	assert.Equal(t, "Alice", stored.Firstname)

	assert.Equal(t, http.StatusNotFound, do(e.asA, http.MethodGet, "/contacts/9999").Code)
	assert.Equal(t, http.StatusBadRequest, do(e.asA, http.MethodGet, "/contacts/abc").Code)
	assert.Equal(t, http.StatusUnauthorized, do(e.anon, http.MethodGet, idPath(victim, "")).Code)
	assert.Equal(t, http.StatusUnauthorized, send(e.anon, http.MethodPost, "/contacts", map[string]any{"firstname": "X"}).Code)
	assert.Equal(t, http.StatusUnauthorized, send(e.anon, http.MethodPut, idPath(victim, ""), map[string]any{"firstname": "X"}).Code)
}

func names(body map[string]any) []string {
	var out []string
	for _, c := range body["contacts"].([]any) {
		out = append(out, c.(map[string]any)["firstname"].(string))
	}
	return out
}

func TestListIsPagedSortedAndScopedToTheActor(t *testing.T) {
	e := newEnv(t)
	for _, n := range []string{"Carol", "Alice", "Bob"} {
		e.contact(t, e.a.ID, n)
	}
	e.contact(t, e.b.ID, "Mallory")

	body := decode(t, do(e.asA, http.MethodGet, "/contacts?sort=firstname&order=asc&limit=2&page=2"))
	assert.Equal(t, []string{"Carol"}, names(body))
	assert.EqualValues(t, 3, body["total"])
	assert.EqualValues(t, 2, body["page"])
	assert.EqualValues(t, 2, body["limit"])

	body = decode(t, do(e.asA, http.MethodGet, "/contacts"))
	assert.Equal(t, []string{"Bob", "Alice", "Carol"}, names(body), "newest first by default")
	assert.EqualValues(t, 25, body["limit"])

	body = decode(t, do(e.asA, http.MethodGet, "/contacts?page=5&limit=2"))
	assert.Empty(t, names(body), "a page past the end is empty")
	assert.EqualValues(t, 3, body["total"])
	assert.EqualValues(t, 5, body["page"])

	body = decode(t, do(e.asA, http.MethodGet, "/contacts?page=abc&limit=1000&sort=bogus&order=sideways"))
	assert.EqualValues(t, 1, body["page"], "bad values fall back to the defaults")
	assert.EqualValues(t, 100, body["limit"], "the limit is capped")
	assert.Len(t, names(body), 3)
}

// @004-US8
func TestListFiltersArchivedAndCircles(t *testing.T) {
	e := newEnv(t)
	e.contact(t, e.a.ID, "Alice", "Friends")
	e.contact(t, e.a.ID, "Bob", "Work")
	old := e.contact(t, e.a.ID, "Old", "Friends")
	require.NoError(t, e.db.Model(&old).Update("archived", true).Error)

	assert.ElementsMatch(t, []string{"Alice", "Bob"}, names(decode(t, do(e.asA, http.MethodGet, "/contacts"))))
	assert.Equal(t, []string{"Old"}, names(decode(t, do(e.asA, http.MethodGet, "/contacts?archived=true"))))
	assert.Len(t, names(decode(t, do(e.asA, http.MethodGet, "/contacts?include_archived=true"))), 3)
	assert.Equal(t, []string{"Alice"}, names(decode(t, do(e.asA, http.MethodGet, "/contacts?circle=Friends"))))
}

// @004-US2
func TestListSearchesNamesEmailsAndPhones(t *testing.T) {
	e := newEnv(t)
	c := models.Contact{
		UserID: e.a.ID, Firstname: "Alice", Lastname: "Johnson", Nickname: "Ali",
		Emails: []models.ContactEmail{{Value: "a@one.example"}, {Value: "second@two.example"}},
		Phones: []models.ContactPhone{{Value: "+15551112222"}},
	}
	require.NoError(t, e.db.Create(&c).Error)
	require.NoError(t, e.db.Create(&models.Contact{UserID: e.a.ID, Firstname: "Bob", Lastname: "Smith", Nickname: "Bobby"}).Error)
	for term, want := range map[string]string{"Johnson": "Alice", "Ali": "Alice", "a@one": "Alice", "5551112": "Alice", "Bobby": "Bob", "Smith": "Bob"} {
		assert.Equal(t, []string{want}, names(decode(t, do(e.asA, http.MethodGet, "/contacts?search="+term))), term)
	}

	assert.Equal(t, []string{"Alice"}, names(decode(t, do(e.asA, http.MethodGet, "/contacts?search=ali"))))
	assert.Equal(t, []string{"Alice"}, names(decode(t, do(e.asA, http.MethodGet, "/contacts?search=two.example"))))
	assert.Empty(t, names(decode(t, do(e.asA, http.MethodGet, "/contacts?search=nobody"))))
}

func TestListFieldsAndIncludes(t *testing.T) {
	e := newEnv(t)
	c := models.Contact{UserID: e.a.ID, Firstname: "Alice", Lastname: "Smith", PhotoThumbnailFile: "t_thumb.jpg"}
	require.NoError(t, e.db.Create(&c).Error)
	require.NoError(t, e.db.Create(&models.Note{UserID: e.a.ID, ContactID: &c.ID, Content: "n", Date: time.Now()}).Error)

	body := decode(t, do(e.asA, http.MethodGet, "/contacts?fields=firstname,photo_thumbnail&includes=notes"))
	first := body["contacts"].([]any)[0].(map[string]any)
	assert.Equal(t, "Alice", first["firstname"])
	assert.Equal(t, "", first["lastname"])
	assert.Contains(t, first["photo_thumbnail"], "/profile_picture?thumbnail=true&v=t")
	assert.Equal(t, float64(c.ID), first["ID"], "ID is added for the thumbnail URL")
	assert.Len(t, first["notes"], 1)

	require.NoError(t, e.db.Create(&models.Reminder{UserID: e.a.ID, ContactID: &c.ID, Message: "call", RemindAt: time.Now(), Recurrence: "once"}).Error)
	first = decode(t, do(e.asA, http.MethodGet, "/contacts?includes=notes,reminders"))["contacts"].([]any)[0].(map[string]any)
	assert.Len(t, first["notes"], 1)
	assert.Len(t, first["reminders"], 1)
	assert.Equal(t, []any{}, decode(t, do(e.asA, http.MethodGet, "/contacts?includes=relationships"))["contacts"].([]any)[0].(map[string]any)["relationships"])

	first = decode(t, do(e.asA, http.MethodGet, "/contacts"))["contacts"].([]any)[0].(map[string]any)
	assert.Equal(t, "Smith", first["lastname"], "all fields without fields=")
	assert.NotContains(t, first, "notes")
}

func TestRandomReturnsAtMostFiveLiveContactsOfTheActor(t *testing.T) {
	e := newEnv(t)
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		e.contact(t, e.a.ID, n)
	}
	e.contact(t, e.b.ID, "Mallory")
	old := e.contact(t, e.a.ID, "Old")
	require.NoError(t, e.db.Model(&old).Update("archived", true).Error)

	got := names(decode(t, do(e.asA, http.MethodGet, "/contacts/random")))
	assert.Len(t, got, 5)
	assert.NotContains(t, got, "Mallory")
	assert.NotContains(t, got, "Old")
}

func TestBirthdaysListTheActorsUpcomingOnes(t *testing.T) {
	e := newEnv(t)
	today := "--" + time.Now().Format("01-02")
	require.NoError(t, e.db.Create(&models.Contact{UserID: e.a.ID, Firstname: "Alice", Birthday: today}).Error)
	require.NoError(t, e.db.Create(&models.Contact{UserID: e.b.ID, Firstname: "Mallory", Birthday: today}).Error)

	body := decode(t, do(e.asA, http.MethodGet, "/contacts/birthdays"))
	list := body["birthdays"].([]any)
	require.Len(t, list, 1)
	assert.Equal(t, "Alice", list[0].(map[string]any)["name"])
	assert.Equal(t, "contact", list[0].(map[string]any)["type"])
}

func TestListsRefuseWithoutAUser(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/contacts", "/contacts/random", "/contacts/birthdays"} {
		assert.Equal(t, http.StatusUnauthorized, do(e.anon, http.MethodGet, p).Code, p)
	}
}
