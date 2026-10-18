package globalsearch_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/contacts"
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

var (
	templateOnce sync.Once
	templatePath string
	templateErr  error
)

// templateDB copies a database that was migrated once, so each test does not run every migration.
func templateDB(t *testing.T, dest string) string {
	t.Helper()
	templateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "globalsearch-template")
		if err != nil {
			templateErr = err
			return
		}
		templatePath = filepath.Join(dir, "template.db")
		db, err := database.InitDB(templatePath)
		if err != nil {
			templateErr = err
			return
		}
		sqlDB, _ := db.DB()
		templateErr = sqlDB.Close()
	})
	require.NoError(t, templateErr)
	data, err := os.ReadFile(templatePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dest, data, 0o600))
	return dest
}

func TestMain(m *testing.M) {
	code := m.Run()
	if templatePath != "" {
		_ = os.RemoveAll(filepath.Dir(templatePath))
	}
	os.Exit(code)
}

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
	db, err := database.InitDB(templateDB(t, filepath.Join(t.TempDir(), "app.db")))
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
			FieldName    string    `json:"matched_field_name"`
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

// @022-US1
func TestEveryContactFieldIsSearchedAndNamedWhenItMatches(t *testing.T) {
	e := newEnv(t)
	e.contact(t, e.a.ID, models.Contact{
		Firstname: "Ada", Lastname: "Lovelace", MiddleName: "Augusta", Organization: "Analytical Engines", Department: "Mathematics",
		JobTitle: "Programmer", Role: "Pioneer", HowWeMet: "at a lecture", FoodPreference: "vegetarian",
		WorkInformation: "writes notes", ContactInformation: "ask her mother", Note: "prefers tea",
		Circles:      []string{"Climbing club"},
		Addresses:    []models.ContactAddress{{Type: "home", Street: "Rosenweg 4", City: "Hamburg", Postal: "20095", Country: "Germany"}},
		CustomFields: map[string]string{"Favourite bird": "Pelican"},
	})
	for term, want := range map[string]struct{ field, name, text string }{
		"augusta":  {"middle_name", "", "Augusta"},
		"analytic": {"organization", "", "Analytical Engines"},
		"mathem":   {"department", "", "Mathematics"},
		"programm": {"job_title", "", "Programmer"},
		"pioneer":  {"role", "", "Pioneer"},
		"lecture":  {"how_we_met", "", "at a lecture"},
		"vegetar":  {"food_preference", "", "vegetarian"},
		"writes":   {"work_information", "", "writes notes"},
		"mother":   {"contact_information", "", "ask her mother"},
		"tea":      {"note", "", "prefers tea"},
		"hamburg":  {"address", "", "Rosenweg 4, Hamburg, 20095, Germany"},
		"climbing": {"circles", "", "Climbing club"},
		"pelican":  {"custom_field", "Favourite bird", "Pelican"},
		"pel":      {"custom_field", "Favourite bird", "Pelican"},
		"lovelace": {"name", "", ""},
	} {
		res := search(t, e.asA, q(term))
		require.Len(t, res.Contacts.Items, 1, term)
		hit := res.Contacts.Items[0]
		assert.Equal(t, want.field, hit.MatchedField, term)
		assert.Equal(t, want.name, hit.FieldName, term)
		assert.Equal(t, want.text, text(hit.Snippet), term)
		if want.field != "name" {
			assert.NotEmpty(t, marked(hit.Snippet), term)
		}
	}
	res := search(t, e.asA, q("bird"))
	assert.Zero(t, res.Contacts.Total, "custom field names are not searched, only their values")
}

// @022-US1
func TestAContactWithOnlyTheLegacyAddressIsFoundByIt(t *testing.T) {
	e := newEnv(t)
	e.contact(t, e.a.ID, models.Contact{Firstname: "Old", Lastname: "Timer", Address: "Old Street 1"})
	e.contact(t, e.a.ID, models.Contact{Firstname: "Cy", Address: "Elm Road 2"})

	res := search(t, e.asA, q("elm"))

	require.Len(t, res.Contacts.Items, 1)
	assert.Equal(t, "address", res.Contacts.Items[0].MatchedField)
	assert.Equal(t, "Elm Road 2", text(res.Contacts.Items[0].Snippet))
}

// @022-US1
func TestTheOtherFieldsAreFoundWithoutTheIndexToo(t *testing.T) {
	e := newEnv(t)
	e.contact(t, e.a.ID, models.Contact{
		Firstname: "Taro", Organization: "株式会社テスト", Circles: []string{"京都の友達"},
		Addresses:    []models.ContactAddress{{City: "大阪市"}},
		CustomFields: map[string]string{"趣味": "囲碁"},
	})
	for term, field := range map[string]string{"株式会社": "organization", "京都": "circles", "大阪": "address", "囲碁": "custom_field"} {
		res := search(t, e.asA, q(term))
		require.Len(t, res.Contacts.Items, 1, term)
		assert.Equal(t, field, res.Contacts.Items[0].MatchedField, term)
	}
}

// @022-US1
func TestEditingAContactChangesWhatItIsFoundBy(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, models.Contact{Firstname: "Ada", Organization: "Engines Ltd", CustomFields: map[string]string{"Bird": "Pelican"}})

	require.NoError(t, e.db.Model(&c).Updates(map[string]any{"organization": "Looms Inc"}).Error)
	require.NoError(t, e.db.Model(&c).Update("custom_fields", `{"Bird":"Heron"}`).Error)

	assert.Zero(t, search(t, e.asA, q("engines")).Contacts.Total)
	assert.Zero(t, search(t, e.asA, q("pelican")).Contacts.Total)
	assert.Equal(t, 1, search(t, e.asA, q("looms")).Contacts.Total)
	assert.Equal(t, 1, search(t, e.asA, q("heron")).Contacts.Total)

	require.NoError(t, e.db.Transaction(func(tx *gorm.DB) error { return models.TrashContact(tx, e.a.ID, c.ID) }))
	assert.Zero(t, search(t, e.asA, q("looms")).Contacts.Total, "a trashed contact is not found")
	require.NoError(t, e.db.Unscoped().Model(&models.Contact{}).Where("id = ?", c.ID).Update("deleted_at", nil).Error)
	assert.Equal(t, 1, search(t, e.asA, q("looms")).Contacts.Total, "a restored contact is found again")
	require.NoError(t, e.db.Unscoped().Delete(&c).Error)
	assert.Zero(t, search(t, e.asA, q("looms")).Contacts.Total)
}

// @022-US1
func TestAnotherUsersFieldsAreNotFoundEither(t *testing.T) {
	e := newEnv(t)
	e.contact(t, e.b.ID, models.Contact{Firstname: "Bea", Organization: "Secret Org", Note: "hidden note", Circles: []string{"Spies"}})

	for _, term := range []string{"secret", "hidden", "spies"} {
		assert.Zero(t, search(t, e.asA, q(term)).Contacts.Total, term)
	}
}

// @022-US1
func TestTheContactListSearchStillIgnoresTheNewFields(t *testing.T) {
	e := newEnv(t)
	e.contact(t, e.a.ID, models.Contact{Firstname: "Ada", Lastname: "Lovelace", Email: "ada@example.com", Organization: "Hamburg Engines", Note: "visits Hamburg"})

	list := func(term string) string {
		w := httptest.NewRecorder()
		r := gin.New()
		r.Use(middleware.ClockMiddleware(clock.NewFake(now)))
		g := r.Group("")
		g.Use(func(c *gin.Context) { c.Set("userID", e.a.ID); c.Next() })
		contacts.New(contacts.Deps{DB: e.db, Config: &config.Config{}}).Register(api.New(r.Group(""), r), g)
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/contacts?"+url.Values{"search": {term}}.Encode(), nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return w.Body.String()
	}

	assert.Contains(t, list("ada"), "Lovelace")
	assert.Contains(t, list("example"), "Lovelace", "e-mail addresses are still searched")
	assert.NotContains(t, list("hamburg"), "Lovelace", "the organization and the note are not")
	assert.Equal(t, 1, search(t, e.asA, q("hamburg")).Contacts.Total, "the global search finds them")
}

// @022-US1
func TestChangingOnlyTheAddressUpdatesTheIndex(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, models.Contact{Firstname: "Ada", Address: "Old Street 1"})

	require.NoError(t, e.db.Model(&c).Update("address", "Elm Road 2").Error)

	assert.Zero(t, search(t, e.asA, q("old")).Contacts.Total)
	assert.Equal(t, 1, search(t, e.asA, q("elm")).Contacts.Total)
}

// @022-US1
func TestAContactThatMatchesInTwoFieldsIsReportedByTheFirstOne(t *testing.T) {
	e := newEnv(t)
	e.contact(t, e.a.ID, models.Contact{Firstname: "Ada", Email: "boris@example.com", Organization: "Boris Ltd", Note: "knows Boris"})

	hit := search(t, e.asA, q("boris")).Contacts.Items[0]

	assert.Equal(t, "email", hit.MatchedField, "e-mail comes before the other fields")

	e.contact(t, e.a.ID, models.Contact{Firstname: "Cy", Organization: "Zed Ltd", Note: "met Zed"})
	other := search(t, e.asA, q("zed")).Contacts.Items[0]
	assert.Equal(t, "organization", other.MatchedField, "organization comes before the note")
}

// @022-US1
func TestALongFieldComesBackAsAnExcerpt(t *testing.T) {
	e := newEnv(t)
	e.contact(t, e.a.ID, models.Contact{Firstname: "Ada", Note: strings.Repeat("filler words ", 80) + "unicorn sighting" + strings.Repeat(" more filler", 80)})

	hit := search(t, e.asA, q("unicorn")).Contacts.Items[0]

	assert.Equal(t, "note", hit.MatchedField)
	assert.Equal(t, []string{"unicorn"}, marked(hit.Snippet))
	assert.Less(t, len([]rune(text(hit.Snippet))), 200)
}
