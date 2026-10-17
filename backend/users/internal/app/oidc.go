package app

import (
	"context"
	"errors"
	"time"

	"meerkat/users/internal/domain"
)

// OIDCStart is where to send the browser, and the state and nonce it must bring back.
type OIDCStart struct{ URL, State, Nonce string }

// StartOIDC makes a fresh state and nonce for one sign-in attempt.
func (u *UseCases) StartOIDC() (OIDCStart, error) {
	state, err := u.identity.NewToken()
	if err != nil {
		return OIDCStart{}, &Failure{Stage: StageState, Err: err}
	}
	nonce, err := u.identity.NewToken()
	if err != nil {
		return OIDCStart{}, &Failure{Stage: StageNonce, Err: err}
	}
	return OIDCStart{URL: u.identity.AuthURL(state, nonce), State: state, Nonce: nonce}, nil
}

// OIDCLoggedIn is the new session, and what to log on the way: the userinfo lookup failing
// does not stop the login.
type OIDCLoggedIn struct {
	Token     string
	EnrichErr error
}

// CompleteOIDC signs in the identity the provider vouches for. Single sign-on is the second
// factor, so an account with two-factor is not asked for a code. Failures are
// *domain.OIDCFailure, domain.ErrOIDCDenied or domain.ErrOIDCNoAccount.
func (u *UseCases) CompleteOIDC(ctx context.Context, cb domain.OIDCCallback, client domain.Client, now time.Time) (OIDCLoggedIn, error) {
	if cb.ProviderError != "" {
		return OIDCLoggedIn{}, domain.ErrOIDCDenied
	}
	if err := cb.Check(); err != nil {
		return OIDCLoggedIn{}, err
	}
	exchanged, err := u.identity.Exchange(ctx, cb.Code)
	if err != nil {
		return OIDCLoggedIn{}, &domain.OIDCFailure{Reason: "token exchange/verification failed", Err: err}
	}
	if !domain.SameSecret(exchanged.Nonce(), cb.NonceCookie) {
		return OIDCLoggedIn{}, &domain.OIDCFailure{Reason: "nonce mismatch", Warn: true}
	}
	result, err := exchanged.Claims(ctx)
	if err != nil {
		return OIDCLoggedIn{}, &domain.OIDCFailure{Reason: "failed to extract claims", Err: err}
	}
	actor, err := u.accounts.ProvisionOIDC(ctx, result.Claims)
	if errors.Is(err, domain.ErrOIDCNoAccount) {
		return OIDCLoggedIn{}, err
	}
	if err != nil {
		return OIDCLoggedIn{}, &domain.OIDCFailure{Reason: "failed to find or provision user", Err: err}
	}
	token, err := u.sessions.Start(ctx, actor, client, now)
	if err != nil {
		return OIDCLoggedIn{}, &domain.OIDCFailure{Reason: "failed to generate JWT", Err: err}
	}
	u.audit.RecordWith(ctx, actor, domain.EventLogin, "method:oidc", client)
	return OIDCLoggedIn{Token: token, EnrichErr: result.EnrichErr}, nil
}
