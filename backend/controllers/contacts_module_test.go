package controllers

import (
	"meerkat/activities"
	"meerkat/api"
	"meerkat/config"
	"meerkat/contacts"
	"meerkat/notes"
	"meerkat/relationships"
	"meerkat/trash"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// mountContacts registers the contacts module's huma operations on router, the way
// routes.RegisterRoutes does, for tests of other controllers that need DELETE /contacts/:id.
func mountContacts(router *gin.Engine, db *gorm.DB, cfg *config.Config) {
	contacts.New(contacts.Deps{DB: db, Config: cfg}).Register(api.New(router.Group(""), router), router)
}

func mountNotes(router *gin.Engine, db *gorm.DB, cfg *config.Config) {
	notes.New(notes.Deps{DB: db, Config: cfg}).Register(api.New(router.Group(""), router), router)
}

func mountActivities(router *gin.Engine, db *gorm.DB, cfg *config.Config) {
	activities.New(activities.Deps{DB: db, Config: cfg}).Register(api.New(router.Group(""), router), router)
}

func mountRelationships(router *gin.Engine, db *gorm.DB, cfg *config.Config) {
	relationships.New(relationships.Deps{DB: db, Config: cfg}).Register(api.New(router.Group(""), router), router)
}

func mountTrash(router *gin.Engine, db *gorm.DB, cfg *config.Config) {
	trash.New(trash.Deps{DB: db, Config: cfg}).Register(api.New(router.Group(""), router), router)
}
