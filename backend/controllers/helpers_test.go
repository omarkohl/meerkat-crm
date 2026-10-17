package controllers

import (
	"meerkat/models"
	"net/http"

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

func withValidated(factory func() any) gin.HandlerFunc {
	return func(c *gin.Context) {
		payload := factory()
		if payload != nil {
			if err := c.ShouldBindJSON(payload); err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.Set("validated", payload)
		}
		c.Next()
	}
}

const (
	strongPassword        = "CorrectHorseBattery42!"
	strongPasswordAlt     = "TrulySecurePassphrase99#"
	strongPasswordAnother = "UltraSafePassphrase88$"
)
