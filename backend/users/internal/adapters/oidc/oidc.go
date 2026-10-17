// Package oidc is the single sign-on provider of the users module, on the OpenID Connect
// client of the services.
package oidc

import (
	"context"

	"meerkat/config"
	"meerkat/services"
	"meerkat/users/internal/app"
	"meerkat/users/internal/domain"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Provider struct {
	provider    *services.OIDCProvider
	providerURL string
}

func New(provider *services.OIDCProvider, cfg *config.Config) *Provider {
	return &Provider{provider: provider, providerURL: cfg.OIDC.ProviderURL}
}

var _ app.Identity = (*Provider)(nil)

func (p *Provider) NewToken() (string, error)          { return services.GenerateStateToken() }
func (p *Provider) AuthURL(state, nonce string) string { return p.provider.BuildAuthURL(state, nonce) }

func (p *Provider) Exchange(ctx context.Context, code string) (app.Exchanged, error) {
	idToken, token, err := p.provider.ExchangeAndVerify(ctx, code)
	if err != nil {
		return nil, err
	}
	return &exchanged{provider: p, idToken: idToken, token: token}, nil
}

type exchanged struct {
	provider *Provider
	idToken  *gooidc.IDToken
	token    *oauth2.Token
}

func (e *exchanged) Nonce() string { return e.idToken.Nonce }

// Claims reads the ID token and, when it lacks the identity, asks the userinfo endpoint.
func (e *exchanged) Claims(ctx context.Context) (app.ClaimsResult, error) {
	claims, err := services.ExtractClaims(e.idToken, e.provider.providerURL)
	if err != nil {
		return app.ClaimsResult{}, err
	}
	var enrichErr error
	if claims.NeedsUserInfo() {
		enrichErr = e.provider.provider.EnrichFromUserInfo(ctx, e.token, claims)
	}
	return app.ClaimsResult{Claims: domain.OIDCClaims{
		Subject: claims.Subject, Email: claims.Email, EmailVerified: claims.EmailVerified,
		Name: claims.Name, PreferredUsername: claims.PreferredUsername, Provider: claims.Provider,
	}, EnrichErr: enrichErr}, nil
}
