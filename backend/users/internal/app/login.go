package app

import (
	"context"
	"errors"
	"time"

	"meerkat/users/internal/domain"
)

// LoggedIn is the outcome of a login request: the token of the new session and the user's
// preferences, or SecondFactorRequired when the password was right but the code is still due.
type LoggedIn struct {
	Token                string
	Language             string
	DateFormat           string
	SecondFactorRequired bool
}

func (u *UseCases) loginLocked(seconds int) error {
	return &domain.LockedError{RetryAfterSeconds: seconds, FromLogin: true}
}

// Login checks the credentials under the lockout of the identifier typed, and for accounts
// with two-factor a code or recovery code under the lockout of the account. Asking for the
// code is not a success: the identifier's counter is only cleared after the whole login, or
// the password alone would reset a code-guessing lockout. at is the real time for codes.
func (u *UseCases) Login(ctx context.Context, c domain.Credentials, client domain.Client, now, at time.Time) (LoggedIn, error) {
	if err := c.Validate(); err != nil {
		return LoggedIn{}, err
	}
	key := c.Key()
	if locked, secs := u.attempts.Locked(key); locked {
		return LoggedIn{}, u.loginLocked(secs)
	}

	acct, err := u.accounts.ForLogin(ctx, key, c.NamesEmail())
	if errors.Is(err, domain.ErrUserNotFound) {
		u.attempts.Fail(key)
		u.audit.LoginFailed(ctx, nil, "unknown_account", client)
		return LoggedIn{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return LoggedIn{}, &Failure{Stage: StageLogin, Err: err}
	}

	if !u.passwords.Matches(acct.PasswordHash, c.Password) {
		locked, secs := u.attempts.Fail(key)
		reason := "wrong_password"
		if locked {
			reason = "wrong_password_locked"
		}
		u.audit.LoginFailed(ctx, &acct.ID, reason, client)
		if locked {
			return LoggedIn{}, u.loginLocked(secs)
		}
		return LoggedIn{}, domain.ErrInvalidCredentials
	}

	if acct.TOTPEnabled {
		if !c.HasSecondFactor() {
			return LoggedIn{SecondFactorRequired: true}, nil
		}
		if err := u.checkSecondFactor(ctx, acct, c, key, client, at); err != nil {
			return LoggedIn{}, err
		}
	}

	u.attempts.Reset(key)
	token, err := u.sessions.Start(ctx, acct.ID, client, now)
	if err != nil {
		return LoggedIn{}, &Failure{Stage: StageStart, Err: err}
	}
	u.audit.Record(ctx, acct.ID, domain.EventLogin, client)
	return LoggedIn{Token: token, Language: acct.Language, DateFormat: acct.DateFormat}, nil
}

// checkSecondFactor verifies the code or recovery code under the account's own lockout, which
// is shared by every identifier of the account and by the two-factor dialogs.
func (u *UseCases) checkSecondFactor(ctx context.Context, acct domain.LoginAccount, c domain.Credentials, key string, client domain.Client, at time.Time) error {
	if locked, secs := u.lockout.Locked(acct.ID); locked {
		return &domain.LockedError{RetryAfterSeconds: secs}
	}
	ok, usedRecovery, err := u.factor.Check(ctx, acct.ID, c.TOTPCode, c.RecoveryCode, at)
	if err != nil {
		return &Failure{Stage: StageQuery, Err: err}
	}
	if ok {
		u.lockout.Reset(acct.ID)
		if usedRecovery {
			u.audit.Record(ctx, acct.ID, domain.EventTOTPRecoveryUsed, client)
		}
		return nil
	}
	accountLocked, accountSecs := u.lockout.Fail(acct.ID)
	locked, secs := u.attempts.Fail(key)
	if accountLocked {
		locked, secs = true, max(accountSecs, secs)
	}
	reason := "wrong_2fa_code"
	if locked {
		reason = "wrong_2fa_code_locked"
	}
	u.audit.LoginFailed(ctx, &acct.ID, reason, client)
	if locked {
		return &domain.LockedError{RetryAfterSeconds: secs}
	}
	return domain.ErrInvalidSecondFactor
}

// Logout ends the session the token names, if it is one of ours. Anything else is ignored: the
// route is public and logging out must always work.
func (u *UseCases) Logout(ctx context.Context, token string, client domain.Client, now time.Time) {
	if token == "" {
		return
	}
	sid := u.login.SessionID(token)
	if sid == "" {
		return
	}
	owner, found := u.sessions.Owner(ctx, sid)
	if !found {
		return
	}
	if revoked, err := u.sessions.Revoke(ctx, owner, sid, now); err == nil && revoked {
		u.audit.Record(ctx, owner, domain.EventLogout, client)
	}
}
