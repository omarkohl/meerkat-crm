package app

import (
	"context"
	"errors"
	"time"

	"meerkat/users/internal/domain"
)

type TwoFactorStatus struct {
	Enabled     bool
	CodesLeft   int
	Available   bool
	ServerReady bool
}

func (u *UseCases) TwoFactorStatus(ctx context.Context, actor domain.UserID) (TwoFactorStatus, error) {
	acct, err := u.accounts.Account(ctx, actor)
	if err != nil {
		return TwoFactorStatus{}, stage(StageQuery, err)
	}
	st := TwoFactorStatus{Enabled: acct.TOTPEnabled, Available: acct.HasPassword(), ServerReady: u.factor.ServerReady()}
	if acct.TOTPEnabled {
		if st.CodesLeft, err = u.factor.RecoveryCodesLeft(ctx, actor); err != nil {
			return TwoFactorStatus{}, stage(StageQuery, err)
		}
	}
	return st, nil
}

type TwoFactorSetup struct{ Secret, URI string }

func (u *UseCases) BeginTwoFactorSetup(ctx context.Context, actor domain.UserID) (TwoFactorSetup, error) {
	acct, err := u.accounts.Account(ctx, actor)
	if err != nil {
		return TwoFactorSetup{}, stage(StageQuery, err)
	}
	if !acct.HasPassword() {
		return TwoFactorSetup{}, domain.ErrTwoFactorNotAvailable
	}
	if !u.factor.ServerReady() {
		return TwoFactorSetup{}, domain.ErrTwoFactorNoServerKey
	}
	secret, uri, err := u.factor.BeginSetup(ctx, actor, acct.Username)
	if errors.Is(err, domain.ErrTwoFactorEnabled) || errors.Is(err, domain.ErrTwoFactorNoServerKey) {
		return TwoFactorSetup{}, err
	}
	if err != nil {
		return TwoFactorSetup{}, &Failure{Stage: StageSetup, Err: err}
	}
	return TwoFactorSetup{Secret: secret, URI: uri}, nil
}

func (u *UseCases) locked(actor domain.UserID) error {
	if locked, secs := u.lockout.Locked(actor); locked {
		return &domain.LockedError{RetryAfterSeconds: secs}
	}
	return nil
}

func (u *UseCases) failed(actor domain.UserID, plain error) error {
	if locked, secs := u.lockout.Fail(actor); locked {
		return &domain.LockedError{RetryAfterSeconds: secs}
	}
	return plain
}

// checkPassword verifies the password; a wrong one counts on the second-factor lockout, so
// the dialogs cannot be used to guess it.
func (u *UseCases) checkPassword(actor domain.UserID, acct domain.Account, password string) error {
	if !u.passwords.Matches(acct.PasswordHash, password) {
		return u.failed(actor, domain.ErrWrongPassword)
	}
	return nil
}

// TwoFactorChanged is the outcome of a change that signs the other browsers out. When that
// failed the change stands and SignOutErr says why.
type TwoFactorChanged struct {
	RecoveryCodes []string
	SignOutErr    error
}

// EnableTwoFactor turns two-factor on after the password and a valid code, and returns the
// recovery codes, which are shown this once. Other sessions are signed out at now. Codes are
// checked at at, the real time: authenticator apps use it whatever the application clock says.
func (u *UseCases) EnableTwoFactor(ctx context.Context, actor domain.UserID, password, code, keepSession string, client domain.Client, now, at time.Time) (TwoFactorChanged, error) {
	acct, err := u.accounts.Account(ctx, actor)
	if err != nil {
		return TwoFactorChanged{}, stage(StageQuery, err)
	}
	if !acct.HasPassword() {
		return TwoFactorChanged{}, domain.ErrTwoFactorNotAvailable
	}
	if acct.TOTPEnabled {
		return TwoFactorChanged{}, domain.ErrTwoFactorEnabled
	}
	if err := u.locked(actor); err != nil {
		return TwoFactorChanged{}, err
	}
	if err := u.checkPassword(actor, acct, password); err != nil {
		return TwoFactorChanged{}, err
	}
	codes, err := u.factor.Enable(ctx, actor, code, at)
	switch {
	case errors.Is(err, domain.ErrInvalidVerificationCode):
		return TwoFactorChanged{}, u.failed(actor, err)
	case errors.Is(err, domain.ErrTwoFactorEnabled):
		return TwoFactorChanged{}, err
	case err != nil:
		return TwoFactorChanged{}, &Failure{Stage: StageEnable, Err: err}
	}
	u.lockout.Reset(actor)
	signOutErr := u.sessions.RevokeOthers(ctx, actor, keepSession, now)
	u.audit.Record(ctx, actor, domain.EventTOTPEnabled, client)
	return TwoFactorChanged{RecoveryCodes: codes, SignOutErr: signOutErr}, nil
}

func (u *UseCases) confirm(ctx context.Context, actor domain.UserID, password, code, recovery string, client domain.Client, at time.Time) error {
	acct, err := u.accounts.Account(ctx, actor)
	if err != nil {
		return stage(StageQuery, err)
	}
	if !acct.TOTPEnabled {
		return domain.ErrTwoFactorNotEnabled
	}
	if err := u.locked(actor); err != nil {
		return err
	}
	if err := u.checkPassword(actor, acct, password); err != nil {
		return err
	}
	ok, usedRecovery, err := u.factor.Check(ctx, actor, code, recovery, at)
	if err != nil {
		return &Failure{Stage: StageQuery, Err: err}
	}
	if !ok {
		return u.failed(actor, domain.ErrInvalidVerificationCode)
	}
	u.lockout.Reset(actor)
	if usedRecovery {
		u.audit.Record(ctx, actor, domain.EventTOTPRecoveryUsed, client)
	}
	return nil
}

func (u *UseCases) DisableTwoFactor(ctx context.Context, actor domain.UserID, password, code, recovery, keepSession string, client domain.Client, now, at time.Time) (TwoFactorChanged, error) {
	if err := u.confirm(ctx, actor, password, code, recovery, client, at); err != nil {
		return TwoFactorChanged{}, err
	}
	if err := u.factor.Disable(ctx, actor); err != nil {
		return TwoFactorChanged{}, &Failure{Stage: StageFactor, Err: err}
	}
	signOutErr := u.sessions.RevokeOthers(ctx, actor, keepSession, now)
	u.audit.Record(ctx, actor, domain.EventTOTPDisabled, client)
	return TwoFactorChanged{SignOutErr: signOutErr}, nil
}

func (u *UseCases) RegenerateRecoveryCodes(ctx context.Context, actor domain.UserID, password, code, recovery string, client domain.Client, at time.Time) ([]string, error) {
	if err := u.confirm(ctx, actor, password, code, recovery, client, at); err != nil {
		return nil, err
	}
	codes, err := u.factor.NewRecoveryCodes(ctx, actor)
	if err != nil {
		return nil, &Failure{Stage: StageFactor, Err: err}
	}
	u.audit.Record(ctx, actor, domain.EventTOTPRecoveryRegenerated, client)
	return codes, nil
}
