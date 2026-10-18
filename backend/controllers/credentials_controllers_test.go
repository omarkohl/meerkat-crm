package controllers

import (
	"net/http"
	"testing"

	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const testJWTSecret = "unit-test-secret"

func credentialRouter(t *testing.T) (*gorm.DB, *gin.Engine, models.User) {
	t.Helper()
	db, router := setupRouter()
	router.Use(func(c *gin.Context) { c.Set("cfg", config.Config{JWTSecretKey: testJWTSecret}); c.Next() })
	var me models.User
	require.NoError(t, db.First(&me).Error)
	router.PUT("/carddav/connection", middleware.ValidateJSONMiddleware(&models.CardDAVConnectionInput{}), SaveCardDAVConnection)
	router.GET("/carddav/connection", GetCardDAVConnection)
	router.DELETE("/carddav/connection", DeleteCardDAVConnection)
	return db, router, me
}

func TestCardDAVConnectionPasswordHandling(t *testing.T) {
	db, router, me := credentialRouter(t)
	body := map[string]any{"base_url": "https://user:inline@dav.example.com/dav", "address_book_path": "/books/a/", "username": "u", "password": "hunter2"}
	w := doJSON(router, "PUT", "/carddav/connection", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "hunter2")
	assert.NotContains(t, w.Body.String(), "inline", "credentials in the URL are dropped")

	var conn models.CardDAVConnection
	require.NoError(t, db.Where("user_id = ?", me.ID).First(&conn).Error)
	assert.NotContains(t, conn.BaseURL, "inline")
	plain, err := services.DecryptCredential(testJWTSecret, conn.PasswordEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "hunter2", plain)

	got := doJSON(router, "GET", "/carddav/connection", nil)
	assert.Contains(t, got.Body.String(), `"has_password":true`)
	assert.NotContains(t, got.Body.String(), conn.PasswordEncrypted)

	// Relative address book paths and non-http schemes are refused.
	assert.Equal(t, http.StatusBadRequest, doJSON(router, "PUT", "/carddav/connection",
		map[string]any{"base_url": "https://dav.example.com", "address_book_path": "books/a"}).Code)
	assert.Equal(t, http.StatusBadRequest, doJSON(router, "PUT", "/carddav/connection",
		map[string]any{"base_url": "file:///etc", "address_book_path": "/x"}).Code)

	// Same server: keeping the stored password when none is sent.
	require.Equal(t, http.StatusOK, doJSON(router, "PUT", "/carddav/connection",
		map[string]any{"base_url": "https://dav.example.com/other", "address_book_path": "/books/b/", "username": "u"}).Code)
	require.NoError(t, db.Where("user_id = ?", me.ID).First(&conn).Error)
	assert.NotEmpty(t, conn.PasswordEncrypted)

	// Another server, a downgrade to http or another port: refused, the stored password stays.
	for _, target := range []string{"https://evil.example.net/dav", "http://dav.example.com/dav", "https://dav.example.com:8443/dav"} {
		w := doJSON(router, "PUT", "/carddav/connection",
			map[string]any{"base_url": target, "address_book_path": "/books/b/", "username": "u"})
		require.Equal(t, http.StatusBadRequest, w.Code, target)
		assert.Contains(t, w.Body.String(), "password required")
		require.NoError(t, db.Where("user_id = ?", me.ID).First(&conn).Error)
		assert.NotEmpty(t, conn.PasswordEncrypted)
		assert.Equal(t, "https://dav.example.com/other", conn.BaseURL)
	}
	require.Equal(t, http.StatusOK, doJSON(router, "PUT", "/carddav/connection",
		map[string]any{"base_url": "https://evil.example.net/dav", "address_book_path": "/books/b/", "username": "u", "password": "new"}).Code)
}

func TestCardDAVSwitchingAddressBookDropsLinks(t *testing.T) {
	db, router, me := credentialRouter(t)
	require.Equal(t, http.StatusCreated, doJSON(router, "PUT", "/carddav/connection",
		map[string]any{"base_url": "https://dav.example.com", "address_book_path": "/a/", "password": "p"}).Code)
	var conn models.CardDAVConnection
	require.NoError(t, db.Where("user_id = ?", me.ID).First(&conn).Error)
	require.NoError(t, db.Model(&conn).Update("sync_token", "tok").Error)
	require.NoError(t, db.Create(&models.CardDAVContactLink{ConnectionID: conn.ID, UserID: me.ID, ContactID: 1, RemoteUID: "u1", RemotePath: "/a/1.vcf"}).Error)

	require.Equal(t, http.StatusOK, doJSON(router, "PUT", "/carddav/connection",
		map[string]any{"base_url": "https://dav.example.com", "address_book_path": "/b/"}).Code)
	var n int64
	db.Model(&models.CardDAVContactLink{}).Where("connection_id = ?", conn.ID).Count(&n)
	assert.Zero(t, n)
	require.NoError(t, db.First(&conn, conn.ID).Error)
	assert.Empty(t, conn.SyncToken)
}
