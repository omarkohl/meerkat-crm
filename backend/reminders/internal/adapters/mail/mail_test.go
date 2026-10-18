package mail

import (
	"testing"
	"time"

	"meerkat/i18n"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatDateFollowsTheUsersFormat(t *testing.T) {
	d := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "09.03.2026", formatDate(d, "eu"))
	assert.Equal(t, "03/09/2026", formatDate(d, "us"))
	assert.Equal(t, "2026-03-09", formatDate(d, "iso"))
}

func TestFormatBirthdayHandlesFullAndYearlessDates(t *testing.T) {
	for _, c := range []struct{ in, format, want string }{
		{"1815-12-10", "eu", "10.12.1815"},
		{"1815-12-10", "us", "12/10/1815"},
		{"1815-12-10", "iso", "1815-12-10"},
		{"--12-10", "eu", "10.12."},
		{"--12-10", "us", "12/10"},
		{"--12-10", "iso", "12-10"},
		{"--12", "eu", "--12"},
		{"", "eu", ""},
	} {
		assert.Equal(t, c.want, formatBirthday(c.in, c.format), c.in+" "+c.format)
	}
}

func TestBadgeCountsDays(t *testing.T) {
	require.NoError(t, i18n.Init())
	_, today := badgeFor("en", 0)
	_, tomorrow := badgeFor("en", 1)
	text, later := badgeFor("en", 5)
	assert.Equal(t, []string{"today", "tomorrow", "future"}, []string{today, tomorrow, later})
	assert.Contains(t, text, "5")
}
