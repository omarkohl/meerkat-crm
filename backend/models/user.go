package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username                 string     `gorm:"unique" validate:"required,min=1,max=50,no_at_sign"`
	Password                 string     `validate:"required,min=8,strong_password"`
	Email                    string     `gorm:"unique" validate:"required,email"`
	Language                 string     `gorm:"default:'en'" json:"language" validate:"omitempty,oneof=en de it es fr"`
	DateFormat               string     `gorm:"default:'eu'" json:"date_format" validate:"omitempty,oneof=eu us iso"`
	IsAdmin                  bool       `gorm:"default:false" json:"is_admin"`
	PasswordResetTokenHash   *string    `gorm:"column:password_reset_token_hash"`
	PasswordResetExpiresAt   *time.Time `gorm:"column:password_reset_expires_at"`
	PasswordResetRequestedAt *time.Time `gorm:"column:password_reset_requested_at"`
	CustomFieldNames         []string   `gorm:"type:text;serializer:json" json:"custom_field_names"`
	EnabledContactFields     []string   `gorm:"type:text;serializer:json" json:"enabled_contact_fields"`
	OIDCSubject              *string    `gorm:"column:oidc_subject"`
	OIDCProvider             *string    `gorm:"column:oidc_provider"`
	TokenVersion             int        `gorm:"->;not null;default:0" json:"-"` // read-only: Save() must never write a stale copy back; services.RevokeAllSessions raises it
	// Two-factor (TOTP). The secret is encrypted with the credential key; LastStep is the
	// last accepted 30-second step (replay protection).
	TOTPSecret   string `gorm:"column:totp_secret;not null;default:''" json:"-"`
	TOTPEnabled  bool   `gorm:"column:totp_enabled;not null;default:false" json:"-"`
	TOTPLastStep int64  `gorm:"column:totp_last_step;not null;default:0" json:"-"`

	// BirthdayMailedOn is the day (YYYY-MM-DD, in the reminder timezone) the birthday mail last went out.
	BirthdayMailedOn string `gorm:"column:birthday_mail_sent_on;not null;default:''" json:"-"`
}
