package notes_test

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

	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/notes"

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
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Note{}, &models.Image{}, &models.Webhook{}, &models.WebhookDelivery{}))
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
		notes.New(notes.Deps{DB: db, Config: cfg}).Register(api.New(r.Group(""), r), authenticated)
		return r
	}
	e.asA, e.asB, e.anon = mount(e.a.ID), mount(e.b.ID), mount(0)
	return e
}

func (e *env) contact(t *testing.T, owner uint, name string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: owner, Firstname: name}
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

func (e *env) note(t *testing.T, owner uint, content string, contact *models.Contact) models.Note {
	t.Helper()
	n := models.Note{UserID: owner, Content: content, Date: time.Now()}
	if contact != nil {
		n.ContactID = &contact.ID
	}
	require.NoError(t, e.db.Create(&n).Error)
	return n
}

func (e *env) subscribe(t *testing.T, owner uint, events ...string) {
	t.Helper()
	require.NoError(t, e.db.Create(&models.Webhook{UserID: owner, Name: "w", URL: "http://127.0.0.1:1/", Events: events, Secret: "s", IsActive: true}).Error)
}

func (e *env) deliveries(event string) int64 {
	var n int64
	e.db.Model(&models.WebhookDelivery{}).Where("event_type = ?", event).Count(&n)
	return n
}

func send(r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
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

func id(n models.Note) string     { return strconv.Itoa(int(n.ID)) }
func cid(c models.Contact) string { return strconv.Itoa(int(c.ID)) }

func contents(body map[string]any) []string {
	var out []string
	for _, n := range body["notes"].([]any) {
		out = append(out, n.(map[string]any)["content"].(string))
	}
	return out
}

// @005-US1
func TestCreateForAContactStoresTheNoteForTheActor(t *testing.T) {
	e := newEnv(t)
	e.subscribe(t, e.a.ID, "note.created")
	c := e.contact(t, e.a.ID, "Alice")

	w := send(e.asA, http.MethodPost, "/contacts/"+cid(c)+"/notes", map[string]any{"content": "met at lunch", "date": "2026-03-01T12:00:00Z", "contact_id": 99999})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Note created successfully", body["message"])
	note := body["note"].(map[string]any)
	assert.Equal(t, "met at lunch", note["content"])
	assert.EqualValues(t, c.ID, note["contact_id"], "the contact of the URL wins over the body")
	for _, key := range []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt", "date"} {
		assert.Contains(t, note, key)
	}

	var stored models.Note
	require.NoError(t, e.db.First(&stored, uint(note["ID"].(float64))).Error)
	assert.Equal(t, e.a.ID, stored.UserID)
	assert.Eventually(t, func() bool { return e.deliveries("note.created") == 1 }, 2*time.Second, 10*time.Millisecond)

	var d models.WebhookDelivery
	require.NoError(t, e.db.Where("event_type = ?", "note.created").First(&d).Error)
	var payload struct{ Data map[string]any }
	require.NoError(t, json.Unmarshal([]byte(d.Payload), &payload))
	assert.Equal(t, "met at lunch", payload.Data["content"])
	assert.EqualValues(t, c.ID, payload.Data["contact_id"])
	assert.NotContains(t, payload.Data, "contact", "the payload is the note, without an embedded contact")
}

func TestCreateForAnotherUsersContactIsNotFound(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	w := send(e.asB, http.MethodPost, "/contacts/"+cid(c)+"/notes", map[string]any{"content": "x", "date": "2026-03-01T12:00:00Z"})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodPost, "/contacts/9999/notes", map[string]any{"content": "x", "date": "2026-03-01T12:00:00Z"}).Code)
	var n int64
	e.db.Model(&models.Note{}).Count(&n)
	assert.Zero(t, n)
}

func TestCreateUnassignedWithAndWithoutAContact(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")

	w := send(e.asA, http.MethodPost, "/notes", map[string]any{"content": "floating", "date": "2026-03-01T12:00:00Z"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Nil(t, decode(t, w)["note"].(map[string]any)["contact_id"])

	w = send(e.asA, http.MethodPost, "/notes", map[string]any{"content": "linked", "date": "2026-03-01T12:00:00Z", "contact_id": c.ID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, c.ID, decode(t, w)["note"].(map[string]any)["contact_id"])

	other := e.contact(t, e.b.ID, "Mallory")
	w = send(e.asA, http.MethodPost, "/notes", map[string]any{"content": "x", "date": "2026-03-01T12:00:00Z", "contact_id": other.ID})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	e := newEnv(t)
	for name, body := range map[string]map[string]any{
		"no content":     {"date": "2026-03-01T12:00:00Z"},
		"empty content":  {"content": "", "date": "2026-03-01T12:00:00Z"},
		"too long":       {"content": strings.Repeat("x", 5001), "date": "2026-03-01T12:00:00Z"},
		"no date":        {"content": "x"},
		"contact_id 0":   {"content": "x", "date": "2026-03-01T12:00:00Z", "contact_id": 0},
		"contact_id < 0": {"content": "x", "date": "2026-03-01T12:00:00Z", "contact_id": -1},
	} {
		w := send(e.asA, http.MethodPost, "/notes", body)
		assert.Equal(t, http.StatusBadRequest, w.Code, name+": "+w.Body.String())
	}
}

func TestGetOwnNoteOnly(t *testing.T) {
	e := newEnv(t)
	n := e.note(t, e.a.ID, "mine", nil)

	w := send(e.asA, http.MethodGet, "/notes/"+id(n), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "mine", decode(t, w)["content"])

	w = send(e.asB, http.MethodGet, "/notes/"+id(n), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, id(n), decode(t, w)["error"].(map[string]any)["details"].(map[string]any)["id"])
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodGet, "/notes/9999", nil).Code)
	assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodGet, "/notes/abc", nil).Code)
}

func TestListShowsTheActorsUnassignedNotesNewestFirstWithPaging(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	e.note(t, e.a.ID, "linked", &c)
	e.note(t, e.b.ID, "foreign", nil)
	old := e.note(t, e.a.ID, "old", nil)
	require.NoError(t, e.db.Model(&old).Update("date", time.Now().AddDate(-1, 0, 0)).Error)
	e.note(t, e.a.ID, "new", nil)

	body := decode(t, send(e.asA, http.MethodGet, "/notes", nil))
	assert.Equal(t, []string{"new", "old"}, contents(body))
	assert.EqualValues(t, 2, body["total"])
	assert.EqualValues(t, 1, body["page"])
	assert.EqualValues(t, 25, body["limit"])

	body = decode(t, send(e.asA, http.MethodGet, "/notes?limit=1&page=2", nil))
	assert.Equal(t, []string{"old"}, contents(body))
	assert.EqualValues(t, 2, body["total"])

	body = decode(t, send(e.asA, http.MethodGet, "/notes?limit=1000&page=x", nil))
	assert.EqualValues(t, 100, body["limit"])
	assert.EqualValues(t, 1, body["page"])
}

// @005-US4
func TestListSearchesAndFiltersByDate(t *testing.T) {
	e := newEnv(t)
	for content, date := range map[string]string{"Call Alice": "2026-03-10", "Call Bob": "2026-04-10"} {
		d, _ := time.Parse("2006-01-02", date)
		require.NoError(t, e.db.Create(&models.Note{UserID: e.a.ID, Content: content, Date: d}).Error)
	}
	list := func(q string) []string { return contents(decode(t, send(e.asA, http.MethodGet, "/notes?"+q, nil))) }

	assert.Equal(t, []string{"Call Alice"}, list("search=alice"))
	assert.Equal(t, []string{"Call Alice"}, list("search=%20ALICE%20"))
	assert.Equal(t, []string{"Call Bob"}, list("fromDate=2026-04-01"))
	assert.Equal(t, []string{"Call Alice"}, list("toDate=2026-03-10"), "the end date is inclusive")
	assert.Len(t, list("fromDate=garbage"), 2, "an unparseable date is ignored")
	assert.Empty(t, list("search=nobody"))
}

func TestContactNotesAreThatContactsNotesOfTheActor(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.contact(t, e.a.ID, "Alice"), e.contact(t, e.a.ID, "Bob")
	e.note(t, e.a.ID, "about alice", &alice)
	e.note(t, e.a.ID, "about bob", &bob)
	e.note(t, e.b.ID, "spy note", &alice)

	w := send(e.asA, http.MethodGet, "/contacts/"+cid(alice)+"/notes", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{"about alice"}, contents(decode(t, w)))

	empty := e.contact(t, e.a.ID, "Empty")
	assert.Equal(t, []any{}, decode(t, send(e.asA, http.MethodGet, "/contacts/"+cid(empty)+"/notes", nil))["notes"])
	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodGet, "/contacts/"+cid(alice)+"/notes", nil).Code)
}

func TestUpdateReplacesContentAndDateAndKeepsTheLinkWhenOmitted(t *testing.T) {
	e := newEnv(t)
	e.subscribe(t, e.a.ID, "note.updated")
	c := e.contact(t, e.a.ID, "Alice")
	n := e.note(t, e.a.ID, "old", &c)

	w := send(e.asA, http.MethodPut, "/notes/"+id(n), map[string]any{"content": "new", "date": "2026-05-01T08:00:00Z"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Note updated successfully", body["message"])
	assert.Equal(t, "new", body["note"].(map[string]any)["content"])
	assert.EqualValues(t, c.ID, body["note"].(map[string]any)["contact_id"])

	var stored models.Note
	require.NoError(t, e.db.First(&stored, n.ID).Error)
	assert.Equal(t, "new", stored.Content)
	assert.Equal(t, 2026, stored.Date.Year())
	assert.True(t, stored.UpdatedAt.After(n.UpdatedAt), "updated_at moves")
	require.NotNil(t, stored.ContactID)
	assert.Eventually(t, func() bool { return e.deliveries("note.updated") == 1 }, 2*time.Second, 10*time.Millisecond)
}

func TestUpdateCanMoveTheNoteToAnotherOwnContactOnly(t *testing.T) {
	e := newEnv(t)
	a, b := e.contact(t, e.a.ID, "Alice"), e.contact(t, e.a.ID, "Bob")
	foreign := e.contact(t, e.b.ID, "Mallory")
	n := e.note(t, e.a.ID, "x", &a)

	require.Equal(t, http.StatusOK, send(e.asA, http.MethodPut, "/notes/"+id(n), map[string]any{"content": "x", "date": "2026-05-01T08:00:00Z", "contact_id": b.ID}).Code)
	var stored models.Note
	require.NoError(t, e.db.First(&stored, n.ID).Error)
	assert.Equal(t, b.ID, *stored.ContactID)

	w := send(e.asA, http.MethodPut, "/notes/"+id(n), map[string]any{"content": "x", "date": "2026-05-01T08:00:00Z", "contact_id": foreign.ID})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestUpdateRefusesOtherUsersNotesAndBadInput(t *testing.T) {
	e := newEnv(t)
	n := e.note(t, e.a.ID, "mine", nil)

	w := send(e.asB, http.MethodPut, "/notes/"+id(n), map[string]any{"content": "hacked", "date": "2026-05-01T08:00:00Z"})
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPut, "/notes/"+id(n), map[string]any{"content": ""}).Code)
	var stored models.Note
	require.NoError(t, e.db.First(&stored, n.ID).Error)
	assert.Equal(t, "mine", stored.Content)
}

func TestDeleteSoftDeletesAndAnnounces(t *testing.T) {
	e := newEnv(t)
	e.subscribe(t, e.a.ID, "note.deleted")
	n := e.note(t, e.a.ID, "bye", nil)

	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodDelete, "/notes/"+id(n), nil).Code)
	var live int64
	e.db.Model(&models.Note{}).Where("id = ?", n.ID).Count(&live)
	require.EqualValues(t, 1, live, "another user cannot delete it")

	w := send(e.asA, http.MethodDelete, "/notes/"+id(n), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Note deleted"}`, w.Body.String())
	e.db.Model(&models.Note{}).Where("id = ?", n.ID).Count(&live)
	assert.Zero(t, live)
	assert.Eventually(t, func() bool { return e.deliveries("note.deleted") == 1 }, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodDelete, "/notes/"+id(n), nil).Code)
}

func TestWithoutAnAuthenticatedUserEverythingRefuses(t *testing.T) {
	e := newEnv(t)
	n := e.note(t, e.a.ID, "mine", nil)
	body := map[string]any{"content": "x", "date": "2026-05-01T08:00:00Z"}
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/notes"}, {http.MethodGet, "/notes/" + id(n)}, {http.MethodPost, "/notes"},
		{http.MethodPut, "/notes/" + id(n)}, {http.MethodDelete, "/notes/" + id(n)},
		{http.MethodGet, "/contacts/1/notes"}, {http.MethodPost, "/contacts/1/notes"},
	} {
		assert.Equal(t, http.StatusUnauthorized, send(e.anon, call.method, call.path, body).Code, call.method+" "+call.path)
	}
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

func imageIDs(body map[string]any) []float64 {
	out := []float64{}
	for _, i := range body["images"].([]any) {
		out = append(out, i.(map[string]any)["id"].(float64))
	}
	return out
}

func (e *env) has(img models.Image) bool {
	_, err := os.Stat(filepath.Join(e.dir, img.File))
	return err == nil
}

// @005-US1
func TestANoteCarriesItsImagesInOrderEverywhereItIsListed(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Ann")
	one, two := e.image(t, e.a.ID, "1"), e.image(t, e.a.ID, "2")

	w := send(e.asA, http.MethodPost, "/contacts/"+cid(c)+"/notes", map[string]any{
		"content": "with pictures", "date": "2026-03-01T12:00:00Z", "image_ids": []uint{two.ID, one.ID},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	created := decode(t, w)["note"].(map[string]any)
	assert.Equal(t, []float64{float64(two.ID), float64(one.ID)}, imageIDs(created))
	first := created["images"].([]any)[0].(map[string]any)
	assert.Equal(t, float64(30), first["width"])
	assert.Contains(t, first["thumb_url"], "/api/v1/images/")

	var n models.Note
	require.NoError(t, e.db.Last(&n).Error)
	assert.Equal(t, []float64{float64(two.ID), float64(one.ID)}, imageIDs(decode(t, send(e.asA, http.MethodGet, "/notes/"+id(n), nil))))
	listed := decode(t, send(e.asA, http.MethodGet, "/contacts/"+cid(c)+"/notes", nil))["notes"].([]any)
	assert.Equal(t, []float64{float64(two.ID), float64(one.ID)}, imageIDs(listed[0].(map[string]any)))
}

func TestAFreeFloatingNoteListsItsImages(t *testing.T) {
	e := newEnv(t)
	img := e.image(t, e.a.ID, "1")
	w := send(e.asA, http.MethodPost, "/notes", map[string]any{"content": "", "date": "2026-03-01T12:00:00Z", "image_ids": []uint{img.ID}})
	require.Equal(t, http.StatusOK, w.Code, "an image is enough: "+w.Body.String())
	plain := e.note(t, e.a.ID, "plain", nil)
	_ = plain

	listed := decode(t, send(e.asA, http.MethodGet, "/notes", nil))["notes"].([]any)
	require.Len(t, listed, 2)
	byContent := map[string][]float64{}
	for _, n := range listed {
		m := n.(map[string]any)
		byContent[m["content"].(string)] = imageIDs(m)
	}
	assert.Equal(t, []float64{float64(img.ID)}, byContent[""])
	assert.Equal(t, []float64{}, byContent["plain"])
}

func TestANoteNeedsTextOrAnImage(t *testing.T) {
	e := newEnv(t)
	w := send(e.asA, http.MethodPost, "/notes", map[string]any{"content": "", "date": "2026-03-01T12:00:00Z"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "A note needs text or an image")

	n := e.note(t, e.a.ID, "text", nil)
	img := e.image(t, e.a.ID, "1")
	require.Equal(t, http.StatusOK, send(e.asA, http.MethodPut, "/notes/"+id(n), map[string]any{
		"content": "text", "date": "2026-03-01T12:00:00Z", "image_ids": []uint{img.ID},
	}).Code)
	w = send(e.asA, http.MethodPut, "/notes/"+id(n), map[string]any{"content": "", "date": "2026-03-01T12:00:00Z"})
	assert.Equal(t, http.StatusOK, w.Code, "the image stays, so the note may lose its text: "+w.Body.String())
	w = send(e.asA, http.MethodPut, "/notes/"+id(n), map[string]any{"content": "", "date": "2026-03-01T12:00:00Z", "image_ids": []uint{}})
	assert.Equal(t, http.StatusBadRequest, w.Code, "removing the last image of a note without text")
}

func TestUpdateKeepsImagesWhenTheFieldIsLeftOutAndRemovesThemWithAnEmptyList(t *testing.T) {
	e := newEnv(t)
	img := e.image(t, e.a.ID, "1")
	w := send(e.asA, http.MethodPost, "/notes", map[string]any{"content": "x", "date": "2026-03-01T12:00:00Z", "image_ids": []uint{img.ID}})
	require.Equal(t, http.StatusOK, w.Code)
	var n models.Note
	require.NoError(t, e.db.Last(&n).Error)

	w = send(e.asA, http.MethodPut, "/notes/"+id(n), map[string]any{"content": "edited", "date": "2026-03-01T12:00:00Z"})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []float64{float64(img.ID)}, imageIDs(decode(t, w)["note"].(map[string]any)))
	assert.True(t, e.has(img))

	w = send(e.asA, http.MethodPut, "/notes/"+id(n), map[string]any{"content": "edited", "date": "2026-03-01T12:00:00Z", "image_ids": []uint{}})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []float64{}, imageIDs(decode(t, w)["note"].(map[string]any)))
	var rows int64
	e.db.Model(&models.Image{}).Count(&rows)
	assert.Zero(t, rows)
	assert.False(t, e.has(img), "the file goes with the row")
}

// @005-US1
func TestDeletingANoteDeletesItsImagesAndFiles(t *testing.T) {
	e := newEnv(t)
	img := e.image(t, e.a.ID, "1")
	require.Equal(t, http.StatusOK, send(e.asA, http.MethodPost, "/notes", map[string]any{"content": "x", "date": "2026-03-01T12:00:00Z", "image_ids": []uint{img.ID}}).Code)
	var n models.Note
	require.NoError(t, e.db.Last(&n).Error)

	require.Equal(t, http.StatusOK, send(e.asA, http.MethodDelete, "/notes/"+id(n), nil).Code)
	var rows int64
	e.db.Model(&models.Image{}).Count(&rows)
	assert.Zero(t, rows)
	assert.False(t, e.has(img))
}

func TestImagesOfOtherUsersOrOtherNotesCannotBeAttached(t *testing.T) {
	e := newEnv(t)
	theirs := e.image(t, e.b.ID, "theirs")
	mine := e.image(t, e.a.ID, "mine")
	body := func(ids ...uint) map[string]any {
		return map[string]any{"content": "x", "date": "2026-03-01T12:00:00Z", "image_ids": ids}
	}

	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodPost, "/notes", body(theirs.ID)).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodPost, "/notes", body(99999)).Code)
	require.Equal(t, http.StatusOK, send(e.asA, http.MethodPost, "/notes", body(mine.ID)).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodPost, "/notes", body(mine.ID)).Code, "an image already on a note")

	var attached int64
	e.db.Model(&models.Image{}).Where("id = ? AND note_id IS NOT NULL", theirs.ID).Count(&attached)
	assert.Zero(t, attached)
	var notes int64
	e.db.Model(&models.Note{}).Count(&notes)
	assert.EqualValues(t, 1, notes, "a refused request creates no note")

	var many []uint
	for i := range 21 {
		many = append(many, e.image(t, e.a.ID, "m"+strconv.Itoa(i)).ID)
	}
	assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPost, "/notes", body(many...)).Code)
}
