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

type factorResult int

const (
	factorOK factorResult = iota
	factorWrong
	// factorLocked: the account was already locked; the 429 answer is written.
	factorLocked
	// factorWrongNowLocked: this wrong attempt triggered the lockout; nothing is written yet.
	factorWrongNowLocked
)

func respondLocked(c *gin.Context, remainingSecs int) {
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error":          "Account temporarily locked",
		"message":        "Too many failed attempts. Please try again later.",
		"retry_after":    remainingSecs,
		"retry_after_at": totpNow().Add(time.Duration(remainingSecs) * time.Second).Format(time.RFC3339),
	})
	c.Abort()
}

// rejectIfTOTPLocked answers 429 and returns true when the account's second factor is locked.
func rejectIfTOTPLocked(c *gin.Context, userID uint) bool {
	if locked, secs := middleware.GetAccountRateLimiter().IsLocked(totpLimiterKey(userID)); locked {
		respondLocked(c, secs)
		return true
	}
	return false
}

// verifySecondFactor checks a code or recovery code under the per-account lockout. On
// factorLocked the 429 answer is already written; on factorWrong and factorWrongNowLocked
// (secs is the lockout length) the caller answers.
func verifySecondFactor(c *gin.Context, db *gorm.DB, user models.User, code, recovery string) (factorResult, int) {
	cfg := currentConfig(c)
	limiter := middleware.GetAccountRateLimiter()
	key := totpLimiterKey(user.ID)
	if rejectIfTOTPLocked(c, user.ID) {
		return factorLocked, 0
	}
	ok, usedRecovery := services.CheckSecondFactor(db, &cfg, user, code, recovery, totpNow())
	if ok {
		limiter.RecordSuccessfulLogin(key)
		if usedRecovery {
			auditFor(c, db, &user.ID, models.AuditTOTPRecoveryUsed, "")
		}
		return factorOK, 0
	}
	if locked, secs := limiter.RecordFailedAttempt(key); locked {
		return factorWrongNowLocked, secs
	}
	return factorWrong, 0
}

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
