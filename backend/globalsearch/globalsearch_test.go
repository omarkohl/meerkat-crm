package globalsearch_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/database"
	"meerkat/globalsearch"
	"meerkat/middleware"
	"meerkat/models"

	"github.com/gin-gonic/gin"
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
	db, err := database.InitDB(filepath.Join(t.TempDir(), "app.db"))
	require.NoError(t, err)
	e := &env{db: db}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)
	mount := func(userID uint) *gin.Engine {
		r := gin.New()
		r.Use(middleware.ClockMiddleware(clock.NewFake(now)))
		authenticated := r.Group("")
		if userID != 0 {
			authenticated.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
		}
		globalsearch.New(globalsearch.Deps{DB: db}).Register(api.New(r.Group(""), r), authenticated)
		return r
	}
	e.asA, e.asB, e.anon = mount(e.a.ID), mount(e.b.ID), mount(0)
	return e
}

func (e *env) contact(t *testing.T, owner uint, c models.Contact) models.Contact {
	t.Helper()
	c.UserID = owner
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

func (e *env) note(t *testing.T, owner uint, contact *models.Contact, content string, daysAgo int) models.Note {
	t.Helper()
	n := models.Note{UserID: owner, Content: content, Date: now.AddDate(0, 0, -daysAgo)}
	if contact != nil {
		n.ContactID = &contact.ID
	}
	require.NoError(t, e.db.Create(&n).Error)
	return n
}

func get(r http.Handler, query string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/search?"+query, nil))
	return w
}

func q(term string, extra ...string) string {
	v := url.Values{"q": {term}}
	for i := 0; i+1 < len(extra); i += 2 {
		v.Set(extra[i], extra[i+1])
	}
	return v.Encode()
}

type segment struct {
	Text  string `json:"text"`
	Match bool   `json:"match"`
}

type result struct {
	Query    string `json:"query"`
	Contacts *struct {
		Total int `json:"total"`
		Items []struct {
			ID           uint      `json:"id"`
			Name         []segment `json:"name"`
			MatchedField string    `json:"matched_field"`
			Snippet      []segment `json:"snippet"`
		} `json:"items"`
	} `json:"contacts"`
	Notes *struct {
		Total int `json:"total"`
		Items []struct {
			ID          uint      `json:"id"`
			ContactID   *uint     `json:"contact_id"`
			ContactName string    `json:"contact_name"`
			Snippet     []segment `json:"snippet"`
		} `json:"items"`
	} `json:"notes"`
	Activities *struct {
		Total int `json:"total"`
		Items []struct {
			ID      uint      `json:"id"`
			Title   []segment `json:"title"`
			Snippet []segment `json:"snippet"`
		} `json:"items"`
	} `json:"activities"`
}

func search(t *testing.T, r http.Handler, query string) result {
	t.Helper()
	w := get(r, query)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res result
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res), w.Body.String())
	return res
}

func marked(segs []segment) []string {
	var out []string
	for _, s := range segs {
		if s.Match {
			out = append(out, s.Text)
		}
	}
	return out
}

func text(segs []segment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return b.String()
}

// @022-US1
func TestEveryGroupAnswersWithItsTotalAndTheTopResults(t *testing.T) {
	e := newEnv(t)
	ada := e.contact(t, e.a.ID, models.Contact{Firstname: "Ada", Lastname: "Lovelace"})
	e.note(t, e.a.ID, &ada, "Ada wrote the first program", 3)
	require.NoError(t, e.db.Create(&models.Activity{UserID: e.a.ID, Title: "Dinner with Ada", Date: now}).Error)

	res := search(t, e.asA, q("ada"))

	assert.Equal(t, "ada", res.Query)
	require.NotNil(t, res.Contacts)
	assert.Equal(t, 1, res.Contacts.Total)
	assert.Equal(t, "name", res.Contacts.Items[0].MatchedField)
	assert.Equal(t, []string{"Ada"}, marked(res.Contacts.Items[0].Name))
	assert.Equal(t, "Ada Lovelace", text(res.Contacts.Items[0].Name))
	require.NotNil(t, res.Notes)
	assert.Equal(t, 1, res.Notes.Total)
	assert.Equal(t, "Ada Lovelace", res.Notes.Items[0].ContactName)
	assert.Equal(t, []string{"Ada"}, marked(res.Notes.Items[0].Snippet))
	require.NotNil(t, res.Activities)
	assert.Equal(t, []string{"Ada"}, marked(res.Activities.Items[0].Title))
}

// @022-US1
func TestAContactMatchedByNameComesBeforeOneMatchedByEmail(t *testing.T) {
	e := newEnv(t)
	byEmail := e.contact(t, e.a.ID, models.Contact{Firstname: "Grace", Lastname: "Hopper", Email: "ada@navy.example"})
	byName := e.contact(t, e.a.ID, models.Contact{Firstname: "Ada", Lastname: "Lovelace"})

	res := search(t, e.asA, q("ada"))

	require.Len(t, res.Contacts.Items, 2)
	assert.Equal(t, byName.ID, res.Contacts.Items[0].ID)
	second := res.Contacts.Items[1]
	assert.Equal(t, byEmail.ID, second.ID)
	assert.Equal(t, "email", second.MatchedField)
	assert.Equal(t, []string{"ada"}, marked(second.Snippet))
	assert.Equal(t, "ada@navy.example", text(second.Snippet))
}

// @022-US1
func TestNotesWhereTheWordIsFrequentComeFirst(t *testing.T) {
	e := newEnv(t)
	faint := e.note(t, e.a.ID, nil, "We talked about chess once and then about many other things at length today", 1)
	strong := e.note(t, e.a.ID, nil, "chess chess chess", 9)

	res := search(t, e.asA, q("chess"))

	require.Len(t, res.Notes.Items, 2)
	assert.Equal(t, strong.ID, res.Notes.Items[0].ID, "the better match first, though it is older")
	assert.Equal(t, faint.ID, res.Notes.Items[1].ID)
}

// @022-US1
func TestAWordIsFoundByItsStartIgnoringCaseAndAccents(t *testing.T) {
	e := newEnv(t)
	e.contact(t, e.a.ID, models.Contact{Firstname: "Zoë", Lastname: "Müllerová"})

	res := search(t, e.asA, q("ZOE mulle"))

	require.Len(t, res.Contacts.Items, 1)
	assert.Equal(t, []string{"Zoë", "Mülle"}, marked(res.Contacts.Items[0].Name))
}

// @022-US1
func TestPhoneDigitsAndCJKTextAreFoundWithoutTheIndex(t *testing.T) {
	e := newEnv(t)
	phone := e.contact(t, e.a.ID, models.Contact{Firstname: "Pia", Phone: "+41 79 5550101"})
	kyoto := e.note(t, e.a.ID, nil, "友達と京都へ行った", 1)

	byPhone := search(t, e.asA, q("795550"))
	byText := search(t, e.asA, q("京都"))

	require.Len(t, byPhone.Contacts.Items, 1)
	assert.Equal(t, phone.ID, byPhone.Contacts.Items[0].ID)
	assert.Equal(t, "phone", byPhone.Contacts.Items[0].MatchedField)
	assert.Equal(t, []string{"79 5550"}, marked(byPhone.Contacts.Items[0].Snippet))
	require.Len(t, byText.Notes.Items, 1)
	assert.Equal(t, kyoto.ID, byText.Notes.Items[0].ID)
	assert.Equal(t, []string{"京都"}, marked(byText.Notes.Items[0].Snippet))
}

// @022-US1
func TestLikeWildcardsInTheTermAreOrdinaryCharacters(t *testing.T) {
	e := newEnv(t)
	e.note(t, e.a.ID, nil, "plain words only", 1)
	literal := e.note(t, e.a.ID, nil, "100% sure", 2)

	res := search(t, e.asA, q("%"))

	require.Len(t, res.Notes.Items, 1)
	assert.Equal(t, literal.ID, res.Notes.Items[0].ID)
}

// @022-US1
func TestTrashedContactsAndTheirNotesAreNeverFound(t *testing.T) {
	e := newEnv(t)
	gone := e.contact(t, e.a.ID, models.Contact{Firstname: "Vanished", Lastname: "Person"})
	e.note(t, e.a.ID, &gone, "vanished note", 1)
	orphan := e.note(t, e.a.ID, nil, "vanished but unassigned", 2)
	require.NoError(t, e.db.Transaction(func(tx *gorm.DB) error { return models.TrashContact(tx, e.a.ID, gone.ID) }))

	res := search(t, e.asA, q("vanished"))

	assert.Zero(t, res.Contacts.Total)
	assert.Equal(t, 1, res.Notes.Total)
	assert.Equal(t, orphan.ID, res.Notes.Items[0].ID)
	assert.Nil(t, res.Notes.Items[0].ContactID)
}

// @022-US1
func TestOneUserNeverFindsAnotherUsersData(t *testing.T) {
	e := newEnv(t)
	theirs := e.contact(t, e.b.ID, models.Contact{Firstname: "Secret", Lastname: "Agent"})
	e.note(t, e.b.ID, &theirs, "secret meeting", 1)
	require.NoError(t, e.db.Create(&models.Activity{UserID: e.b.ID, Title: "Secret dinner", Date: now}).Error)
	e.contact(t, e.a.ID, models.Contact{Firstname: "Open", Lastname: "Book"})

	for _, term := range []string{"secret", "agent", "meeting"} {
		res := search(t, e.asA, q(term))
		assert.Zero(t, res.Contacts.Total+res.Notes.Total+res.Activities.Total, term)
	}
	assert.Equal(t, 1, search(t, e.asB, q("secret")).Contacts.Total)
}

// @022-US1
func TestATypeAsksForOneGroupAndPagesThroughIt(t *testing.T) {
	e := newEnv(t)
	for i := range 3 {
		e.note(t, e.a.ID, nil, "pagetest note", i)
	}
	e.contact(t, e.a.ID, models.Contact{Firstname: "Pagetest"})

	first := search(t, e.asA, q("pagetest", "type", "notes", "limit", "2"))
	second := search(t, e.asA, q("pagetest", "type", "notes", "limit", "2", "offset", "2"))

	assert.Nil(t, first.Contacts)
	assert.Nil(t, first.Activities)
	assert.Equal(t, 3, first.Notes.Total)
	assert.Len(t, first.Notes.Items, 2)
	assert.Equal(t, 3, second.Notes.Total)
	require.Len(t, second.Notes.Items, 1)
	assert.NotEqual(t, first.Notes.Items[0].ID, second.Notes.Items[0].ID)
	assert.NotEqual(t, first.Notes.Items[1].ID, second.Notes.Items[0].ID)
}

// @022-US1
func TestALongNoteIsCutAroundTheMatch(t *testing.T) {
	e := newEnv(t)
	e.note(t, e.a.ID, nil, strings.Repeat("filler words ", 40)+"unicorn sighting"+strings.Repeat(" more filler", 40), 1)

	res := search(t, e.asA, q("unicorn"))

	snippet := res.Notes.Items[0].Snippet
	assert.Equal(t, []string{"unicorn"}, marked(snippet))
	assert.Less(t, len([]rune(text(snippet))), 200)
	assert.True(t, strings.HasPrefix(text(snippet), "…"))
}

// @022-US1
func TestARequestThatCannotBeAnsweredIsRefused(t *testing.T) {
	e := newEnv(t)
	for _, query := range []string{"", "q=+", q("x", "type", "people"), q("x", "limit", "0"), q("x", "limit", "51"), q("x", "limit", "many"), q("x", "offset", "-1"), q("x", "offset", "5"), q(strings.Repeat("x", 201))} {
		w := get(e.asA, query)
		assert.Equal(t, http.StatusBadRequest, w.Code, query)
	}
	assert.Equal(t, http.StatusUnauthorized, get(e.anon, q("x")).Code)
}

// @022-US1
func TestAFullNameIsFoundWithoutTheIndexWhateverTheNickname(t *testing.T) {
	e := newEnv(t)
	plain := e.contact(t, e.a.ID, models.Contact{Firstname: "山田", Lastname: "太郎"})
	nick := e.contact(t, e.a.ID, models.Contact{Firstname: "花子", Nickname: "ハナ", Lastname: "山田"})

	res := search(t, e.asA, q("山田 太郎"))
	assert.Equal(t, plain.ID, res.Contacts.Items[0].ID)
	assert.Len(t, res.Contacts.Items, 1)

	res = search(t, e.asA, q("花子 ハナ"))
	require.Len(t, res.Contacts.Items, 1)
	assert.Equal(t, nick.ID, res.Contacts.Items[0].ID)
}
