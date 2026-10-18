package services

import (
	"fmt"
	"meerkat/clock"
	"meerkat/logger"
	"meerkat/metrics"
	"meerkat/models"
	"os"
	"time"

	"gorm.io/gorm"
)

// Default minimum interval between reminder job runs (prevents duplicates during restarts)
const DefaultReminderMinInterval = 1 * time.Hour

// ReminderMinInterval can be overridden for testing
var ReminderMinInterval = DefaultReminderMinInterval

// getInstanceID returns a unique identifier for this server instance
func getInstanceID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

// acquireJobLock attempts to acquire a lock for the given job.
// Returns true if the lock was acquired, false if the job was run recently
// or is currently locked by another instance.
func acquireJobLock(db *gorm.DB, jobName string, minInterval time.Duration, now time.Time) (bool, error) {
	instanceID := getInstanceID()
	lockTimeout := 5 * time.Minute // Consider locks stale after 5 minutes

	return db.Transaction(func(tx *gorm.DB) error {
		var job models.JobExecution

		// Try to find existing job execution record
		err := tx.Where("job_name = ?", jobName).First(&job).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}

		if err == gorm.ErrRecordNotFound {
			// First time running this job - create the record and acquire lock
			job = models.JobExecution{
				JobName:   jobName,
				LastRunAt: now,
				LockedAt:  &now,
				LockedBy:  instanceID,
			}
			if err := tx.Create(&job).Error; err != nil {
				return err
			}
			logger.Info().Str("job", jobName).Str("instance", instanceID).Msg("Acquired job lock (first run)")
			return nil
		}

		// Job exists - check if we should run
		timeSinceLastRun := now.Sub(job.LastRunAt)
		if timeSinceLastRun < minInterval {
			logger.Info().
				Str("job", jobName).
				Dur("since_last_run", timeSinceLastRun).
				Dur("min_interval", minInterval).
				Msg("Skipping job - ran too recently")
			return fmt.Errorf("job ran too recently")
		}

		// Check if another instance has the lock
		if job.LockedAt != nil {
			lockAge := now.Sub(*job.LockedAt)
			if lockAge < lockTimeout && job.LockedBy != instanceID {
				logger.Info().
					Str("job", jobName).
					Str("locked_by", job.LockedBy).
					Dur("lock_age", lockAge).
					Msg("Skipping job - locked by another instance")
				return fmt.Errorf("job locked by another instance")
			}
			// Lock is stale, we can take over
			if lockAge >= lockTimeout {
				logger.Warn().
					Str("job", jobName).
					Str("previous_instance", job.LockedBy).
					Dur("lock_age", lockAge).
					Msg("Taking over stale lock")
			}
		}

		// Acquire the lock
		job.LockedAt = &now
		job.LockedBy = instanceID
		if err := tx.Save(&job).Error; err != nil {
			return err
		}

		logger.Info().Str("job", jobName).Str("instance", instanceID).Msg("Acquired job lock")
		return nil
	}) == nil, nil
}

// releaseJobLock releases the lock and updates the last run time
func releaseJobLock(db *gorm.DB, jobName string, success bool, now time.Time) error {
	instanceID := getInstanceID()

	return db.Transaction(func(tx *gorm.DB) error {
		var job models.JobExecution
		if err := tx.Where("job_name = ?", jobName).First(&job).Error; err != nil {
			return err
		}

		// Only update if we still hold the lock
		if job.LockedBy != instanceID {
			logger.Warn().
				Str("job", jobName).
				Str("expected", instanceID).
				Str("actual", job.LockedBy).
				Msg("Lock was taken by another instance")
			return nil
		}

		if success {
			job.LastRunAt = now
			job.LastSuccessAt = &now
		}
		job.LockedAt = nil
		job.LockedBy = ""

		return tx.Save(&job).Error
	})
}

// SendRemindersWithRateLimit runs send, the daily reminder e-mail job, behind a distributed
// lock to prevent duplicate sends during rapid restarts.
func SendRemindersWithRateLimit(db *gorm.DB, clk clock.Clock, send func(clock.Clock) error) error {
	acquired, err := acquireJobLock(db, models.JobNameDailyReminders, ReminderMinInterval, clk.Now())
	if err != nil {
		logger.Error().Err(err).Msg("Error checking job lock")
		metrics.JobResult(JobReminders, metrics.ResultFailure)
		return err
	}

	if !acquired {
		logger.Info().Msg("Skipping reminder job - rate limited")
		metrics.JobResult(JobReminders, metrics.ResultSkipped)
		return nil
	}

	err = send(clk)
	metrics.JobDone(JobReminders, err)

	// Release the lock, marking success if no error
	if releaseErr := releaseJobLock(db, models.JobNameDailyReminders, err == nil, clk.Now()); releaseErr != nil {
		logger.Error().Err(releaseErr).Msg("Error releasing job lock")
	}

	return err
}

// addMonths adds the specified number of months to a date, clamping to the last
// valid day of the target month to handle edge cases like Jan 31 + 1 month -> Feb 28/29
func addMonths(t time.Time, months int) time.Time {
	// Get the original day of month
	originalDay := t.Day()

	// Add months using Go's AddDate (which may overflow into next month)
	result := t.AddDate(0, months, 0)

	// If the day changed unexpectedly (overflow occurred), clamp to last day of target month
	// For example: Jan 31 + 1 month = March 3 (in non-leap year), we want Feb 28
	if result.Day() != originalDay {
		// Go back to the last day of the previous month (the intended target month)
		result = result.AddDate(0, 0, -result.Day())
	}

	return result
}

// addYears adds the specified number of years to a date, handling Feb 29 edge case
func addYears(t time.Time, years int) time.Time {
	originalDay := t.Day()
	result := t.AddDate(years, 0, 0)

	// Handle Feb 29 -> Feb 28 transition for leap year edge case
	if result.Day() != originalDay {
		result = result.AddDate(0, 0, -result.Day())
	}

	return result
}
