package domain

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var at = time.Date(2026, 3, 10, 15, 30, 0, 0, time.UTC)

func TestRenderCSVHasEverySectionEvenWhenEmpty(t *testing.T) {
	out, err := RenderCSV(Dataset{})
	require.NoError(t, err)
	for _, s := range []string{"=== CONTACTS ===", "=== RELATIONSHIPS ===", "=== ACTIVITIES ===", "=== NOTES ===", "=== REMINDERS ==="} {
		assert.Contains(t, string(out), s)
	}
}

func TestRenderCSVAddsOneColumnPerCustomFieldAndQuotesSeparators(t *testing.T) {
	out, err := RenderCSV(Dataset{
		CustomFieldNames: []string{"Pet", "Shoe size"},
		Contacts: []Contact{{ID: 1, Firstname: "Ada", Lastname: "L, Jr", Circles: []string{"Friends", "Work"},
			CustomFields: map[string]string{"Pet": "cat"}, CreatedAt: at, UpdatedAt: at}},
	})
	require.NoError(t, err)
	lines := strings.Split(string(out), "\n")
	assert.True(t, strings.HasSuffix(lines[1], ",Pet,Shoe size"))
	assert.Equal(t, `1,Ada,"L, Jr",,,,,,,,,,,Friends; Work,2026-03-10T15:30:00Z,2026-03-10T15:30:00Z,cat,`, lines[2])
}

func TestRenderCSVReminderFlagsDefault(t *testing.T) {
	no, yes := false, true
	out, err := RenderCSV(Dataset{Reminders: []Reminder{
		{ID: 1, Message: "default", RemindAt: at},
		{ID: 2, Message: "set", RemindAt: at, ByMail: &yes, ReoccurFromCompletion: &no, Completed: true},
	}})
	require.NoError(t, err)
	assert.Contains(t, string(out), "default,"+"2026-03-10T15:30:00Z,,false,true,false,")
	assert.Contains(t, string(out), "set,2026-03-10T15:30:00Z,,true,false,true,")
}

func TestFilenameCarriesTheDate(t *testing.T) {
	assert.Equal(t, "meerkat-export-2026-03-10.csv", Filename("meerkat-export", "csv", at))
}

// @016-US1
func TestContactFilenameIsTheNameWithoutControlCharactersAndWithAFallback(t *testing.T) {
	assert.Equal(t, "Alice Johnson.vcf", ContactFilename("Alice", "Johnson"))
	assert.Equal(t, "Zoë Müller.vcf", ContactFilename("Zoë", "Müller"))
	assert.Equal(t, "Bob.vcf", ContactFilename("Bob", ""))
	assert.Equal(t, "contact.vcf", ContactFilename("", ""))
	assert.Equal(t, "A\"b  Set-Cookie: x=1 O'Brien.vcf", ContactFilename("A\"b\r\nSet-Cookie: x=1", "O'Brien"))
}

// @016-US1
func TestContactFilenameIsCutOnACharacterBoundary(t *testing.T) {
	name := ContactFilename(strings.Repeat("é", 150), "")
	assert.LessOrEqual(t, len(name), maxNameBytes+len(".vcf"))
	assert.True(t, utf8.ValidString(name))
	assert.True(t, strings.HasSuffix(name, "é.vcf"))
}
