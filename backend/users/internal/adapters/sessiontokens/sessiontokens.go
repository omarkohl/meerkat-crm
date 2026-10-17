// Package sessiontokens reads the session out of a login token.
package sessiontokens

import (
	"meerkat/config"
	"meerkat/services"
	"meerkat/users/internal/app"
)

type JWT struct{ cfg *config.Config }

func New(cfg *config.Config) JWT { return JWT{cfg: cfg} }

var _ app.SessionTokens = JWT{}

func (t JWT) SessionID(token string) string { return services.SessionIDFromToken(token, t.cfg) }
