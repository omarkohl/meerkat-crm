package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEndOfDayIsInTheLocationOfNow(t *testing.T) {
	loc := time.FixedZone("x", 13*3600)
	got := EndOfDay(time.Date(2026, 3, 11, 4, 30, 0, 0, loc))
	assert.Equal(t, time.Date(2026, 3, 11, 23, 59, 59, 0, loc), got)
}

func TestBirthdayIsTodayOnlyAtZeroDays(t *testing.T) {
	assert.True(t, Birthday{DaysUntil: 0}.Today())
	assert.False(t, Birthday{DaysUntil: 1}.Today())
}
