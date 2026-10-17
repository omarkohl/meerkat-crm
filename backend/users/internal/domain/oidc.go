package domain

import (
	"crypto/subtle"
	"errors"
)

var (
	ErrOIDCDenied    = errors.New("single sign-on was denied")
	ErrOIDCNoAccount = errors.New("no account for this single sign-on identity")
)

type OIDCFailure struct {
	Reason string
	Err    error
	// Warn is true for a callback that was merely wrong (a missing cookie, a mismatch) and false
	// for one that failed on the way (the provider or the database).
	Warn bool
}

func (f *OIDCFailure) Error() string { return "single sign-on failed: " + f.Reason }
func (f *OIDCFailure) Unwrap() error { return f.Err }

type OIDCCallback struct {
	ProviderError string
	State, Code   string
	StateCookie   string
	NonceCookie   string
}

func (c OIDCCallback) Check() error {
	switch {
	case c.StateCookie == "":
		return &OIDCFailure{Reason: "missing state cookie", Warn: true}
	case c.NonceCookie == "":
		return &OIDCFailure{Reason: "missing nonce cookie", Warn: true}
	case !SameSecret(c.StateCookie, c.State):
		return &OIDCFailure{Reason: "state mismatch", Warn: true}
	case c.Code == "":
		return &OIDCFailure{Reason: "missing code", Warn: true}
	}
	return nil
}

func SameSecret(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

type OIDCClaims struct {
	Subject           string
	Email             string
	EmailVerified     bool
	Name              string
	PreferredUsername string
	Provider          string
}
