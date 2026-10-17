// Package app holds the use cases of the users module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"
	"time"

	"meerkat/users/internal/domain"
)

type Preferences interface {
	CustomFieldNames(ctx context.Context, actor domain.UserID) ([]string, error)
	EnabledContactFields(ctx context.Context, actor domain.UserID) ([]string, error)
	SetLanguage(ctx context.Context, actor domain.UserID, language string) error
	SetDateFormat(ctx context.Context, actor domain.UserID, format string) error
	SetCustomFieldNames(ctx context.Context, actor domain.UserID, names []string) error
	SetEnabledContactFields(ctx context.Context, actor domain.UserID, fields []string) error
}

type Sessions interface {
	Live(ctx context.Context, actor domain.UserID, now time.Time) ([]domain.Session, error)
	// Revoke ends a live session of the actor and reports whether there was one.
	Revoke(ctx context.Context, actor domain.UserID, id string, now time.Time) (bool, error)
	// RevokeAll ends every session of the actor and raises the token version, so tokens
	// issued earlier die too.
	RevokeAll(ctx context.Context, actor domain.UserID, now time.Time) error
	RevokeOthers(ctx context.Context, actor domain.UserID, keep string, now time.Time) error
	Start(ctx context.Context, actor domain.UserID, client domain.Client, now time.Time) (string, error)
}

type Accounts interface {
	PasswordHash(ctx context.Context, actor domain.UserID) (string, error)
	Account(ctx context.Context, actor domain.UserID) (domain.Account, error)
	// ByEmail finds an account for a password reset; domain.ErrUserNotFound when there is none.
	ByEmail(ctx context.Context, email string) (domain.ResetTarget, error)
	StoreResetToken(ctx context.Context, actor domain.UserID, hash string, expires, requested time.Time) error
	// ResetToken finds the token with this hash; domain.ErrResetTokenInvalid when there is none.
	ResetToken(ctx context.Context, hash string) (domain.ResetToken, error)
	// ClearResetToken removes the token with this hash; a newer token stays.
	ClearResetToken(ctx context.Context, actor domain.UserID, hash string) error
	// ResetPassword stores the hash and ends every session if the account still holds the
	// token with tokenHash, which is what makes a token work once. Otherwise it returns
	// domain.ErrResetTokenInvalid.
	ResetPassword(ctx context.Context, actor domain.UserID, tokenHash, hash string, now time.Time) error
	// ChangePassword stores the new hash, clears any pending password reset and ends every
	// session, as one change.
	ChangePassword(ctx context.Context, actor domain.UserID, hash string, now time.Time) error
}

// SecondFactor is the authenticator-app (TOTP) secret and recovery codes of the actor.
type SecondFactor interface {
	RecoveryCodesLeft(ctx context.Context, actor domain.UserID) (int, error)
	// BeginSetup stores a pending secret and returns it with the URI for the QR code. It
	// returns domain.ErrTwoFactorEnabled when two-factor is already on.
	BeginSetup(ctx context.Context, actor domain.UserID, username string) (secret, uri string, err error)
	// Enable turns two-factor on if code fits the pending secret and returns the recovery
	// codes. Errors: domain.ErrInvalidVerificationCode, domain.ErrTwoFactorEnabled.
	Enable(ctx context.Context, actor domain.UserID, code string, at time.Time) ([]string, error)
	// Check accepts an authenticator code or, with none, a recovery code; each works once.
	Check(ctx context.Context, actor domain.UserID, code, recovery string, at time.Time) (ok, usedRecovery bool, err error)
	Disable(ctx context.Context, actor domain.UserID) error
	NewRecoveryCodes(ctx context.Context, actor domain.UserID) ([]string, error)
}

// Lockout counts wrong second-factor attempts per account; login shares the same counter.
type Lockout interface {
	Locked(actor domain.UserID) (bool, int)
	Fail(actor domain.UserID) (locked bool, seconds int)
	Reset(actor domain.UserID)
}

type Passwords interface {
	Matches(hash, password string) bool
	// Hash returns domain.ErrPasswordTooLong for a password bcrypt cannot take.
	Hash(password string) (string, error)
	Strength(password string) domain.Strength
}

// ResetTokens makes password reset tokens. Only the hash is stored; the token goes to the
// user in the mail.
type ResetTokens interface {
	New() (token, hash string, err error)
	Hash(token string) string
	Expiry(now time.Time) time.Time
}

// ResetMail sends the reset instructions; the mail is queued, so a down mail server does not
// fail the request.
type ResetMail interface {
	Send(ctx context.Context, to domain.ResetTarget, token string) error
}

// Audit records security events; it never fails the use case.
type Audit interface {
	Record(ctx context.Context, actor domain.UserID, event string, client domain.Client)
}

type Stage string

const (
	StageQuery         Stage = "query"
	StageUpdate        Stage = "update"
	StageSessionList   Stage = "session-list"
	StageSessionRevoke Stage = "session-revoke"
	StageHash          Stage = "hash"
	StageSetup         Stage = "setup"
	StageToken         Stage = "token"
	StageMail          Stage = "mail"
	StageEnable        Stage = "enable"
	StageFactor        Stage = "factor"
)

// Failure wraps an infrastructure error with the stage it happened in. A missing user and a
// value that is not supported are domain errors.
type Failure struct {
	Stage Stage
	Err   error
}

func (f *Failure) Error() string { return string(f.Stage) + ": " + f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

type UseCases struct {
	prefs     Preferences
	languages []string
	sessions  Sessions
	accounts  Accounts
	passwords Passwords
	audit     Audit
	factor    SecondFactor
	lockout   Lockout
	tokens    ResetTokens
	mail      ResetMail
}

func New(prefs Preferences, supportedLanguages []string, sessions Sessions, accounts Accounts, passwords Passwords, audit Audit, factor SecondFactor, lockout Lockout, tokens ResetTokens, mail ResetMail) *UseCases {
	return &UseCases{
		prefs: prefs, languages: supportedLanguages, sessions: sessions, accounts: accounts, passwords: passwords,
		audit: audit, factor: factor, lockout: lockout, tokens: tokens, mail: mail,
	}
}

func stage(s Stage, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrUserNotFound) {
		return err
	}
	return &Failure{Stage: s, Err: err}
}

func (u *UseCases) SetLanguage(ctx context.Context, actor domain.UserID, language string) error {
	if err := domain.ValidateLanguage(language, u.languages); err != nil {
		return err
	}
	return stage(StageUpdate, u.prefs.SetLanguage(ctx, actor, language))
}

func (u *UseCases) SetDateFormat(ctx context.Context, actor domain.UserID, format string) error {
	if err := domain.ValidateDateFormat(format); err != nil {
		return err
	}
	return stage(StageUpdate, u.prefs.SetDateFormat(ctx, actor, format))
}

func (u *UseCases) CustomFieldNames(ctx context.Context, actor domain.UserID) ([]string, error) {
	names, err := u.prefs.CustomFieldNames(ctx, actor)
	if err != nil {
		return nil, stage(StageQuery, err)
	}
	if names == nil {
		names = []string{}
	}
	return names, nil
}

func (u *UseCases) SetCustomFieldNames(ctx context.Context, actor domain.UserID, names []string) error {
	return stage(StageUpdate, u.prefs.SetCustomFieldNames(ctx, actor, names))
}

// EnabledContactFields returns the user's fields, nil when never configured so the client
// applies its defaults.
func (u *UseCases) EnabledContactFields(ctx context.Context, actor domain.UserID) ([]string, error) {
	fields, err := u.prefs.EnabledContactFields(ctx, actor)
	return fields, stage(StageQuery, err)
}

func (u *UseCases) SetEnabledContactFields(ctx context.Context, actor domain.UserID, fields []string) error {
	return stage(StageUpdate, u.prefs.SetEnabledContactFields(ctx, actor, fields))
}

func (u *UseCases) ListSessions(ctx context.Context, actor domain.UserID, current string, now time.Time) ([]domain.Session, error) {
	list, err := u.sessions.Live(ctx, actor, now)
	if err != nil {
		return nil, stage(StageSessionList, err)
	}
	for i := range list {
		list[i].Current = current != "" && list[i].ID == current
	}
	return list, nil
}

func (u *UseCases) RevokeSession(ctx context.Context, actor domain.UserID, id string, client domain.Client, now time.Time) error {
	if !domain.ValidSessionID(id) {
		return domain.ErrSessionNotFound
	}
	revoked, err := u.sessions.Revoke(ctx, actor, id, now)
	if err != nil {
		return stage(StageSessionRevoke, err)
	}
	if !revoked {
		return domain.ErrSessionNotFound
	}
	u.audit.Record(ctx, actor, domain.EventSessionRevoked, client)
	return nil
}

func (u *UseCases) SignOutEverywhere(ctx context.Context, actor domain.UserID, client domain.Client, now time.Time) error {
	if err := u.sessions.RevokeAll(ctx, actor, now); err != nil {
		return stage(StageSessionRevoke, err)
	}
	u.audit.Record(ctx, actor, domain.EventSignOutEverywhere, client)
	return nil
}

// PasswordChanged carries the token of the session started after a password change; when it
// could not be started the change still stands and SessionErr says why.
type PasswordChanged struct {
	Token      string
	SessionErr error
}

// ChangePassword rotates the password. The hash is made before the transaction. Every other
// browser is signed out and the caller gets a new session unless startSession is false.
func (u *UseCases) ChangePassword(ctx context.Context, actor domain.UserID, current, next string, startSession bool, client domain.Client, now time.Time) (PasswordChanged, error) {
	hash, err := u.accounts.PasswordHash(ctx, actor)
	if err != nil {
		return PasswordChanged{}, stage(StageQuery, err)
	}
	if !u.passwords.Matches(hash, current) {
		return PasswordChanged{}, domain.ErrCurrentPasswordWrong
	}
	if u.passwords.Matches(hash, next) {
		return PasswordChanged{}, domain.ErrPasswordUnchanged
	}
	newHash, err := u.passwords.Hash(next)
	if errors.Is(err, domain.ErrPasswordTooLong) {
		return PasswordChanged{}, err
	}
	if err != nil {
		return PasswordChanged{}, &Failure{Stage: StageHash, Err: err}
	}
	if err := u.accounts.ChangePassword(ctx, actor, newHash, now); err != nil {
		return PasswordChanged{}, stage(StageUpdate, err)
	}
	var out PasswordChanged
	if startSession {
		out.Token, out.SessionErr = u.sessions.Start(ctx, actor, client, now)
	}
	u.audit.Record(ctx, actor, domain.EventPasswordChanged, client)
	return out, nil
}
