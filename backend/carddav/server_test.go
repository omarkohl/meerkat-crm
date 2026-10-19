package carddav

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"meerkat/middleware"
	"meerkat/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type server struct {
	db     *gorm.DB
	router *gin.Engine
	alice  models.User
}

const alicePassword = "Correct-horse-1!"

func newServer(t *testing.T) *server {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.ApiToken{}, &models.Note{}, &models.Relationship{}, &models.Reminder{}))
	hash, err := bcrypt.GenerateFromPassword([]byte(alicePassword), bcrypt.MinCost)
	require.NoError(t, err)
	alice := models.User{Username: "alice", Email: "alice@example.com", Password: string(hash)}
	bob := models.User{Username: "bob", Email: "bob@example.com", Password: string(hash)}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: bob.ID, Firstname: "Bobs", Lastname: "Friend", VCardUID: "bobs-friend"}).Error)

	t.Cleanup(func() {
		for _, id := range []string{"alice", "alice@example.com", "bob", "nobody"} {
			middleware.GetAccountRateLimiter().RecordSuccessfulLogin(id)
		}
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/.well-known/carddav", WellKnownRedirect)
	g := r.Group("/carddav")
	g.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	g.Use(BasicAuthMiddleware())
	handle := NewHandler(db, t.TempDir()).GinHandler()
	g.Any("/*path", handle)
	for _, m := range []string{"PROPFIND", "REPORT", "MKCOL", "COPY", "MOVE"} {
		g.Handle(m, "/*path", handle)
	}
	return &server{db: db, router: r, alice: alice}
}

const allProps = `<?xml version="1.0"?><propfind xmlns="DAV:"><allprop/></propfind>`

func (s *server) do(method, path, user, password string, headers ...string) *httptest.ResponseRecorder {
	body := ""
	if method == "PROPFIND" {
		body = allProps
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if user != "" {
		req.SetBasicAuth(user, password)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

func (s *server) propfind(path, user, depth string) *httptest.ResponseRecorder {
	return s.do("PROPFIND", path, user, alicePassword, "Depth", depth, "Content-Type", "application/xml")
}

// @020-US1
func TestAClientFindsTheAddressBookViaWellKnownPrincipalAndHomeSet(t *testing.T) {
	s := newServer(t)

	w := s.do("GET", "/.well-known/carddav", "", "")
	assert.Equal(t, http.StatusMovedPermanently, w.Code)
	assert.Equal(t, "/carddav/", w.Header().Get("Location"))

	root := s.propfind("/carddav/", "alice", "0")
	assert.Equal(t, http.StatusMultiStatus, root.Code)
	assert.Contains(t, root.Body.String(), "/carddav/principals/alice/")

	principal := s.propfind("/carddav/principals/alice/", "alice", "0")
	assert.Equal(t, http.StatusMultiStatus, principal.Code)
	assert.Contains(t, principal.Body.String(), "/carddav/addressbooks/alice/")

	home := s.propfind("/carddav/addressbooks/alice/", "alice", "1")
	assert.Equal(t, http.StatusMultiStatus, home.Code)
	assert.Contains(t, home.Body.String(), "/carddav/addressbooks/alice/contacts/")
	assert.Contains(t, home.Body.String(), "Contacts")
}

// @020-US1
func TestAClientCannotSeeAnotherUsersAddressBook(t *testing.T) {
	s := newServer(t)

	w := s.propfind("/carddav/addressbooks/bob/contacts/", "alice", "1")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), "Bobs")

	own := s.propfind("/carddav/addressbooks/alice/contacts/", "alice", "1")
	assert.Equal(t, http.StatusMultiStatus, own.Code)
	assert.NotContains(t, own.Body.String(), "bobs-friend")
}

// @020-US1
func TestBasicAuthAcceptsUsernameOrEmailWithThePassword(t *testing.T) {
	s := newServer(t)
	for _, login := range []string{"alice", "Alice", "alice@example.com"} {
		w := s.propfind("/carddav/", login, "0")
		assert.Equal(t, http.StatusMultiStatus, w.Code, login)
	}
}

// @020-US1
func TestBasicAuthRefusesWhatIsNotTheAccountPassword(t *testing.T) {
	s := newServer(t)

	plain := "meerkat_" + strings.Repeat("ab", 16)
	require.NoError(t, s.db.Create(&models.ApiToken{
		UserID: s.alice.ID, Name: "sync", TokenHash: fmt.Sprintf("%x", sha256.Sum256([]byte(plain))),
	}).Error)

	none := s.do("PROPFIND", "/carddav/", "", "")
	assert.Equal(t, http.StatusUnauthorized, none.Code)
	assert.Contains(t, none.Header().Get("WWW-Authenticate"), "Basic")

	for name, c := range map[string][2]string{
		"wrong password": {"alice", "wrong"},
		"unknown user":   {"nobody", alicePassword},
		"an API token":   {"alice", plain},
	} {
		w := s.do("PROPFIND", "/carddav/", c[0], c[1])
		assert.Equal(t, http.StatusUnauthorized, w.Code, name)
		assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Basic", name)
	}
}

// @020-US1
func TestRepeatedWrongPasswordsLockTheAccountOut(t *testing.T) {
	s := newServer(t)

	var last *httptest.ResponseRecorder
	for range middleware.MaxLoginAttempts {
		last = s.do("PROPFIND", "/carddav/", "alice", "wrong")
	}
	assert.Equal(t, http.StatusTooManyRequests, last.Code)

	locked := s.do("PROPFIND", "/carddav/", "alice", alicePassword)
	assert.Equal(t, http.StatusTooManyRequests, locked.Code, "even the right password waits")
	assert.NotEmpty(t, locked.Header().Get("Retry-After"))
}

func vcardBody(uid, given, family string, extra ...string) string {
	lines := []string{"BEGIN:VCARD", "VERSION:3.0", "UID:" + uid, "FN:" + given + " " + family, "N:" + family + ";" + given + ";;;"}
	lines = append(lines, extra...)
	return strings.Join(append(lines, "END:VCARD", ""), "\r\n")
}

func (s *server) put(uid, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("PUT", "/carddav/addressbooks/alice/contacts/"+uid+".vcf", strings.NewReader(body))
	req.SetBasicAuth("alice", alicePassword)
	req.Header.Set("Content-Type", "text/vcard")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	return w
}

// go-webdav answers 201 for an update too, where RFC 4918 expects 204; any success will do.
func assertUpdated(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.True(t, w.Code >= 200 && w.Code < 300, "update answered %d", w.Code)
}

func (s *server) get(uid string) *httptest.ResponseRecorder {
	return s.do("GET", "/carddav/addressbooks/alice/contacts/"+uid+".vcf", "alice", alicePassword)
}

func (s *server) addressBook() string {
	return s.propfind("/carddav/addressbooks/alice/contacts/", "alice", "1").Body.String()
}

// @020-US1
func TestAClientCreatesAndUpdatesAContactWithPut(t *testing.T) {
	s := newServer(t)

	created := s.put("new-uid", vcardBody("new-uid", "Carla", "Cole", "TEL:+4930123"))
	assert.Equal(t, http.StatusCreated, created.Code)
	etag := created.Header().Get("ETag")
	assert.NotEmpty(t, etag)

	got := s.get("new-uid")
	assert.Equal(t, http.StatusOK, got.Code)
	assert.Contains(t, got.Body.String(), "Carla")
	assert.Contains(t, got.Body.String(), "Cole")

	updated := s.put("new-uid", vcardBody("new-uid", "Carla", "Colt"), "If-Match", etag)
	assertUpdated(t, updated)
	assert.Contains(t, s.get("new-uid").Body.String(), "Colt")
	assert.NotEqual(t, etag, updated.Header().Get("ETag"), "a change gives a new ETag")
	assert.Equal(t, 1, strings.Count(s.addressBook(), "new-uid.vcf"), "the update did not create a second contact")
}

// @020-US1
func TestAStalePutIsRefusedAndKeepsTheServerVersion(t *testing.T) {
	s := newServer(t)
	first := s.put("c-1", vcardBody("c-1", "Dana", "Dunn"))
	require.Equal(t, http.StatusCreated, first.Code)
	stale := first.Header().Get("ETag")
	assertUpdated(t, s.put("c-1", vcardBody("c-1", "Dana", "Dunne"), "If-Match", stale))

	conflict := s.put("c-1", vcardBody("c-1", "Dana", "Overwritten"), "If-Match", stale)
	assert.Equal(t, http.StatusPreconditionFailed, conflict.Code)
	body := s.get("c-1").Body.String()
	assert.Contains(t, body, "Dunne")
	assert.NotContains(t, body, "Overwritten")
}

// @020-US1
func TestPutThatMustCreateRefusesAnExistingContact(t *testing.T) {
	s := newServer(t)
	require.Equal(t, http.StatusCreated, s.put("c-2", vcardBody("c-2", "Eli", "East")).Code)

	w := s.put("c-2", vcardBody("c-2", "Eli", "Overwritten"), "If-None-Match", "*")
	assert.Equal(t, http.StatusPreconditionFailed, w.Code)
	assert.NotContains(t, s.get("c-2").Body.String(), "Overwritten")

	fresh := s.put("c-3", vcardBody("c-3", "Fay", "Fox"), "If-None-Match", "*")
	assert.Equal(t, http.StatusCreated, fresh.Code)
}

// @020-US1
func TestPutThatMustCreateRefusesAnExistingContactWithoutAStoredETag(t *testing.T) {
	s := newServer(t)
	require.Equal(t, http.StatusCreated, s.put("c-6", vcardBody("c-6", "Gil", "Gold")).Code)
	require.NoError(t, s.db.Model(&models.Contact{}).Where("vcard_uid = ?", "c-6").UpdateColumn("etag", "").Error)

	w := s.put("c-6", vcardBody("c-6", "Gil", "Overwritten"), "If-None-Match", "*")
	assert.Equal(t, http.StatusPreconditionFailed, w.Code)
	assert.NotContains(t, s.get("c-6").Body.String(), "Overwritten")
}

// @020-US1
func TestPutThatMustUpdateRefusesAMissingContact(t *testing.T) {
	s := newServer(t)
	w := s.put("ghost", vcardBody("ghost", "Gus", "Gray"), "If-Match", `"anything"`)
	assert.Equal(t, http.StatusPreconditionFailed, w.Code)
	assert.Equal(t, http.StatusNotFound, s.get("ghost").Code)
}

// @020-US1
func TestADeletedContactLeavesTheAddressBookAndGoesToTheTrash(t *testing.T) {
	s := newServer(t)
	require.Equal(t, http.StatusCreated, s.put("c-4", vcardBody("c-4", "Hana", "Hill")).Code)
	require.Contains(t, s.addressBook(), "c-4")

	w := s.do("DELETE", "/carddav/addressbooks/alice/contacts/c-4.vcf", "alice", alicePassword)
	assert.Equal(t, http.StatusNoContent, w.Code)

	assert.NotContains(t, s.addressBook(), "c-4")
	assert.Equal(t, http.StatusNotFound, s.get("c-4").Code)
	var trashed models.Contact
	require.NoError(t, s.db.Unscoped().Where("user_id = ? AND vcard_uid = ?", s.alice.ID, "c-4").First(&trashed).Error)
	assert.True(t, trashed.DeletedAt.Valid, "the contact is in the trash, not gone")
}

// @020-US1
func TestMissingContactsAnswerNotFound(t *testing.T) {
	s := newServer(t)
	assert.Equal(t, http.StatusNotFound, s.get("nobody-here").Code)
	assert.Equal(t, http.StatusNotFound, s.do("DELETE", "/carddav/addressbooks/alice/contacts/nobody-here.vcf", "alice", alicePassword).Code)
}

// @020-US1
func TestAVCardWithAPhotoGivesTheContactAPhoto(t *testing.T) {
	s := newServer(t)
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 60, 60))))
	photo := "PHOTO;ENCODING=b;TYPE=PNG:" + base64.StdEncoding.EncodeToString(buf.Bytes())

	assert.Equal(t, http.StatusCreated, s.put("c-5", vcardBody("c-5", "Ivo", "Ives", photo)).Code)

	assert.Contains(t, s.get("c-5").Body.String(), "PHOTO", "the client gets the profile picture back")
}

// @020-US1
func TestATrashedContactLeavesTheAddressBookAndTheEditedOneGetsANewETagLeavesTheAddressBookAndTheTargetGetsANewETag(t *testing.T) {
	s := newServer(t)
	target := s.put("m-target", vcardBody("m-target", "Anna", "Smith"))
	require.Equal(t, http.StatusCreated, target.Code)
	require.Equal(t, http.StatusCreated, s.put("m-source", vcardBody("m-source", "Anna", "Jones", "TEL:+4930123")).Code)
	before := s.addressBook()
	require.Contains(t, before, "m-target")
	require.Contains(t, before, "m-source")

	var merged models.Contact
	require.NoError(t, s.db.Where("vcard_uid = ?", "m-target").First(&merged).Error)
	var source models.Contact
	require.NoError(t, s.db.Where("vcard_uid = ?", "m-source").First(&source).Error)
	merged.Phones = source.Phones
	require.NoError(t, s.db.Save(&merged).Error)
	require.NoError(t, s.db.Transaction(func(tx *gorm.DB) error {
		return models.TrashContact(tx, s.alice.ID, source.ID)
	}))

	after := s.addressBook()
	assert.Contains(t, after, "m-target")
	assert.NotContains(t, after, "m-source")
	assert.NotEqual(t, target.Header().Get("ETag"), s.get("m-target").Header().Get("ETag"))
	assert.Equal(t, http.StatusNotFound, s.get("m-source").Code)
	assert.Equal(t, http.StatusNotFound, s.do("DELETE", "/carddav/addressbooks/alice/contacts/m-source.vcf", "alice", alicePassword).Code)
	assert.Equal(t, http.StatusPreconditionFailed, s.put("m-source", vcardBody("m-source", "Anna", "Revived"), "If-Match", `"old"`).Code, "a device update of a trashed contact is refused")
	assert.NotContains(t, s.addressBook(), "m-source")
}
