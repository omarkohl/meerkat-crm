// Package lockout counts wrong second-factor attempts per account on the limiter that the
// login uses too.
package lockout

import (
	"meerkat/middleware"
	"meerkat/users/internal/app"
	"meerkat/users/internal/domain"
)

// Identifiers counts failed logins by the identifier typed; the key is the identifier itself,
// as it is for the CardDAV login.
type Identifiers struct{}

var _ app.Attempts = Identifiers{}

func (Identifiers) Locked(identifier string) (bool, int) {
	return middleware.GetAccountRateLimiter().IsLocked(identifier)
}

func (Identifiers) Fail(identifier string) (bool, int) {
	return middleware.GetAccountRateLimiter().RecordFailedAttempt(identifier)
}

func (Identifiers) Reset(identifier string) {
	middleware.GetAccountRateLimiter().RecordSuccessfulLogin(identifier)
}

type Limiter struct{}

var _ app.Lockout = Limiter{}

func (Limiter) Locked(actor domain.UserID) (bool, int) {
	return middleware.GetAccountRateLimiter().IsLocked(middleware.SecondFactorKey(uint(actor)))
}

func (Limiter) Fail(actor domain.UserID) (bool, int) {
	return middleware.GetAccountRateLimiter().RecordFailedAttempt(middleware.SecondFactorKey(uint(actor)))
}

func (Limiter) Reset(actor domain.UserID) {
	middleware.GetAccountRateLimiter().RecordSuccessfulLogin(middleware.SecondFactorKey(uint(actor)))
}
