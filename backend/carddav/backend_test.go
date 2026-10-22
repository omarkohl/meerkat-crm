package carddav

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"github.com/gin-gonic/gin"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-vcard"

	"meerkat/models"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// @020-US1
func TestDeleteAddressObjectTrashesDependants(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Note{}, &models.Relationship{}, &models.Reminder{}))
	user := models.User{Username: "u", Password: "password123!A", Email: "u@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Al", VCardUID: "uid-1"}
	require.NoError(t, db.Create(&contact).Error)
	cid := contact.ID
	require.NoError(t, db.Create(&models.Reminder{UserID: user.ID, Message: "r", RemindAt: time.Now(), Recurrence: "once", ContactID: &cid}).Error)

	b := NewBackend(db, "")
	ctx := ContextWithUser(context.Background(), user.ID, "u", db, "")
	require.NoError(t, b.DeleteAddressObject(ctx, "/carddav/addressbooks/u/contacts/uid-1.vcf"))

	var live int64
	require.NoError(t, db.Model(&models.Reminder{}).Where("contact_id = ?", cid).Count(&live).Error)
	assert.Zero(t, live)
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", cid).Count(&live).Error)
	assert.Zero(t, live)
}

func putPhotoBackend(t *testing.T) (*Backend, context.Context, *gorm.DB, models.User, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}))
	user := models.User{Username: "u", Password: "password123!A", Email: "u@example.com"}
	require.NoError(t, db.Create(&user).Error)
	dir := t.TempDir()
	return NewBackend(db, dir), ContextWithUser(context.Background(), user.ID, "u", db, dir), db, user, dir
}

func photoCard(t *testing.T) vcard.Card {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 60, 60))))
	src := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:uid-1\r\nFN:Al Smith\r\nN:Smith;Al;;;\r\nPHOTO;ENCODING=b;TYPE=PNG:" +
		base64.StdEncoding.EncodeToString(buf.Bytes()) + "\r\nEND:VCARD\r\n"
	card, err := vcard.NewDecoder(strings.NewReader(src)).Decode()
	require.NoError(t, err)
	return card
}

// @020-US1
func TestPutAddressObjectHandsThePhotoOfTheCardToTheImporter(t *testing.T) {
	b, ctx, db, user, _ := putPhotoBackend(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Al", VCardUID: "uid-1"}
	require.NoError(t, db.Create(&contact).Error)
	var gotContact, gotSize int
	b.SetPhotoImporter(func(_ context.Context, _ *gorm.DB, _ string, userID, contactID uint, data []byte) (bool, error) {
		assert.Equal(t, user.ID, userID)
		gotContact, gotSize = int(contactID), len(data)
		return false, nil
	})

	_, err := b.PutAddressObject(ctx, "/carddav/addressbooks/u/contacts/uid-1.vcf", photoCard(t), nil)
	require.NoError(t, err)

	assert.Equal(t, int(contact.ID), gotContact)
	assert.Positive(t, gotSize)
}

func TestPutAddressObjectKeepsTheCardWhenTheImporterFails(t *testing.T) {
	b, ctx, db, user, _ := putPhotoBackend(t)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Al", VCardUID: "uid-1"}).Error)
	b.SetPhotoImporter(func(context.Context, *gorm.DB, string, uint, uint, []byte) (bool, error) {
		return false, errors.New("unreadable")
	})

	_, err := b.PutAddressObject(ctx, "/carddav/addressbooks/u/contacts/uid-1.vcf", photoCard(t), nil)
	require.NoError(t, err)

	var got models.Contact
	require.NoError(t, db.Where("vcard_uid = ?", "uid-1").First(&got).Error)
	assert.Equal(t, "Al", got.Firstname)
}

func TestPutAddressObjectIgnoresThePhotoWithoutAnImporter(t *testing.T) {
	b, ctx, _, _, dir := putPhotoBackend(t)
	_, err := b.PutAddressObject(ctx, "/carddav/addressbooks/u/contacts/uid-2.vcf", photoCard(t), nil)
	require.NoError(t, err)
	entries, _ := os.ReadDir(dir)
	assert.Empty(t, entries)
}

type twoUsers struct {
	db         *gorm.DB
	a, b       models.User
	theirs     models.Contact
	backend    *Backend
	asA        context.Context
	photoDir   string
	handlerFor func(models.User) http.Handler
}

func newTwoUsers(t *testing.T) *twoUsers {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Note{}, &models.Relationship{}, &models.Reminder{}))
	e := &twoUsers{db: db, a: models.User{Username: "a", Password: "password123!A", Email: "a@example.com"}, b: models.User{Username: "b", Password: "password123!A", Email: "b@example.com"}}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)
	e.theirs = models.Contact{UserID: e.b.ID, Firstname: "Theirs", VCardUID: "shared-uid"}
	require.NoError(t, db.Create(&e.theirs).Error)
	e.backend = NewBackend(db, "")
	e.asA = ContextWithUser(context.Background(), e.a.ID, "a", db, "")
	e.handlerFor = func(u models.User) http.Handler {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		h := NewHandler(db, "").GinHandler()
		r.Any("/carddav/*path", func(c *gin.Context) { c.Set("userID", u.ID); c.Set("username", u.Username); h(c) })
		return r
	}
	return e
}

func (e *twoUsers) theirsIsUntouched(t *testing.T) {
	t.Helper()
	var reloaded models.Contact
	require.NoError(t, e.db.First(&reloaded, e.theirs.ID).Error, "the other user's contact is not deleted")
	assert.Equal(t, "Theirs", reloaded.Firstname)
	var n int64
	e.db.Model(&models.Contact{}).Where("user_id = ?", e.a.ID).Count(&n)
	assert.Zero(t, n, "nothing was created for the caller")
}

func TestACardDAVUserCannotReadOrDeleteAnotherUsersContact(t *testing.T) {
	e := newTwoUsers(t)
	for _, path := range []string{
		"/carddav/addressbooks/a/contacts/shared-uid.vcf",
		"/carddav/addressbooks/a/contacts/" + strconv.Itoa(int(e.theirs.ID)) + ".vcf",
		"/carddav/addressbooks/b/contacts/shared-uid.vcf",
	} {
		_, err := e.backend.GetAddressObject(e.asA, path, nil)
		assert.Error(t, err, "get %s", path)
		assert.Error(t, e.backend.DeleteAddressObject(e.asA, path), "delete %s", path)
	}
	list, err := e.backend.ListAddressObjects(e.asA, "/carddav/addressbooks/b/contacts/", nil)
	require.NoError(t, err)
	assert.Empty(t, list, "the list holds only the caller's contacts")
	e.theirsIsUntouched(t)
}

func TestACardDAVPutWithAnotherUsersUIDCreatesTheCallersOwnContact(t *testing.T) {
	e := newTwoUsers(t)
	card := vcard.Card{}
	card.SetValue(vcard.FieldVersion, "3.0")
	card.SetValue(vcard.FieldUID, "shared-uid")
	card.SetValue(vcard.FieldFormattedName, "Mine")
	card.SetName(&vcard.Name{GivenName: "Mine"})
	_, err := e.backend.PutAddressObject(e.asA, "/carddav/addressbooks/a/contacts/shared-uid.vcf", card, nil)
	require.NoError(t, err)

	var reloaded models.Contact
	require.NoError(t, e.db.First(&reloaded, e.theirs.ID).Error)
	assert.Equal(t, "Theirs", reloaded.Firstname, "the other user's contact is not overwritten")
	var mine models.Contact
	require.NoError(t, e.db.Where("user_id = ?", e.a.ID).First(&mine).Error)
	assert.Equal(t, "Mine", mine.Firstname)

	require.NoError(t, e.backend.DeleteAddressObject(e.asA, "/carddav/addressbooks/a/contacts/shared-uid.vcf"))
	require.NoError(t, e.db.First(&reloaded, e.theirs.ID).Error, "deleting by UID only reaches the caller's contact")
}

func TestOtherCardDAVMethodsCannotReachAnotherUsersContactEither(t *testing.T) {
	e := newTwoUsers(t)
	h := e.handlerFor(e.a)
	target := "/carddav/addressbooks/a/contacts/shared-uid.vcf"
	for _, tc := range []struct{ method, path, destination string }{
		{"COPY", target, "/carddav/addressbooks/a/contacts/copy.vcf"},
		{"MOVE", target, "/carddav/addressbooks/a/contacts/moved.vcf"},
		{"MOVE", "/carddav/addressbooks/b/contacts/shared-uid.vcf", "/carddav/addressbooks/a/contacts/stolen.vcf"},
		{"MKCOL", "/carddav/addressbooks/b/contacts/new/", ""},
		{"PATCH", target, ""},
		{"POST", target, ""},
		{"PUT", "/carddav/addressbooks/b/contacts/shared-uid.vcf", ""},
		{"DELETE", "/carddav/addressbooks/b/contacts/shared-uid.vcf", ""},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Hacked\r\nUID:shared-uid\r\nEND:VCARD\r\n"))
		if tc.destination != "" {
			req.Header.Set("Destination", tc.destination)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.GreaterOrEqual(t, w.Code, 300, "%s %s answered %d", tc.method, tc.path, w.Code)
	}
	var reloaded models.Contact
	require.NoError(t, e.db.First(&reloaded, e.theirs.ID).Error)
	assert.Equal(t, "Theirs", reloaded.Firstname)
}
