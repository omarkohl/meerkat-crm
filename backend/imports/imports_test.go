package imports_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/imports"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"

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

func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Note{}, &models.Relationship{}))
	e := &env{db: db, clk: clock.NewFake(now)}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)

	module := imports.New(imports.Deps{DB: db, Config: &config.Config{ProfilePhotoDir: t.TempDir()}})
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

func upload(r http.Handler, path, filename, content string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", filename)
	_, _ = part.Write([]byte(content))
	_ = mw.Close()
	req := httptest.NewRequest("POST", path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func post(r http.Handler, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type uploadResp struct {
	SessionID         string `json:"session_id"`
	RowCount          int    `json:"row_count"`
	SuggestedMappings []struct {
		ContactField string `json:"contact_field"`
	} `json:"suggested_mappings"`
}

type previewResp struct {
	SessionID string `json:"session_id"`
	TotalRows int    `json:"total_rows"`
	ValidRows int    `json:"valid_rows"`
	Rows      []struct {
		RowIndex       int `json:"row_index"`
		DuplicateMatch *struct {
			MatchReason string `json:"match_reason"`
		} `json:"duplicate_match"`
	} `json:"rows"`
}

func uploadCSV(t *testing.T, r http.Handler, content string) uploadResp {
	t.Helper()
	w := upload(r, "/contacts/import/upload", "c.csv", content)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp uploadResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

var firstnameMapping = []map[string]any{{"csv_column": "First Name", "contact_field": "firstname", "group": 0}}

func TestCSVUploadRejectsBadFilesWithA400(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, http.StatusBadRequest, upload(e.asA, "/contacts/import/upload", "contacts.exe", "a,b\n1,2\n").Code, "extension")
	assert.Equal(t, http.StatusBadRequest, upload(e.asA, "/contacts/import/upload", "c.csv", "").Code, "empty")
	assert.Equal(t, http.StatusBadRequest, upload(e.asA, "/contacts/import/upload", "c.csv", "only,a,header\n").Code, "no data rows")

	var many strings.Builder
	many.WriteString("First Name\n")
	for i := 0; i <= services.MaxCSVRows; i++ {
		fmt.Fprintf(&many, "p%d\n", i)
	}
	assert.Equal(t, http.StatusBadRequest, upload(e.asA, "/contacts/import/upload", "c.csv", many.String()).Code, "row limit")

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/contacts/import/upload", strings.NewReader("x"))
	e.asA.ServeHTTP(w, req)
	assert.Less(t, w.Code, 500, "no multipart body at all")
	assert.GreaterOrEqual(t, w.Code, 400)

	var form bytes.Buffer
	mw := multipart.NewWriter(&form)
	_ = mw.WriteField("other", "x")
	_ = mw.Close()
	req = httptest.NewRequest("POST", "/contacts/import/upload", &form)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w = httptest.NewRecorder()
	e.asA.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, "no file field")
	assert.Contains(t, w.Body.String(), "No file uploaded")
}

func TestCSVUploadAcceptsAnyCaseExtensionAndSuggestsMappings(t *testing.T) {
	e := newEnv(t)
	w := upload(e.asA, "/contacts/import/upload", "C.CSV", "First Name,Email\nAda,ada@example.com\n")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp uploadResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.SessionID)
	assert.Equal(t, 1, resp.RowCount)
	assert.Equal(t, "firstname", resp.SuggestedMappings[0].ContactField)
}

func TestCSVWizardImportsPreviewedRowsForTheUploader(t *testing.T) {
	e := newEnv(t)
	require.NoError(t, e.db.Create(&models.Contact{UserID: e.a.ID, Firstname: "Ada", Lastname: "Lovelace"}).Error)
	up := uploadCSV(t, e.asA, "First Name,Last Name\nAda,Lovelace\nGrace,Hopper\n")

	w := post(e.asA, "/contacts/import/preview", map[string]any{"session_id": up.SessionID, "mappings": []map[string]any{
		{"csv_column": "First Name", "contact_field": "firstname", "group": 0}, {"csv_column": "Last Name", "contact_field": "lastname", "group": 0}}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"duplicate_match":null`)
	var pv previewResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pv))
	require.Len(t, pv.Rows, 2)
	require.NotNil(t, pv.Rows[0].DuplicateMatch)
	assert.Equal(t, "name", pv.Rows[0].DuplicateMatch.MatchReason)
	assert.Nil(t, pv.Rows[1].DuplicateMatch)

	w = post(e.asA, "/contacts/import/confirm", map[string]any{"session_id": up.SessionID,
		"actions": []map[string]any{{"row_index": 0, "action": "skip"}, {"row_index": 1, "action": "add"}}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res struct {
		Created, Skipped int
		SkippedRows      []struct{ Name, Reason string } `json:"skipped_rows"`
		Errors           []string
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 1, res.Skipped)
	assert.Equal(t, "skipped", res.SkippedRows[0].Reason)
	assert.Empty(t, res.Errors)
	var grace models.Contact
	require.NoError(t, e.db.Where("firstname = ?", "Grace").First(&grace).Error)
	assert.Equal(t, e.a.ID, grace.UserID)

	assert.Equal(t, http.StatusNotFound, post(e.asA, "/contacts/import/confirm", map[string]any{"session_id": up.SessionID, "actions": []map[string]any{}}).Code,
		"a session is used up by its import")
}

func TestImportSessionsBelongToTheirUser(t *testing.T) {
	e := newEnv(t)
	up := uploadCSV(t, e.asA, "First Name\nAda\n")

	w := post(e.asB, "/contacts/import/preview", map[string]any{"session_id": up.SessionID, "mappings": firstnameMapping})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	w = post(e.asB, "/contacts/import/confirm", map[string]any{"session_id": up.SessionID, "actions": []map[string]any{{"row_index": 0, "action": "add"}}})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	var n int64
	e.db.Model(&models.Contact{}).Where("user_id = ?", e.b.ID).Count(&n)
	assert.Zero(t, n)

	assert.Equal(t, http.StatusNotFound, post(e.asA, "/contacts/import/preview", map[string]any{"session_id": "nope", "mappings": firstnameMapping}).Code)
}

func TestConfirmNeedsAPreviewFirst(t *testing.T) {
	e := newEnv(t)
	up := uploadCSV(t, e.asA, "First Name\nAda\n")
	w := post(e.asA, "/contacts/import/confirm", map[string]any{"session_id": up.SessionID, "actions": []map[string]any{{"row_index": 0, "action": "add"}}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Please generate a preview first")
}

func TestSessionsExpireWithTheApplicationClock(t *testing.T) {
	e := newEnv(t)
	up := uploadCSV(t, e.asA, "First Name\nAda\n")
	e.clk.Advance(time.Hour)
	w := post(e.asA, "/contacts/import/preview", map[string]any{"session_id": up.SessionID, "mappings": firstnameMapping})
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "expired")
}

func TestRequestBodiesAreValidated(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, http.StatusBadRequest, post(e.asA, "/contacts/import/preview", map[string]any{"mappings": firstnameMapping}).Code, "no session")
	assert.Equal(t, http.StatusBadRequest, post(e.asA, "/contacts/import/preview", map[string]any{"session_id": "x"}).Code, "no mappings")
	assert.Equal(t, http.StatusBadRequest, post(e.asA, "/contacts/import/confirm", map[string]any{"session_id": "x", "actions": []map[string]any{{"row_index": 0, "action": "delete"}}}).Code, "unknown action")
	assert.Equal(t, http.StatusBadRequest, post(e.asA, "/contacts/import/vcf/confirm", map[string]any{"session_id": "x"}).Code, "no actions")
}

const ada = "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Ada Lovelace\r\nEMAIL:ada@example.com\r\nEND:VCARD\r\n"

func TestVCFWizardPreviewsOnUploadAndImportsOnConfirm(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, http.StatusBadRequest, upload(e.asA, "/contacts/import/vcf/upload", "c.txt", ada).Code)

	w := upload(e.asA, "/contacts/import/vcf/upload", "c.vcf", ada)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var pv previewResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pv))
	assert.Equal(t, 1, pv.TotalRows)

	w = post(e.asA, "/contacts/import/vcf/confirm", map[string]any{"session_id": pv.SessionID, "actions": []map[string]any{{"row_index": 0, "action": "add"}}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var c models.Contact
	require.NoError(t, e.db.Where("firstname = ?", "Ada").First(&c).Error)
	assert.Equal(t, e.a.ID, c.UserID)
}

func TestVCFConfirmRefusesACSVSession(t *testing.T) {
	e := newEnv(t)
	up := uploadCSV(t, e.asA, "First Name\nAda\n")
	w := post(e.asA, "/contacts/import/vcf/confirm", map[string]any{"session_id": up.SessionID, "actions": []map[string]any{}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGarbageVCardIsNeverAServerError(t *testing.T) {
	e := newEnv(t)
	assert.Less(t, upload(e.asA, "/contacts/import/vcf/upload", "c.vcf", "this is not a vcard").Code, 500)
}

func TestImportEndpointsNeedAUser(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, http.StatusUnauthorized, upload(e.anon, "/contacts/import/upload", "c.csv", "First Name\nAda\n").Code)
	assert.Equal(t, http.StatusUnauthorized, upload(e.anon, "/contacts/import/vcf/upload", "c.vcf", ada).Code)
	assert.Equal(t, http.StatusUnauthorized, post(e.anon, "/contacts/import/preview", map[string]any{"session_id": "x", "mappings": firstnameMapping}).Code)
	assert.Equal(t, http.StatusUnauthorized, post(e.anon, "/contacts/import/confirm", map[string]any{"session_id": "x", "actions": []map[string]any{}}).Code)
	assert.Equal(t, http.StatusUnauthorized, post(e.anon, "/contacts/import/vcf/confirm", map[string]any{"session_id": "x", "actions": []map[string]any{}}).Code)
}
