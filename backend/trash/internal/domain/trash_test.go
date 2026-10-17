package domain_test

import (
	"testing"
	"time"

	"meerkat/trash/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestRetentionFallsBackToThirtyDays(t *testing.T) {
	assert.Equal(t, 30, domain.Retention(0))
	assert.Equal(t, 30, domain.Retention(-5))
	assert.Equal(t, 7, domain.Retention(7))
}

func TestAContactIsPurgedRetentionDaysAfterItsDeletion(t *testing.T) {
	deleted := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC), domain.PurgeAt(deleted, 7))
}
