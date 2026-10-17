// Package resettokens makes password reset tokens on the services the other flows share.
package resettokens

import (
	"time"

	"meerkat/services"
	"meerkat/users/internal/app"
)

type Tokens struct{}

var _ app.ResetTokens = Tokens{}

func (Tokens) New() (string, string, error)   { return services.GeneratePasswordResetToken() }
func (Tokens) Hash(token string) string       { return services.HashPasswordResetToken(token) }
func (Tokens) Expiry(now time.Time) time.Time { return services.PasswordResetExpiry(now) }
