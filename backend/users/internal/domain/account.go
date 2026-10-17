package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrRegistrationDisabled = errors.New("registration is disabled")
	ErrUserExists           = errors.New("user already exists")
)

// NewUser is an account to create. The first account of an instance becomes its administrator.
type NewUser struct {
	Username     string
	Email        string
	PasswordHash string
	Language     string
}

// Normalise lower-cases what identifies the account, so that matching is case-insensitive.
func (u *NewUser) Normalise() {
	u.Username, u.Email = strings.ToLower(u.Username), strings.ToLower(u.Email)
}

// Profile is the signed-in user as the UI needs them.
type Profile struct {
	ID                   UserID
	Username             string
	Email                string
	Language             string
	DateFormat           string
	IsAdmin              bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
	CustomFieldNames     []string
	EnabledContactFields []string
}
