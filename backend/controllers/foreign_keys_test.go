package controllers

import (
	adminmod "meerkat/admin"
	"meerkat/api"
	"meerkat/config"
	"meerkat/database"
	"meerkat/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests run against the real migrated schema (InitDB) with
// foreign_keys=ON, as in production, so delete paths that would violate a
// foreign key are caught.
func TestDeletePathsWithForeignKeysEnforced(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db, err := database.InitDB(filepath.Join(t.TempDir(), "fk.db"))
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	admin := models.User{Username: "admin", Password: "password123", Email: "admin@example.com", IsAdmin: true}
	victim := models.User{Username: "victim", Password: "password123", Email: "victim@example.com"}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&victim).Error)

	c1 := models.Contact{UserID: victim.ID, Firstname: "A"}
	c2 := models.Contact{UserID: victim.ID, Firstname: "B"}
	require.NoError(t, db.Create(&c1).Error)
	require.NoError(t, db.Create(&c2).Error)
	require.NoError(t, db.Create(&models.Note{UserID: victim.ID, ContactID: &c1.ID, Content: "n"}).Error)
	require.NoError(t, db.Create(&models.Reminder{UserID: victim.ID, ContactID: &c1.ID, Message: "r", RemindAt: time.Now(), Recurrence: "once"}).Error)
	require.NoError(t, db.Create(&models.Relationship{UserID: victim.ID, ContactID: c1.ID, RelatedContactID: &c2.ID, Name: "Friend", Type: "friend"}).Error)
	act := models.Activity{UserID: victim.ID, Title: "t", Date: time.Now(), Contacts: []models.Contact{c1, c2}}
	require.NoError(t, db.Create(&act).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", victim.ID)
		c.Next()
	})
	mountContacts(router, db, &config.Config{})
	adminRouter := gin.New()
	adminRouter.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", admin.ID)
		c.Next()
	})
	group := adminRouter.Group("")
	adminmod.New(adminmod.Deps{DB: db, Config: &config.Config{}}).Register(api.New(group, adminRouter), group)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/contacts/"+strconv.Itoa(int(c1.ID)), nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/admin/users/"+strconv.Itoa(int(victim.ID)), nil)
	adminRouter.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
