// Package persistence implements the admin module's ports: the backup files through the
// backup service, the audit log and the user count through GORM.
package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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

// RecordUser words the detail as the admin page expects it: ;-separated key:value segments,
// with a ; in the username replaced so it cannot break the format.
func (a *Audit) RecordUser(ctx context.Context, actor domain.UserID, action domain.UserAction, u domain.User, changed []string, client domain.Client) {
	event := map[domain.UserAction]string{
		domain.UserUpdated:      models.AuditAdminUserUpdated,
		domain.UserDeleted:      models.AuditAdminUserDeleted,
		domain.UserTwoFactorOff: models.AuditTOTPAdminReset,
	}[action]
	detail := fmt.Sprintf("user:%s#%d", strings.ReplaceAll(u.Username, ";", ","), u.ID)
	if len(changed) > 0 {
		detail += ";changed:" + strings.Join(changed, ",")
	}
	services.WriteUserAudit(a.db.WithContext(ctx), uint(actor), event, detail, client.IP, client.UserAgent)
}

func (a *Audit) RecordReminderRun(ctx context.Context, actor domain.UserID, client domain.Client) {
	services.WriteUserAudit(a.db.WithContext(ctx), uint(actor), models.AuditAdminReminders, "", client.IP, client.UserAgent)
}

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

func user(u models.User) domain.User {
	return domain.User{ID: domain.UserID(u.ID), Username: u.Username, Email: u.Email, Language: u.Language, DateFormat: u.DateFormat,
		IsAdmin: u.IsAdmin, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt, TwoFactor: u.TOTPEnabled}
}

func (r *Users) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.User{}).Count(&n).Error
	return n, err
}

func (r *Users) ImageTotals(ctx context.Context) (int64, int64, error) {
	var t struct{ N, Bytes int64 }
	err := r.db.WithContext(ctx).Model(&models.Image{}).Select("COUNT(*) AS n, COALESCE(SUM(bytes), 0) AS bytes").Scan(&t).Error
	return t.N, t.Bytes, err
}

func (r *Users) CountAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.User{}).Where("is_admin = ?", true).Count(&n).Error
	return n, err
}

func (r *Users) Page(ctx context.Context, offset, limit int) ([]domain.User, int64, error) {
	total, err := r.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	var rows []models.User
	if err := r.db.WithContext(ctx).Order("id ASC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.User, len(rows))
	for i, u := range rows {
		out[i] = user(u)
	}
	return out, total, nil
}

func (r *Users) Get(ctx context.Context, id domain.UserID) (domain.User, error) {
	var u models.User
	if err := r.db.WithContext(ctx).First(&u, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.User{}, domain.ErrUserNotFound
		}
		return domain.User{}, err
	}
	return user(u), nil
}

func (r *Users) Save(ctx context.Context, d domain.User, password string, now time.Time) (domain.User, error) {
	var hash string
	if password != "" {
		var err error
		if hash, err = services.HashPassword(password); err != nil {
			if errors.Is(err, services.ErrPasswordTooLong) {
				return domain.User{}, domain.ErrPasswordTooLong
			}
			return domain.User{}, fmt.Errorf("%w: %w", domain.ErrCannotHash, err)
		}
	}
	var saved models.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&saved, uint(d.ID)).Error; err != nil {
			return err
		}
		saved.Username, saved.Email, saved.IsAdmin = d.Username, d.Email, d.IsAdmin
		if hash != "" {
			saved.Password = hash
		}
		if err := tx.Save(&saved).Error; err != nil {
			return err
		}
		if hash != "" {
			return services.RevokeAllSessions(tx, saved.ID, now)
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return domain.User{}, domain.ErrAlreadyExists
		}
		return domain.User{}, err
	}
	return user(saved), nil
}

func (r *Users) ResetTwoFactor(ctx context.Context, id domain.UserID, now time.Time) error {
	db := r.db.WithContext(ctx)
	if err := services.DisableTOTP(db, uint(id)); err != nil {
		return err
	}
	if err := services.RevokeAllSessions(db, uint(id), now); err != nil {
		return err
	}
	return nil
}

func (r *Users) Delete(ctx context.Context, d domain.User) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		userID := uint(d.ID)
		for _, step := range []func() error{
			func() error { return tx.Where("user_id = ?", userID).Delete(&models.Reminder{}).Error },
			func() error { return tx.Where("user_id = ?", userID).Delete(&models.Note{}).Error },
			func() error {
				return tx.Exec("DELETE FROM activity_contacts WHERE activity_id IN (SELECT id FROM activities WHERE user_id = ?)", userID).Error
			},
			func() error { return tx.Where("user_id = ?", userID).Delete(&models.Activity{}).Error },
			func() error { return tx.Where("user_id = ?", userID).Delete(&models.Relationship{}).Error },
			func() error { return tx.Where("user_id = ?", userID).Delete(&models.Contact{}).Error },
			// The user is soft-deleted, so the foreign key cascades do not run: sessions, the
			// calendar feed and API tokens would otherwise stay valid.
			func() error { return tx.Where("user_id = ?", userID).Delete(&models.Session{}).Error },
			func() error { return tx.Where("user_id = ?", userID).Delete(&models.IcsFeed{}).Error },
			func() error { return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.ApiToken{}).Error },
			// Stored credentials and background syncs: nothing of the user may remain. Links go
			// first (they reference the connection or subscription).
			func() error {
				return tx.Unscoped().Where("connection_id IN (?)", tx.Model(&models.CardDAVConnection{}).Unscoped().Select("id").Where("user_id = ?", userID)).Delete(&models.CardDAVContactLink{}).Error
			},
			func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.CardDAVConnection{}).Error
			},
			func() error {
				return tx.Unscoped().Where("subscription_id IN (?)", tx.Model(&models.CalendarSubscription{}).Unscoped().Select("id").Where("user_id = ?", userID)).Delete(&models.CalendarEventLink{}).Error
			},
			func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.CalendarSubscription{}).Error
			},
			func() error {
				return tx.Unscoped().Where("webhook_id IN (?)", tx.Model(&models.Webhook{}).Unscoped().Select("id").Where("user_id = ?", userID)).Delete(&models.WebhookDelivery{}).Error
			},
			func() error { return tx.Unscoped().Where("user_id = ?", userID).Delete(&models.Webhook{}).Error },
			func() error { return tx.Where("user_id = ?", userID).Delete(&models.EmailOutbox{}).Error },
			// The row stays (soft delete), so release what is unique on it: the person must be
			// able to register or log in again (OIDC) as a new account.
			func() error {
				return tx.Unscoped().Model(&models.User{}).Where("id = ?", userID).Updates(map[string]any{
					"email":         fmt.Sprintf("deleted+%d@deleted.invalid", userID),
					"username":      fmt.Sprintf("deleted-%d", userID),
					"oidc_subject":  nil,
					"oidc_provider": nil,
				}).Error
			},
			// Two-factor: the secret and the recovery codes go with the account.
			func() error { return tx.Where("user_id = ?", userID).Delete(&models.RecoveryCode{}).Error },
			func() error {
				return tx.Unscoped().Model(&models.User{}).Where("id = ?", userID).
					Updates(map[string]any{"totp_secret": "", "totp_enabled": false}).Error
			},
			func() error { return tx.Delete(&models.User{}, userID).Error },
		} {
			if err := step(); err != nil {
				return err
			}
		}
		return nil
	})
}
