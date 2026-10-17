package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"meerkat/users/internal/domain"
)

func (u *UseCases) PasswordStrength(password string) domain.Strength {
	return u.passwords.Strength(password)
}

// RequestPasswordReset mails reset instructions. An unknown email does nothing and is no
// error, so the answer does not say which accounts exist.
func (u *UseCases) RequestPasswordReset(ctx context.Context, email string, now time.Time) error {
	target, err := u.accounts.ByEmail(ctx, strings.ToLower(email))
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return &Failure{Stage: StageQuery, Err: err}
	}
	token, hash, err := u.tokens.New()
	if err != nil {
		return &Failure{Stage: StageToken, Err: err}
	}
	if err := u.accounts.StoreResetToken(ctx, target.ID, hash, u.tokens.Expiry(now), now); err != nil {
		return &Failure{Stage: StageUpdate, Err: err}
	}
	if err := u.mail.Send(ctx, target, token); err != nil {
		return &Failure{Stage: StageMail, Err: err}
	}
	return nil
}

// ResetTokenRejected is domain.ErrResetTokenInvalid; ClearErr says why an expired token could
// not be removed.
type ResetTokenRejected struct{ ClearErr error }

func (e *ResetTokenRejected) Error() string        { return domain.ErrResetTokenInvalid.Error() }
func (e *ResetTokenRejected) Is(target error) bool { return target == domain.ErrResetTokenInvalid }

// ConfirmPasswordReset sets a new password for the holder of a valid token, which then works
// no more, and ends every session: the old password may be known to someone else.
func (u *UseCases) ConfirmPasswordReset(ctx context.Context, token, password string, client domain.Client, now time.Time) error {
	tokenHash := u.tokens.Hash(token)
	found, err := u.accounts.ResetToken(ctx, tokenHash)
	if errors.Is(err, domain.ErrResetTokenInvalid) {
		return &ResetTokenRejected{}
	}
	if err != nil {
		return &Failure{Stage: StageQuery, Err: err}
	}
	if found.Expired(now) {
		return &ResetTokenRejected{ClearErr: u.accounts.ClearResetToken(ctx, found.UserID, tokenHash)}
	}
	hash, err := u.passwords.Hash(password)
	if errors.Is(err, domain.ErrPasswordTooLong) {
		return err
	}
	if err != nil {
		return &Failure{Stage: StageHash, Err: err}
	}
	err = u.accounts.ResetPassword(ctx, found.UserID, tokenHash, hash, now)
	if errors.Is(err, domain.ErrResetTokenInvalid) {
		return &ResetTokenRejected{}
	}
	if err != nil {
		return &Failure{Stage: StageUpdate, Err: err}
	}
	u.audit.Record(ctx, found.UserID, domain.EventPasswordReset, client)
	return nil
}
