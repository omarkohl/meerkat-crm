package domain

import (
	"errors"
	"time"
)

var ErrResetTokenInvalid = errors.New("password reset token is invalid or expired")

const EventPasswordReset = "password_reset"

type ResetTarget struct {
	ID       UserID
	Email    string
	Language string
}

type ResetToken struct {
	UserID    UserID
	ExpiresAt *time.Time
}

// Expired is true once the token's time is up, and for a token that has none.
func (t ResetToken) Expired(now time.Time) bool { return t.ExpiresAt == nil || now.After(*t.ExpiresAt) }

type Strength struct {
	IsValid     bool
	Entropy     float64
	Score       int
	Feedback    string
	MinEntropy  float64
	CharSetSize int
	Length      int
}
