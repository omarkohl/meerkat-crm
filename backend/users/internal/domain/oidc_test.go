package domain_test

import (
	"errors"
	"testing"

	"meerkat/users/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestACallbackNeedsBothCookiesAMatchingStateAndACode(t *testing.T) {
	ok := domain.OIDCCallback{State: "s", Code: "c", StateCookie: "s", NonceCookie: "n"}
	assert.NoError(t, ok.Check())

	for name, c := range map[string]domain.OIDCCallback{
		"no state cookie": {State: "s", Code: "c", NonceCookie: "n"},
		"no nonce cookie": {State: "s", Code: "c", StateCookie: "s"},
		"no state":        {Code: "c", StateCookie: "s", NonceCookie: "n"},
		"other state":     {State: "x", Code: "c", StateCookie: "s", NonceCookie: "n"},
		"no code":         {State: "s", StateCookie: "s", NonceCookie: "n"},
	} {
		var failure *domain.OIDCFailure
		assert.True(t, errors.As(c.Check(), &failure), name)
		assert.True(t, failure.Warn, name)
	}
}
