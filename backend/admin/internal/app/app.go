// Package app holds the use cases of the admin module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"
	"time"

	"meerkat/admin/internal/domain"
)

type Backups interface {
	List(ctx context.Context) ([]domain.Backup, error)
	Create(ctx context.Context, now time.Time) (domain.Backup, error)
	// Prune deletes the oldest backups beyond the retention count.
	Prune(ctx context.Context) error
	// Path is where an existing backup file is, or domain.ErrBackupNotFound.
	Path(ctx context.Context, name string) (string, error)
	Delete(ctx context.Context, name string) error
}

type AuditLog interface {
	Count(ctx context.Context, f domain.AuditFilter) (int64, error)
	Page(ctx context.Context, f domain.AuditFilter) ([]domain.AuditEntry, error)
	Events() []string
}

type Audit interface {
	RecordBackup(ctx context.Context, actor domain.UserID, action domain.BackupAction, name string, client domain.Client)
}

type Users interface {
	Count(ctx context.Context) (int64, error)
}

type Stage string

const (
	StageListBackups  Stage = "list backups"
	StageCreateBackup Stage = "create backup"
	StageDeleteBackup Stage = "delete backup"
	StageAuditCount   Stage = "audit count"
	StageAuditQuery   Stage = "audit query"
	StageCountUsers   Stage = "count users"
)

// Failure wraps an infrastructure error with the stage it happened in. A backup that does
// not exist is not a Failure: it is domain.ErrBackupNotFound.
type Failure struct {
	Stage Stage
	Err   error
}

func (f *Failure) Error() string { return string(f.Stage) + ": " + f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

type UseCases struct {
	backups  Backups
	auditLog AuditLog
	audit    Audit
	users    Users
	settings domain.SystemSettings
}

func New(backups Backups, auditLog AuditLog, audit Audit, users Users, settings domain.SystemSettings) *UseCases {
	return &UseCases{backups: backups, auditLog: auditLog, audit: audit, users: users, settings: settings}
}

type BackupListing struct {
	Backups  []domain.Backup
	Settings domain.BackupSettings
}

func (u *UseCases) ListBackups(ctx context.Context) (BackupListing, error) {
	list, err := u.backups.List(ctx)
	if err != nil {
		return BackupListing{}, &Failure{Stage: StageListBackups, Err: err}
	}
	return BackupListing{Backups: list, Settings: u.settings.Backup}, nil
}

type CreatedBackup struct {
	domain.Backup
	// PruneErr is why old backups could not be removed; the new one exists anyway.
	PruneErr error
}

func (u *UseCases) CreateBackup(ctx context.Context, actor domain.UserID, now time.Time, client domain.Client) (CreatedBackup, error) {
	// A client that goes away must not leave half a backup or lose the audit entry.
	ctx = context.WithoutCancel(ctx)
	b, err := u.backups.Create(ctx, now)
	if err != nil {
		return CreatedBackup{}, &Failure{Stage: StageCreateBackup, Err: err}
	}
	pruneErr := u.backups.Prune(ctx)
	u.audit.RecordBackup(ctx, actor, domain.BackupCreated, b.Name, client)
	return CreatedBackup{Backup: b, PruneErr: pruneErr}, nil
}

// OpenBackup returns the path of the backup to send and records the download.
func (u *UseCases) OpenBackup(ctx context.Context, actor domain.UserID, name string, client domain.Client) (string, error) {
	path, err := u.backups.Path(ctx, name)
	if err != nil {
		return "", err
	}
	u.audit.RecordBackup(context.WithoutCancel(ctx), actor, domain.BackupDownloaded, name, client)
	return path, nil
}

func (u *UseCases) DeleteBackup(ctx context.Context, actor domain.UserID, name string, client domain.Client) error {
	err := u.backups.Delete(ctx, name)
	if errors.Is(err, domain.ErrBackupNotFound) {
		return err
	}
	if err != nil {
		return &Failure{Stage: StageDeleteBackup, Err: err}
	}
	u.audit.RecordBackup(context.WithoutCancel(ctx), actor, domain.BackupDeleted, name, client)
	return nil
}

type AuditPage struct {
	Entries       []domain.AuditEntry
	Total         int64
	Page, Limit   int
	Events        []string
	RetentionDays int
}

func (u *UseCases) ListAuditLog(ctx context.Context, f domain.AuditFilter) (AuditPage, error) {
	f = f.Normalised()
	total, err := u.auditLog.Count(ctx, f)
	if err != nil {
		return AuditPage{}, &Failure{Stage: StageAuditCount, Err: err}
	}
	entries, err := u.auditLog.Page(ctx, f)
	if err != nil {
		return AuditPage{}, &Failure{Stage: StageAuditQuery, Err: err}
	}
	return AuditPage{Entries: entries, Total: total, Page: f.Page, Limit: f.Limit, Events: u.auditLog.Events(), RetentionDays: u.settings.AuditRetentionDays}, nil
}

func (u *UseCases) SystemStatus(ctx context.Context) (domain.SystemStatus, error) {
	users, err := u.users.Count(ctx)
	if err != nil {
		return domain.SystemStatus{}, &Failure{Stage: StageCountUsers, Err: err}
	}
	return domain.SystemStatus{
		PrivateURLsAllowed: u.settings.PrivateURLsAllowed,
		UserCount:          users,
		PrivateURLWarning:  domain.PrivateURLWarning(u.settings.PrivateURLsAllowed, users),
		Features:           u.settings.PrivateURLFeatures,
	}, nil
}
