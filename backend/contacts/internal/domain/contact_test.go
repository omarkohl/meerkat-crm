package domain_test

import (
	"testing"
	"time"

	"meerkat/contacts/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestArchiveAndUnarchive(t *testing.T) {
	c := domain.Contact{Firstname: "Alice"}
	assert.False(t, c.Archived)

	c.Archive()
	assert.True(t, c.Archived)

	c.Unarchive()
	assert.False(t, c.Archived)
}

func TestPrimaryScalarsFollowTheFirstEntryOfTheirLists(t *testing.T) {
	c := domain.Contact{
		Email: "old@example.com", Phone: "1", Address: "old",
		Emails:    []domain.Value{{Value: "a@example.com"}, {Value: "b@example.com"}},
		Phones:    []domain.Value{{Value: "+49 30 1234"}},
		Addresses: []domain.Address{{Street: "Main St 1", City: "Berlin", Country: "DE"}},
	}
	c.SyncPrimary()
	assert.Equal(t, "a@example.com", c.Email)
	assert.Equal(t, "+49 30 1234", c.Phone)
	assert.Equal(t, "Main St 1, Berlin, DE", c.Address)
}

func TestPrimaryScalarsStayWhenTheListsAreEmpty(t *testing.T) {
	c := domain.Contact{Email: "a@example.com", Phone: "1", Address: "x"}
	c.SyncPrimary()
	assert.Equal(t, "a@example.com", c.Email)
	assert.Equal(t, "1", c.Phone)
	assert.Equal(t, "x", c.Address)
}

func TestReplaceKeepsIdentityPhotoAndArchiveState(t *testing.T) {
	c := domain.Contact{ID: 7, UserID: 3, Firstname: "Alice", Photo: "p.jpg", Archived: true}
	c.Replace(domain.Contact{ID: 99, UserID: 4, Firstname: "Alicia", Lastname: "Smith", Photo: "x", Emails: []domain.Value{{Value: "a@example.com"}}})
	assert.Equal(t, domain.ContactID(7), c.ID)
	assert.Equal(t, domain.UserID(3), c.UserID)
	assert.Equal(t, "p.jpg", c.Photo)
	assert.True(t, c.Archived)
	assert.Equal(t, "Alicia", c.Firstname)
	assert.Equal(t, "a@example.com", c.Email)
}

func TestDeceasedByAcceptsAnyDateThatHasBegunSomewhereOnEarth(t *testing.T) {
	now := time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC)
	assert.True(t, domain.Contact{}.DeceasedBy(now))
	assert.True(t, domain.Contact{DeceasedDate: "2020-01-15"}.DeceasedBy(now))
	assert.True(t, domain.Contact{DeceasedDate: "2026-01-01"}.DeceasedBy(now))
	assert.True(t, domain.Contact{DeceasedDate: "2026-01-02"}.DeceasedBy(now))
	assert.False(t, domain.Contact{DeceasedDate: "2026-01-03"}.DeceasedBy(now))
}
