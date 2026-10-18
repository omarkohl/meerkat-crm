package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"meerkat/config"
	"meerkat/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ftsRouter is setupRouter plus the FTS indexes, built by the real migration files.
func ftsRouter(t *testing.T) (*gorm.DB, *gin.Engine, models.User) {
	t.Helper()
	db, router := setupRouter()
	sqlDB, _ := db.DB()
	for _, file := range []string{"000029_add_fts_search.up.sql", "000034_search_all_contact_fields.up.sql"} {
		sql, err := os.ReadFile("../database/migrations/" + file)
		require.NoError(t, err)
		_, err = sqlDB.Exec(string(sql))
		require.NoError(t, err)
	}
	var user models.User
	require.NoError(t, db.First(&user).Error)
	mountContacts(router, db, &config.Config{})
	mountNotes(router, db, &config.Config{})
	mountActivities(router, db, &config.Config{})
	return db, router, user
}

func getJSON(t *testing.T, router *gin.Engine, path string, params url.Values) map[string]any {
	t.Helper()
	req := httptest.NewRequest("GET", path+"?"+params.Encode(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out
}

func firstnames(out map[string]any) []string {
	var names []string
	for _, c := range out["contacts"].([]any) {
		names = append(names, c.(map[string]any)["firstname"].(string))
	}
	return names
}

func TestContactSearchFoldsAccentsAndMatchesPrefixesAcrossFields(t *testing.T) {
	db, r, u := ftsRouter(t)
	require.NoError(t, db.Create(&[]models.Contact{
		{UserID: u.ID, Firstname: "Zoë", Lastname: "Müller", Emails: []models.ContactEmail{{Type: "home", Value: "zoe@work.example"}}},
		{UserID: u.ID, Firstname: "Bob", Lastname: "Smith", Phones: []models.ContactPhone{{Type: "mobile", Value: "+41 79 555 0101"}}},
		{UserID: u.ID, Firstname: "José", Lastname: "García", Nickname: "Pepe"},
	}).Error)

	for term, want := range map[string]string{
		"zoe": "Zoë", "ZOË": "Zoë", "mull": "Zoë", "zoe muller": "Zoë", "muller zoe": "Zoë",
		"work.exa": "Zoë", "555": "Bob", "0101": "Bob", "jose": "José", "garcia": "José", "pepe": "José",
	} {
		out := getJSON(t, r, "/contacts", url.Values{"search": {term}})
		assert.Equal(t, []string{want}, firstnames(out), term)
		assert.EqualValues(t, 1, out["total"], term)
	}
	assert.Empty(t, firstnames(getJSON(t, r, "/contacts", url.Values{"search": {"nobody"}})))
	// every word must match
	assert.Empty(t, firstnames(getJSON(t, r, "/contacts", url.Values{"search": {"zoe smith"}})))
}

func TestPhoneNumberSearchMatchesInTheMiddleOfTheNumber(t *testing.T) {
	db, r, u := ftsRouter(t)
	require.NoError(t, db.Create(&[]models.Contact{
		{UserID: u.ID, Firstname: "Plain", Phone: "+41795550101", Phones: []models.ContactPhone{{Value: "+41795550101"}}},
		{UserID: u.ID, Firstname: "Spaced", Phones: []models.ContactPhone{{Value: "030 1234567"}}},
		{UserID: u.ID, Firstname: "Other", Phones: []models.ContactPhone{{Value: "+1 202 555 0000"}}},
	}).Error)
	for term, want := range map[string]string{
		"795550": "Plain", "5550101": "Plain", "+4179": "Plain", "1234567": "Spaced", "030 123": "Spaced", "202 555": "Other",
	} {
		assert.Equal(t, []string{want}, firstnames(getJSON(t, r, "/contacts", url.Values{"search": {term}})), term)
	}
}

func TestSearchInScriptsWithoutSpacesUsesTheSubstringFallback(t *testing.T) {
	db, r, u := ftsRouter(t)
	require.NoError(t, db.Create(&[]models.Contact{
		{UserID: u.ID, Firstname: "太郎", Lastname: "山田"},
		{UserID: u.ID, Firstname: "민수", Lastname: "김"},
		{UserID: u.ID, Firstname: "Anna"},
	}).Error)
	assert.Equal(t, []string{"太郎"}, firstnames(getJSON(t, r, "/contacts", url.Values{"search": {"山田"}})))
	assert.Equal(t, []string{"太郎"}, firstnames(getJSON(t, r, "/contacts", url.Values{"search": {"太"}})))
	assert.Equal(t, []string{"민수"}, firstnames(getJSON(t, r, "/contacts", url.Values{"search": {"민수"}})))

	now := time.Now()
	require.NoError(t, db.Create(&[]models.Note{{UserID: u.ID, Content: "来週東京で会議", Date: now}}).Error)
	assert.EqualValues(t, 1, getJSON(t, r, "/notes", url.Values{"search": {"東京"}})["total"])
}

func TestVeryLongSearchTermsAreHandled(t *testing.T) {
	db, r, u := ftsRouter(t)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Ann"}).Error)
	long := strings.Repeat("ann ", 500)
	out := getJSON(t, r, "/contacts", url.Values{"search": {long}})
	assert.Equal(t, []string{"Ann"}, firstnames(out))
}

func TestContactSearchSurvivesFTSSyntaxAndPunctuation(t *testing.T) {
	db, r, u := ftsRouter(t)
	require.NoError(t, db.Create(&[]models.Contact{
		{UserID: u.ID, Firstname: "Pat", Lastname: "O'Brien", Phones: []models.ContactPhone{{Value: "+49 30 1234"}}},
	}).Error)
	for _, term := range []string{`"`, `a"b`, "AND", "OR NOT", "pat*", "(pat)", "col:pat", "-", "'", "pat OR", "NEAR(pat"} {
		getJSON(t, r, "/contacts", url.Values{"search": {term}}) // must not be a 500
	}
	assert.Equal(t, []string{"Pat"}, firstnames(getJSON(t, r, "/contacts", url.Values{"search": {"o'brien"}})))
	// nothing but punctuation falls back to LIKE: "+" still finds the phone
	assert.Equal(t, []string{"Pat"}, firstnames(getJSON(t, r, "/contacts", url.Values{"search": {"+"}})))
}

func TestContactSearchIsScopedFiltersDeletedAndKeepsArchiveRules(t *testing.T) {
	db, r, u := ftsRouter(t)
	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&[]models.Contact{
		{UserID: u.ID, Firstname: "Mine"},
		{UserID: other.ID, Firstname: "Mine"},
		{UserID: u.ID, Firstname: "MineGone"},
		{UserID: u.ID, Firstname: "MineOld"},
	}).Error)
	require.NoError(t, db.Where("firstname = ?", "MineGone").Delete(&models.Contact{}).Error)
	require.NoError(t, db.Model(&models.Contact{}).Where("firstname = ?", "MineOld").Update("archived", true).Error)

	out := getJSON(t, r, "/contacts", url.Values{"search": {"mine"}})
	assert.Equal(t, []string{"Mine"}, firstnames(out))
	out = getJSON(t, r, "/contacts", url.Values{"search": {"mine"}, "include_archived": {"true"}})
	assert.ElementsMatch(t, []string{"Mine", "MineOld"}, firstnames(out))
	assert.EqualValues(t, 2, out["total"])
}

func TestContactSearchRanksUnlessASortIsRequested(t *testing.T) {
	db, r, u := ftsRouter(t)
	// the long entry has the newer id, so without ranking it would come first
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Ann", Lastname: "Lee"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Ann", Lastname: "Marie Louise Van Der Berg Of Somewhere"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Zed", Lastname: "Adams", Nickname: "Annie"}).Error)

	out := getJSON(t, r, "/contacts", url.Values{"search": {"ann lee"}})
	assert.Equal(t, []string{"Ann"}, firstnames(out))

	ranked := getJSON(t, r, "/contacts", url.Values{"search": {"ann"}})
	list := ranked["contacts"].([]any)
	require.Len(t, list, 3)
	assert.Equal(t, "Lee", list[0].(map[string]any)["lastname"], "shorter, more specific match first")

	sorted := getJSON(t, r, "/contacts", url.Values{"search": {"ann"}, "sort": {"lastname"}, "order": {"asc"}})
	assert.Equal(t, "Adams", sorted["contacts"].([]any)[0].(map[string]any)["lastname"])

	paged := getJSON(t, r, "/contacts", url.Values{"search": {"ann"}, "limit": {"2"}, "page": {"2"}})
	assert.Len(t, paged["contacts"], 1)
	assert.EqualValues(t, 3, paged["total"])
}

func TestNoteAndActivitySearch(t *testing.T) {
	db, r, u := ftsRouter(t)
	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	now := time.Now()
	require.NoError(t, db.Create(&[]models.Note{
		{UserID: u.ID, Content: "Trip to Zürich next week", Date: now},
		{UserID: u.ID, Content: "Groceries", Date: now},
		{UserID: other.ID, Content: "Zürich is not yours", Date: now},
	}).Error)
	out := getJSON(t, r, "/notes", url.Values{"search": {"zurich"}})
	assert.EqualValues(t, 1, out["total"])
	assert.Len(t, getJSON(t, r, "/notes", url.Values{"search": {"zür"}})["notes"], 1)

	contact := models.Contact{UserID: u.ID, Firstname: "Renée", Lastname: "Dupont", Phones: []models.ContactPhone{{Value: "555 0100"}}}
	require.NoError(t, db.Create(&contact).Error)
	byTitle := models.Activity{UserID: u.ID, Title: "Café catch-up", Description: "Planning", Location: "Münster", Date: now}
	byContact := models.Activity{UserID: u.ID, Title: "Dinner", Date: now, Contacts: []models.Contact{contact}}
	foreign := models.Activity{UserID: other.ID, Title: "Café of someone else", Date: now}
	require.NoError(t, db.Create(&[]models.Activity{byTitle, byContact, foreign}).Error)

	titles := func(term string) []string {
		var out []string
		for _, a := range getJSON(t, r, "/activities", url.Values{"search": {term}})["activities"].([]any) {
			out = append(out, a.(map[string]any)["title"].(string))
		}
		return out
	}
	assert.Equal(t, []string{"Café catch-up"}, titles("cafe"))
	assert.Equal(t, []string{"Café catch-up"}, titles("munster"))
	assert.Equal(t, []string{"Dinner"}, titles("renee"))
	assert.Equal(t, []string{"Dinner"}, titles("dupo"))
	assert.Empty(t, titles("555"), "activities match contact names, not their phone numbers")
	assert.Empty(t, titles("someone"), "other users' activities are not searched")
}

func TestSearchOn20kContactsIsFast(t *testing.T) {
	if testing.Short() {
		t.Skip("performance check")
	}
	db, r, u := ftsRouter(t)
	const n = 20000
	batch := make([]models.Contact, 0, 1000)
	for i := 0; i < n; i++ {
		batch = append(batch, models.Contact{
			UserID: u.ID, Firstname: fmt.Sprintf("First%d", i), Lastname: fmt.Sprintf("Family%d", i%500),
			Emails: []models.ContactEmail{{Value: fmt.Sprintf("person%d@example.org", i)}},
		})
		if len(batch) == cap(batch) {
			require.NoError(t, db.CreateInBatches(&batch, 200).Error)
			batch = batch[:0]
		}
	}
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Needle", Lastname: "Haystack"}).Error)

	timeIt := func(term string) (time.Duration, map[string]any) {
		start := time.Now()
		out := getJSON(t, r, "/contacts", url.Values{"search": {term}, "limit": {"25"}})
		return time.Since(start), out
	}
	d, out := timeIt("needle")
	assert.EqualValues(t, 1, out["total"])
	t.Logf("FTS word search over %d contacts: %v", n, d)

	d, out = timeIt("family49")
	assert.EqualValues(t, 440, out["total"], "Family49 and Family490..499, 40 contacts each")
	t.Logf("FTS prefix search: %v", d)
}
