package relationships_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/relationships"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var day = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

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
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Relationship{}, &models.Activity{}))
	e := &env{db: db}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)

	mount := func(userID uint) *gin.Engine {
		r := gin.New()
		r.Use(middleware.ClockMiddleware(clock.Real()))
		authenticated := r.Group("")
		if userID != 0 {
			authenticated.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
		}
		relationships.New(relationships.Deps{DB: db, Config: &config.Config{}}).Register(api.New(r.Group(""), r), authenticated)
		return r
	}
	e.asA, e.asB, e.anon = mount(e.a.ID), mount(e.b.ID), mount(0)
	return e
}

func (e *env) contact(t *testing.T, owner uint, first string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: owner, Firstname: first, Lastname: "L", Gender: "female", Birthday: "1990-01-02", Email: "private@example.com"}
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

func (e *env) link(t *testing.T, owner uint, from models.Contact, to *models.Contact, name string) models.Relationship {
	t.Helper()
	r := models.Relationship{UserID: owner, ContactID: from.ID, Name: name, Type: "Friend"}
	if to != nil {
		r.RelatedContactID = &to.ID
	}
	require.NoError(t, e.db.Create(&r).Error)
	return r
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

func keys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func names(body map[string]any, list string) []string {
	var out []string
	for _, r := range body[list].([]any) {
		out = append(out, r.(map[string]any)["name"].(string))
	}
	return out
}

func input(extra map[string]any) map[string]any {
	m := map[string]any{"name": "Bob", "type": "Friend"}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestOutgoingRelationshipsShowTheRelatedContactsNameOnly(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.contact(t, e.a.ID, "Alice"), e.contact(t, e.a.ID, "Bob")
	e.link(t, e.a.ID, alice, &bob, "Bob")
	e.link(t, e.a.ID, alice, nil, "Cousin Carl")
	e.link(t, e.a.ID, bob, &alice, "Alice")

	w := send(e.asA, http.MethodGet, "/contacts/"+sid(alice.ID)+"/relationships", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.ElementsMatch(t, []string{"Bob", "Cousin Carl"}, names(body, "relationships"))
	for _, r := range body["relationships"].([]any) {
		rel := r.(map[string]any)
		if rel["name"] == "Bob" {
			assert.ElementsMatch(t, []string{"ID", "firstname", "lastname", "gender", "birthday"}, keys(rel["related_contact"].(map[string]any)))
		} else {
			assert.NotContains(t, rel, "related_contact")
		}
	}

	empty := e.contact(t, e.a.ID, "Empty")
	assert.Equal(t, []any{}, decode(t, send(e.asA, http.MethodGet, "/contacts/"+sid(empty.ID)+"/relationships", nil))["relationships"])
	w = send(e.asB, http.MethodGet, "/contacts/"+sid(alice.ID)+"/relationships", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, sid(alice.ID), decode(t, w)["error"].(map[string]any)["details"].(map[string]any)["id"])
}

func TestIncomingRelationshipsShowTheSourceContactsName(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.contact(t, e.a.ID, "Alice"), e.contact(t, e.a.ID, "Bob")
	e.link(t, e.a.ID, bob, &alice, "Alice")
	mallory := e.contact(t, e.b.ID, "Mallory")
	e.link(t, e.b.ID, mallory, &alice, "Alice")

	w := send(e.asA, http.MethodGet, "/contacts/"+sid(alice.ID)+"/incoming-relationships", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	list := decode(t, w)["incoming_relationships"].([]any)
	require.Len(t, list, 1)
	source := list[0].(map[string]any)["source_contact"].(map[string]any)
	assert.Equal(t, "Bob", source["firstname"])
	assert.ElementsMatch(t, []string{"ID", "firstname", "lastname"}, keys(source))
	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodGet, "/contacts/"+sid(alice.ID)+"/incoming-relationships", nil).Code)
}

// @007-US1
func TestCreateStoresTheRelationshipForTheContactAndActor(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.contact(t, e.a.ID, "Alice"), e.contact(t, e.a.ID, "Bob")

	w := send(e.asA, http.MethodPost, "/contacts/"+sid(alice.ID)+"/relationships", input(map[string]any{
		"gender": "male", "birthday": "--03-04", "related_contact_id": bob.ID,
	}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	rel := decode(t, w)["relationship"].(map[string]any)
	assert.Equal(t, "Bob", rel["name"])
	assert.EqualValues(t, alice.ID, rel["contact_id"])
	assert.EqualValues(t, bob.ID, rel["related_contact_id"])
	for _, key := range []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt", "type", "gender", "birthday"} {
		assert.Contains(t, rel, key)
	}

	var stored models.Relationship
	require.NoError(t, e.db.First(&stored, uint(rel["ID"].(float64))).Error)
	assert.Equal(t, e.a.ID, stored.UserID)
}

func TestCreateRefusesOtherUsersContactsRelatedContactsAndBadInput(t *testing.T) {
	e := newEnv(t)
	alice := e.contact(t, e.a.ID, "Alice")
	foreign := e.contact(t, e.b.ID, "Mallory")
	path := "/contacts/" + sid(alice.ID) + "/relationships"

	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodPost, path, input(nil)).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodPost, "/contacts/9999/relationships", input(nil)).Code)
	w := send(e.asA, http.MethodPost, path, input(map[string]any{"related_contact_id": foreign.ID}))
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.Equal(t, sid(foreign.ID), decode(t, w)["error"].(map[string]any)["details"].(map[string]any)["id"])
	for name, extra := range map[string]map[string]any{
		"no name": {"name": ""}, "long name": {"name": strings.Repeat("x", 101)}, "no type": {"type": ""},
		"bad gender": {"gender": "robot"}, "bad birthday": {"birthday": "31-02-1990"},
	} {
		assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPost, path, input(extra)).Code, name)
	}
	var n int64
	e.db.Model(&models.Relationship{}).Count(&n)
	assert.Zero(t, n)
}

func TestUpdateReplacesTheFieldsAndKeepsTheContact(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.contact(t, e.a.ID, "Alice"), e.contact(t, e.a.ID, "Bob")
	rel := e.link(t, e.a.ID, alice, &bob, "old")

	w := send(e.asA, http.MethodPut, "/contacts/"+sid(alice.ID)+"/relationships/"+sid(rel.ID), input(map[string]any{"name": "new", "type": "Sibling"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "new", body["name"])
	assert.Nil(t, body["related_contact_id"], "left out means unlinked")

	var stored models.Relationship
	require.NoError(t, e.db.First(&stored, rel.ID).Error)
	assert.Equal(t, "new", stored.Name)
	assert.Equal(t, "Sibling", stored.Type)
	assert.Nil(t, stored.RelatedContactID)
	assert.Equal(t, alice.ID, stored.ContactID)
}

func TestUpdateRefusesOtherUsersRelationshipsRelatedContactsAndBadInput(t *testing.T) {
	e := newEnv(t)
	alice := e.contact(t, e.a.ID, "Alice")
	foreign := e.contact(t, e.b.ID, "Mallory")
	rel := e.link(t, e.a.ID, alice, nil, "mine")
	path := "/contacts/" + sid(alice.ID) + "/relationships/" + sid(rel.ID)

	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodPut, path, input(map[string]any{"name": "hacked"})).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodPut, path, input(map[string]any{"related_contact_id": foreign.ID})).Code)
	assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPut, path, input(map[string]any{"name": ""})).Code)
	var stored models.Relationship
	require.NoError(t, e.db.First(&stored, rel.ID).Error)
	assert.Equal(t, "mine", stored.Name)
}

func TestDeleteRemovesOwnRelationshipOnly(t *testing.T) {
	e := newEnv(t)
	alice := e.contact(t, e.a.ID, "Alice")
	rel := e.link(t, e.a.ID, alice, nil, "mine")
	path := "/contacts/" + sid(alice.ID) + "/relationships/" + sid(rel.ID)

	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodDelete, path, nil).Code)
	w := send(e.asA, http.MethodDelete, path, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Relationship deleted"}`, w.Body.String())
	var live int64
	e.db.Model(&models.Relationship{}).Where("id = ?", rel.ID).Count(&live)
	assert.Zero(t, live)
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodDelete, path, nil).Code)
}

// @019-US1
func TestGraphDrawsEdgesOnlyBetweenVisibleNodes(t *testing.T) {
	e := newEnv(t)
	ann := models.Contact{UserID: e.a.ID, Firstname: "Ann", Circles: []string{"Friends"}, PhotoThumbnailFile: "p_thumb.jpg"}
	bob := models.Contact{UserID: e.a.ID, Firstname: "Bob", Lastname: "B"}
	old := models.Contact{UserID: e.a.ID, Firstname: "Old", Archived: true}
	nameless := models.Contact{UserID: e.a.ID}
	zed := models.Contact{UserID: e.b.ID, Firstname: "Zed"}
	for _, c := range []*models.Contact{&ann, &bob, &old, &nameless, &zed} {
		require.NoError(t, e.db.Create(c).Error)
	}
	linked := e.link(t, e.a.ID, ann, &bob, "n")
	e.link(t, e.a.ID, ann, &old, "n")
	e.link(t, e.a.ID, ann, nil, "unlinked")
	dinner := models.Activity{UserID: e.a.ID, Title: "Dinner", Date: day, Contacts: []models.Contact{ann, bob}}
	hike := models.Activity{UserID: e.a.ID, Title: "Hike", Date: day, Contacts: []models.Contact{ann, old}}
	theirs := models.Activity{UserID: e.b.ID, Title: "Secret", Date: day, Contacts: []models.Contact{zed}}
	for _, a := range []*models.Activity{&dinner, &hike, &theirs} {
		require.NoError(t, e.db.Create(a).Error)
	}

	w := send(e.asA, http.MethodGet, "/graph", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)

	nodes := map[string]map[string]any{}
	for _, n := range body["nodes"].([]any) {
		node := n.(map[string]any)
		nodes[node["id"].(string)] = node
	}
	id := func(prefix string, n uint) string { return prefix + "-" + sid(n) }
	assert.Contains(t, nodes, id("c", ann.ID))
	assert.Contains(t, nodes, id("c", bob.ID))
	assert.NotContains(t, nodes, id("c", old.ID), "archived contacts are not drawn")
	assert.NotContains(t, nodes, id("c", zed.ID), "other users' contacts never appear")
	assert.Equal(t, "Unknown", nodes[id("c", nameless.ID)]["label"])
	assert.Equal(t, "Bob B", nodes[id("c", bob.ID)]["label"])
	assert.Contains(t, nodes[id("c", ann.ID)]["photo_thumbnail"], "thumbnail=true&v=p")
	assert.Equal(t, []any{"Friends"}, nodes[id("c", ann.ID)]["circles"])
	assert.NotContains(t, nodes[id("c", bob.ID)], "photo_thumbnail")
	assert.Contains(t, nodes, id("a", dinner.ID))
	assert.NotContains(t, nodes, id("a", hike.ID), "one visible participant is not enough for an activity node")
	assert.NotContains(t, nodes, id("a", theirs.ID))

	var edgeIDs []string
	for _, e := range body["edges"].([]any) {
		edge := e.(map[string]any)
		assert.Contains(t, nodes, edge["source"], "edge %v has no source node", edge["id"])
		assert.Contains(t, nodes, edge["target"], "edge %v has no target node", edge["id"])
		edgeIDs = append(edgeIDs, edge["id"].(string))
	}
	assert.ElementsMatch(t, []string{
		"r-" + sid(linked.ID), "ae-" + sid(dinner.ID) + "-" + sid(ann.ID), "ae-" + sid(dinner.ID) + "-" + sid(bob.ID),
	}, edgeIDs)
}

func TestGraphWithNothingIsEmptyLists(t *testing.T) {
	e := newEnv(t)
	body := decode(t, send(e.asA, http.MethodGet, "/graph", nil))
	assert.Equal(t, []any{}, body["nodes"])
	assert.Equal(t, []any{}, body["edges"])
}

func TestWithoutAnAuthenticatedUserEverythingRefuses(t *testing.T) {
	e := newEnv(t)
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/contacts/1/relationships"}, {http.MethodGet, "/contacts/1/incoming-relationships"},
		{http.MethodPost, "/contacts/1/relationships"}, {http.MethodPut, "/contacts/1/relationships/1"},
		{http.MethodDelete, "/contacts/1/relationships/1"}, {http.MethodGet, "/graph"},
	} {
		assert.Equal(t, http.StatusUnauthorized, send(e.anon, call.method, call.path, input(nil)).Code, call.method+" "+call.path)
	}
}
