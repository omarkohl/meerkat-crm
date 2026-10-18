package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sameHost(a, b string) bool { return strings.Split(a, "/")[2] == strings.Split(b, "/")[2] }

func TestCheckURLAcceptsHttpHttpsAndWebcalOnly(t *testing.T) {
	for _, ok := range []string{"https://cal.example.com/a.ics", "http://x.example", "webcal://x.example/a.ics", " https://x.example "} {
		assert.NoError(t, CheckURL(ok), ok)
	}
	for _, bad := range []string{"file:///etc/passwd", "ftp://x.example/a", "gopher://x", "https://", "not a url", ""} {
		assert.ErrorIs(t, CheckURL(bad), ErrInvalidURL, bad)
	}
}

func TestNewSubscriptionDefaults(t *testing.T) {
	s, err := New(Input{Name: "Work", URL: "https://cal.example.com/a.ics", Password: "x"})
	require.NoError(t, err)
	assert.True(t, s.SyncEnabled)
	assert.True(t, s.HasPassword)
	assert.Equal(t, DefaultPastDays, s.PastDays)
	assert.Equal(t, DefaultFutureDays, s.FutureDays)

	off, days := false, 30
	s, err = New(Input{Name: "x", URL: "https://x.example", SyncEnabled: &off, PastDays: &days})
	require.NoError(t, err)
	assert.False(t, s.SyncEnabled)
	assert.Equal(t, 30, s.PastDays)
}

func TestChangeKeepsThePasswordOnTheSameServerOnly(t *testing.T) {
	s := Subscription{URL: "https://cal.example.com/a.ics", HasPassword: true, SyncEnabled: true}

	got, pwd, err := s.Change(Input{Name: "n", URL: "https://cal.example.com/b.ics"}, sameHost)
	require.NoError(t, err)
	assert.Equal(t, KeepPassword, pwd.Mode)
	assert.True(t, got.HasPassword)

	_, _, err = s.Change(Input{Name: "n", URL: "https://evil.example.net/b.ics"}, sameHost)
	assert.ErrorIs(t, err, ErrPasswordRequired)

	got, pwd, err = s.Change(Input{Name: "n", URL: "https://other.example.net/b.ics", Password: "new"}, sameHost)
	require.NoError(t, err)
	assert.Equal(t, PasswordChange{Mode: SetPassword, Plain: "new"}, pwd)
	assert.True(t, got.HasPassword)

	got, pwd, err = s.Change(Input{Name: "n", URL: "https://other.example.net/b.ics", ClearPassword: true}, sameHost)
	require.NoError(t, err)
	assert.Equal(t, ClearPassword, pwd.Mode)
	assert.False(t, got.HasPassword)
}

func TestChangeRefusesABadURLBeforeAnythingElse(t *testing.T) {
	_, _, err := Subscription{HasPassword: true}.Change(Input{URL: "file:///x"}, sameHost)
	assert.ErrorIs(t, err, ErrInvalidURL)
}
