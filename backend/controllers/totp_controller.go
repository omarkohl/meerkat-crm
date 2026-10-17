package controllers

import (
	"errors"
	"fmt"
	"meerkat/clock"
	"net/http"
	"time"

	apperrors "meerkat/errors"
	"meerkat/idutil"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func totpLimiterKey(userID uint) string { return middleware.SecondFactorKey(userID) }

// ResetUserTOTP lets an admin switch off a user's two-factor (lost phone and recovery
// codes). The user's sessions end, since whoever asked for the reset is not verified here.
func ResetUserTOTP(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	id, err := idutil.Parse(c.Param("id"))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("id", "Invalid user ID"))
		return
	}
	var user models.User
	if err := db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("User"))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("query user").WithError(err))
		}
		return
	}
	if !user.TOTPEnabled {
		apperrors.AbortWithError(c, apperrors.ErrConflict("Two-factor authentication is not enabled for this user"))
		return
	}
	if err := services.DisableTOTP(db, user.ID); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("update").WithError(err))
		return
	}
	if err := services.RevokeAllSessions(db, user.ID, middleware.RequestNow(c)); err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("update").WithError(err))
		return
	}
	middleware.GetAccountRateLimiter().RecordSuccessfulLogin(totpLimiterKey(user.ID))
	audit(c, db, models.AuditTOTPAdminReset, fmt.Sprintf("user:%s#%d", auditValue(user.Username), user.ID))
	c.JSON(http.StatusOK, gin.H{"message": "Two-factor authentication reset"})
}

// totpNow is the time codes are checked against. Authenticator apps compute them from the
// real time, so FAKE_NOW must not move it or two-factor users would be locked out.
func totpNow() time.Time { return clock.Real().Now() }
