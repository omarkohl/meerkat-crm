package images_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
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
	"meerkat/images"
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
	dir    string
	module *images.Module
	asA    *gin.Engine
	asB    *gin.Engine
	anon   *gin.Engine
}

func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Note{}, &models.Activity{}, &models.Image{}))
	e := &env{db: db, dir: t.TempDir()}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)
	e.module = images.New(images.Deps{DB: db, Config: &config.Config{ProfilePhotoDir: e.dir}})

	mount := func(userID uint) *gin.Engine {
		r := gin.New()
		r.Use(middleware.ClockMiddleware(clock.Real()), middleware.DefaultBodySizeLimitMiddleware())
		authenticated := r.Group("/api/v1")
		if userID != 0 {
			authenticated.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
		} else {
			authenticated.Use(func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
		}
		e.module.Register(api.New(r.Group("/api/v1"), r), authenticated)
		return r
	}
	e.asA, e.asB, e.anon = mount(e.a.ID), mount(e.b.ID), mount(0)
	return e
}

func goodPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 60, 40))))
	return b.Bytes()
}

func upload(r http.Handler, field, filename string, content []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile(field, filename)
	_, _ = part.Write(content)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/images", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func get(r http.Handler, path string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func (e *env) uploaded(t *testing.T, r http.Handler) models.ImageRef {
	t.Helper()
	w := upload(r, "file", "a.png", goodPNG(t))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var ref models.ImageRef
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ref))
	return ref
}

func TestAnUploadIsStoredAsAPendingImageAndServedBothWays(t *testing.T) {
	e := newEnv(t)
	ref := e.uploaded(t, e.asA)

	assert.Equal(t, 60, ref.Width)
	assert.Equal(t, 40, ref.Height)
	assert.Equal(t, "image/webp", ref.Mime)
	assert.Contains(t, ref.URL, "/api/v1/images/"+strconv.Itoa(int(ref.ID))+"/image?v=")
	assert.Contains(t, ref.ThumbURL, "/thumb?v=")

	var row models.Image
	require.NoError(t, e.db.First(&row, ref.ID).Error)
	assert.Equal(t, e.a.ID, row.UserID)
	assert.Nil(t, row.NoteID)
	assert.Equal(t, ref, row.Ref(), "the API and the model build the same URLs")
	for _, name := range []string{row.File, row.ThumbFile} {
		info, err := os.Stat(filepath.Join(e.dir, name))
		require.NoError(t, err)
		assert.Zero(t, info.Mode().Perm()&0o007)
	}

	for _, url := range []string{ref.URL, ref.ThumbURL} {
		w := get(e.asA, url)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "image/webp", w.Header().Get("Content-Type"))
		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		assert.Contains(t, w.Header().Get("Cache-Control"), "immutable")
		assert.Equal(t, "RIFF", w.Body.String()[:4])
		etag := w.Header().Get("ETag")
		require.NotEmpty(t, etag)
		assert.Equal(t, http.StatusNotModified, get(e.asA, url, "If-None-Match", etag).Code)
	}
	assert.Empty(t, get(e.asA, ref.URL).Header().Get("Content-Disposition"))
	assert.Contains(t, get(e.asA, ref.URL+"&download=1").Header().Get("Content-Disposition"), "attachment")
}

func TestDownloadNameFollowsTheStoredFormat(t *testing.T) {
	e := newEnv(t)
	ref := e.uploaded(t, e.asA)
	require.NoError(t, e.db.Model(&models.Image{}).Where("id = ?", ref.ID).Update("mime", "image/png").Error)

	assert.Contains(t, get(e.asA, ref.URL+"&download=1").Header().Get("Content-Disposition"), ".png\"")
}

func TestImagesBelongToTheirUploader(t *testing.T) {
	e := newEnv(t)
	ref := e.uploaded(t, e.asA)

	assert.Equal(t, http.StatusNotFound, get(e.asB, ref.URL).Code)
	assert.Equal(t, http.StatusNotFound, get(e.asB, ref.ThumbURL).Code)
	assert.Equal(t, http.StatusUnauthorized, upload(e.anon, "file", "a.png", goodPNG(t)).Code)
	assert.Equal(t, http.StatusUnauthorized, get(e.anon, ref.URL).Code)

	other := e.uploaded(t, e.asB)
	var rows []models.Image
	require.NoError(t, e.db.Where("user_id = ?", e.b.ID).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, other.ID, rows[0].ID)
	assert.Contains(t, rows[0].File, "images/u"+strconv.Itoa(int(e.b.ID))+"/")
}

func TestUploadRefusesWhatTheUserCanFixAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, http.StatusBadRequest, upload(e.asA, "file", "a.txt", []byte("not an image")).Code)
	assert.Equal(t, http.StatusBadRequest, upload(e.asA, "file", "a.png", goodPNG(t)[:30]).Code)
	assert.Equal(t, http.StatusBadRequest, upload(e.asA, "other", "a.png", goodPNG(t)).Code, "no file")
	assert.Equal(t, http.StatusBadRequest, upload(e.asA, "file", "big.png", append(goodPNG(t), make([]byte, 26<<20)...)).Code)

	var n int64
	e.db.Model(&models.Image{}).Count(&n)
	assert.Zero(t, n)
	entries, _ := filepath.Glob(filepath.Join(e.dir, "images", "*", "*"))
	assert.Empty(t, entries)
}

func TestUploadIsRefusedInDemoMode(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")
	e := newEnv(t)
	assert.Equal(t, http.StatusForbidden, upload(e.asA, "file", "a.png", goodPNG(t)).Code)
}

func TestAMissingFileIsNotFoundAndAPathOutsideTheDirectoryIsRefused(t *testing.T) {
	e := newEnv(t)
	ref := e.uploaded(t, e.asA)
	var row models.Image
	require.NoError(t, e.db.First(&row, ref.ID).Error)
	require.NoError(t, os.Remove(filepath.Join(e.dir, row.File)))
	assert.Equal(t, http.StatusNotFound, get(e.asA, ref.URL).Code)

	require.NoError(t, e.db.Model(&row).Update("file", "../secret.txt").Error)
	w := get(e.asA, ref.URL)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSweepDropsOldPendingImagesAndOrphanFilesButKeepsClaimedAndYoungOnes(t *testing.T) {
	e := newEnv(t)
	old, claimed, young := e.uploaded(t, e.asA), e.uploaded(t, e.asA), e.uploaded(t, e.asA)
	contact := models.Contact{UserID: e.a.ID, Firstname: "X"}
	require.NoError(t, e.db.Create(&contact).Error)
	note := models.Note{UserID: e.a.ID, Content: "n", Date: time.Now(), ContactID: &contact.ID}
	require.NoError(t, e.db.Create(&note).Error)

	longAgo := time.Now().Add(-48 * time.Hour)
	require.NoError(t, e.db.Model(&models.Image{}).Where("id IN ?", []uint{old.ID, claimed.ID}).Update("created_at", longAgo).Error)
	require.NoError(t, e.db.Model(&models.Image{}).Where("id = ?", claimed.ID).Update("note_id", note.ID).Error)

	orphan := filepath.Join(e.dir, "images", "u1", "orphan_image.webp")
	youngOrphan := filepath.Join(e.dir, "images", "u1", "young_image.webp")
	for _, p := range []string{orphan, youngOrphan} {
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
	}
	require.NoError(t, os.Chtimes(orphan, longAgo, longAgo))

	var oldRow models.Image
	require.NoError(t, e.db.First(&oldRow, old.ID).Error)

	pending, orphans, err := e.module.Sweep(t.Context(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, pending)
	assert.Equal(t, 1, orphans)

	var ids []uint
	require.NoError(t, e.db.Model(&models.Image{}).Order("id").Pluck("id", &ids).Error)
	assert.Equal(t, []uint{claimed.ID, young.ID}, ids)
	assert.NoFileExists(t, filepath.Join(e.dir, oldRow.File))
	assert.NoFileExists(t, filepath.Join(e.dir, oldRow.ThumbFile))
	assert.NoFileExists(t, orphan)
	assert.FileExists(t, youngOrphan)
	assert.Equal(t, http.StatusOK, get(e.asA, claimed.URL).Code)
	assert.Equal(t, http.StatusOK, get(e.asA, young.URL).Code)
}

func putJSON(r http.Handler, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func (e *env) contact(t *testing.T, owner uint) models.Contact {
	t.Helper()
	c := models.Contact{UserID: owner, Firstname: "Ann"}
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

func contactPath(c models.Contact, tail string) string {
	return "/api/v1/contacts/" + strconv.Itoa(int(c.ID)) + tail
}

// @023-US3
func TestImagesOfAContactAreSetOrderedAndTheFirstBecomesItsMainImage(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID)
	one, two := e.uploaded(t, e.asA), e.uploaded(t, e.asA)

	w := putJSON(e.asA, contactPath(c, "/images"), map[string]any{"image_ids": []uint{two.ID, one.ID}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got []models.ImageRef
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got, 2)
	assert.Equal(t, []uint{two.ID, one.ID}, []uint{got[0].ID, got[1].ID})
	require.NotNil(t, got[0].Crop, "the main image has its centred crop")
	assert.Equal(t, models.ImageCrop{X: 10, Y: 0, Size: 40}, *got[0].Crop)

	require.NoError(t, e.db.First(&c, c.ID).Error)
	assert.Equal(t, two.ID, *c.MainImageID)
	assert.FileExists(t, filepath.Join(e.dir, c.Photo))
	assert.FileExists(t, filepath.Join(e.dir, c.PhotoThumbnailFile))
}

// @023-US3
func TestSetMainImageCutsTheAvatarFromTheCropAndRefusesAnythingElse(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID)
	img := e.uploaded(t, e.asA)
	require.Equal(t, http.StatusOK, putJSON(e.asA, contactPath(c, "/images"), map[string]any{"image_ids": []uint{img.ID}}).Code)

	crop := func(x, y, size int) map[string]any {
		return map[string]any{"image_id": img.ID, "crop": map[string]int{"x": x, "y": y, "size": size}}
	}
	w := putJSON(e.asA, contactPath(c, "/main_image"), crop(0, 0, 30))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out struct {
		MainImageID uint            `json:"main_image_id"`
		Image       models.ImageRef `json:"image"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, img.ID, out.MainImageID)
	assert.Equal(t, models.ImageCrop{X: 0, Y: 0, Size: 30}, *out.Image.Crop)

	assert.Equal(t, http.StatusBadRequest, putJSON(e.asA, contactPath(c, "/main_image"), crop(50, 0, 30)).Code, "outside the image")
	assert.Equal(t, http.StatusBadRequest, putJSON(e.asA, contactPath(c, "/main_image"), crop(0, 0, 0)).Code)
	assert.Equal(t, http.StatusNotFound, putJSON(e.asA, contactPath(c, "/main_image"), map[string]any{"image_id": 9999, "crop": map[string]int{"x": 0, "y": 0, "size": 5}}).Code)
}

func TestContactImagesAreIsolatedBetweenUsers(t *testing.T) {
	e := newEnv(t)
	mine := e.contact(t, e.a.ID)
	img := e.uploaded(t, e.asA)
	require.Equal(t, http.StatusOK, putJSON(e.asA, contactPath(mine, "/images"), map[string]any{"image_ids": []uint{img.ID}}).Code)
	theirs := e.uploaded(t, e.asB)

	assert.Equal(t, http.StatusNotFound, putJSON(e.asB, contactPath(mine, "/images"), map[string]any{"image_ids": []uint{theirs.ID}}).Code, "another user's contact")
	assert.Equal(t, http.StatusNotFound, putJSON(e.asB, contactPath(mine, "/main_image"), map[string]any{"image_id": img.ID, "crop": map[string]int{"x": 0, "y": 0, "size": 5}}).Code)
	other := e.contact(t, e.b.ID)
	assert.Equal(t, http.StatusNotFound, putJSON(e.asB, contactPath(other, "/images"), map[string]any{"image_ids": []uint{img.ID}}).Code, "another user's image")
	assert.Equal(t, http.StatusUnauthorized, putJSON(e.anon, contactPath(mine, "/images"), map[string]any{"image_ids": []uint{}}).Code)

	var n int64
	e.db.Model(&models.Image{}).Where("id = ? AND contact_id = ?", img.ID, mine.ID).Count(&n)
	assert.EqualValues(t, 1, n, "the image stays where it was")
}

func TestAnEmptyListRemovesAllImagesAndTheAvatar(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID)
	img := e.uploaded(t, e.asA)
	require.Equal(t, http.StatusOK, putJSON(e.asA, contactPath(c, "/images"), map[string]any{"image_ids": []uint{img.ID}}).Code)
	require.NoError(t, e.db.First(&c, c.ID).Error)
	avatar := c.Photo

	require.Equal(t, http.StatusOK, putJSON(e.asA, contactPath(c, "/images"), map[string]any{"image_ids": []uint{}}).Code)
	require.NoError(t, e.db.First(&c, c.ID).Error)
	assert.Empty(t, c.Photo)
	assert.Nil(t, c.MainImageID)
	assert.NoFileExists(t, filepath.Join(e.dir, avatar))
}

func TestMoreThanFiftyImagesAreRefused(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID)
	ids := make([]uint, 51)
	for i := range ids {
		ids[i] = uint(i + 1)
	}
	assert.Equal(t, http.StatusBadRequest, putJSON(e.asA, contactPath(c, "/images"), map[string]any{"image_ids": ids}).Code)
}

type relatedPage struct {
	Images []struct {
		Image  models.ImageRef `json:"image"`
		Source struct {
			Type  string    `json:"type"`
			ID    uint      `json:"id"`
			Label string    `json:"label"`
			Date  time.Time `json:"date"`
		} `json:"source"`
	} `json:"images"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

func (e *env) claim(t *testing.T, r http.Handler, field string, id uint) uint {
	t.Helper()
	img := e.uploaded(t, r)
	require.NoError(t, e.db.Model(&models.Image{}).Where("id = ?", img.ID).Update(field, id).Error)
	return img.ID
}

// @023-US6
func TestRelatedImagesListTheImagesOfNotesAndActivitiesOfTheContactNewestFirst(t *testing.T) {
	e := newEnv(t)
	c, other := e.contact(t, e.a.ID), e.contact(t, e.a.ID)
	day := func(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC) }

	oldNote := models.Note{UserID: e.a.ID, Content: "Hiking", Date: day(1), ContactID: &c.ID}
	otherNote := models.Note{UserID: e.a.ID, Content: "Not about Ann", Date: day(5), ContactID: &other.ID}
	trip := models.Activity{UserID: e.a.ID, Title: "Trip", Date: day(10), Contacts: []models.Contact{c}}
	elsewhere := models.Activity{UserID: e.a.ID, Title: "Elsewhere", Date: day(11), Contacts: []models.Contact{other}}
	for _, v := range []any{&oldNote, &otherNote, &trip, &elsewhere} {
		require.NoError(t, e.db.Create(v).Error)
	}
	first := e.claim(t, e.asA, "note_id", oldNote.ID)
	second := e.claim(t, e.asA, "note_id", oldNote.ID)
	third := e.claim(t, e.asA, "activity_id", trip.ID)
	e.claim(t, e.asA, "note_id", otherNote.ID)
	e.claim(t, e.asA, "activity_id", elsewhere.ID)
	require.NoError(t, e.db.Model(&models.Image{}).Where("id = ?", second).Update("position", 1).Error)

	w := get(e.asA, contactPath(c, "/related_images"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got relatedPage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, 3, got.Total)
	require.Len(t, got.Images, 3)
	assert.Equal(t, []uint{third, first, second}, []uint{got.Images[0].Image.ID, got.Images[1].Image.ID, got.Images[2].Image.ID})
	assert.Equal(t, "activity", got.Images[0].Source.Type)
	assert.Equal(t, "Trip", got.Images[0].Source.Label)
	assert.Equal(t, trip.ID, got.Images[0].Source.ID)
	assert.Equal(t, "note", got.Images[1].Source.Type)
	assert.Equal(t, "Hiking", got.Images[1].Source.Label)

	paged := get(e.asA, contactPath(c, "/related_images")+"?limit=2&page=2")
	require.Equal(t, http.StatusOK, paged.Code)
	var rest relatedPage
	require.NoError(t, json.Unmarshal(paged.Body.Bytes(), &rest))
	assert.Equal(t, 3, rest.Total)
	require.Len(t, rest.Images, 1)
	assert.Equal(t, second, rest.Images[0].Image.ID)
}

// @023-US6
func TestRelatedImagesOfAnotherUsersContactAreNotFound(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID)
	note := models.Note{UserID: e.a.ID, Content: "Mine", Date: time.Now(), ContactID: &c.ID}
	require.NoError(t, e.db.Create(&note).Error)
	e.claim(t, e.asA, "note_id", note.ID)

	assert.Equal(t, http.StatusNotFound, get(e.asB, contactPath(c, "/related_images")).Code)
	assert.Equal(t, http.StatusUnauthorized, get(e.anon, contactPath(c, "/related_images")).Code)
}
