// Package domain is the vocabulary of the users module: what a user can set about how the
// application looks and behaves for them. It imports nothing but the standard library
// (docs/adr/0012-module-layout.md).
package domain

import (
	"errors"
	"slices"
	"strings"
)

type UserID uint

// DateFormats are the supported date formats.
var DateFormats = []string{"eu", "us", "iso"}

var ErrUserNotFound = errors.New("user not found")

// UnsupportedError means a value is not one of the supported ones.
type UnsupportedError struct {
	What      string
	Supported []string
}

func (e *UnsupportedError) Error() string {
	return "unsupported " + e.What + ". Supported: " + strings.Join(e.Supported, ", ")
}

func ValidateLanguage(language string, supported []string) error {
	if !slices.Contains(supported, language) {
		return &UnsupportedError{What: "language", Supported: supported}
	}
	return nil
}

func ValidateDateFormat(format string) error {
	if !slices.Contains(DateFormats, format) {
		return &UnsupportedError{What: "date format", Supported: DateFormats}
	}
	return nil
}
