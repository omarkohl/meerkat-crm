package domain

import (
	"strings"
	"testing"
	"time"

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
