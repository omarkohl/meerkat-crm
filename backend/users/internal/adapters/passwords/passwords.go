// Package passwords checks and makes password hashes with bcrypt.
package passwords

import (
	"errors"

	"meerkat/services"
	"meerkat/users/internal/app"
	"meerkat/users/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

type Bcrypt struct{}

var _ app.Passwords = Bcrypt{}

func (Bcrypt) Matches(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (Bcrypt) Hash(password string) (string, error) {
	hash, err := services.HashPassword(password)
	if errors.Is(err, services.ErrPasswordTooLong) {
		return "", domain.ErrPasswordTooLong
	}
	return hash, err
}
