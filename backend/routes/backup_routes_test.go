package routes

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"meerkat/config"
	"meerkat/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// An administrator's API token must not reach the backup routes: they hold the
// data of every user. Uses the real route table and the real auth middleware.
func TestBackupRoutesRefuseAdminAPIToken(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.ApiToken{}))
	admin := models.User{Username: "root", Email: "root@example.com", Password: "x", IsAdmin: true}
	require.NoError(t, db.Create(&admin).Error)
	plaintext := "meerkat_admintoken123456789"
	require.NoError(t, db.Create(&models.ApiToken{
		UserID: admin.ID, Name: "t", TokenHash: fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext))),
	}).Error)

	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!", BackupDir: filepath.Join(t.TempDir(), "backups")}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})
	RegisterRoutes(r, cfg, db, NewModules(db, cfg, nil))

	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/admin/backups"},
		{"POST", "/api/v1/admin/backups"},
		{"GET", "/api/v1/admin/backups/meerkat-backup-20260101-000000.tar.gz"},
		{"DELETE", "/api/v1/admin/backups/meerkat-backup-20260101-000000.tar.gz"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s: %s", tc.method, tc.path, w.Body.String())
	}
	assert.NoDirExists(t, cfg.BackupDir, "a refused request creates nothing")
}
