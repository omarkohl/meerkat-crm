package carddav

import (
	"crypto/sha256"
	"fmt"
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
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.ApiToken{}))
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
	handle := NewHandler(db, "").GinHandler()
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
