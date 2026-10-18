package services

import (
	"errors"
	"meerkat/clock"
	"meerkat/models"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func setupTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	db.AutoMigrate(&models.Contact{}, &models.Activity{}, &models.Note{}, models.Relationship{}, models.Reminder{}, models.User{}, models.JobExecution{}, models.Webhook{}, models.WebhookDelivery{})

	return db
}

// TestAddMonths tests the month addition helper for edge cases
func TestAddMonths(t *testing.T) {
	tests := []struct {
		name     string
		start    time.Time
		months   int
		expected time.Time
	}{
		{
			name:     "Jan 31 + 1 month = Feb 28 (non-leap year)",
			start:    time.Date(2023, 1, 31, 12, 0, 0, 0, time.UTC),
			months:   1,
			expected: time.Date(2023, 2, 28, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "Jan 31 + 1 month = Feb 29 (leap year)",
			start:    time.Date(2024, 1, 31, 12, 0, 0, 0, time.UTC),
			months:   1,
			expected: time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "Jan 30 + 1 month = Feb 28 (non-leap year)",
			start:    time.Date(2023, 1, 30, 12, 0, 0, 0, time.UTC),
			months:   1,
			expected: time.Date(2023, 2, 28, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "Jan 29 + 1 month = Feb 28 (non-leap year)",
			start:    time.Date(2023, 1, 29, 12, 0, 0, 0, time.UTC),
			months:   1,
			expected: time.Date(2023, 2, 28, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "Jan 28 + 1 month = Feb 28 (no clamping needed)",
			start:    time.Date(2023, 1, 28, 12, 0, 0, 0, time.UTC),
			months:   1,
			expected: time.Date(2023, 2, 28, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "Jan 15 + 1 month = Feb 15 (normal case)",
			start:    time.Date(2023, 1, 15, 12, 0, 0, 0, time.UTC),
			months:   1,
			expected: time.Date(2023, 2, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "Mar 31 + 1 month = Apr 30",
			start:    time.Date(2023, 3, 31, 12, 0, 0, 0, time.UTC),
			months:   1,
			expected: time.Date(2023, 4, 30, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "Aug 31 + 3 months (quarterly) = Nov 30",
			start:    time.Date(2023, 8, 31, 12, 0, 0, 0, time.UTC),
			months:   3,
			expected: time.Date(2023, 11, 30, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "Aug 31 + 6 months = Feb 28",
			start:    time.Date(2023, 8, 31, 12, 0, 0, 0, time.UTC),
			months:   6,
			expected: time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC), // 2024 is a leap year
		},
		{
			name:     "Dec 31 + 1 month = Jan 31 (year rollover)",
			start:    time.Date(2023, 12, 31, 12, 0, 0, 0, time.UTC),
			months:   1,
			expected: time.Date(2024, 1, 31, 12, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addMonths(tt.start, tt.months)
			assert.Equal(t, tt.expected, result, "Expected %v but got %v", tt.expected, result)
		})
	}
}

// TestAddYears tests the year addition helper for edge cases
func TestAddYears(t *testing.T) {
	tests := []struct {
		name     string
		start    time.Time
		years    int
		expected time.Time
	}{
		{
			name:     "Feb 29 2024 + 1 year = Feb 28 2025 (leap to non-leap)",
			start:    time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC),
			years:    1,
			expected: time.Date(2025, 2, 28, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "Feb 28 2023 + 1 year = Feb 28 2024 (normal case)",
			start:    time.Date(2023, 2, 28, 12, 0, 0, 0, time.UTC),
			years:    1,
			expected: time.Date(2024, 2, 28, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "Feb 29 2024 + 4 years = Feb 29 2028 (leap to leap)",
			start:    time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC),
			years:    4,
			expected: time.Date(2028, 2, 29, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "Jan 15 2023 + 1 year = Jan 15 2024 (normal case)",
			start:    time.Date(2023, 1, 15, 12, 0, 0, 0, time.UTC),
			years:    1,
			expected: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addYears(tt.start, tt.years)
			assert.Equal(t, tt.expected, result, "Expected %v but got %v", tt.expected, result)
		})
	}
}

func countingJob(calls *int) func(clock.Clock) error {
	return func(clock.Clock) error { *calls++; return nil }
}

func TestSendRemindersWithRateLimit_FirstRun(t *testing.T) {
	db := setupTestDB()
	originalInterval := ReminderMinInterval
	ReminderMinInterval = 100 * time.Millisecond
	defer func() { ReminderMinInterval = originalInterval }()

	calls := 0
	assert.NoError(t, SendRemindersWithRateLimit(db, clock.Real(), countingJob(&calls)))
	assert.Equal(t, 1, calls, "First run should send reminders")

	var job models.JobExecution
	assert.NoError(t, db.Where("job_name = ?", models.JobNameDailyReminders).First(&job).Error)
	assert.NotZero(t, job.LastRunAt)
}

func TestSendRemindersWithRateLimit_RateLimited(t *testing.T) {
	db := setupTestDB()
	originalInterval := ReminderMinInterval
	ReminderMinInterval = 1 * time.Hour
	defer func() { ReminderMinInterval = originalInterval }()

	calls := 0
	assert.NoError(t, SendRemindersWithRateLimit(db, clock.Real(), countingJob(&calls)))
	assert.NoError(t, SendRemindersWithRateLimit(db, clock.Real(), countingJob(&calls)))
	assert.Equal(t, 1, calls, "Second run should be rate limited")
}

func TestSendRemindersWithRateLimit_AllowsAfterInterval(t *testing.T) {
	db := setupTestDB()
	originalInterval := ReminderMinInterval
	ReminderMinInterval = 50 * time.Millisecond
	defer func() { ReminderMinInterval = originalInterval }()

	calls := 0
	assert.NoError(t, SendRemindersWithRateLimit(db, clock.Real(), countingJob(&calls)))
	var job models.JobExecution
	db.Where("job_name = ?", models.JobNameDailyReminders).First(&job)
	firstRunTime := job.LastRunAt

	time.Sleep(100 * time.Millisecond)
	assert.NoError(t, SendRemindersWithRateLimit(db, clock.Real(), countingJob(&calls)))
	assert.Equal(t, 2, calls, "Should allow run after interval passes")
	db.Where("job_name = ?", models.JobNameDailyReminders).First(&job)
	assert.True(t, job.LastRunAt.After(firstRunTime), "LastRunAt should be updated after second run")
}

func TestSendRemindersWithRateLimit_FailureIsReturnedAndDoesNotCountAsARun(t *testing.T) {
	db := setupTestDB()
	boom := errors.New("boom")
	err := SendRemindersWithRateLimit(db, clock.Real(), func(clock.Clock) error { return boom })
	assert.ErrorIs(t, err, boom)

	var job models.JobExecution
	assert.NoError(t, db.Where("job_name = ?", models.JobNameDailyReminders).First(&job).Error)
	assert.Nil(t, job.LastSuccessAt)
}
