package controllers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"meerkat/config"
	apperrors "meerkat/errors"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// LoginInput represents the DTO for login requests
type LoginInput struct {
	Identifier string `json:"identifier"` // Can be username or email
	Email      string `json:"email"`      // Legacy field for backward compatibility
	Password   string `json:"password"`
	// Second step for accounts with two-factor: an authenticator code or a recovery code.
	TOTPCode     string `json:"totp_code"`
	RecoveryCode string `json:"recovery_code"`
}

func LoginUser(context *gin.Context, cfg *config.Config) {
	var input LoginInput
	var foundUser models.User

	err := context.ShouldBindJSON(&input)
	if err != nil {
		apperrors.AbortWithError(context, apperrors.ErrInvalidInput("", err.Error()))
		return
	}

	// Support both "identifier" and legacy "email" field
	identifier := input.Identifier
	if identifier == "" {
		identifier = input.Email
	}

	if identifier == "" {
		apperrors.AbortWithError(context, apperrors.ErrMissingField("identifier"))
		return
	}

	// Normalize to lowercase for case-insensitive matching
	identifier = strings.ToLower(identifier)

	if input.Password == "" {
		apperrors.AbortWithError(context, apperrors.ErrMissingField("password"))
		return
	}

	// Check per-account rate limiting before attempting authentication
	accountLimiter := middleware.GetAccountRateLimiter()
	if isLocked, remainingSecs := accountLimiter.IsLocked(identifier); isLocked {
		context.JSON(http.StatusTooManyRequests, gin.H{
			"error":          "Account temporarily locked",
			"message":        "Too many failed login attempts. Please try again later.",
			"retry_after":    remainingSecs,
			"retry_after_at": middleware.RequestNow(context).Add(time.Duration(remainingSecs) * time.Second).Format(time.RFC3339),
		})
		context.Abort()
		return
	}

	db := context.MustGet("db").(*gorm.DB)

	// Check if identifier contains @ to determine if it's an email or username
	var query *gorm.DB
	if strings.Contains(identifier, "@") {
		query = db.Where("email = ?", identifier)
	} else {
		query = db.Where("username = ?", identifier)
	}

	if err := query.First(&foundUser).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Record failed attempt even for non-existent users to prevent enumeration
			accountLimiter.RecordFailedAttempt(identifier)
			// No user id and no identifier: what was typed may be a password.
			auditFor(context, db, nil, models.AuditLoginFailed, "reason:unknown_account")
			apperrors.AbortWithError(context, apperrors.ErrInvalidCredentials())
		} else {
			apperrors.AbortWithError(context, apperrors.ErrDatabase("Failed to query user").WithError(err))
		}
		return
	}

	// Compare the hashed password
	if err := bcrypt.CompareHashAndPassword([]byte(foundUser.Password), []byte(input.Password)); err != nil {
		// Record failed attempt for password mismatch
		isLocked, lockoutSecs := accountLimiter.RecordFailedAttempt(identifier)
		failedDetail := "reason:wrong_password"
		if isLocked {
			failedDetail = "reason:wrong_password_locked"
		}
		auditFor(context, db, &foundUser.ID, models.AuditLoginFailed, failedDetail)
		if isLocked {
			context.JSON(http.StatusTooManyRequests, gin.H{
				"error":          "Account temporarily locked",
				"message":        "Too many failed login attempts. Please try again later.",
				"retry_after":    lockoutSecs,
				"retry_after_at": middleware.RequestNow(context).Add(time.Duration(lockoutSecs) * time.Second).Format(time.RFC3339),
			})
			context.Abort()
			return
		}
		apperrors.AbortWithError(context, apperrors.ErrInvalidCredentials())
		return
	}

	// Second step. Asking for the code is not a success: the failed-attempt counter is only
	// cleared after the whole login, or the password alone would reset a code-guessing lockout.
	// (OIDC logins never come through here: the identity provider is the second factor.)
	if foundUser.TOTPEnabled {
		if input.TOTPCode == "" && input.RecoveryCode == "" {
			context.JSON(http.StatusOK, gin.H{"totp_required": true})
			return
		}
		result, lockSecs := verifySecondFactor(context, db, foundUser, input.TOTPCode, input.RecoveryCode)
		switch result {
		case factorLocked:
			return
		case factorWrong, factorWrongNowLocked:
			isLocked, lockoutSecs := accountLimiter.RecordFailedAttempt(identifier)
			if result == factorWrongNowLocked {
				isLocked, lockoutSecs = true, max(lockSecs, lockoutSecs)
			}
			detail := "reason:wrong_2fa_code"
			if isLocked {
				detail = "reason:wrong_2fa_code_locked"
			}
			auditFor(context, db, &foundUser.ID, models.AuditLoginFailed, detail)
			if isLocked {
				respondLocked(context, lockoutSecs)
				return
			}
			apperrors.AbortWithError(context, apperrors.ErrInvalidTOTPCode())
			return
		}
	}

	// Successful login - clear any failed attempt tracking
	accountLimiter.RecordSuccessfulLogin(identifier)

	// Create JWT token
	tokenString, err := services.IssueSession(db, foundUser, cfg, context.ClientIP(), context.Request.UserAgent(), middleware.RequestNow(context))
	if err != nil {
		apperrors.AbortWithError(context, apperrors.ErrInternal("Could not generate token").WithError(err))
		return
	}
	auditFor(context, db, &foundUser.ID, models.AuditLogin, "")

	// Set httpOnly cookie with the JWT token
	maxAge := cfg.JWTExpiryHours * 3600 // Convert hours to seconds
	context.SetSameSite(http.SameSiteLaxMode)
	context.SetCookie(
		"auth_token",     // name
		tokenString,      // value
		maxAge,           // maxAge in seconds
		"/",              // path
		cfg.CookieDomain, // domain (empty = current domain)
		cfg.CookieSecure, // secure (true = HTTPS only)
		true,             // httpOnly (not accessible via JavaScript)
	)

	// Return user preferences (token is now in httpOnly cookie)
	context.JSON(http.StatusOK, gin.H{
		"language":    foundUser.Language,
		"date_format": foundUser.DateFormat,
	})
}

// LogoutUser clears the auth cookie to log out the user
func LogoutUser(context *gin.Context, cfg *config.Config) {
	// The route is public: revoke the session named by the cookie, if it is one of ours.
	if raw, err := context.Cookie("auth_token"); err == nil && raw != "" {
		if sid := services.SessionIDFromToken(raw, cfg); sid != "" {
			if v, ok := context.Get("db"); ok {
				revokeSessionOnLogout(context, v.(*gorm.DB), sid)
			}
		}
	}
	context.SetSameSite(http.SameSiteLaxMode)
	context.SetCookie(
		"auth_token",     // name
		"",               // value (empty)
		-1,               // maxAge -1 = delete cookie
		"/",              // path
		cfg.CookieDomain, // domain
		cfg.CookieSecure, // secure
		true,             // httpOnly
	)
	context.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func revokeSessionOnLogout(c *gin.Context, db *gorm.DB, sid string) {
	var session models.Session
	if err := db.Where("public_id = ?", sid).Take(&session).Error; err != nil {
		return
	}
	if revoked, err := services.RevokeSession(db, session.UserID, sid, middleware.RequestNow(c)); err == nil && revoked {
		uid := session.UserID
		auditFor(c, db, &uid, models.AuditLogout, "")
	}
}
