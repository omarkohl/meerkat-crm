package domain

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sameHost(a, b string) bool {
	ua, _ := url.Parse(a)
	ub, _ := url.Parse(b)
	return ua.Host == ub.Host
}

func TestNormalizeBaseURLDropsCredentialsAndFragments(t *testing.T) {
	got, err := NormalizeBaseURL(" https://user:inline@dav.example.com/dav#frag ")
	require.NoError(t, err)
	assert.Equal(t, "https://dav.example.com/dav", got)
	for _, bad := range []string{"file:///etc", "ftp://x.example", "webcal://x.example", "https://", "", "not a url"} {
		_, err := NormalizeBaseURL(bad)
		assert.ErrorIs(t, err, ErrInvalidBaseURL, bad)
	}
}

func TestSaveANewConnectionDefaults(t *testing.T) {
	s, err := Save(nil, Input{BaseURL: "https://dav.example.com", AddressBookPath: "/a/", Password: "p"}, sameHost)
	require.NoError(t, err)
	assert.True(t, s.IsNew)
	assert.True(t, s.Connection.SyncEnabled)
	assert.True(t, s.Connection.HasPassword)
	assert.Equal(t, TwoWay, s.Connection.Direction)
	assert.False(t, s.ResetLinks)
}

func TestSaveRefusesARelativePathBeforeLookingAtTheStoredConnection(t *testing.T) {
	_, err := Save(&Connection{HasPassword: true, BaseURL: "https://a.example/x"}, Input{BaseURL: "https://b.example", AddressBookPath: "books"}, sameHost)
	assert.ErrorIs(t, err, ErrRelativePath)
}

func TestSwitchingServerOrBookResetsLinksAndKeepsTheRest(t *testing.T) {
	existing := Connection{ID: 1, BaseURL: "https://dav.example.com", AddressBookPath: "/a/", Direction: Pull, SyncEnabled: false, HasPassword: true}

	s, err := Save(&existing, Input{BaseURL: "https://dav.example.com", AddressBookPath: "/b/"}, sameHost)
	require.NoError(t, err)
	assert.True(t, s.ResetLinks)
	assert.Equal(t, Pull, s.Connection.Direction, "an unset direction stays")
	assert.False(t, s.Connection.SyncEnabled, "an unset flag stays")
	assert.Equal(t, KeepPassword, s.Password.Mode)

	s, err = Save(&existing, Input{BaseURL: "https://dav.example.com", AddressBookPath: "/a/"}, sameHost)
	require.NoError(t, err)
	assert.False(t, s.ResetLinks)
}

func TestAStoredPasswordDoesNotFollowAnotherServer(t *testing.T) {
	existing := Connection{BaseURL: "https://dav.example.com", AddressBookPath: "/a/", HasPassword: true}
	_, err := Save(&existing, Input{BaseURL: "https://evil.example.net", AddressBookPath: "/a/"}, sameHost)
	assert.ErrorIs(t, err, ErrPasswordRequired)

	s, err := Save(&existing, Input{BaseURL: "https://evil.example.net", AddressBookPath: "/a/", Password: "new"}, sameHost)
	require.NoError(t, err)
	assert.Equal(t, PasswordChange{Mode: SetPassword, Plain: "new"}, s.Password)

	s, err = Save(&existing, Input{BaseURL: "https://evil.example.net", AddressBookPath: "/a/", ClearPassword: true}, sameHost)
	require.NoError(t, err)
	assert.False(t, s.Connection.HasPassword)
	assert.Equal(t, ClearPassword, s.Password.Mode)
}
