package controllers

import (
	"strings"

	"meerkat/models"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Audit details are machine-readable, not English: ";"-separated "key:value" segments that the
// admin page translates (frontend AuditLogPage). auditValue keeps a free-text value (a token
// or file name) from breaking that format.
func auditValue(s string) string {
	return strings.ReplaceAll(s, ";", ",")
}

// auditFor records an event about the given account (nil for none), with the request's
// address and user agent. It never fails the request.
func auditFor(c *gin.Context, db *gorm.DB, userID *uint, event, detail string) {
	if userID != nil {
		services.WriteUserAudit(db, *userID, event, detail, c.ClientIP(), c.Request.UserAgent())
		return
	}
	services.WriteAudit(db, models.AuditLog{Event: event, IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), Detail: detail})
}

// audit records an event for the authenticated user.
func audit(c *gin.Context, db *gorm.DB, event, detail string) {
	var uid *uint
	if v, ok := c.Get("userID"); ok {
		if id, ok := v.(uint); ok {
			uid = &id
		}
	}
	auditFor(c, db, uid, event, detail)
}
