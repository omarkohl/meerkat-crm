package persistence_test

import (
	"context"
	"testing"
	"time"

	"meerkat/models"
	"meerkat/users/internal/adapters/persistence"
	"meerkat/users/internal/domain"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var now = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func accountsWithToken(t *testing.T, hash string) (*persistence.Accounts, *gorm.DB, models.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Session{}))
	expires := now.Add(time.Hour)
	user := models.User{Username: "a", Email: "a@example.com", Password: "old", PasswordResetTokenHash: &hash, PasswordResetExpiresAt: &expires, PasswordResetRequestedAt: &now}
	require.NoError(t, db.Create(&user).Error)
	return persistence.NewAccounts(db), db, user
}

func TestAResetTokenWorksForExactlyOneConfirmation(t *testing.T) {
	accounts, db, user := accountsWithToken(t, "hash-1")
	ctx := context.Background()

	require.NoError(t, accounts.ResetPassword(ctx, domain.UserID(user.ID), "hash-1", "new-hash", now))
	err := accounts.ResetPassword(ctx, domain.UserID(user.ID), "hash-1", "other-hash", now)

	assert.ErrorIs(t, err, domain.ErrResetTokenInvalid, "the token was used up by the first confirmation")
	var got models.User
	require.NoError(t, db.First(&got, user.ID).Error)
	assert.Equal(t, "new-hash", got.Password)
}

func TestAResetWithAnOutdatedTokenLeavesTheNewerOneAndThePasswordAlone(t *testing.T) {
	accounts, db, user := accountsWithToken(t, "fresh-hash")
	ctx := context.Background()

	err := accounts.ResetPassword(ctx, domain.UserID(user.ID), "stale-hash", "new-hash", now)
	assert.ErrorIs(t, err, domain.ErrResetTokenInvalid)
	require.NoError(t, accounts.ClearResetToken(ctx, domain.UserID(user.ID), "stale-hash"))

	var got models.User
	require.NoError(t, db.First(&got, user.ID).Error)
	assert.Equal(t, "old", got.Password)
	require.NotNil(t, got.PasswordResetTokenHash)
	assert.Equal(t, "fresh-hash", *got.PasswordResetTokenHash, "clearing an old token does not delete a newer one")

	require.NoError(t, accounts.ClearResetToken(ctx, domain.UserID(user.ID), "fresh-hash"))
	require.NoError(t, db.First(&got, user.ID).Error)
	assert.Nil(t, got.PasswordResetTokenHash)
}
