// Package domain holds the vocabulary and rules of the admin module (backups, the audit log
// and the system status). It imports the standard library only.
package domain

import (
	"errors"
	"strings"
	"time"
)

// ErrBackupNotFound means there is no such backup: the name is not one the service creates, or
// the file is gone.
var ErrBackupNotFound = errors.New("backup not found")

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrOwnAdminStatus  = errors.New("cannot remove your own admin status")
	ErrOwnAccount      = errors.New("cannot delete your own account")
	ErrLastAdmin       = errors.New("cannot remove or delete the last admin")
	ErrTwoFactorOff    = errors.New("two-factor authentication is not enabled for this user")
	ErrAlreadyExists   = errors.New("username or email already in use")
	ErrPasswordTooLong = errors.New("password must not exceed 72 characters")
	ErrCannotHash      = errors.New("could not hash password")
)

type UserID uint

// User is an account as an administrator sees it: never a password or a secret.
type User struct {
	ID         UserID
	Username   string
	Email      string
	Language   string
	DateFormat string
	IsAdmin    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
	// TwoFactor tells an administrator whether a reset is needed.
	TwoFactor bool
}

// UserChanges are the fields an administrator sets; nil leaves a field alone.
type UserChanges struct {
	Username *string
	Email    *string
	Password *string
	IsAdmin  *bool
}

// Fields names what the changes touch, in the order the audit entry lists them.
func (c UserChanges) Fields() []string {
	var out []string
	for _, f := range []struct {
		name string
		set  bool
	}{{"username", c.Username != nil}, {"email", c.Email != nil}, {"password", c.Password != nil}, {"is_admin", c.IsAdmin != nil}} {
		if f.set {
			out = append(out, f.name)
		}
	}
	return out
}

// Apply returns the user with the changes made: username and email are stored lower-case.
func (c UserChanges) Apply(u User) User {
	if c.Username != nil {
		u.Username = strings.ToLower(*c.Username)
	}
	if c.Email != nil {
		u.Email = strings.ToLower(*c.Email)
	}
	if c.IsAdmin != nil {
		u.IsAdmin = *c.IsAdmin
	}
	return u
}

type UserAction string

const (
	UserUpdated      UserAction = "updated"
	UserDeleted      UserAction = "deleted"
	UserTwoFactorOff UserAction = "two-factor reset"
)

type Client struct{ IP, UserAgent string }

type Backup struct {
	Name      string
	Size      int64
	CreatedAt time.Time
}

type BackupSettings struct {
	Enabled        bool
	Time           string
	RetentionCount int
	Directory      string
}

type BackupAction string

const (
	BackupCreated    BackupAction = "created"
	BackupDownloaded BackupAction = "downloaded"
	BackupDeleted    BackupAction = "deleted"
)

type AuditEntry struct {
	ID        uint
	CreatedAt time.Time
	UserID    *UserID
	Username  string
	Event     string
	IP        string
	UserAgent string
	Detail    string
}

// AuditFilter narrows the audit log. Event is an exact match and Username a case-insensitive
// substring.
type AuditFilter struct {
	Event    string
	Username string
	Page     int
	Limit    int
}

const (
	DefaultLimit = 25
	MaxLimit     = 100
)

// Normalised puts the filter into the shape the log is queried with: a page of 1 to MaxLimit
// entries, and a trimmed, lower-case username.
func (f AuditFilter) Normalised() AuditFilter {
	f.Page, f.Limit = Paging(f.Page, f.Limit)
	f.Username = strings.ToLower(strings.TrimSpace(f.Username))
	return f
}

// Paging gives the page and limit a list is read with: from 1, and 1 to MaxLimit entries.
func Paging(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = DefaultLimit
	}
	return page, min(limit, MaxLimit)
}

func (f AuditFilter) Offset() int { return (f.Page - 1) * f.Limit }

// SystemSettings are the deployment settings the status and the listings report.
type SystemSettings struct {
	Backup             BackupSettings
	AuditRetentionDays int
	PrivateURLsAllowed bool
	PrivateURLFeatures []string
}

type SystemStatus struct {
	PrivateURLsAllowed bool
	UserCount          int64
	PrivateURLWarning  bool
	Features           []string
}

// PrivateURLWarning is true when outbound features can reach private addresses on an instance
// with more than one user, where one user could point them at the server's internal network.
func PrivateURLWarning(privateURLsAllowed bool, users int64) bool {
	return privateURLsAllowed && users > 1
}
