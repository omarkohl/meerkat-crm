package controllers

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"meerkat/config"
	apperrors "meerkat/errors"
	"meerkat/logger"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func RegisterUser(cfg *config.Config) gin.HandlerFunc {
	return func(context *gin.Context) {
		if cfg.RegistrationDisabled {
			context.JSON(http.StatusForbidden, gin.H{"error": gin.H{
				"code":    "registration_disabled",
				"message": "Registration is disabled.",
			}})
			return
		}

		// Get validated registration input from middleware
		// Uses UserRegistrationInput DTO which intentionally excludes IsAdmin to prevent mass assignment
		input, err := middleware.GetValidated[models.UserRegistrationInput](context)
		if err != nil {
			apperrors.AbortWithError(context, err)
			return
		}

		hashedPassword, hashErr := services.HashPassword(input.Password)
		if hashErr != nil {
			if errors.Is(hashErr, services.ErrPasswordTooLong) {
				apperrors.AbortWithError(context, apperrors.ErrValidation(hashErr.Error()))
			} else {
				apperrors.AbortWithError(context, apperrors.ErrInternal("Could not hash password").WithError(hashErr))
			}
			return
		}

		db := context.MustGet("db").(*gorm.DB)

		// Grant admin to the first registered user
		var userCount int64
		db.Model(&models.User{}).Count(&userCount)

		user := models.User{
			Username: strings.ToLower(input.Username),
			Email:    strings.ToLower(input.Email),
			Password: hashedPassword,
			Language: input.Language,
			IsAdmin:  userCount == 0,
		}

		if err := db.Create(&user).Error; err != nil {
			apperrors.AbortWithError(context, apperrors.ErrAlreadyExists("User").WithDetails("email", user.Email))
			return
		}

		context.JSON(http.StatusCreated, gin.H{"message": "User registered successfully"})
	}
}

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

// CheckPasswordStrength evaluates password strength without registration
func CheckPasswordStrength(context *gin.Context) {
	var request struct {
		Password string `json:"password" binding:"required"`
	}

	if err := context.ShouldBindJSON(&request); err != nil {
		apperrors.AbortWithError(context, apperrors.ErrMissingField("password"))
		return
	}

	strength := middleware.EvaluatePasswordStrength(request.Password)
	context.JSON(http.StatusOK, strength)
}

// RequestPasswordReset generates a reset token and sends instructions to the user.
func RequestPasswordReset(context *gin.Context, cfg *config.Config) {
	// Check if demo mode is enabled - password changes are disabled in demo
	if os.Getenv("DEMO_MODE") == "true" {
		apperrors.AbortWithError(context, apperrors.ErrForbidden("Password changes are disabled in demo mode"))
		return
	}

	log := logger.FromContext(context)

	validated, exists := context.Get("validated")
	if !exists {
		apperrors.AbortWithError(context, apperrors.ErrInvalidInput("", "validation data not found"))
		return
	}

	inputPtr, ok := validated.(*models.PasswordResetRequestInput)
	if !ok || inputPtr == nil {
		apperrors.AbortWithError(context, apperrors.ErrInvalidInput("", "invalid validation data type"))
		return
	}
	input := *inputPtr

	// Normalize email to lowercase for case-insensitive matching
	email := strings.ToLower(input.Email)

	db := context.MustGet("db").(*gorm.DB)

	var user models.User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			context.JSON(http.StatusOK, gin.H{"message": "If an account exists, password reset instructions were sent"})
			return
		}

		log.Error().Err(err).Msg("Failed to lookup user for password reset")
		apperrors.AbortWithError(context, apperrors.ErrDatabase("query user").WithError(err))
		return
	}

	token, hash, err := services.GeneratePasswordResetToken()
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate password reset token")
		apperrors.AbortWithError(context, apperrors.ErrInternal("Could not generate password reset token").WithError(err))
		return
	}

	requested := middleware.RequestNow(context)
	expires := services.PasswordResetExpiry(requested)

	user.PasswordResetTokenHash = &hash
	user.PasswordResetExpiresAt = &expires
	user.PasswordResetRequestedAt = &requested

	if err := db.Save(&user).Error; err != nil {
		log.Error().Err(err).Uint("user_id", user.ID).Msg("Failed to persist password reset token")
		apperrors.AbortWithError(context, apperrors.ErrDatabase("update user").WithError(err))
		return
	}

	if err := services.SendPasswordResetEmail(db, user.ID, user.Email, token, user.Language, cfg, middleware.RequestClock(context)); err != nil {
		log.Error().Err(err).Uint("user_id", user.ID).Msg("Failed to send password reset email")
		apperrors.AbortWithError(context, apperrors.ErrExternal("email", "Failed to queue password reset email").WithError(err))
		return
	}

	context.JSON(http.StatusOK, gin.H{"message": "If an account exists, password reset instructions were sent"})
}

// ConfirmPasswordReset validates the token and updates the password.
func ConfirmPasswordReset(context *gin.Context) {
	// Check if demo mode is enabled - password changes are disabled in demo
	if os.Getenv("DEMO_MODE") == "true" {
		apperrors.AbortWithError(context, apperrors.ErrForbidden("Password changes are disabled in demo mode"))
		return
	}

	log := logger.FromContext(context)

	validated, exists := context.Get("validated")
	if !exists {
		apperrors.AbortWithError(context, apperrors.ErrInvalidInput("", "validation data not found"))
		return
	}

	inputPtr, ok := validated.(*models.PasswordResetConfirmInput)
	if !ok || inputPtr == nil {
		apperrors.AbortWithError(context, apperrors.ErrInvalidInput("", "invalid validation data type"))
		return
	}
	input := *inputPtr

	db := context.MustGet("db").(*gorm.DB)

	tokenHash := services.HashPasswordResetToken(input.Token)

	var user models.User
	if err := db.Where("password_reset_token_hash = ?", tokenHash).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(context, apperrors.ErrInvalidInput("token", "Password reset token is invalid or expired"))
			return
		}

		log.Error().Err(err).Msg("Failed to lookup password reset token")
		apperrors.AbortWithError(context, apperrors.ErrDatabase("query user").WithError(err))
		return
	}

	if user.PasswordResetExpiresAt == nil || middleware.RequestNow(context).After(*user.PasswordResetExpiresAt) {
		user.PasswordResetTokenHash = nil
		user.PasswordResetExpiresAt = nil
		user.PasswordResetRequestedAt = nil
		if err := db.Save(&user).Error; err != nil {
			log.Error().Err(err).Uint("user_id", user.ID).Msg("Failed to clear expired reset token")
		}
		apperrors.AbortWithError(context, apperrors.ErrInvalidInput("token", "Password reset token is invalid or expired"))
		return
	}

	hashedPassword, err := services.HashPassword(input.Password)
	if err != nil {
		if errors.Is(err, services.ErrPasswordTooLong) {
			apperrors.AbortWithError(context, apperrors.ErrValidation(err.Error()))
		} else {
			log.Error().Err(err).Msg("Failed to hash password during reset")
			apperrors.AbortWithError(context, apperrors.ErrInternal("Could not hash password").WithError(err))
		}
		return
	}

	user.Password = hashedPassword
	user.PasswordResetTokenHash = nil
	user.PasswordResetExpiresAt = nil
	user.PasswordResetRequestedAt = nil

	// A reset is what you do when the password may be known to someone else: the new
	// password and the end of every session are one change, or the request fails.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&user).Error; err != nil {
			return err
		}
		return services.RevokeAllSessions(tx, user.ID, middleware.RequestNow(context))
	}); err != nil {
		log.Error().Err(err).Uint("user_id", user.ID).Msg("Failed to persist password reset")
		apperrors.AbortWithError(context, apperrors.ErrDatabase("update user").WithError(err))
		return
	}

	auditFor(context, db, &user.ID, models.AuditPasswordReset, "")

	context.JSON(http.StatusOK, gin.H{"message": "Password reset successful"})
}

// ChangePassword lets authenticated users rotate their password.
func ChangePassword(context *gin.Context) {
	// Check if demo mode is enabled - password changes are disabled in demo
	if os.Getenv("DEMO_MODE") == "true" {
		apperrors.AbortWithError(context, apperrors.ErrForbidden("Password changes are disabled in demo mode"))
		return
	}

	log := logger.FromContext(context)

	validated, exists := context.Get("validated")
	if !exists {
		apperrors.AbortWithError(context, apperrors.ErrInvalidInput("", "validation data not found"))
		return
	}

	inputPtr, ok := validated.(*models.ChangePasswordInput)
	if !ok || inputPtr == nil {
		apperrors.AbortWithError(context, apperrors.ErrInvalidInput("", "invalid validation data type"))
		return
	}
	input := *inputPtr

	usernameValue, exists := context.Get("username")
	if !exists {
		apperrors.AbortWithError(context, apperrors.ErrUnauthorized("Authentication required"))
		return
	}

	username, ok := usernameValue.(string)
	if !ok || username == "" {
		apperrors.AbortWithError(context, apperrors.ErrUnauthorized("Authentication required"))
		return
	}

	db := context.MustGet("db").(*gorm.DB)

	var user models.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(context, apperrors.ErrUnauthorized("Authentication required"))
			return
		}

		log.Error().Err(err).Msg("Failed to lookup user for password change")
		apperrors.AbortWithError(context, apperrors.ErrDatabase("query user").WithError(err))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.CurrentPassword)); err != nil {
		apperrors.AbortWithError(context, apperrors.ErrInvalidInput("current_password", "Current password is incorrect"))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.NewPassword)); err == nil {
		apperrors.AbortWithError(context, apperrors.ErrInvalidInput("new_password", "New password must differ from current password"))
		return
	}

	hashedPassword, err := services.HashPassword(input.NewPassword)
	if err != nil {
		if errors.Is(err, services.ErrPasswordTooLong) {
			apperrors.AbortWithError(context, apperrors.ErrValidation(err.Error()))
		} else {
			log.Error().Err(err).Msg("Failed to hash password during change")
			apperrors.AbortWithError(context, apperrors.ErrInternal("Could not hash password").WithError(err))
		}
		return
	}

	user.Password = hashedPassword
	user.PasswordResetTokenHash = nil
	user.PasswordResetExpiresAt = nil
	user.PasswordResetRequestedAt = nil

	// New password and the end of all other sessions are one change, or the request fails.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&user).Error; err != nil {
			return err
		}
		return services.RevokeAllSessions(tx, user.ID, middleware.RequestNow(context))
	}); err != nil {
		log.Error().Err(err).Uint("user_id", user.ID).Msg("Failed to persist password change")
		apperrors.AbortWithError(context, apperrors.ErrDatabase("update user").WithError(err))
		return
	}

	// Every other browser is signed out; this one gets a fresh session.
	if cfg := currentConfig(context); cfg.JWTSecretKey != "" && !context.GetBool("isAPIToken") {
		fresh := user
		fresh.TokenVersion++
		if token, err := services.IssueSession(db, fresh, &cfg, context.ClientIP(), context.Request.UserAgent(), middleware.RequestNow(context)); err == nil {
			context.SetSameSite(http.SameSiteLaxMode)
			context.SetCookie("auth_token", token, cfg.JWTExpiryHours*3600, "/", cfg.CookieDomain, cfg.CookieSecure, true)
		} else {
			log.Error().Err(err).Msg("Failed to start a new session after password change")
		}
	}
	auditFor(context, db, &user.ID, models.AuditPasswordChanged, "")

	context.JSON(http.StatusOK, gin.H{"message": "Password updated successfully"})
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
