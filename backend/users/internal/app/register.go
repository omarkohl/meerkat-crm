package app

import (
	"context"
	"errors"

	"meerkat/users/internal/domain"
)

// Register creates an account. The password is hashed before anything is written.
func (u *UseCases) Register(ctx context.Context, user domain.NewUser, password string) error {
	if u.registrationDisabled {
		return domain.ErrRegistrationDisabled
	}
	hash, err := u.passwords.Hash(password)
	if errors.Is(err, domain.ErrPasswordTooLong) {
		return err
	}
	if err != nil {
		return &Failure{Stage: StageHash, Err: err}
	}
	user.PasswordHash = hash
	user.Normalise()
	return u.accounts.Register(ctx, user)
}

func (u *UseCases) Profile(ctx context.Context, actor domain.UserID) (domain.Profile, error) {
	p, err := u.accounts.Profile(ctx, actor)
	if err != nil {
		return domain.Profile{}, stage(StageQuery, err)
	}
	return p, nil
}
