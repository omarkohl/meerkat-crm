package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	mcarddav "meerkat/carddav"
	"meerkat/clock"
	"meerkat/models"

	"github.com/emersion/go-webdav"
	carddavlib "github.com/emersion/go-webdav/carddav"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bookPaths(books []models.DiscoveredAddressBook) []string {
	var paths []string
	for _, b := range books {
		paths = append(paths, b.Path)
	}
	return paths
}

type noPrincipalBackend struct{ *mcarddav.Backend }

func (noPrincipalBackend) CurrentUserPrincipal(context.Context) (string, error) {
	return "", errors.New("no principal")
}

func newMeerkatServer(t *testing.T, wellKnownHits *atomic.Int32) (baseURL, bookPath string) {
	return newMeerkatServerWith(t, wellKnownHits, false)
}

func newMeerkatServerWith(t *testing.T, wellKnownHits *atomic.Int32, withoutPrincipal bool) (baseURL, bookPath string) {
	t.Helper()
	db := newContactSyncDB(t)
	user := models.User{Username: "remote", Password: "password123!A", Email: "remote@example.com"}
	require.NoError(t, db.Create(&user).Error)
	photoDir := t.TempDir()
	meerkat := mcarddav.NewBackend(db, photoDir)
	meerkat.SetPhotoImporter(ImportContactPhotoAt)
	var backend carddavlib.Backend = meerkat
	if withoutPrincipal {
		backend = noPrincipalBackend{meerkat}
	}
	lib := &carddavlib.Handler{Backend: backend, Prefix: "/carddav"}
	r := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/.well-known/carddav" || req.URL.Path == "/" {
			if req.URL.Path == "/.well-known/carddav" && req.Method == http.MethodGet && !withoutPrincipal {
				wellKnownHits.Add(1)
				http.Redirect(w, req, "/carddav/", http.StatusMovedPermanently)
				return
			}
			http.NotFound(w, req)
			return
		}
		if req.URL.Path == "/carddav/" || strings.HasPrefix(req.URL.Path, "/carddav/principals/") {
			webdav.ServePrincipal(w, req, &webdav.ServePrincipalOptions{
				CurrentUserPrincipalPath: "/carddav/principals/remote/",
				HomeSets:                 []webdav.BackendSuppliedHomeSet{carddavlib.NewAddressBookHomeSet("/carddav/addressbooks/remote/")},
				Capabilities:             []webdav.Capability{carddavlib.CapabilityAddressBook},
			})
			return
		}
		ctx := mcarddav.ContextWithUser(req.Context(), user.ID, user.Username, db, photoDir)
		lib.ServeHTTP(w, req.WithContext(ctx))
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv.URL, "/carddav/addressbooks/remote/contacts/"
}

// @017-US4
func TestDiscoverFindsTheAddressBookFromAServerOrAnAddressBookAddress(t *testing.T) {
	var hits atomic.Int32
	base, book := newMeerkatServer(t, &hits)
	svc := NewContactSyncService(false)

	books, err := svc.Discover(context.Background(), base, "", "")
	require.NoError(t, err)
	assert.Contains(t, bookPaths(books), book)
	assert.Equal(t, int32(1), hits.Load(), "the server address was resolved through /.well-known/carddav")

	books, err = svc.Discover(context.Background(), base+book, "", "")
	require.NoError(t, err)
	assert.Contains(t, bookPaths(books), book)
}

// @017-US4
func TestDiscoverAcceptsAnAddressBookAddressOnAServerWithoutPrincipals(t *testing.T) {
	base, book := newMeerkatServerWith(t, nil, true)

	books, err := NewContactSyncService(false).Discover(context.Background(), base+book, "", "")

	require.NoError(t, err)
	assert.Equal(t, []string{book}, bookPaths(books))
	assert.Equal(t, "contacts", books[0].Name)
}

// @017-US4
func TestDiscoverTellsTheUserWhatWentWrong(t *testing.T) {
	respond := func(status int, body string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		return srv
	}

	cases := []struct {
		name    string
		address string
		want    error
	}{
		{"a wrong password", respond(http.StatusUnauthorized, "").URL, ErrContactSyncUnauthorized},
		{"a web page that is not CardDAV", respond(http.StatusOK, "<html>hello</html>").URL, ErrContactSyncUnreachable},
		{"an address that does not exist", respond(http.StatusNotFound, "").URL, ErrContactSyncUnreachable},
		{"an address that is not a web address", "ftp://example.com/dav", ErrContactSyncInvalidURL},
	}
	for _, tc := range cases {
		_, err := NewContactSyncService(false).Discover(context.Background(), tc.address, "user", "pw")
		assert.ErrorIs(t, err, tc.want, tc.name)
	}
}

// @017-US4
func TestDiscoverDoesNotSendThePasswordToAnotherHostViaWellKnown(t *testing.T) {
	var hits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(other.Close)
	otherURL := strings.Replace(other.URL, "127.0.0.1", "localhost", 1)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/carddav" {
			http.Redirect(w, r, otherURL+"/dav/", http.StatusMovedPermanently)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(origin.Close)

	_, err := NewContactSyncService(false).Discover(context.Background(), origin.URL, "user", "secret")
	assert.ErrorIs(t, err, ErrContactSyncUnreachable)
	assert.Zero(t, hits.Load(), "the redirect target was never contacted")
}

func pngBase64(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 60, 60))))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// @017-US4
func TestContactSyncPhotos(t *testing.T) {
	runContactSync(t, models.CardDAVDirectionTwoWay, func(t *testing.T, env *contactSyncEnv) {
		env.remote.putRaw(t, "uid-photo", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:uid-photo\r\nFN:Pia Photo\r\nN:Photo;Pia;;;\r\n"+
			"PHOTO;ENCODING=b;TYPE=PNG:"+pngBase64(t)+"\r\nEND:VCARD\r\n")
		env.remote.putRaw(t, "uid-blocked", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:uid-blocked\r\nFN:Bo Blocked\r\nN:Blocked;Bo;;;\r\n"+
			"PHOTO;VALUE=uri:http://127.0.0.1:1/missing.png\r\nEND:VCARD\r\n")

		stats := env.sync(t)

		assert.Equal(t, 2, stats.PulledCreated)
		assert.Zero(t, stats.Errors, "a photo that cannot be fetched does not fail the sync")
		byName := map[string]models.Contact{}
		for _, c := range env.localContacts(t) {
			byName[c.Firstname] = c
		}
		assert.NotEmpty(t, byName["Pia"].Photo, "the embedded photo became the profile picture")
		assert.Empty(t, byName["Bo"].Photo)
		assert.Equal(t, "Blocked", byName["Bo"].Lastname, "the contact itself was still imported")
	})
}

// @017-US4
func TestContactSyncPushesAProfilePictureToTheServer(t *testing.T) {
	runContactSync(t, models.CardDAVDirectionTwoWay, func(t *testing.T, env *contactSyncEnv) {
		contact := env.createContact(t, "Quin", "Quick", "")
		data, err := base64.StdEncoding.DecodeString(pngBase64(t))
		require.NoError(t, err)
		var eff ImageEffects
		_, err = AddContactImage(context.Background(), env.db, env.cfg.ProfilePhotoDir, contact.UserID, contact.ID, data, true, &eff)
		require.NoError(t, err)

		env.sync(t)

		var uid string
		require.NoError(t, env.db.Model(&models.Contact{}).Select("vcard_uid").Where("id = ?", contact.ID).Scan(&uid).Error)
		card, ok := env.remote.card(t, uid)
		require.True(t, ok)
		assert.NotEmpty(t, card.Value("PHOTO"), "the server has the picture")
	})
}

// @017-US4
func TestScheduledSyncRunsEveryEnabledConnectionEvenWhenOneFails(t *testing.T) {
	remote := newMeerkatRemote(t)
	env := setupContactSyncEnv(t, models.CardDAVDirectionTwoWay, remote)
	remote.putContact(t, "uid-sched", "Sam", "Scheduled", "")

	broken := models.User{Username: "broken", Password: "password123!A", Email: "broken@example.com"}
	require.NoError(t, env.db.Create(&broken).Error)
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	t.Cleanup(dead.Close)
	conn := models.CardDAVConnection{UserID: broken.ID, BaseURL: dead.URL, AddressBookPath: "/x/", Direction: models.CardDAVDirectionTwoWay, SyncEnabled: true}
	require.NoError(t, env.db.Create(&conn).Error)
	second := models.User{Username: "second", Password: "password123!A", Email: "second@example.com"}
	require.NoError(t, env.db.Create(&second).Error)
	require.NoError(t, env.db.Create(&models.CardDAVConnection{UserID: second.ID, BaseURL: remote.proxyURL, AddressBookPath: remote.addressBookPath(), Direction: models.CardDAVDirectionTwoWay, SyncEnabled: true}).Error)
	off := models.User{Username: "off", Password: "password123!A", Email: "off@example.com"}
	require.NoError(t, env.db.Create(&off).Error)
	disabled := models.CardDAVConnection{UserID: off.ID, BaseURL: remote.proxyURL, AddressBookPath: remote.addressBookPath(), Direction: models.CardDAVDirectionTwoWay}
	require.NoError(t, env.db.Create(&disabled).Error)
	require.NoError(t, env.db.Model(&disabled).Update("sync_enabled", false).Error)

	SyncAllContactConnections(env.db, env.cfg, clock.Real())

	var names []string
	for _, c := range env.localContacts(t) {
		names = append(names, c.Firstname)
	}
	assert.Equal(t, []string{"Sam"}, names, "the first connection synced")
	var afterTheFailure int64
	require.NoError(t, env.db.Model(&models.Contact{}).Where("user_id = ?", second.ID).Count(&afterTheFailure).Error)
	assert.EqualValues(t, 1, afterTheFailure, "the connection after the failing one still synced")
	var stored models.CardDAVConnection
	require.NoError(t, env.db.First(&stored, conn.ID).Error)
	assert.Equal(t, models.CardDAVSyncStatusError, stored.LastSyncStatus)
	assert.NotEmpty(t, stored.LastSyncError, "the user sees why it failed")
	var untouched int64
	require.NoError(t, env.db.Model(&models.Contact{}).Where("user_id = ?", off.ID).Count(&untouched).Error)
	assert.Zero(t, untouched, "a disabled connection is not synced")
}

// @023-US4
func TestContactSyncAddsOneImageForAPhotoThatComesBackUnchanged(t *testing.T) {
	runContactSync(t, models.CardDAVDirectionTwoWay, func(t *testing.T, env *contactSyncEnv) {
		card := func(last string) string {
			return "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:uid-photo\r\nFN:Pia " + last + "\r\nN:" + last + ";Pia;;;\r\n" +
				"PHOTO;ENCODING=b;TYPE=PNG:" + pngBase64(t) + "\r\nEND:VCARD\r\n"
		}
		env.remote.putRaw(t, "uid-photo", card("Photo"))
		env.sync(t)
		env.remote.putRaw(t, "uid-photo", card("Photographer"))
		env.sync(t)

		contacts := env.localContacts(t)
		require.Len(t, contacts, 1)
		assert.Equal(t, "Photographer", contacts[0].Lastname, "the second sync pulled the change")
		var images int64
		require.NoError(t, env.db.Model(&models.Image{}).Where("contact_id = ?", contacts[0].ID).Count(&images).Error)
		assert.EqualValues(t, 1, images)
	})
}

// serves the remote's address book with its ETags rewritten by etag, as servers that do not quote
// them or that send weak ETags do
func proxyWithETags(t *testing.T, remote *davRemote, etag func(opaque string) string) string {
	t.Helper()
	target, err := url.Parse(remote.directURL)
	require.NoError(t, err)
	quoted := regexp.MustCompile(`(<[^>/]*getetag[^>]*>)(?:"|&#34;)([^"&<]*)(?:"|&#34;)(</)`)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(resp *http.Response) error {
		if h := resp.Header.Get("ETag"); h != "" {
			resp.Header.Set("ETag", etag(strings.Trim(h, `"`)))
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		body = quoted.ReplaceAllFunc(body, func(m []byte) []byte {
			parts := quoted.FindSubmatch(m)
			var out bytes.Buffer
			_ = xml.EscapeText(&out, []byte(etag(string(parts[2]))))
			return []byte(string(parts[1]) + out.String() + string(parts[3]))
		})
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return nil
	}
	srv := httptest.NewServer(proxy)
	t.Cleanup(srv.Close)
	return srv.URL
}

// @017-US4
func TestContactSyncWorksWithServersThatSendUnusualETags(t *testing.T) {
	cases := map[string]func(string) string{
		"unquoted": func(o string) string { return o },
		"weak":     func(o string) string { return `W/"` + o + `"` },
	}
	for name, etag := range cases {
		t.Run(name, func(t *testing.T) {
			remote := newMeerkatRemote(t)
			remote.putContact(t, "uid-remote", "Rita", "Remote", "")
			env := setupContactSyncEnv(t, models.CardDAVDirectionTwoWay, remote)
			env.conn.BaseURL = proxyWithETags(t, remote, etag)
			env.createContact(t, "Lou", "Local", "")

			stats := env.sync(t)

			assert.Equal(t, 1, stats.PulledCreated)
			assert.Equal(t, 1, stats.PushedCreated)
			assert.Zero(t, stats.Errors, env.conn.LastSyncError)
			assert.Equal(t, 2, remote.count(t))

			again := env.sync(t)
			assert.Zero(t, again.PulledUpdated+again.PushedUpdated+again.Conflicts+again.Errors, "a second run changes nothing")
		})
	}
}

// @017-US4
func TestContactSyncTellsTheUserWhatWentWrong(t *testing.T) {
	respond := func(status int, body string) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		return srv.URL
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()

	cases := []struct {
		name    string
		address string
		want    error
		detail  string
	}{
		{"a response that is not valid XML", respond(http.StatusMultiStatus, `<d:multistatus xmlns:d="DAV:"><d:response>`), ErrContactSyncInvalidData, "XML syntax error"},
		{"a server error", respond(http.StatusInternalServerError, "<error>database is down</error>"), ErrContactSyncServerError, "500 Internal Server Error: <error>database is down</error>"},
		{"a long server error", respond(http.StatusBadGateway, strings.Repeat("x", 5000)), ErrContactSyncServerError, "xxx […]"},
		{"a server that does not answer", closed.URL, ErrContactSyncUnreachable, ""},
	}
	for _, tc := range cases {
		env := setupContactSyncEnv(t, models.CardDAVDirectionTwoWay, &davRemote{proxyURL: tc.address, path: "/book/"})

		_, err := env.service.SyncConnection(context.Background(), env.db, env.cfg, env.conn, clock.Real())

		assert.ErrorIs(t, err, tc.want, tc.name)
		assert.Contains(t, env.conn.LastSyncError, tc.want.Error(), tc.name)
		assert.Contains(t, env.conn.LastSyncError, tc.detail, tc.name)
		assert.NotContains(t, env.conn.LastSyncError, strings.Repeat("x", 301), tc.name)
	}
}
