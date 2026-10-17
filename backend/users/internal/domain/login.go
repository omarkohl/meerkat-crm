package domain

import (
	"errors"
	"strings"
)

var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidSecondFactor = errors.New("invalid second-factor code")
)

// MissingFieldError names the field a login request left out.
type MissingFieldError struct{ Field string }

func (e *MissingFieldError) Error() string { return "missing field " + e.Field }

// Audit events of the login flow.
const (
	EventLogin       = "login"
	EventLoginFailed = "login_failed"
	EventLogout      = "logout"
)

// LoginAccount is the account a login names.
type LoginAccount struct {
	ID           UserID
	PasswordHash string
	TOTPEnabled  bool
	Language     string
	DateFormat   string
}

// Credentials are what a login request carries. Identifier is a username or an email.
type Credentials struct {
	Identifier   string
	Password     string
	TOTPCode     string
	RecoveryCode string
}

// Key is the identifier the failed attempts are counted under: lower case, whatever it names.
func (c Credentials) Key() string { return strings.ToLower(c.Identifier) }

// NamesEmail is true when the identifier is an email address rather than a username.
func (c Credentials) NamesEmail() bool { return strings.Contains(c.Identifier, "@") }

// HasSecondFactor is true when the request carries a code for the second step.
func (c Credentials) HasSecondFactor() bool { return c.TOTPCode != "" || c.RecoveryCode != "" }

// Validate says which required field is missing.
func (c Credentials) Validate() error {
	if c.Identifier == "" {
		return &MissingFieldError{Field: "identifier"}
	}
	if c.Password == "" {
		return &MissingFieldError{Field: "password"}
	}
	return nil
}
