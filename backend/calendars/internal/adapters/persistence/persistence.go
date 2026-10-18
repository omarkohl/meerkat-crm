// Package persistence implements the ports of the calendars module on top of GORM and the
// existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md), and the place where every query is scoped to the actor.
// Fetching and importing a calendar is done by the sync service, which the scheduled job shares.
package persistence

import (
	"context"
	"errors"

	"meerkat/calendars/internal/app"
	"meerkat/calendars/internal/domain"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/models"
	"meerkat/services"

	"gorm.io/gorm"
)

type Repository struct {
	db  *gorm.DB
	cfg config.Config
}

func New(db *gorm.DB, cfg config.Config) *Repository { return &Repository{db: db, cfg: cfg} }

var _ app.Subscriptions = (*Repository)(nil)

func subscription(m models.CalendarSubscription) domain.Subscription {
	return domain.Subscription{
		ID: domain.SubscriptionID(m.ID), Name: m.Name, URL: m.URL, Username: m.Username, HasPassword: m.PasswordEncrypted != "",
		SyncEnabled: m.SyncEnabled, PastDays: m.PastDays, FutureDays: m.FutureDays, LastSyncedAt: m.LastSyncedAt,
		LastSyncStatus: m.LastSyncStatus, LastSyncError: m.LastSyncError, CreatedAt: m.CreatedAt,
	}
}

func (r *Repository) find(ctx context.Context, actor domain.UserID, id domain.SubscriptionID) (models.CalendarSubscription, error) {
	var m models.CalendarSubscription
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", uint(id), uint(actor)).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return m, domain.ErrNotFound
	}
	return m, err
}

func (r *Repository) List(ctx context.Context, actor domain.UserID) ([]domain.Subscription, error) {
	var rows []models.CalendarSubscription
	if err := r.db.WithContext(ctx).Where("user_id = ?", uint(actor)).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Subscription, len(rows))
	for i, m := range rows {
		out[i] = subscription(m)
	}
	return out, nil
}

func (r *Repository) Count(ctx context.Context, actor domain.UserID) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.CalendarSubscription{}).Where("user_id = ? AND deleted_at IS NULL", uint(actor)).Count(&n).Error
	return int(n), err
}

func (r *Repository) Create(ctx context.Context, actor domain.UserID, s domain.Subscription, encrypted string) (domain.Subscription, error) {
	m := models.CalendarSubscription{
		UserID: uint(actor), Name: s.Name, URL: s.URL, Username: s.Username, PasswordEncrypted: encrypted,
		SyncEnabled: s.SyncEnabled, PastDays: s.PastDays, FutureDays: s.FutureDays,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return domain.Subscription{}, err
	}
	return subscription(m), nil
}

func (r *Repository) Find(ctx context.Context, actor domain.UserID, id domain.SubscriptionID) (domain.Subscription, error) {
	m, err := r.find(ctx, actor, id)
	return subscription(m), err
}

func (r *Repository) Update(ctx context.Context, actor domain.UserID, s domain.Subscription, password domain.PasswordChange, encrypted string) (domain.Subscription, error) {
	m, err := r.find(ctx, actor, s.ID)
	if err != nil {
		return domain.Subscription{}, err
	}
	m.Name, m.URL, m.Username, m.SyncEnabled, m.PastDays, m.FutureDays = s.Name, s.URL, s.Username, s.SyncEnabled, s.PastDays, s.FutureDays
	switch password.Mode {
	case domain.ClearPassword:
		m.PasswordEncrypted = ""
	case domain.SetPassword:
		m.PasswordEncrypted = encrypted
	}
	if err := r.db.WithContext(ctx).Save(&m).Error; err != nil {
		return domain.Subscription{}, err
	}
	return subscription(m), nil
}

func (r *Repository) Delete(ctx context.Context, actor domain.UserID, id domain.SubscriptionID) error {
	m, err := r.find(ctx, actor, id)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("subscription_id = ?", m.ID).Delete(&models.CalendarEventLink{}).Error; err != nil {
			return err
		}
		return tx.Delete(&m).Error
	})
}

func (r *Repository) Sync(ctx context.Context, actor domain.UserID, id domain.SubscriptionID, clk clock.Clock) (domain.SyncStats, error) {
	m, err := r.find(ctx, actor, id)
	if err != nil {
		return domain.SyncStats{}, err
	}
	stats, err := services.NewCalendarSyncService(r.cfg.CalDAVBlockPrivateURLs).SyncSubscription(ctx, r.db, r.cfg, &m, clk)
	if err != nil {
		return domain.SyncStats{}, &domain.SyncError{Kind: syncKind(err), Err: err}
	}
	return domain.SyncStats{Created: stats.Created, Updated: stats.Updated, Skipped: stats.Skipped}, nil
}

func syncKind(err error) domain.SyncKind {
	switch {
	case errors.Is(err, services.ErrCalendarInvalidURL):
		return domain.SyncInvalidURL
	case errors.Is(err, services.ErrCalendarUnauthorized):
		return domain.SyncUnauthorized
	case errors.Is(err, services.ErrCalendarNotFound):
		return domain.SyncNotFound
	case errors.Is(err, services.ErrCalendarPrivateAddress):
		return domain.SyncPrivateAddress
	case errors.Is(err, services.ErrCalendarTooLarge):
		return domain.SyncTooLarge
	case errors.Is(err, services.ErrCalendarInvalidData):
		return domain.SyncInvalidData
	case errors.Is(err, services.ErrCalendarUnreachable):
		return domain.SyncUnreachable
	}
	return domain.SyncFailed
}

type Secrets struct{ key string }

func NewSecrets(key string) *Secrets { return &Secrets{key: key} }

var _ app.Secrets = (*Secrets)(nil)

func (s *Secrets) Encrypt(plain string) (string, error) {
	return services.EncryptCredential(s.key, plain)
}
