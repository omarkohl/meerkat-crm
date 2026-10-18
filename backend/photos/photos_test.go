package photos_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/photos"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type env struct {
	db   *gorm.DB
	a, b models.User
	dir  string
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
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}))
	e := &env{db: db, dir: t.TempDir()}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)

	mount := func(userID uint) *gin.Engine {
		r := gin.New()
		r.Use(middleware.ClockMiddleware(clock.Real()), middleware.DefaultBodySizeLimitMiddleware())
		authenticated := r.Group("")
		if userID != 0 {
			authenticated.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
		}
		photos.New(photos.Deps{DB: db, Config: &config.Config{ProfilePhotoDir: e.dir}}).Register(api.New(r.Group(""), r), authenticated)
		return r
	}
	e.asA, e.asB, e.anon = mount(e.a.ID), mount(e.b.ID), mount(0)
	return e
}

func (e *env) contact(t *testing.T, owner uint, c models.Contact) models.Contact {
	t.Helper()
	c.UserID = owner
	if c.Firstname == "" {
		c.Firstname = "X"
	}
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

func (e *env) write(t *testing.T, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(e.dir, name), []byte(content), 0o600))
}

func pic(id uint) string { return "/contacts/" + strconv.Itoa(int(id)) + "/profile_picture" }

func get(r http.Handler, path string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func upload(r http.Handler, id uint, filename string, content []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("photo", filename)
	_, _ = part.Write(content)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, pic(id), &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func goodPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 60, 60))))
	return b.Bytes()
}

func TestTheFullPhotoIsServedToItsOwnerFromTheConfiguredDirectory(t *testing.T) {
	e := newEnv(t)
	e.write(t, "a.jpg", "jpeg-bytes")
	secret := filepath.Join(filepath.Dir(e.dir), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("secret"), 0o600))
	t.Cleanup(func() { os.Remove(secret) })
	mine := e.contact(t, e.a.ID, models.Contact{Photo: "a.jpg"})
	theirs := e.contact(t, e.b.ID, models.Contact{Photo: "a.jpg"})
	none := e.contact(t, e.a.ID, models.Contact{})
	missing := e.contact(t, e.a.ID, models.Contact{Photo: "gone.jpg"})
	traversal := e.contact(t, e.a.ID, models.Contact{Photo: "../secret.txt"})
	absolute := e.contact(t, e.a.ID, models.Contact{Photo: secret})

	w := get(e.asA, pic(mine.ID))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "jpeg-bytes", w.Body.String())
	assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
	assert.Equal(t, http.StatusNotFound, get(e.asA, pic(theirs.ID)).Code, "another user's photo")
	assert.Equal(t, http.StatusNotFound, get(e.asA, pic(none.ID)).Code)
	assert.Equal(t, http.StatusNotFound, get(e.asA, pic(missing.ID)).Code)
	for _, id := range []uint{traversal.ID, absolute.ID} {
		w := get(e.asA, pic(id))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.NotContains(t, w.Body.String(), "secret")
	}
	assert.Equal(t, http.StatusBadRequest, get(e.asA, "/contacts/abc/profile_picture").Code)
	assert.Equal(t, http.StatusNotFound, get(e.asA, pic(9999)).Code)
}

func TestTheThumbnailIsServedWithAnETagAndALongPrivateCache(t *testing.T) {
	e := newEnv(t)
	e.write(t, "abc_thumb.jpg", "jpegbytes")
	mine := e.contact(t, e.a.ID, models.Contact{PhotoThumbnailFile: "abc_thumb.jpg"})
	theirs := e.contact(t, e.b.ID, models.Contact{PhotoThumbnailFile: "abc_thumb.jpg"})
	none := e.contact(t, e.a.ID, models.Contact{})
	path := func(id uint) string { return pic(id) + "?thumbnail=true" }

	w := get(e.asA, path(mine.ID))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "jpegbytes", w.Body.String())
	assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Cache-Control"), "max-age=31536000")
	assert.Contains(t, w.Header().Get("Cache-Control"), "private")
	etag := w.Header().Get("ETag")
	require.NotEmpty(t, etag)

	w = get(e.asA, path(mine.ID), "If-None-Match", etag)
	assert.Equal(t, http.StatusNotModified, w.Code)
	assert.Empty(t, w.Body.String())
	assert.Equal(t, http.StatusOK, get(e.asA, path(mine.ID), "If-None-Match", `"other"`).Code)

	assert.Equal(t, http.StatusNotFound, get(e.asA, path(theirs.ID)).Code, "another user's thumbnail")
	assert.Equal(t, http.StatusNotFound, get(e.asA, path(none.ID)).Code)
}

func TestAThumbnailNameCannotEscapeTheDirectory(t *testing.T) {
	e := newEnv(t)
	secret := filepath.Join(filepath.Dir(e.dir), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("secret"), 0o600))
	t.Cleanup(func() { os.Remove(secret) })
	for _, name := range []string{"../secret.txt", secret, "sub/x.jpg", `..\secret.txt`} {
		c := e.contact(t, e.a.ID, models.Contact{PhotoThumbnailFile: name})
		w := get(e.asA, pic(c.ID)+"?thumbnail=true")
		assert.Equal(t, http.StatusBadRequest, w.Code, name)
		assert.NotContains(t, w.Body.String(), "secret", name)
	}
}

func TestAThumbnailStillStoredInTheDatabaseIsServed(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, models.Contact{LegacyThumbnail: "data:image/jpeg;base64,anBlZ2J5dGVz"})
	w := get(e.asA, pic(c.ID)+"?thumbnail=true")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "jpegbytes", w.Body.String())
	assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Cache-Control"), "no-cache")

	broken := e.contact(t, e.a.ID, models.Contact{LegacyThumbnail: "data:nothing"})
	assert.Equal(t, http.StatusInternalServerError, get(e.asA, pic(broken.ID)+"?thumbnail=true").Code)
	garbled := e.contact(t, e.a.ID, models.Contact{LegacyThumbnail: "data:image/jpeg;base64,!!!"})
	assert.Equal(t, http.StatusInternalServerError, get(e.asA, pic(garbled.ID)+"?thumbnail=true").Code)
}

func TestAThumbnailIsServedForTheOwnersTrashedContactsOnly(t *testing.T) {
	e := newEnv(t)
	e.write(t, "abc_thumb.jpg", "jpegbytes")
	e.write(t, "abc_photo.jpg", "full")
	mine := e.contact(t, e.a.ID, models.Contact{PhotoThumbnailFile: "abc_thumb.jpg", Photo: "abc_photo.jpg"})
	theirs := e.contact(t, e.b.ID, models.Contact{PhotoThumbnailFile: "abc_thumb.jpg"})
	require.NoError(t, e.db.Delete(&mine).Error)
	require.NoError(t, e.db.Delete(&theirs).Error)

	w := get(e.asA, pic(mine.ID)+"?thumbnail=true")
	require.Equal(t, http.StatusOK, w.Code, "the trash shows avatars")
	assert.Equal(t, "jpegbytes", w.Body.String())
	assert.Equal(t, http.StatusNotFound, get(e.asA, pic(theirs.ID)+"?thumbnail=true").Code)
	assert.Equal(t, http.StatusNotFound, get(e.asA, pic(mine.ID)).Code, "the full photo stays with live contacts")
}

// @004-US9
func TestUploadStoresAJPEGUnderAGeneratedNameAndDropsTheOldPicture(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, models.Contact{LegacyThumbnail: "data:image/jpeg;base64,anBlZw=="})

	w := upload(e.asA, c.ID, "../../evil.php", goodPNG(t))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var first models.Contact
	require.NoError(t, e.db.First(&first, c.ID).Error)
	assert.True(t, strings.HasSuffix(first.Photo, "_photo.jpg"))
	assert.True(t, strings.HasSuffix(first.PhotoThumbnailFile, "_thumb.jpg"))
	assert.Empty(t, first.LegacyThumbnail, "the old picture's database copy is dropped")
	entries, _ := os.ReadDir(e.dir)
	assert.Len(t, entries, 2)

	require.Equal(t, http.StatusOK, upload(e.asA, c.ID, "p.png", goodPNG(t)).Code)
	var second models.Contact
	require.NoError(t, e.db.First(&second, c.ID).Error)
	assert.NotEqual(t, first.Photo, second.Photo)
	entries, _ = os.ReadDir(e.dir)
	assert.Len(t, entries, 2, "the old photo and thumbnail are gone")
	_, err := os.Stat(filepath.Join(e.dir, first.PhotoThumbnailFile))
	assert.True(t, os.IsNotExist(err))
	assert.Equal(t, http.StatusOK, get(e.asA, pic(c.ID)).Code, "the new photo is served")
}

func TestUploadRefusesWhatTheUserCanFixAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	mine := e.contact(t, e.a.ID, models.Contact{})
	theirs := e.contact(t, e.b.ID, models.Contact{})
	good := goodPNG(t)

	for name, content := range map[string][]byte{
		"text file":       []byte("this is not an image at all"),
		"empty file":      {},
		"html with image": []byte("<html><script>alert(1)</script></html>"),
		"truncated png":   good[:40],
	} {
		w := upload(e.asA, mine.ID, "p.png", content)
		assert.Equal(t, http.StatusBadRequest, w.Code, name+": "+w.Body.String())
	}

	bomb := append([]byte{}, good...)
	binary.BigEndian.PutUint32(bomb[16:], 30000)
	binary.BigEndian.PutUint32(bomb[20:], 30000)
	binary.BigEndian.PutUint32(bomb[29:], crc32.ChecksumIEEE(bomb[12:29]))
	assert.Equal(t, http.StatusBadRequest, upload(e.asA, mine.ID, "p.png", bomb).Code, "a header that claims a gigantic image")

	assert.Equal(t, http.StatusNotFound, upload(e.asA, theirs.ID, "p.png", good).Code)
	assert.Equal(t, http.StatusNotFound, upload(e.asB, mine.ID, "p.png", good).Code)
	entries, _ := os.ReadDir(e.dir)
	assert.Empty(t, entries)
	var stored models.Contact
	require.NoError(t, e.db.First(&stored, mine.ID).Error)
	assert.Empty(t, stored.Photo)
}

func TestUploadWithoutAFileChangesNothing(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, models.Contact{Photo: "keep.jpg"})
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, pic(c.ID), &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	e.asA.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var stored models.Contact
	require.NoError(t, e.db.First(&stored, c.ID).Error)
	assert.Equal(t, "keep.jpg", stored.Photo)
}

func TestUploadsAreRefusedInDemoMode(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")
	e := newEnv(t)
	c := e.contact(t, e.a.ID, models.Contact{})
	assert.Equal(t, http.StatusForbidden, upload(e.asA, c.ID, "p.png", goodPNG(t)).Code)
}

func TestWithoutAnAuthenticatedUserEverythingRefuses(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, models.Contact{})
	assert.Equal(t, http.StatusUnauthorized, get(e.anon, pic(c.ID)).Code)
	assert.Equal(t, http.StatusUnauthorized, get(e.anon, pic(c.ID)+"?thumbnail=true").Code)
	assert.Equal(t, http.StatusUnauthorized, upload(e.anon, c.ID, "p.png", goodPNG(t)).Code)
}

func TestUploadsOverTheBodyLimitAreRefusedAndChangeNothing(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, models.Contact{Photo: "keep.jpg"})

	w := upload(e.asA, c.ID, "p.png", bytes.Repeat([]byte("x"), 11<<20))

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "too large")
	var stored models.Contact
	require.NoError(t, e.db.First(&stored, c.ID).Error)
	assert.Equal(t, "keep.jpg", stored.Photo)
	entries, _ := os.ReadDir(e.dir)
	assert.Empty(t, entries)
}

func TestAFailedDatabaseUpdateLeavesNoNewFilesBehind(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, models.Contact{})
	require.NoError(t, e.db.Exec(`CREATE TRIGGER no_updates BEFORE UPDATE ON contacts BEGIN SELECT RAISE(ABORT, 'boom'); END`).Error)

	w := upload(e.asA, c.ID, "p.png", goodPNG(t))

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	entries, _ := os.ReadDir(e.dir)
	assert.Empty(t, entries)
}
