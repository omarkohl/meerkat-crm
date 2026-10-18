package controllers

import (
	"meerkat/admin"
	"meerkat/api"
	"meerkat/config"
	"meerkat/models"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupRouter() (*gorm.DB, *gin.Engine) {
	gin.SetMode(gin.ReleaseMode)

	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	db.AutoMigrate(&models.Contact{}, &models.Activity{}, &models.Note{}, models.Relationship{}, models.Reminder{}, models.User{}, models.ApiToken{}, models.CardDAVConnection{}, models.CardDAVContactLink{}, models.CalendarSubscription{}, models.CalendarEventLink{}, models.Webhook{}, models.WebhookDelivery{}, models.EmailOutbox{}, models.Session{}, models.AuditLog{}, models.RecoveryCode{})

	user := models.User{Username: "tester", Password: "password123", Email: "tester@example.com"}
	if err := db.Create(&user).Error; err != nil {
		panic("failed to seed user")
	}

	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})

	return db, router
}

func mountAdmin(r *gin.Engine, db *gorm.DB) {
	group := r.Group("")
	admin.New(admin.Deps{DB: db, Config: &config.Config{}}).Register(api.New(group, r), group)
}

const (
	strongPassword        = "CorrectHorseBattery42!"
	strongPasswordAlt     = "TrulySecurePassphrase99#"
	strongPasswordAnother = "UltraSafePassphrase88$"
)

func cookie(w *httptest.ResponseRecorder, name string) *http.Cookie {
	var found *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			found = c
		}
	}
	return found
}

func doReq(r *gin.Engine, method, path string, hdr ...string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
