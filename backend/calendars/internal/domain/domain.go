// Package domain is the vocabulary of the calendars module: a user's calendar subscriptions and
// the rules for changing them. It imports nothing but the standard library
// (docs/adr/0012-module-layout.md).
package domain

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

type UserID uint

type SubscriptionID uint

const (
	MaxPerUser        = 10
	DefaultPastDays   = 5
	DefaultFutureDays = 10
)

var (
	ErrNotFound         = errors.New("calendar subscription not found")
	ErrLimit            = errors.New("calendar subscription limit reached")
	ErrInvalidURL       = errors.New("must be an http, https, or webcal URL")
	ErrPasswordRequired = errors.New("password required when the server address changes")
)

type Subscription struct {
	ID             SubscriptionID
	Name           string
	URL            string
	Username       string
	HasPassword    bool
	SyncEnabled    bool
	PastDays       int
	FutureDays     int
	LastSyncedAt   *time.Time
	LastSyncStatus string
	LastSyncError  string
	CreatedAt      time.Time
}

type Input struct {
	Name          string
	URL           string
	Username      string
	Password      string
	ClearPassword bool
	SyncEnabled   *bool
	PastDays      *int
	FutureDays    *int
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

type SyncStats struct{ Created, Updated, Skipped int }

type SyncKind int

const (
	SyncFailed SyncKind = iota
	SyncInvalidURL
	SyncUnauthorized
	SyncNotFound
	SyncPrivateAddress
	SyncTooLarge
	SyncInvalidData
	SyncUnreachable
)

type SyncError struct {
	Kind SyncKind
	Err  error
}

func (e *SyncError) Error() string { return e.Err.Error() }
func (e *SyncError) Unwrap() error { return e.Err }

func CheckURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ErrInvalidURL
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https" && scheme != "webcal") || parsed.Hostname() == "" {
		return ErrInvalidURL
	}
	return nil
}

func New(in Input) (Subscription, error) {
	if err := CheckURL(in.URL); err != nil {
		return Subscription{}, err
	}
	s := Subscription{
		Name: in.Name, URL: in.URL, Username: in.Username, HasPassword: in.Password != "",
		SyncEnabled: in.SyncEnabled == nil || *in.SyncEnabled, PastDays: DefaultPastDays, FutureDays: DefaultFutureDays,
	}
	if in.PastDays != nil {
		s.PastDays = *in.PastDays
	}
	if in.FutureDays != nil {
		s.FutureDays = *in.FutureDays
	}
	return s, nil
}

// Change applies in to s. A stored password may not follow the subscription to another server:
// that needs a new password or clearing it.
func (s Subscription) Change(in Input, sameServer func(a, b string) bool) (Subscription, PasswordChange, error) {
	if err := CheckURL(in.URL); err != nil {
		return s, PasswordChange{}, err
	}
	if s.HasPassword && in.Password == "" && !in.ClearPassword && !sameServer(s.URL, in.URL) {
		return s, PasswordChange{}, ErrPasswordRequired
	}
	s.Name, s.URL, s.Username = in.Name, in.URL, in.Username
	if in.SyncEnabled != nil {
		s.SyncEnabled = *in.SyncEnabled
	}
	if in.PastDays != nil {
		s.PastDays = *in.PastDays
	}
	if in.FutureDays != nil {
		s.FutureDays = *in.FutureDays
	}
	switch {
	case in.ClearPassword:
		s.HasPassword = false
		return s, PasswordChange{Mode: ClearPassword}, nil
	case in.Password != "":
		s.HasPassword = true
		return s, PasswordChange{Mode: SetPassword, Plain: in.Password}, nil
	}
	return s, PasswordChange{}, nil
}
