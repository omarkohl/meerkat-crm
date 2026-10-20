package services

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"meerkat/config"
	"meerkat/models"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func totpTestDB(t *testing.T) (*gorm.DB, *config.Config, models.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.RecoveryCode{}))
	cfg := &config.Config{JWTSecretKey: testJWTSecret, EncryptionKey: base64.StdEncoding.EncodeToString(testEncryptionKey(1))}
	u := models.User{Username: "u", Email: "u@example.com", Password: "x"}
	require.NoError(t, db.Create(&u).Error)
	return db, cfg, u
}

func TestTOTPCodeMatchesRFC6238Vectors(t *testing.T) {
	// RFC 6238 appendix B, SHA1, secret "12345678901234567890"; the RFC lists 8 digits,
	// the last 6 of those are the 6-digit code.
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	cases := map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"}
	for unix, want := range cases {
		assert.Equal(t, want, TOTPCode(secret, time.Unix(unix, 0)), "t=%d", unix)
	}
}

func TestVerifyTOTPWindowAndReplay(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	require.NoError(t, err)
	now := time.Unix(1_700_000_000, 0)
	step := now.Unix() / 30

	got, ok := VerifyTOTP(secret, TOTPCode(secret, now), now, 0)
	require.True(t, ok)
	assert.Equal(t, step, got)

	// one step of clock drift either way is fine
	_, ok = VerifyTOTP(secret, TOTPCode(secret, now.Add(-30*time.Second)), now, 0)
	assert.True(t, ok)
	_, ok = VerifyTOTP(secret, TOTPCode(secret, now.Add(30*time.Second)), now, 0)
	assert.True(t, ok)
	// two steps is not
	_, ok = VerifyTOTP(secret, TOTPCode(secret, now.Add(-90*time.Second)), now, 0)
	assert.False(t, ok)
	// a step at or before the last accepted one is a replay
	_, ok = VerifyTOTP(secret, TOTPCode(secret, now), now, step)
	assert.False(t, ok)
	// garbage
	for _, bad := range []string{"", "12345", "1234567", "abcdef", "      "} {
		_, ok = VerifyTOTP(secret, bad, now, 0)
		assert.False(t, ok, bad)
	}
	// spaces are tolerated (authenticator apps show "123 456")
	code := TOTPCode(secret, now)
	_, ok = VerifyTOTP(secret, code[:3]+" "+code[3:], now, 0)
	assert.True(t, ok)
}

func TestTOTPURI(t *testing.T) {
	uri := TOTPURI("Meerkat CRM", "al ice@example.com", "ABC")
	assert.True(t, strings.HasPrefix(uri, "otpauth://totp/Meerkat%20CRM:al%20ice@example.com?"), uri)
	assert.Contains(t, uri, "secret=ABC")
	assert.Contains(t, uri, "issuer=Meerkat%20CRM")
	assert.Contains(t, uri, "digits=6")
	assert.Contains(t, uri, "period=30")
}

func TestSetupStoresAnEncryptedSecretAndEnableNeedsAValidCode(t *testing.T) {
	db, cfg, u := totpTestDB(t)
	secret, err := BeginTOTPSetup(db, cfg, u.ID)
	require.NoError(t, err)

	var stored models.User
	require.NoError(t, db.First(&stored, u.ID).Error)
	assert.NotEmpty(t, stored.TOTPSecret)
	assert.NotContains(t, stored.TOTPSecret, secret, "secret must be encrypted at rest")
	assert.False(t, stored.TOTPEnabled)

	_, err = EnableTOTP(db, cfg, u.ID, "000000", time.Now())
	assert.ErrorIs(t, err, ErrInvalidSecondFactor)
	require.NoError(t, db.First(&stored, u.ID).Error)
	assert.False(t, stored.TOTPEnabled)

	now := time.Now()
	codes, err := EnableTOTP(db, cfg, u.ID, TOTPCode(secret, now), now)
	require.NoError(t, err)
	assert.Len(t, codes, RecoveryCodeCount)
	require.NoError(t, db.First(&stored, u.ID).Error)
	assert.True(t, stored.TOTPEnabled)
	assert.Equal(t, RecoveryCodeCount, RecoveryCodesRemaining(db, u.ID))

	// the same code cannot be used again at login
	ok, _ := CheckSecondFactor(db, cfg, stored, TOTPCode(secret, now), "", now)
	assert.False(t, ok, "enabling consumed that step")
	ok, usedRecovery := CheckSecondFactor(db, cfg, stored, TOTPCode(secret, now.Add(30*time.Second)), "", now)
	assert.True(t, ok)
	assert.False(t, usedRecovery)
}

func TestSetupRefusedWhenAlreadyEnabled(t *testing.T) {
	db, cfg, u := totpTestDB(t)
	secret, _ := BeginTOTPSetup(db, cfg, u.ID)
	_, err := EnableTOTP(db, cfg, u.ID, TOTPCode(secret, time.Now()), time.Now())
	require.NoError(t, err)
	_, err = BeginTOTPSetup(db, cfg, u.ID)
	assert.ErrorIs(t, err, ErrTOTPAlreadyEnabled)
}

func TestRecoveryCodesAreSingleUseAndStoredHashed(t *testing.T) {
	db, cfg, u := totpTestDB(t)
	secret, _ := BeginTOTPSetup(db, cfg, u.ID)
	now := time.Now()
	codes, err := EnableTOTP(db, cfg, u.ID, TOTPCode(secret, now), now)
	require.NoError(t, err)

	var rows []models.RecoveryCode
	require.NoError(t, db.Find(&rows).Error)
	for _, r := range rows {
		for _, c := range codes {
			assert.NotContains(t, r.CodeHash, strings.ReplaceAll(c, "-", ""))
		}
	}

	var user models.User
	require.NoError(t, db.First(&user, u.ID).Error)
	// typed in lower case and without the dash
	typed := strings.ToLower(strings.ReplaceAll(codes[0], "-", ""))
	ok, usedRecovery := CheckSecondFactor(db, cfg, user, "", typed, now)
	assert.True(t, ok)
	assert.True(t, usedRecovery)
	ok, _ = CheckSecondFactor(db, cfg, user, "", codes[0], now)
	assert.False(t, ok, "single use")
	assert.Equal(t, RecoveryCodeCount-1, RecoveryCodesRemaining(db, u.ID))
	ok, _ = CheckSecondFactor(db, cfg, user, "", "AAAAA-AAAAA", now)
	assert.False(t, ok)
}

func TestRegenerateReplacesTheOldCodes(t *testing.T) {
	db, cfg, u := totpTestDB(t)
	secret, _ := BeginTOTPSetup(db, cfg, u.ID)
	now := time.Now()
	old, _ := EnableTOTP(db, cfg, u.ID, TOTPCode(secret, now), now)
	fresh, err := RegenerateRecoveryCodes(db, cfg, u.ID)
	require.NoError(t, err)
	assert.Len(t, fresh, RecoveryCodeCount)
	var user models.User
	require.NoError(t, db.First(&user, u.ID).Error)
	ok, _ := CheckSecondFactor(db, cfg, user, "", old[0], now)
	assert.False(t, ok)
	ok, _ = CheckSecondFactor(db, cfg, user, "", fresh[0], now)
	assert.True(t, ok)
}

func TestDisableClearsEverything(t *testing.T) {
	db, cfg, u := totpTestDB(t)
	secret, _ := BeginTOTPSetup(db, cfg, u.ID)
	now := time.Now()
	_, err := EnableTOTP(db, cfg, u.ID, TOTPCode(secret, now), now)
	require.NoError(t, err)
	require.NoError(t, DisableTOTP(db, u.ID))
	var user models.User
	require.NoError(t, db.First(&user, u.ID).Error)
	assert.False(t, user.TOTPEnabled)
	assert.Empty(t, user.TOTPSecret)
	assert.Zero(t, RecoveryCodesRemaining(db, u.ID))
}

func TestCheckSecondFactorFailsWithoutEnabledTOTP(t *testing.T) {
	db, cfg, u := totpTestDB(t)
	ok, _ := CheckSecondFactor(db, cfg, u, "123456", "", time.Now())
	assert.False(t, ok)
}

func TestTwoFactorNeedsTheEncryptionKey(t *testing.T) {
	db, cfg, u := totpTestDB(t)
	cfg.EncryptionKey = ""
	_, err := BeginTOTPSetup(db, cfg, u.ID)
	assert.ErrorIs(t, err, ErrNoEncryptionKey)
	var stored models.User
	require.NoError(t, db.First(&stored, u.ID).Error)
	assert.Empty(t, stored.TOTPSecret)
}

func TestRecoveryHashDependsOnTheEncryptionKeyOnly(t *testing.T) {
	keys := testKeys()
	a := hashRecoveryCode(keys, "AAAAA-BBBBB")
	assert.Equal(t, a, hashRecoveryCode(keys, "aaaaa bbbbb"), "normalised")
	assert.NotEqual(t, a, hashRecoveryCode(NewCredentialKeys(testEncryptionKey(2), testJWTSecret), "AAAAA-BBBBB"))
	assert.Equal(t, a, hashRecoveryCode(NewCredentialKeys(testEncryptionKey(1), "a-new-jwt-secret-after-rotation!!!"), "AAAAA-BBBBB"))
	// the plain SHA-256 of the code (what an attacker with the table could test) does not match
	plain := sha256.Sum256([]byte("AAAAABBBBB"))
	assert.NotEqual(t, hex.EncodeToString(plain[:]), a)
}

func TestRevokeOtherSessionsKeepsTheCurrentOne(t *testing.T) {
	db, _, u := totpTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Session{}))
	for _, id := range []string{"keep", "other1", "other2"} {
		require.NoError(t, db.Create(&models.Session{UserID: u.ID, PublicID: id, ExpiresAt: time.Now().Add(time.Hour)}).Error)
	}
	require.NoError(t, RevokeOtherSessions(db, u.ID, "keep", time.Now()))
	live, err := ListSessions(db, u.ID, time.Now())
	require.NoError(t, err)
	require.Len(t, live, 1)
	assert.Equal(t, "keep", live[0].PublicID)
}
