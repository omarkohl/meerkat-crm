// Package lockout counts wrong second-factor attempts per account on the limiter that the
// login uses too.
package lockout

import (
	"meerkat/middleware"
	"meerkat/users/internal/app"
	"meerkat/users/internal/domain"
)

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
