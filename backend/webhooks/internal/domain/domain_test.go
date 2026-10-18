package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSecretIsRandomAndUrlSafe(t *testing.T) {
	a, err := NewSecret()
	require.NoError(t, err)
	b, _ := NewSecret()
	assert.NotEqual(t, a, b)
	assert.Len(t, a, 43)
}
