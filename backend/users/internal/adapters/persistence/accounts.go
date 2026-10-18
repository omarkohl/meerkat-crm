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

// Sessions, Accounts and Audit work on the login sessions, credentials and audit log tables
// through the services, which the authentication middleware and the other login flows share
// until they are modules too.
type Sessions struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewSessions(db *gorm.DB, cfg *config.Config) *Sessions { return &Sessions{db: db, cfg: cfg} }

var _ app.Sessions = (*Sessions)(nil)

func (s *Sessions) Live(ctx context.Context, actor domain.UserID, now time.Time) ([]domain.Session, error) {
	found, err := services.ListSessions(s.db.WithContext(ctx), uint(actor), now)
	out := make([]domain.Session, len(found))
	for i, m := range found {
		out[i] = domain.Session{ID: m.PublicID, CreatedAt: m.CreatedAt, UserAgent: m.UserAgent, IP: m.IP, LastSeenAt: m.LastSeenAt, ExpiresAt: m.ExpiresAt}
	}
	return out, err
}

func (s *Sessions) Revoke(ctx context.Context, actor domain.UserID, id string, now time.Time) (bool, error) {
	return services.RevokeSession(s.db.WithContext(ctx), uint(actor), id, now)
}

func (s *Sessions) RevokeAll(ctx context.Context, actor domain.UserID, now time.Time) error {
	return services.RevokeAllSessions(s.db.WithContext(ctx), uint(actor), now)
}

func (s *Sessions) RevokeOthers(ctx context.Context, actor domain.UserID, keep string, now time.Time) error {
	return services.RevokeOtherSessions(s.db.WithContext(ctx), uint(actor), keep, now)
}

func (s *Sessions) Owner(ctx context.Context, id string) (domain.UserID, bool) {
	var session models.Session
	if err := s.db.WithContext(ctx).Where("public_id = ?", id).Take(&session).Error; err != nil {
		return 0, false
	}
	return domain.UserID(session.UserID), true
}

func (s *Sessions) Start(ctx context.Context, actor domain.UserID, client domain.Client, now time.Time) (string, error) {
	db := s.db.WithContext(ctx)
	var user models.User
	if err := db.First(&user, uint(actor)).Error; err != nil {
		return "", err
	}
	return services.IssueSession(db, user, s.cfg, client.IP, client.UserAgent, now)
}

type Accounts struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewAccounts(db *gorm.DB, cfg *config.Config) *Accounts { return &Accounts{db: db, cfg: cfg} }

var _ app.Accounts = (*Accounts)(nil)

func (a *Accounts) PasswordHash(ctx context.Context, actor domain.UserID) (string, error) {
	var user models.User
	err := a.db.WithContext(ctx).Select("id", "password").First(&user, uint(actor)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", domain.ErrUserNotFound
	}
	return user.Password, err
}

func (a *Accounts) Account(ctx context.Context, actor domain.UserID) (domain.Account, error) {
	var user models.User
	err := a.db.WithContext(ctx).First(&user, uint(actor)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Account{}, domain.ErrUserNotFound
	}
	return domain.Account{Username: user.Username, PasswordHash: user.Password, TOTPEnabled: user.TOTPEnabled}, err
}

func (a *Accounts) ByEmail(ctx context.Context, email string) (domain.ResetTarget, error) {
	var user models.User
	err := a.db.WithContext(ctx).Select("id", "email", "language").Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ResetTarget{}, domain.ErrUserNotFound
	}
	return domain.ResetTarget{ID: domain.UserID(user.ID), Email: user.Email, Language: user.Language}, err
}

func (a *Accounts) StoreResetToken(ctx context.Context, actor domain.UserID, hash string, expires, requested time.Time) error {
	user := models.User{Model: gorm.Model{ID: uint(actor)}, PasswordResetTokenHash: &hash, PasswordResetExpiresAt: &expires, PasswordResetRequestedAt: &requested}
	return a.db.WithContext(ctx).Model(&user).
		Select("PasswordResetTokenHash", "PasswordResetExpiresAt", "PasswordResetRequestedAt").Updates(&user).Error
}

func (a *Accounts) ResetToken(ctx context.Context, hash string) (domain.ResetToken, error) {
	var user models.User
	err := a.db.WithContext(ctx).Select("id", "password_reset_expires_at").Where("password_reset_token_hash = ?", hash).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ResetToken{}, domain.ErrResetTokenInvalid
	}
	return domain.ResetToken{UserID: domain.UserID(user.ID), ExpiresAt: user.PasswordResetExpiresAt}, err
}

func (a *Accounts) ClearResetToken(ctx context.Context, actor domain.UserID, hash string) error {
	return a.db.WithContext(ctx).Model(&models.User{}).Where("id = ? AND password_reset_token_hash = ?", uint(actor), hash).
		Updates(map[string]any{"password_reset_token_hash": nil, "password_reset_expires_at": nil, "password_reset_requested_at": nil}).Error
}

func (a *Accounts) ResetPassword(ctx context.Context, actor domain.UserID, tokenHash, hash string, now time.Time) error {
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.User{}).Where("id = ? AND password_reset_token_hash = ?", uint(actor), tokenHash).
			Updates(map[string]any{
				"password": hash, "password_reset_token_hash": nil, "password_reset_expires_at": nil, "password_reset_requested_at": nil,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrResetTokenInvalid
		}
		return services.RevokeAllSessions(tx, uint(actor), now)
	})
}

func (a *Accounts) Register(ctx context.Context, user domain.NewUser) error {
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.User{}).Count(&count).Error; err != nil {
			return domain.ErrUserExists
		}
		row := models.User{Username: user.Username, Email: user.Email, Password: user.PasswordHash, Language: user.Language, IsAdmin: count == 0}
		if err := tx.Create(&row).Error; err != nil {
			return domain.ErrUserExists
		}
		return nil
	})
}

// ProvisionOIDC uses the services' account rules; cfg says whether accounts may be created and
// whether the provider's email is trusted without verification.
func (a *Accounts) ProvisionOIDC(ctx context.Context, claims domain.OIDCClaims) (domain.UserID, error) {
	user, err := services.FindOrProvisionUser(a.db.WithContext(ctx), &services.OIDCClaims{
		Subject: claims.Subject, Email: claims.Email, EmailVerified: claims.EmailVerified,
		Name: claims.Name, PreferredUsername: claims.PreferredUsername, Provider: claims.Provider,
	}, a.cfg)
	if errors.Is(err, services.ErrOIDCUserNotFound) {
		return 0, domain.ErrOIDCNoAccount
	}
	if err != nil {
		return 0, err
	}
	return domain.UserID(user.ID), nil
}

func (a *Accounts) ForLogin(ctx context.Context, identifier string, email bool) (domain.LoginAccount, error) {
	column := "username"
	if email {
		column = "email"
	}
	var user models.User
	err := a.db.WithContext(ctx).Where(column+" = ?", identifier).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.LoginAccount{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.LoginAccount{}, err
	}
	return domain.LoginAccount{
		ID: domain.UserID(user.ID), PasswordHash: user.Password, TOTPEnabled: user.TOTPEnabled,
		Language: user.Language, DateFormat: user.DateFormat,
	}, nil
}

func (a *Accounts) Profile(ctx context.Context, actor domain.UserID) (domain.Profile, error) {
	var user models.User
	err := a.db.WithContext(ctx).First(&user, uint(actor)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Profile{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.Profile{}, err
	}
	return domain.Profile{
		ID: domain.UserID(user.ID), Username: user.Username, Email: user.Email, Language: user.Language, DateFormat: user.DateFormat,
		IsAdmin: user.IsAdmin, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
		CustomFieldNames: user.CustomFieldNames, EnabledContactFields: user.EnabledContactFields,
	}, nil
}

func (a *Accounts) ChangePassword(ctx context.Context, actor domain.UserID, hash string, now time.Time) error {
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user := models.User{Model: gorm.Model{ID: uint(actor)}, Password: hash}
		res := tx.Model(&user).Select("Password", "PasswordResetTokenHash", "PasswordResetExpiresAt", "PasswordResetRequestedAt").Updates(&user)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrUserNotFound
		}
		return services.RevokeAllSessions(tx, uint(actor), now)
	})
}

type Audit struct{ db *gorm.DB }

func NewAudit(db *gorm.DB) *Audit { return &Audit{db: db} }

var _ app.Audit = (*Audit)(nil)

func (a *Audit) LoginFailed(ctx context.Context, actor *domain.UserID, reason string, client domain.Client) {
	db := a.db.WithContext(ctx)
	entry := models.AuditLog{Event: models.AuditLoginFailed, IP: client.IP, UserAgent: client.UserAgent, Detail: "reason:" + reason}
	if actor != nil {
		id := uint(*actor)
		entry.UserID = &id
		var name string
		if err := db.Model(&models.User{}).Where("id = ?", id).Limit(1).Pluck("username", &name).Error; err == nil {
			entry.Username = name
		}
	}
	services.WriteAudit(db, entry)
}

func (a *Audit) Record(ctx context.Context, actor domain.UserID, event string, client domain.Client) {
	a.RecordWith(ctx, actor, event, "", client)
}

func (a *Audit) RecordWith(ctx context.Context, actor domain.UserID, event, detail string, client domain.Client) {
	services.WriteUserAudit(a.db.WithContext(ctx), uint(actor), event, detail, client.IP, client.UserAgent)
}
