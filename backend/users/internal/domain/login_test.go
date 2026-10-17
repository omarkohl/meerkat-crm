package domain_test

import (
	"errors"
	"testing"

	"meerkat/users/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestLoginCredentialsNameTheMissingFieldAndTheKeyIsLowerCase(t *testing.T) {
	var missing *domain.MissingFieldError
	assert.True(t, errors.As(domain.Credentials{Password: "x"}.Validate(), &missing))
	assert.Equal(t, "identifier", missing.Field)
	assert.True(t, errors.As(domain.Credentials{Identifier: "a"}.Validate(), &missing))
	assert.Equal(t, "password", missing.Field)
	assert.NoError(t, domain.Credentials{Identifier: "a", Password: "x"}.Validate())

	c := domain.Credentials{Identifier: "Alice@Example.com"}
	assert.Equal(t, "alice@example.com", c.Key())
	assert.True(t, c.NamesEmail())
	assert.False(t, domain.Credentials{Identifier: "alice"}.NamesEmail())
	assert.False(t, domain.Credentials{}.HasSecondFactor())
	assert.True(t, domain.Credentials{RecoveryCode: "x"}.HasSecondFactor())
}
