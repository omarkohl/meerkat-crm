package domain

import (
	"errors"
	"regexp"
	"time"
)

var (
	ErrSessionNotFound      = errors.New("session not found")
	ErrCurrentPasswordWrong = errors.New("current password is incorrect")
	ErrPasswordUnchanged    = errors.New("new password must differ from current password")
	ErrPasswordTooLong      = errors.New("password must not exceed 72 characters")
)

var sessionIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ValidSessionID reports whether id has the shape of a session ID, which saves a lookup.
func ValidSessionID(id string) bool { return sessionIDPattern.MatchString(id) }

// Session is one browser login of a user.
type Session struct {
	ID         string
	CreatedAt  time.Time
	UserAgent  string
	IP         string
	LastSeenAt time.Time
	ExpiresAt  time.Time
	Current    bool
}

// Client describes where a request came from.
type Client struct {
	IP        string
	UserAgent string
}

// Audit events the module records.
const (
	EventSessionRevoked    = "session_revoked"
	EventSignOutEverywhere = "sign_out_everywhere"
	EventPasswordChanged   = "password_changed"
)
