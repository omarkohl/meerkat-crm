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

type UserID uint

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
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 {
		f.Limit = DefaultLimit
	}
	f.Limit = min(f.Limit, MaxLimit)
	f.Username = strings.ToLower(strings.TrimSpace(f.Username))
	return f
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
