package domain_test

import (
	"errors"
	"testing"

	"meerkat/users/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestOnlySupportedLanguagesAreValid(t *testing.T) {
	supported := []string{"en", "de"}
	assert.NoError(t, domain.ValidateLanguage("de", supported))
	err := domain.ValidateLanguage("xx", supported)
	var unsupported *domain.UnsupportedError
	assert.True(t, errors.As(err, &unsupported))
	assert.Equal(t, supported, unsupported.Supported)
	assert.Error(t, domain.ValidateLanguage("", supported))
}

func TestOnlyKnownDateFormatsAreValid(t *testing.T) {
	for _, f := range []string{"eu", "us", "iso"} {
		assert.NoError(t, domain.ValidateDateFormat(f))
	}
	assert.EqualError(t, domain.ValidateDateFormat("dmy"), "unsupported date format. Supported: eu, us, iso")
}

func TestSessionIDsAre32LowercaseHexCharacters(t *testing.T) {
	assert.True(t, domain.ValidSessionID("0123456789abcdef0123456789abcdef"))
	for _, id := range []string{"", "0123456789ABCDEF0123456789ABCDEF", "0123456789abcdef0123456789abcde", "0123456789abcdef0123456789abcdeg"} {
		assert.False(t, domain.ValidSessionID(id), id)
	}
}

func TestAnAccountWithoutAPasswordHasNoPasswordStep(t *testing.T) {
	assert.False(t, domain.Account{}.HasPassword())
	assert.True(t, domain.Account{PasswordHash: "x"}.HasPassword())
}
