// Package persistence implements the admin module's ports: the backup files through the
// backup service, the audit log and the user count through GORM.
package persistence

import (
	"context"
	"errors"
	"os"
	"time"

	"meerkat/admin/internal/app"
	"meerkat/admin/internal/domain"
	"meerkat/models"
	"meerkat/services"

	"gorm.io/gorm"
)

type Backups struct {
	db        *gorm.DB
	dir       string
	photoDir  string
	retention int
}

func NewBackups(db *gorm.DB, dir, photoDir string, retention int) *Backups {
	return &Backups{db: db, dir: dir, photoDir: photoDir, retention: retention}
}

var _ app.Backups = (*Backups)(nil)

func backup(b services.BackupInfo) domain.Backup {
	return domain.Backup{Name: b.Name, Size: b.Size, CreatedAt: b.CreatedAt}
}

func (r *Backups) List(_ context.Context) ([]domain.Backup, error) {
	infos, err := services.ListBackups(r.dir)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Backup, len(infos))
	for i, b := range infos {
		out[i] = backup(b)
	}
	return out, nil
}

func (r *Backups) Create(ctx context.Context, now time.Time) (domain.Backup, error) {
	b, err := services.CreateBackup(r.db.WithContext(ctx), r.dir, r.photoDir, now)
	if err != nil {
		return domain.Backup{}, err
	}
	return backup(b), nil
}

func (r *Backups) Prune(_ context.Context) error {
	_, err := services.PruneBackups(r.dir, r.retention)
	return err
}

func (r *Backups) Path(_ context.Context, name string) (string, error) {
	path, err := services.BackupPath(r.dir, name)
	if err != nil {
		return "", domain.ErrBackupNotFound
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return "", domain.ErrBackupNotFound
	}
	return path, nil
}

func (r *Backups) Delete(_ context.Context, name string) error {
	err := services.DeleteBackup(r.dir, name)
	if errors.Is(err, services.ErrBackupNotFound) || errors.Is(err, services.ErrInvalidBackupName) {
		return domain.ErrBackupNotFound
	}
	return err
}

type AuditLog struct{ db *gorm.DB }

func NewAuditLog(db *gorm.DB) *AuditLog { return &AuditLog{db: db} }

var _ app.AuditLog = (*AuditLog)(nil)

func (r *AuditLog) filtered(ctx context.Context, f domain.AuditFilter) *gorm.DB {
	query := r.db.WithContext(ctx).Model(&models.AuditLog{})
	if f.Event != "" {
		query = query.Where("event = ?", f.Event)
	}
	if f.Username != "" {
		query = query.Where("instr(lower(username), ?) > 0", f.Username)
	}
	return query
}

func (r *AuditLog) Count(ctx context.Context, f domain.AuditFilter) (int64, error) {
	var total int64
	err := r.filtered(ctx, f).Count(&total).Error
	return total, err
}

func (r *AuditLog) Page(ctx context.Context, f domain.AuditFilter) ([]domain.AuditEntry, error) {
	var rows []models.AuditLog
	if err := r.filtered(ctx, f).Order("id DESC").Limit(f.Limit).Offset(f.Offset()).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.AuditEntry, len(rows))
	for i, e := range rows {
		var user *domain.UserID
		if e.UserID != nil {
			id := domain.UserID(*e.UserID)
			user = &id
		}
		out[i] = domain.AuditEntry{ID: e.ID, CreatedAt: e.CreatedAt, UserID: user, Username: e.Username,
			Event: e.Event, IP: e.IP, UserAgent: e.UserAgent, Detail: e.Detail}
	}
	return out, nil
}

func (r *AuditLog) Events() []string { return models.AuditEvents }

type Audit struct{ db *gorm.DB }

func NewAudit(db *gorm.DB) *Audit { return &Audit{db: db} }

var _ app.Audit = (*Audit)(nil)

func (a *Audit) RecordBackup(ctx context.Context, actor domain.UserID, action domain.BackupAction, name string, client domain.Client) {
	event := map[domain.BackupAction]string{
		domain.BackupCreated:    models.AuditBackupCreated,
		domain.BackupDownloaded: models.AuditBackupDownloaded,
		domain.BackupDeleted:    models.AuditBackupDeleted,
	}[action]
	services.WriteUserAudit(a.db.WithContext(ctx), uint(actor), event, "backup:"+name, client.IP, client.UserAgent)
}

type Users struct{ db *gorm.DB }

func NewUsers(db *gorm.DB) *Users { return &Users{db: db} }

var _ app.Users = (*Users)(nil)

func (r *Users) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.User{}).Count(&n).Error
	return n, err
}
