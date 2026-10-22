package domain_test

import (
	"testing"
	"time"

	"meerkat/activities/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestEditReplacesTheDescribingFieldsButNotTheContacts(t *testing.T) {
	a := domain.Activity{Title: "old", Contacts: []domain.ContactRef{{ID: 1}}}
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	end := day.AddDate(0, 0, 2)
	a.Edit("new", "d", "l", day, &end)

	assert.Equal(t, "new", a.Title)
	assert.Equal(t, "d", a.Description)
	assert.Equal(t, "l", a.Location)
	assert.Equal(t, day, a.Date)
	assert.Equal(t, &end, a.EndDate)
	assert.Len(t, a.Contacts, 1)
}

func TestEndsBeforeStartComparesDaysNotTimes(t *testing.T) {
	start := time.Date(2026, 3, 1, 14, 0, 0, 0, time.UTC)
	at := func(d int) *time.Time { v := time.Date(2026, 3, d, 0, 0, 0, 0, time.UTC); return &v }

	assert.False(t, domain.Activity{Date: start}.EndsBeforeStart())
	assert.False(t, domain.Activity{Date: start, EndDate: at(1)}.EndsBeforeStart(), "the start day itself")
	assert.False(t, domain.Activity{Date: start, EndDate: at(3)}.EndsBeforeStart())
	assert.True(t, domain.Activity{Date: start, EndDate: at(0)}.EndsBeforeStart())
}
