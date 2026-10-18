// Package domain is the CardDAV connection of the contactsync module and the rules for changing
// it; standard library only (docs/adr/0012-module-layout.md).
package domain

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

type UserID uint

const (
	TwoWay = "two_way"
	Pull   = "pull"
	Push   = "push"
)

var (
	ErrNotFound         = errors.New("CardDAV connection not found")
	ErrInvalidBaseURL   = errors.New("must be an http or https URL")
	ErrRelativePath     = errors.New("must be an absolute path")
	ErrPasswordRequired = errors.New("password required when the server address changes")
	ErrSyncRunning      = errors.New("a contact sync is already running for this connection")
)

type SyncStats struct {
	PulledCreated, PulledUpdated, PulledDeleted int
	PushedCreated, PushedUpdated, PushedDeleted int
	Conflicts, Skipped, Errors                  int
}

type Connection struct {
	ID              uint
	BaseURL         string
	AddressBookPath string
	AddressBookName string
	Username        string
	HasPassword     bool
	Direction       string
	SyncEnabled     bool
	SyncRunning     bool
	LastSyncedAt    *time.Time
	LastSyncStatus  string
	LastSyncError   string
	LastSyncStats   *SyncStats
	CreatedAt       time.Time
}

type Input struct {
	BaseURL         string
	AddressBookPath string
	AddressBookName string
	Username        string
	Password        string
	ClearPassword   bool
	Direction       string
	SyncEnabled     *bool
}

type PasswordMode int

const (
	KeepPassword PasswordMode = iota
	ClearPassword
	SetPassword
)

type PasswordChange struct {
	Mode  PasswordMode
	Plain string
}

type AddressBook struct{ Path, Name, Description string }

type SyncKind int

const (
	SyncFailed SyncKind = iota
	SyncInvalidURL
	SyncUnauthorized
	SyncForbidden
	SyncNotFound
	SyncPrivateAddress
	SyncTooLarge
	SyncTooMany
	SyncNoAddressBooks
	SyncInvalidData
	SyncUnreachable
)

type SyncError struct {
	Kind SyncKind
	Err  error
}

func (e *SyncError) Error() string { return e.Err.Error() }
func (e *SyncError) Unwrap() error { return e.Err }

// NormalizeBaseURL is the stored form of a server address: no inline credentials, no fragment.
func NormalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return "", ErrInvalidBaseURL
	}
	parsed.Fragment = ""
	parsed.User = nil
	return parsed.String(), nil
}

// Saved is the connection after a save with what the store must do besides writing its fields:
// change the password, and drop the link state when another address book was chosen.
type Saved struct {
	Connection Connection
	Password   PasswordChange
	IsNew      bool
	ResetLinks bool
}

// Save applies in to the connection, which is nil when the user has none. The link state is
// scoped to one address book, so switching servers or books resets it. A stored password may not
// follow the connection to another server.
func Save(existing *Connection, in Input, sameServer func(a, b string) bool) (Saved, error) {
	baseURL, err := NormalizeBaseURL(in.BaseURL)
	if err != nil {
		return Saved{}, err
	}
	if !strings.HasPrefix(in.AddressBookPath, "/") {
		return Saved{}, ErrRelativePath
	}
	out := Saved{IsNew: existing == nil}
	var c Connection
	if existing != nil {
		c = *existing
		out.ResetLinks = c.BaseURL != baseURL || c.AddressBookPath != in.AddressBookPath
		if c.HasPassword && in.Password == "" && !in.ClearPassword && !sameServer(c.BaseURL, baseURL) {
			return Saved{}, ErrPasswordRequired
		}
	}
	c.BaseURL, c.AddressBookPath, c.AddressBookName, c.Username = baseURL, in.AddressBookPath, in.AddressBookName, in.Username
	switch {
	case in.Direction != "":
		c.Direction = in.Direction
	case c.Direction == "":
		c.Direction = TwoWay
	}
	if in.SyncEnabled != nil {
		c.SyncEnabled = *in.SyncEnabled
	} else if out.IsNew {
		c.SyncEnabled = true
	}
	switch {
	case in.ClearPassword:
		c.HasPassword = false
		out.Password = PasswordChange{Mode: ClearPassword}
	case in.Password != "":
		c.HasPassword = true
		out.Password = PasswordChange{Mode: SetPassword, Plain: in.Password}
	}
	if out.ResetLinks {
		c.LastSyncedAt, c.LastSyncStatus, c.LastSyncError, c.LastSyncStats = nil, "", "", nil
	}
	out.Connection = c
	return out, nil
}
