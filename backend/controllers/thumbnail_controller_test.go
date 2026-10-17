package controllers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"meerkat/config"
	"meerkat/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type thumbEnv struct {
	db    *gorm.DB
	r     *gin.Engine
	me    models.User
	other models.User
	dir   string
}

func newThumbEnv(t *testing.T) *thumbEnv {
	t.Helper()
	db, r := setupRouter()
	e := &thumbEnv{db: db, r: r, dir: t.TempDir()}
	require.NoError(t, db.First(&e.me).Error)
	e.other = models.User{Username: "other", Email: "o@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.other).Error)
	cfg := &config.Config{ProfilePhotoDir: e.dir}
	mountContacts(r, db, cfg)
	mountRelationships(r, db, cfg)
	return e
}

func (e *thumbEnv) get(path string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	return w
}

func TestListsCarryAThumbnailURLNotTheImage(t *testing.T) {
	e := newThumbEnv(t)
	big := "data:image/jpeg;base64," + strings.Repeat("A", 4000)
	with := models.Contact{UserID: e.me.ID, Firstname: "With", PhotoThumbnailFile: "11111111-2222-3333-4444-555555555555_thumb.jpg", LegacyThumbnail: big}
	without := models.Contact{UserID: e.me.ID, Firstname: "Without", LegacyThumbnail: big}
	require.NoError(t, e.db.Create(&[]*models.Contact{&with, &without}).Error)

	for _, q := range []string{"", "?fields=ID,firstname,photo_thumbnail", "?search=with"} {
		w := e.get("/contacts" + q)
		require.Equal(t, http.StatusOK, w.Code, q)
		assert.NotContains(t, w.Body.String(), "data:image", q)
		assert.Less(t, w.Body.Len(), 3000, "response size %s", q)
		var out struct {
			Contacts []map[string]any `json:"contacts"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		for _, c := range out.Contacts {
			url, _ := c["photo_thumbnail"].(string)
			if c["firstname"] == "With" {
				want := "/api/v1/contacts/" + strconv.Itoa(int(with.ID)) + "/profile_picture?thumbnail=true&v=11111111-2222-3333-4444-555555555555"
				assert.Equal(t, want, url, q)
			} else {
				assert.Empty(t, url, q)
			}
		}
	}

	w := e.get("/graph")
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "data:image")
	assert.Contains(t, w.Body.String(), `profile_picture?thumbnail=true&v=11111111`)
}

const legacyForUpload = "data:image/jpeg;base64,anBlZw=="

// newMultipart writes a one-file "photo" form into body and returns its content type.
func newMultipart(body *bytes.Buffer, data []byte) string {
	mw := multipart.NewWriter(body)
	part, _ := mw.CreateFormFile("photo", "p.png")
	part.Write(data)
	mw.Close()
	return mw.FormDataContentType()
}

func TestListWithThumbnailFieldButNoIDStillGetsWorkingURLs(t *testing.T) {
	e := newThumbEnv(t)
	c := models.Contact{UserID: e.me.ID, Firstname: "With", PhotoThumbnailFile: "11111111-2222-3333-4444-555555555555_thumb.jpg"}
	require.NoError(t, e.db.Create(&c).Error)
	w := e.get("/contacts?fields=firstname,photo_thumbnail")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "/contacts/"+strconv.Itoa(int(c.ID))+"/profile_picture?thumbnail=true")
}
