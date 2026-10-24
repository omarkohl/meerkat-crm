// Package persistence implements the ports of the contactsync module on GORM and the models,
// scoped to the actor (docs/adr/0012-module-layout.md). The sync runs in the shared sync service.
package persistence

import (
	"context"
	"encoding/json"
	"errors"

	"meerkat/clock"
	"meerkat/config"
	"meerkat/contactsync/internal/app"
	"meerkat/contactsync/internal/domain"
	"meerkat/models"
	"meerkat/services"

	"gorm.io/gorm"
)

type Repository struct {
	db  *gorm.DB
	cfg config.Config
}

func New(db *gorm.DB, cfg config.Config) *Repository { return &Repository{db: db, cfg: cfg} }

var (
	_ app.Connections = (*Repository)(nil)
	_ app.Directory   = (*Repository)(nil)
)

func connection(m models.CardDAVConnection) domain.Connection {
	c := domain.Connection{
		ID: m.ID, BaseURL: m.BaseURL, AddressBookPath: m.AddressBookPath, AddressBookName: m.AddressBookName, Username: m.Username,
		HasPassword: m.PasswordEncrypted != "", Direction: m.Direction, SyncEnabled: m.SyncEnabled,
		SyncRunning: services.IsContactSyncRunning(m.ID), LastSyncedAt: m.LastSyncedAt, LastSyncStatus: m.LastSyncStatus,
		LastSyncError: m.LastSyncError, CreatedAt: m.CreatedAt,
	}
	var stats services.ContactSyncStats
	if m.LastSyncStats != "" && json.Unmarshal([]byte(m.LastSyncStats), &stats) == nil {
		c.LastSyncStats = &domain.SyncStats{
			PulledCreated: stats.PulledCreated, PulledUpdated: stats.PulledUpdated, PulledDeleted: stats.PulledDeleted,
			PushedCreated: stats.PushedCreated, PushedUpdated: stats.PushedUpdated, PushedDeleted: stats.PushedDeleted,
			Conflicts: stats.Conflicts, Skipped: stats.Skipped, Errors: stats.Errors,
		}
	}
	return c
}

func (r *Repository) find(ctx context.Context, actor domain.UserID) (models.CardDAVConnection, error) {
	var m models.CardDAVConnection
	err := r.db.WithContext(ctx).Where("user_id = ?", uint(actor)).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return m, domain.ErrNotFound
	}
	return m, err
}

func (r *Repository) Find(ctx context.Context, actor domain.UserID) (domain.Connection, error) {
	m, err := r.find(ctx, actor)
	return connection(m), err
}

func (r *Repository) Save(ctx context.Context, actor domain.UserID, s domain.Saved, encrypted string) (domain.Connection, error) {
	var m models.CardDAVConnection
	if !s.IsNew {
		var err error
		if m, err = r.find(ctx, actor); err != nil {
			return domain.Connection{}, err
		}
	}
	c := s.Connection
	m.UserID, m.BaseURL, m.AddressBookPath, m.AddressBookName, m.Username = uint(actor), c.BaseURL, c.AddressBookPath, c.AddressBookName, c.Username
	m.Direction, m.SyncEnabled = c.Direction, c.SyncEnabled
	switch s.Password.Mode {
	case domain.ClearPassword:
		m.PasswordEncrypted = ""
	case domain.SetPassword:
		m.PasswordEncrypted = encrypted
	}
	if s.ResetLinks {
		m.SyncToken, m.LastSyncedAt, m.LastSyncStatus, m.LastSyncError, m.LastSyncStats = "", nil, "", "", ""
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if s.ResetLinks {
			if err := tx.Where("connection_id = ?", m.ID).Delete(&models.CardDAVContactLink{}).Error; err != nil {
				return err
			}
		}
		return tx.Save(&m).Error
	})
	if err != nil {
		return domain.Connection{}, err
	}
	return connection(m), nil
}

func (r *Repository) Delete(ctx context.Context, actor domain.UserID) error {
	m, err := r.find(ctx, actor)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("connection_id = ?", m.ID).Delete(&models.CardDAVContactLink{}).Error; err != nil {
			return err
		}
		return tx.Delete(&m).Error
	})
}

func (r *Repository) Credentials(ctx context.Context, actor domain.UserID) (string, func() string, bool) {
	m, err := r.find(ctx, actor)
	if err != nil {
		return "", nil, false
	}
	return m.BaseURL, func() string {
		password, err := services.DecryptCredential(services.CredentialKeysOf(r.cfg), m.PasswordEncrypted)
		if err != nil {
			return ""
		}
		return password
	}, true
}

func (r *Repository) StartSync(ctx context.Context, actor domain.UserID, clk clock.Clock) (bool, error) {
	m, err := r.find(ctx, actor)
	if err != nil {
		return false, err
	}
	return services.StartContactSync(r.db, r.cfg, m, clk), nil
}

func (r *Repository) Discover(ctx context.Context, baseURL, username, password string) ([]domain.AddressBook, error) {
	books, err := services.NewContactSyncService(r.cfg.CardDAVBlockPrivateURLs).Discover(ctx, baseURL, username, password)
	if err != nil {
		return nil, &domain.SyncError{Kind: syncKind(err), Err: err}
	}
	out := make([]domain.AddressBook, len(books))
	for i, b := range books {
		out[i] = domain.AddressBook{Path: b.Path, Name: b.Name, Description: b.Description}
	}
	return out, nil
}

func syncKind(err error) domain.SyncKind {
	kinds := []struct {
		err  error
		kind domain.SyncKind
	}{
		{services.ErrContactSyncInvalidURL, domain.SyncInvalidURL},
		{services.ErrContactSyncUnauthorized, domain.SyncUnauthorized},
		{services.ErrContactSyncForbidden, domain.SyncForbidden},
		{services.ErrContactSyncNotFound, domain.SyncNotFound},
		{services.ErrContactSyncPrivateAddress, domain.SyncPrivateAddress},
		{services.ErrContactSyncTooLarge, domain.SyncTooLarge},
		{services.ErrContactSyncTooMany, domain.SyncTooMany},
		{services.ErrContactSyncNoAddressBooks, domain.SyncNoAddressBooks},
		{services.ErrContactSyncInvalidData, domain.SyncInvalidData},
		{services.ErrContactSyncServerError, domain.SyncServerError},
		{services.ErrContactSyncUnreachable, domain.SyncUnreachable},
	}
	for _, k := range kinds {
		if errors.Is(err, k.err) {
			return k.kind
		}
	}
	return domain.SyncFailed
}

type Secrets struct{ key services.CredentialKeys }

func NewSecrets(cfg config.Config) *Secrets { return &Secrets{key: services.CredentialKeysOf(cfg)} }

func (s *Secrets) Encrypt(plain string) (string, error) {
	return services.EncryptCredential(s.key, plain)
}

var _ app.Secrets = (*Secrets)(nil)
