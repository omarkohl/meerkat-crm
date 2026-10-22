package admin_test

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"meerkat/admin"
	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
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
	db     *gorm.DB
	dir    string
	photos string
	admin  models.User
	plain  models.User
	// reminders stands in for the daily reminder job.
	reminders func(clock.Clock) error
}

func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.AuditLog{}, &models.Session{}, &models.Activity{}, &models.Note{},
		&models.Relationship{}, &models.Reminder{}, &models.ApiToken{}, &models.IcsFeed{}, &models.CardDAVConnection{}, &models.CardDAVContactLink{},
		&models.CalendarSubscription{}, &models.CalendarEventLink{}, &models.Webhook{}, &models.WebhookDelivery{}, &models.EmailOutbox{}, &models.RecoveryCode{}, &models.Image{}))
	e := &env{reminders: func(clock.Clock) error { return nil }, db: db, dir: filepath.Join(t.TempDir(), "backups"), photos: t.TempDir()}
	e.admin = models.User{Username: "root", Email: "root@example.com", Password: "x", IsAdmin: true}
	e.plain = models.User{Username: "plain", Email: "plain@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.admin).Error)
	require.NoError(t, db.Create(&e.plain).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: e.plain.ID, Firstname: "Secret"}).Error)
	require.NoError(t, os.WriteFile(filepath.Join(e.photos, "p.jpg"), []byte("img"), 0o600))
	return e
}

func (e *env) config() config.Config {
	return config.Config{BackupDir: e.dir, ProfilePhotoDir: e.photos, BackupEnabled: true, BackupTime: "02:30", BackupRetentionCount: 2, AuditLogRetentionDays: 90}
}

// as mounts the module behind the admin middleware for the given user (none: unauthenticated).
func (e *env) as(userID uint, cfg config.Config, wrap ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ClockMiddleware(clock.NewFake(now)))
	r.Use(wrap...)
	group := r.Group("")
	group.Use(func(c *gin.Context) {
		c.Set("db", e.db)
		if userID != 0 {
			c.Set("userID", userID)
		}
		c.Next()
	}, middleware.AdminMiddleware())
	admin.New(admin.Deps{DB: e.db, Config: &cfg, SendReminders: e.reminders}).Register(api.New(group, r), group)
	return r
}

func do(r http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m), w.Body.String())
	return m
}

// @003-US4
// @011-US2
func TestEveryRouteRefusesAPlainUserAndTouchesNothing(t *testing.T) {
	e := newEnv(t)
	created := do(e.as(e.admin.ID, e.config()), http.MethodPost, "/admin/backups")
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	name := decode(t, created)["name"].(string)
	services.WriteAudit(e.db, models.AuditLog{Event: models.AuditLogin, Username: "secret-name"})

	user := e.as(e.plain.ID, e.config())
	for _, tc := range []struct{ method, path string }{
		{"GET", "/admin/backups"}, {"POST", "/admin/backups"}, {"GET", "/admin/backups/" + name}, {"DELETE", "/admin/backups/" + name},
		{"GET", "/admin/audit-log"}, {"GET", "/admin/system-status"},
	} {
		w := do(user, tc.method, tc.path)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s", tc.method, tc.path)
		assert.NotContains(t, w.Body.String(), "Secret")
		assert.NotContains(t, w.Body.String(), "secret-name")
	}
	list, err := os.ReadDir(e.dir)
	require.NoError(t, err)
	assert.Len(t, list, 1, "a plain user neither created nor deleted anything")

	for _, path := range []string{"/admin/backups", "/admin/audit-log", "/admin/system-status"} {
		assert.Equal(t, http.StatusUnauthorized, do(e.as(0, e.config()), "GET", path).Code, path)
	}
}

// @011-US1
func TestBackupsCanBeCreatedListedDownloadedAndDeleted(t *testing.T) {
	e := newEnv(t)
	r := e.as(e.admin.ID, e.config())

	list := decode(t, do(r, "GET", "/admin/backups"))
	assert.Equal(t, []any{}, list["backups"])
	assert.Equal(t, true, list["enabled"])
	assert.Equal(t, "02:30", list["time"])
	assert.EqualValues(t, 2, list["retention_count"])
	assert.Equal(t, e.dir, list["directory"])

	w := do(r, "POST", "/admin/backups")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decode(t, w)
	name := created["name"].(string)
	assert.Regexp(t, `^meerkat-backup-20260310-153000\.tar\.gz$`, name)
	assert.Positive(t, created["size"])
	assert.Equal(t, now.Format(time.RFC3339), created["created_at"])

	assert.Len(t, decode(t, do(r, "GET", "/admin/backups"))["backups"], 1)

	w = do(r, "GET", "/admin/backups/"+name)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), name)
	assert.Contains(t, w.Header().Get("Cache-Control"), "no-store")
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	gz, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, h.Name)
	}
	assert.ElementsMatch(t, []string{"meerkat.db", "photos/p.jpg"}, names)

	w = do(r, "DELETE", "/admin/backups/"+name)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Backup deleted", decode(t, w)["message"])
	assert.Equal(t, http.StatusNotFound, do(r, "GET", "/admin/backups/"+name).Code)
	assert.Equal(t, http.StatusNotFound, do(r, "DELETE", "/admin/backups/"+name).Code)
}

func TestCreatingBackupsKeepsOnlyTheRetentionCount(t *testing.T) {
	e := newEnv(t)
	r := e.as(e.admin.ID, e.config())
	for range 3 {
		require.Equal(t, http.StatusCreated, do(r, "POST", "/admin/backups").Code)
	}
	entries, err := os.ReadDir(e.dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestBackupNamesCannotLeaveTheBackupDirectory(t *testing.T) {
	e := newEnv(t)
	r := e.as(e.admin.ID, e.config())
	outside := filepath.Join(filepath.Dir(e.dir), "secret.tar.gz")
	require.NoError(t, os.MkdirAll(e.dir, 0o700))
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(e.dir, "notes.txt"), []byte("n"), 0o600))

	for _, name := range []string{"..", "..%2Fsecret.tar.gz", "%2e%2e%2fsecret.tar.gz", "secret.tar.gz", "notes.txt", "meerkat-backup-1.tar.gz", "meerkat-backup-20260101-000000.tar.gz"} {
		for _, method := range []string{"GET", "DELETE"} {
			w := do(r, method, "/admin/backups/"+name)
			assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, w.Code, "%s %s", method, name)
			assert.False(t, strings.Contains(w.Body.String(), "secret"), "%s %s", method, name)
		}
	}
	assert.FileExists(t, outside)
	assert.FileExists(t, filepath.Join(e.dir, "notes.txt"))
}

func TestBackupActionsAreAudited(t *testing.T) {
	e := newEnv(t)
	r := e.as(e.admin.ID, e.config())
	name := decode(t, do(r, "POST", "/admin/backups"))["name"].(string)
	do(r, "GET", "/admin/backups/"+name)
	do(r, "DELETE", "/admin/backups/"+name)
	do(r, "GET", "/admin/backups/"+name) // refused, not audited

	var entries []models.AuditLog
	require.NoError(t, e.db.Order("id").Find(&entries).Error)
	require.Len(t, entries, 3)
	for i, event := range []string{models.AuditBackupCreated, models.AuditBackupDownloaded, models.AuditBackupDeleted} {
		assert.Equal(t, event, entries[i].Event)
		assert.Equal(t, "backup:"+name, entries[i].Detail)
		assert.Equal(t, "root", entries[i].Username)
		require.NotNil(t, entries[i].UserID)
		assert.Equal(t, e.admin.ID, *entries[i].UserID)
	}
}

type deadlineWriter struct {
	gin.ResponseWriter
	deadlines *[]time.Time
}

func (w deadlineWriter) SetWriteDeadline(t time.Time) error {
	*w.deadlines = append(*w.deadlines, t)
	return nil
}

func TestCreatingAndDownloadingABackupLiftTheServersWriteTimeout(t *testing.T) {
	e := newEnv(t)
	var deadlines []time.Time
	r := e.as(e.admin.ID, e.config(), func(c *gin.Context) {
		c.Writer = deadlineWriter{ResponseWriter: c.Writer, deadlines: &deadlines}
	})

	w := do(r, "POST", "/admin/backups")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Len(t, deadlines, 1)
	assert.True(t, deadlines[0].IsZero(), "create removes the write deadline")

	require.Equal(t, http.StatusOK, do(r, "GET", "/admin/backups/"+decode(t, w)["name"].(string)).Code)
	require.Len(t, deadlines, 2)
	assert.True(t, deadlines[1].IsZero(), "download removes the write deadline")
}

// @003-US3
func TestAuditLogListsFiltersAndPagesNewestFirst(t *testing.T) {
	e := newEnv(t)
	r := e.as(e.admin.ID, e.config())
	for i := range 5 {
		services.WriteAudit(e.db, models.AuditLog{Event: models.AuditLogin, Username: "Alice", Detail: fmt.Sprint(i), IP: "10.0.0.1", UserAgent: "ua"})
	}
	services.WriteAudit(e.db, models.AuditLog{Event: models.AuditLoginFailed, Username: "bob"})
	services.WriteAudit(e.db, models.AuditLog{Event: models.AuditExport, Username: "alice_b", UserID: &e.plain.ID})

	get := func(q string) map[string]any {
		w := do(r, "GET", "/admin/audit-log"+q)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return decode(t, w)
	}
	all := get("")
	assert.EqualValues(t, 7, all["total"])
	assert.EqualValues(t, 1, all["page"])
	assert.EqualValues(t, 25, all["limit"])
	assert.EqualValues(t, 90, all["retention_days"])
	assert.Len(t, all["events"], len(models.AuditEvents))
	first := all["entries"].([]any)[0].(map[string]any)
	assert.Equal(t, models.AuditExport, first["event"], "newest first")
	assert.EqualValues(t, e.plain.ID, first["user_id"])
	last := all["entries"].([]any)[6].(map[string]any)
	assert.Nil(t, last["user_id"])
	assert.Equal(t, "10.0.0.1", last["ip"])
	assert.Equal(t, "ua", last["user_agent"])

	assert.EqualValues(t, 1, get("?event=login_failed")["total"])
	assert.EqualValues(t, 5, get("?event=login")["total"], "exact event match")
	assert.EqualValues(t, 6, get("?username=ALI")["total"], "case-insensitive substring")
	assert.EqualValues(t, 5, get("?event=login&username=alice")["total"])
	assert.EqualValues(t, 0, get("?event=login&username=bob")["total"])
	assert.EqualValues(t, 0, get("?username=%25")["total"], "wildcards are literal")
	assert.Equal(t, []any{}, get("?event=nothing")["entries"])

	page := get("?limit=3&page=2")
	assert.Len(t, page["entries"], 3)
	assert.EqualValues(t, 7, page["total"])
	assert.EqualValues(t, 2, page["page"])
	bad := get("?limit=abc&page=-1")
	assert.EqualValues(t, 25, bad["limit"])
	assert.EqualValues(t, 1, bad["page"])
	assert.EqualValues(t, 100, get("?limit=5000")["limit"])
}

// @012-US1
func TestSystemStatusWarnsOnMultiUserInstancesThatAllowPrivateAddresses(t *testing.T) {
	e := newEnv(t)
	status := func(cfg config.Config) map[string]any {
		w := do(e.as(e.admin.ID, cfg), "GET", "/admin/system-status")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return decode(t, w)
	}
	out := status(config.Config{})
	assert.Equal(t, true, out["private_url_warning"])
	assert.Equal(t, true, out["private_urls_allowed"])
	assert.Equal(t, "BLOCK_PRIVATE_URLS", out["setting"])
	assert.EqualValues(t, 2, out["user_count"])
	assert.Equal(t, []any{"webhooks", "caldav", "carddav", "monica"}, out["features"])

	all := config.Config{WebhookBlockPrivateURLs: true, CalDAVBlockPrivateURLs: true, CardDAVBlockPrivateURLs: true, MonicaBlockPrivateURLs: true}
	blocked := status(all)
	assert.Equal(t, false, blocked["private_url_warning"])
	assert.Equal(t, []any{}, blocked["features"])
	all.MonicaBlockPrivateURLs = false
	assert.Equal(t, true, status(all)["private_url_warning"])
	assert.Equal(t, []any{"monica"}, status(all)["features"])

	require.NoError(t, e.db.Delete(&e.plain).Error)
	out = status(config.Config{})
	assert.Equal(t, false, out["private_url_warning"], "a single-user instance is not warned; deleted users do not count")
	assert.EqualValues(t, 1, out["user_count"])
}

// @023-US3
func TestSystemStatusCountsTheImagesOfAllUsers(t *testing.T) {
	e := newEnv(t)
	for _, u := range []uint{e.admin.ID, e.plain.ID} {
		require.NoError(t, e.db.Create(&models.Image{UserID: u, File: "a", ThumbFile: "b", Mime: "image/webp", Width: 1, Height: 1, Bytes: 1000}).Error)
	}
	w := do(e.as(e.admin.ID, config.Config{}), "GET", "/admin/system-status")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	out := decode(t, w)
	assert.EqualValues(t, 2, out["image_count"])
	assert.EqualValues(t, 2000, out["image_bytes"])
}

func TestThePrivateURLWarningIsAvailableForStartup(t *testing.T) {
	e := newEnv(t)
	warn, users, err := admin.New(admin.Deps{DB: e.db, Config: &config.Config{}}).PrivateURLWarning(t.Context())
	require.NoError(t, err)
	assert.True(t, warn)
	assert.EqualValues(t, 2, users)
	warn, _, _ = admin.New(admin.Deps{DB: e.db, Config: &config.Config{WebhookBlockPrivateURLs: true, CalDAVBlockPrivateURLs: true, CardDAVBlockPrivateURLs: true, MonicaBlockPrivateURLs: true}}).PrivateURLWarning(t.Context())
	assert.False(t, warn)
}

func TestTheSpecDescribesTheOperationsAtTheirFullPaths(t *testing.T) {
	e := newEnv(t)
	r := gin.New()
	a := api.New(r.Group(""), r)
	admin.New(admin.Deps{DB: e.db}).Register(a, r.Group(""))
	for _, path := range []string{"/admin/system-status", "/admin/audit-log", "/admin/backups", "/admin/backups/{name}", "/admin/users", "/admin/users/{id}", "/admin/users/{id}/2fa/reset", "/admin/trigger-reminders"} {
		assert.Contains(t, a.OpenAPI().Paths, path)
	}
}
