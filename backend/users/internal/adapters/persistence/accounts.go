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

func (s *Sessions) Start(ctx context.Context, actor domain.UserID, client domain.Client, now time.Time) (string, error) {
	db := s.db.WithContext(ctx)
	var user models.User
	if err := db.First(&user, uint(actor)).Error; err != nil {
		return "", err
	}
	return services.IssueSession(db, user, s.cfg, client.IP, client.UserAgent, now)
}

type Accounts struct{ db *gorm.DB }

func NewAccounts(db *gorm.DB) *Accounts { return &Accounts{db: db} }

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

func (a *Audit) Record(ctx context.Context, actor domain.UserID, event string, client domain.Client) {
	db := a.db.WithContext(ctx)
	id := uint(actor)
	entry := models.AuditLog{UserID: &id, Event: event, IP: client.IP, UserAgent: client.UserAgent}
	var name string
	if err := db.Model(&models.User{}).Where("id = ?", id).Limit(1).Pluck("username", &name).Error; err == nil {
		entry.Username = name
	}
	services.WriteAudit(db, entry)
}
