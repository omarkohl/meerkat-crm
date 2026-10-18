// Package app holds the use cases of the admin module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"
	"time"

	"meerkat/admin/internal/domain"
	"meerkat/clock"
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
	// RecordUser notes what an administrator did to an account; changed is empty unless the
	// account was updated.
	RecordUser(ctx context.Context, actor domain.UserID, action domain.UserAction, user domain.User, changed []string, client domain.Client)
	RecordReminderRun(ctx context.Context, actor domain.UserID, client domain.Client)
}

type Users interface {
	Count(ctx context.Context) (int64, error)
	// Page lists accounts oldest first, with the total.
	Page(ctx context.Context, offset, limit int) ([]domain.User, int64, error)
	// Get returns domain.ErrUserNotFound for an account that does not exist.
	Get(ctx context.Context, id domain.UserID) (domain.User, error)
	CountAdmins(ctx context.Context) (int64, error)
	// Save stores the account and, when password is not empty, its hash, ending the account's
	// sessions in the same change. It returns domain.ErrAlreadyExists on a taken username or
	// email and domain.ErrPasswordTooLong for a password that cannot be hashed.
	Save(ctx context.Context, user domain.User, password string, now time.Time) (domain.User, error)
	// Delete removes the account and everything it owns, in one change.
	Delete(ctx context.Context, user domain.User) error
	// ResetTwoFactor switches two-factor off and ends the sessions.
	ResetTwoFactor(ctx context.Context, id domain.UserID, now time.Time) error
}

// Lockouts holds the count of wrong second-factor codes per account.
type Lockouts interface {
	ClearSecondFactor(ctx context.Context, id domain.UserID)
}

// Reminders sends the daily reminder e-mails now.
type Reminders interface {
	Send(ctx context.Context, clk clock.Clock) error
}

type Stage string

const (
	StageListBackups  Stage = "list backups"
	StageCreateBackup Stage = "create backup"
	StageDeleteBackup Stage = "delete backup"
	StageAuditCount   Stage = "audit count"
	StageAuditQuery   Stage = "audit query"
	StageCountUsers   Stage = "count users"
	StageListUsers    Stage = "list users"
	StageGetUser      Stage = "get user"
	StageCountAdmins  Stage = "count admins"
	StageUpdateUser   Stage = "update user"
	StageDeleteUser   Stage = "delete user"
	StageResetTwoFA   Stage = "reset two-factor"
	StageReminders    Stage = "send reminders"
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
	backups   Backups
	auditLog  AuditLog
	audit     Audit
	users     Users
	reminders Reminders
	lockouts  Lockouts
	settings  domain.SystemSettings
}

func New(backups Backups, auditLog AuditLog, audit Audit, users Users, reminders Reminders, lockouts Lockouts, settings domain.SystemSettings) *UseCases {
	return &UseCases{backups: backups, auditLog: auditLog, audit: audit, users: users, reminders: reminders, lockouts: lockouts, settings: settings}
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

type UserPage struct {
	Users       []domain.User
	Total       int64
	Page, Limit int
}

func (u *UseCases) ListUsers(ctx context.Context, page, limit int) (UserPage, error) {
	page, limit = domain.Paging(page, limit)
	users, total, err := u.users.Page(ctx, (page-1)*limit, limit)
	if err != nil {
		return UserPage{}, &Failure{Stage: StageListUsers, Err: err}
	}
	return UserPage{Users: users, Total: total, Page: page, Limit: limit}, nil
}

func (u *UseCases) GetUser(ctx context.Context, id domain.UserID) (domain.User, error) {
	user, err := u.users.Get(ctx, id)
	if err != nil {
		return domain.User{}, userFailure(StageGetUser, err)
	}
	return user, nil
}

func userFailure(stage Stage, err error) error {
	if errors.Is(err, domain.ErrUserNotFound) {
		return err
	}
	return &Failure{Stage: stage, Err: err}
}

func (u *UseCases) UpdateUser(ctx context.Context, actor, id domain.UserID, changes domain.UserChanges, now time.Time, client domain.Client) (domain.User, error) {
	user, err := u.users.Get(ctx, id)
	if err != nil {
		return domain.User{}, userFailure(StageGetUser, err)
	}
	if changes.IsAdmin != nil && !*changes.IsAdmin {
		if user.ID == actor {
			return domain.User{}, domain.ErrOwnAdminStatus
		}
		if user.IsAdmin {
			if err := u.requireAnotherAdmin(ctx); err != nil {
				return domain.User{}, err
			}
		}
	}
	password := ""
	if changes.Password != nil {
		password = *changes.Password
	}
	saved, err := u.users.Save(ctx, changes.Apply(user), password, now)
	if errors.Is(err, domain.ErrAlreadyExists) || errors.Is(err, domain.ErrPasswordTooLong) || errors.Is(err, domain.ErrCannotHash) {
		return domain.User{}, err
	}
	if err != nil {
		return domain.User{}, &Failure{Stage: StageUpdateUser, Err: err}
	}
	u.audit.RecordUser(ctx, actor, domain.UserUpdated, saved, changes.Fields(), client)
	return saved, nil
}

func (u *UseCases) requireAnotherAdmin(ctx context.Context) error {
	n, err := u.users.CountAdmins(ctx)
	if err != nil {
		return &Failure{Stage: StageCountAdmins, Err: err}
	}
	if n <= 1 {
		return domain.ErrLastAdmin
	}
	return nil
}

func (u *UseCases) DeleteUser(ctx context.Context, actor, id domain.UserID, client domain.Client) error {
	if id == actor {
		return domain.ErrOwnAccount
	}
	user, err := u.users.Get(ctx, id)
	if err != nil {
		return userFailure(StageGetUser, err)
	}
	if user.IsAdmin {
		if err := u.requireAnotherAdmin(ctx); err != nil {
			return err
		}
	}
	if err := u.users.Delete(ctx, user); err != nil {
		return &Failure{Stage: StageDeleteUser, Err: err}
	}
	u.audit.RecordUser(ctx, actor, domain.UserDeleted, user, nil, client)
	return nil
}

// ResetTwoFactor is for an account whose phone and recovery codes are lost. Its sessions end,
// since whoever asked for the reset is not verified here.
func (u *UseCases) ResetTwoFactor(ctx context.Context, actor, id domain.UserID, now time.Time, client domain.Client) error {
	user, err := u.users.Get(ctx, id)
	if err != nil {
		return userFailure(StageGetUser, err)
	}
	if !user.TwoFactor {
		return domain.ErrTwoFactorOff
	}
	if err := u.users.ResetTwoFactor(ctx, id, now); err != nil {
		return &Failure{Stage: StageResetTwoFA, Err: err}
	}
	u.lockouts.ClearSecondFactor(ctx, id)
	u.audit.RecordUser(ctx, actor, domain.UserTwoFactorOff, user, nil, client)
	return nil
}

func (u *UseCases) SendReminders(ctx context.Context, actor domain.UserID, clk clock.Clock, client domain.Client) error {
	if err := u.reminders.Send(ctx, clk); err != nil {
		return &Failure{Stage: StageReminders, Err: err}
	}
	u.audit.RecordReminderRun(ctx, actor, client)
	return nil
}
