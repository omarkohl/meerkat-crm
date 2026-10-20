package persistence

import (
	"context"
	"errors"
	"time"

	"meerkat/config"
	"meerkat/models"
	"meerkat/services"
	"meerkat/users/internal/app"
	"meerkat/users/internal/domain"

	"gorm.io/gorm"
)

const totpIssuer = "Meerkat CRM"

// TwoFactor keeps the encrypted secret and the recovery codes through the services, which
// the login shares.
type TwoFactor struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewTwoFactor(db *gorm.DB, cfg *config.Config) *TwoFactor { return &TwoFactor{db: db, cfg: cfg} }

var _ app.SecondFactor = (*TwoFactor)(nil)

func (t *TwoFactor) ServerReady() bool { return services.CredentialKeysOf(*t.cfg).Ready() }

func (t *TwoFactor) RecoveryCodesLeft(ctx context.Context, actor domain.UserID) (int, error) {
	return services.RecoveryCodesRemaining(t.db.WithContext(ctx), uint(actor)), nil
}

func (t *TwoFactor) BeginSetup(ctx context.Context, actor domain.UserID, username string) (string, string, error) {
	secret, err := services.BeginTOTPSetup(t.db.WithContext(ctx), t.cfg, uint(actor))
	switch {
	case errors.Is(err, services.ErrTOTPAlreadyEnabled):
		return "", "", domain.ErrTwoFactorEnabled
	case errors.Is(err, services.ErrNoEncryptionKey):
		return "", "", domain.ErrTwoFactorNoServerKey
	}
	if err != nil {
		return "", "", err
	}
	return secret, services.TOTPURI(totpIssuer, username, secret), nil
}

func (t *TwoFactor) Enable(ctx context.Context, actor domain.UserID, code string, at time.Time) ([]string, error) {
	codes, err := services.EnableTOTP(t.db.WithContext(ctx), t.cfg, uint(actor), code, at)
	switch {
	case errors.Is(err, services.ErrInvalidSecondFactor):
		return nil, domain.ErrInvalidVerificationCode
	case errors.Is(err, services.ErrTOTPAlreadyEnabled):
		return nil, domain.ErrTwoFactorEnabled
	}
	return codes, err
}

func (t *TwoFactor) Check(ctx context.Context, actor domain.UserID, code, recovery string, at time.Time) (bool, bool, error) {
	db := t.db.WithContext(ctx)
	var user models.User
	if err := db.First(&user, uint(actor)).Error; err != nil {
		return false, false, err
	}
	ok, usedRecovery := services.CheckSecondFactor(db, t.cfg, user, code, recovery, at)
	return ok, usedRecovery, nil
}

func (t *TwoFactor) Disable(ctx context.Context, actor domain.UserID) error {
	return services.DisableTOTP(t.db.WithContext(ctx), uint(actor))
}

func (t *TwoFactor) NewRecoveryCodes(ctx context.Context, actor domain.UserID) ([]string, error) {
	return services.RegenerateRecoveryCodes(t.db.WithContext(ctx), t.cfg, uint(actor))
}
