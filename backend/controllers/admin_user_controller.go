package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"meerkat/config"
	apperrors "meerkat/errors"
	"meerkat/idutil"
	"meerkat/logger"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TriggerReminders manually triggers the reminder email job (admin only)
func TriggerReminders(c *gin.Context, cfg config.Config) {
	log := logger.FromContext(c)
	db := c.MustGet("db").(*gorm.DB)

	if err := services.SendReminders(db, cfg, middleware.RequestClock(c)); err != nil {
		log.Error().Err(err).Msg("Failed to trigger reminder emails")
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to send reminder emails").WithError(err))
		return
	}

	audit(c, db, models.AuditAdminReminders, "")
	c.JSON(http.StatusOK, gin.H{"message": "Reminder emails sent successfully"})
}

// ListUsers returns a paginated list of all users (admin only)
func ListUsers(c *gin.Context) {
	log := logger.FromContext(c)
	db := c.MustGet("db").(*gorm.DB)
	pagination := GetPaginationParams(c)

	var total int64
	if err := db.Model(&models.User{}).Count(&total).Error; err != nil {
		log.Error().Err(err).Msg("Failed to count users")
		apperrors.AbortWithError(c, apperrors.ErrDatabase("count users").WithError(err))
		return
	}

	var users []models.User
	if err := db.Order("id ASC").Offset(pagination.Offset).Limit(pagination.Limit).Find(&users).Error; err != nil {
		log.Error().Err(err).Msg("Failed to list users")
		apperrors.AbortWithError(c, apperrors.ErrDatabase("list users").WithError(err))
		return
	}

	userResponses := make([]models.AdminUserResponse, len(users))
	for i, user := range users {
		userResponses[i] = models.AdminUserResponse{
			ID:         user.ID,
			Username:   user.Username,
			Email:      user.Email,
			Language:   user.Language,
			DateFormat: user.DateFormat,
			IsAdmin:    user.IsAdmin,
			CreatedAt:  user.CreatedAt,
			UpdatedAt:  user.UpdatedAt,

			TwoFactorEnabled: user.TOTPEnabled,
		}
	}

	totalPages := int(total) / pagination.Limit
	if int(total)%pagination.Limit > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, models.AdminUsersListResponse{
		Users:      userResponses,
		Total:      total,
		Page:       pagination.Page,
		Limit:      pagination.Limit,
		TotalPages: totalPages,
	})
}

// GetUser returns a single user by ID (admin only)
func GetUser(c *gin.Context) {
	log := logger.FromContext(c)
	db := c.MustGet("db").(*gorm.DB)

	idStr := c.Param("id")
	id, err := idutil.Parse(idStr)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("id", "Invalid user ID"))
		return
	}

	var user models.User
	if err := db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("User"))
			return
		}
		log.Error().Err(err).Uint("user_id", id).Msg("Failed to get user")
		apperrors.AbortWithError(c, apperrors.ErrDatabase("get user").WithError(err))
		return
	}

	c.JSON(http.StatusOK, models.AdminUserResponse{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		Language:   user.Language,
		DateFormat: user.DateFormat,
		IsAdmin:    user.IsAdmin,
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,

		TwoFactorEnabled: user.TOTPEnabled,
	})
}

// updates a user's information (admin only)
func UpdateUser(c *gin.Context) {
	log := logger.FromContext(c)
	db := c.MustGet("db").(*gorm.DB)

	currentUserID, ok := currentUserID(c)
	if !ok {
		return
	}

	idStr := c.Param("id")
	id, err := idutil.Parse(idStr)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("id", "Invalid user ID"))
		return
	}

	input, appErr := middleware.GetValidated[models.AdminUserUpdateInput](c)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	var user models.User
	if err := db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("User"))
			return
		}
		log.Error().Err(err).Uint("user_id", id).Msg("Failed to get user for update")
		apperrors.AbortWithError(c, apperrors.ErrDatabase("get user").WithError(err))
		return
	}

	// Prevent admin from removing their own admin status
	if input.IsAdmin != nil && !*input.IsAdmin && user.ID == currentUserID {
		apperrors.AbortWithError(c, apperrors.ErrForbidden("Cannot remove your own admin status"))
		return
	}

	// Check if trying to remove the last admin
	if input.IsAdmin != nil && !*input.IsAdmin && user.IsAdmin {
		var adminCount int64
		if err := db.Model(&models.User{}).Where("is_admin = ?", true).Count(&adminCount).Error; err != nil {
			log.Error().Err(err).Msg("Failed to count admins")
			apperrors.AbortWithError(c, apperrors.ErrDatabase("count admins").WithError(err))
			return
		}
		if adminCount <= 1 {
			apperrors.AbortWithError(c, apperrors.ErrForbidden("Cannot remove the last admin"))
			return
		}
	}

	// Apply updates
	var changed []string
	if input.Username != nil {
		changed = append(changed, "username")
	}
	if input.Email != nil {
		changed = append(changed, "email")
	}
	if input.Password != nil {
		changed = append(changed, "password")
	}
	if input.IsAdmin != nil {
		changed = append(changed, "is_admin")
	}
	if input.Username != nil {
		user.Username = strings.ToLower(*input.Username)
	}
	if input.Email != nil {
		user.Email = strings.ToLower(*input.Email)
	}
	if input.Password != nil {
		hashedPassword, err := services.HashPassword(*input.Password)
		if err != nil {
			if errors.Is(err, services.ErrPasswordTooLong) {
				apperrors.AbortWithError(c, apperrors.ErrValidation(err.Error()))
			} else {
				log.Error().Err(err).Msg("Failed to hash password during admin update")
				apperrors.AbortWithError(c, apperrors.ErrInternal("Could not hash password").WithError(err))
			}
			return
		}
		user.Password = hashedPassword
	}
	if input.IsAdmin != nil {
		user.IsAdmin = *input.IsAdmin
	}

	// A new password set by an admin ends the user's sessions in the same change.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&user).Error; err != nil {
			return err
		}
		if input.Password != nil {
			return services.RevokeAllSessions(tx, user.ID, middleware.RequestNow(c))
		}
		return nil
	}); err != nil {
		log.Error().Err(err).Uint("user_id", user.ID).Msg("Failed to update user")
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("User").WithDetails("field", "username or email"))
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("update user").WithError(err))
		return
	}

	audit(c, db, models.AuditAdminUserUpdated, fmt.Sprintf("user:%s#%d;changed:%s", auditValue(user.Username), user.ID, strings.Join(changed, ",")))

	c.JSON(http.StatusOK, models.AdminUserResponse{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		Language:   user.Language,
		DateFormat: user.DateFormat,
		IsAdmin:    user.IsAdmin,
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,

		TwoFactorEnabled: user.TOTPEnabled,
	})
}

// DeleteUser deletes a user and all their data (admin only)
func DeleteUser(c *gin.Context) {
	log := logger.FromContext(c)
	db := c.MustGet("db").(*gorm.DB)

	currentUserID, ok := currentUserID(c)
	if !ok {
		return
	}

	idStr := c.Param("id")
	id, err := idutil.Parse(idStr)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("id", "Invalid user ID"))
		return
	}

	// Prevent admin from deleting themselves
	if id == currentUserID {
		apperrors.AbortWithError(c, apperrors.ErrForbidden("Cannot delete your own account"))
		return
	}

	var user models.User
	if err := db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("User"))
			return
		}
		log.Error().Err(err).Uint("user_id", id).Msg("Failed to get user for deletion")
		apperrors.AbortWithError(c, apperrors.ErrDatabase("get user").WithError(err))
		return
	}

	// Check if trying to delete the last admin
	if user.IsAdmin {
		var adminCount int64
		if err := db.Model(&models.User{}).Where("is_admin = ?", true).Count(&adminCount).Error; err != nil {
			log.Error().Err(err).Msg("Failed to count admins")
			apperrors.AbortWithError(c, apperrors.ErrDatabase("count admins").WithError(err))
			return
		}
		if adminCount <= 1 {
			apperrors.AbortWithError(c, apperrors.ErrForbidden("Cannot delete the last admin"))
			return
		}
	}

	// Delete user's data in a transaction
	err = db.Transaction(func(tx *gorm.DB) error {
		userID := id

		// Delete reminders
		if err := tx.Where("user_id = ?", userID).Delete(&models.Reminder{}).Error; err != nil {
			return err
		}

		// Delete notes
		if err := tx.Where("user_id = ?", userID).Delete(&models.Note{}).Error; err != nil {
			return err
		}

		// Delete activity_contacts associations (many-to-many)
		if err := tx.Exec("DELETE FROM activity_contacts WHERE activity_id IN (SELECT id FROM activities WHERE user_id = ?)", userID).Error; err != nil {
			return err
		}

		// Delete activities
		if err := tx.Where("user_id = ?", userID).Delete(&models.Activity{}).Error; err != nil {
			return err
		}

		// Delete relationships
		if err := tx.Where("user_id = ?", userID).Delete(&models.Relationship{}).Error; err != nil {
			return err
		}

		// Delete contacts
		if err := tx.Where("user_id = ?", userID).Delete(&models.Contact{}).Error; err != nil {
			return err
		}

		// Sessions: the user is soft-deleted, so the foreign key cascade does not run
		if err := tx.Where("user_id = ?", userID).Delete(&models.Session{}).Error; err != nil {
			return err
		}

		// Delete the calendar feed (the user row is soft-deleted, so the foreign key
		// cascade does not remove it and the token would stay valid)
		if err := tx.Where("user_id = ?", userID).Delete(&models.IcsFeed{}).Error; err != nil {
			return err
		}

		// API tokens would otherwise stay valid for a soft-deleted user (the auth
		// middleware also refuses them, this keeps the table clean)
		if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.ApiToken{}).Error; err != nil {
			return err
		}

		// Stored credentials and background syncs: nothing of the user may remain.
		// Links go first (they reference the connection or subscription).
		if err := tx.Unscoped().Where("connection_id IN (?)", tx.Model(&models.CardDAVConnection{}).Unscoped().Select("id").Where("user_id = ?", userID)).Delete(&models.CardDAVContactLink{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.CardDAVConnection{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("subscription_id IN (?)", tx.Model(&models.CalendarSubscription{}).Unscoped().Select("id").Where("user_id = ?", userID)).Delete(&models.CalendarEventLink{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.CalendarSubscription{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("webhook_id IN (?)", tx.Model(&models.Webhook{}).Unscoped().Select("id").Where("user_id = ?", userID)).Delete(&models.WebhookDelivery{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("user_id = ?", userID).Delete(&models.Webhook{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&models.EmailOutbox{}).Error; err != nil {
			return err
		}

		// The row stays (soft delete), so release what is unique on it: the person must be
		// able to register or log in again (OIDC) as a new account. The partial unique
		// index on (oidc_subject, oidc_provider) ignores NULLs.
		if err := tx.Unscoped().Model(&models.User{}).Where("id = ?", userID).Updates(map[string]any{
			"email":         fmt.Sprintf("deleted+%d@deleted.invalid", userID),
			"username":      fmt.Sprintf("deleted-%d", userID),
			"oidc_subject":  nil,
			"oidc_provider": nil,
		}).Error; err != nil {
			return err
		}

		// Two-factor: the secret and the recovery codes go with the account
		if err := tx.Where("user_id = ?", userID).Delete(&models.RecoveryCode{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Model(&models.User{}).Where("id = ?", userID).
			Updates(map[string]any{"totp_secret": "", "totp_enabled": false}).Error; err != nil {
			return err
		}

		// Delete user
		if err := tx.Delete(&user).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		log.Error().Err(err).Uint("user_id", id).Msg("Failed to delete user")
		apperrors.AbortWithError(c, apperrors.ErrDatabase("delete user").WithError(err))
		return
	}

	audit(c, db, models.AuditAdminUserDeleted, fmt.Sprintf("user:%s#%d", auditValue(user.Username), id))
	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}
