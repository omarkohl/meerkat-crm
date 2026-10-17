package domain

import (
	"errors"
	"strconv"
)

var (
	ErrTwoFactorEnabled        = errors.New("two-factor authentication is already enabled")
	ErrTwoFactorNotEnabled     = errors.New("two-factor authentication is not enabled")
	ErrTwoFactorNotAvailable   = errors.New("two-factor authentication is not available for single sign-on accounts")
	ErrWrongPassword           = errors.New("password is incorrect")
	ErrInvalidVerificationCode = errors.New("invalid verification code")
)

// LockedError means too many wrong second-factor attempts: the account is locked for a while.
type LockedError struct {
	RetryAfterSeconds int
	// FromLogin is true for the lock on the identifier typed at login, false for the lock on
	// the account's second factor; they answer in different words.
	FromLogin bool
}

func (e *LockedError) Error() string {
	return "account locked for " + strconv.Itoa(e.RetryAfterSeconds) + " seconds"
}

// Account is what the two-factor flows need to know about the actor.
type Account struct {
	Username     string
	PasswordHash string
	TOTPEnabled  bool
}

// HasPassword is false for accounts that only sign in through single sign-on, which have no
// password step to protect and cannot confirm changes.
func (a Account) HasPassword() bool { return a.PasswordHash != "" }

// Audit events of the two-factor flows.
const (
	EventTOTPEnabled             = "totp_enabled"
	EventTOTPDisabled            = "totp_disabled"
	EventTOTPRecoveryUsed        = "totp_recovery_used"
	EventTOTPRecoveryRegenerated = "totp_recovery_regenerated"
)
