package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAPITokenSecretIsPrefixedAndHashed(t *testing.T) {
	plain, hash, err := NewAPITokenSecret()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(plain, "meerkat_"))
	assert.Equal(t, HashSecret(plain), hash)
	assert.NotContains(t, hash, "meerkat_")
	other, _, _ := NewAPITokenSecret()
	assert.NotEqual(t, plain, other)
}

func TestNewFeedTokenIs43CharactersAndAcceptedByParseFeedFile(t *testing.T) {
	token, hash, err := NewFeedToken()
	require.NoError(t, err)
	assert.Len(t, token, 43)
	assert.Equal(t, HashSecret(token), hash)
	got, ok := ParseFeedFile(token + ".ics")
	assert.True(t, ok)
	assert.Equal(t, token, got)
}

func TestParseFeedFileRejectsAnythingThatIsNotAToken(t *testing.T) {
	for name, file := range map[string]string{
		"too short":      "abc.ics",
		"bad characters": strings.Repeat("!", 43) + ".ics",
		"no suffix":      strings.Repeat("A", 43),
		"wrong suffix":   strings.Repeat("A", 43) + ".txt",
	} {
		_, ok := ParseFeedFile(file)
		assert.False(t, ok, name)
	}
}

func TestAuditDetailKeepsSemicolonsOutOfTheName(t *testing.T) {
	assert.Equal(t, "token:a,b", TokenAuditDetail("a;b"))
}

func TestFeedLanguageFallsBackToEnglish(t *testing.T) {
	assert.Equal(t, "en", FeedLanguage(""))
	assert.Equal(t, "de", FeedLanguage("de"))
}
