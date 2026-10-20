package services

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 default; the authenticator apps all support SHA-1
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"meerkat/config"
	"meerkat/models"

	"gorm.io/gorm"
)

const (
	totpPeriod = 30
	totpDigits = 6
	// totpSkew is how many steps either side of now are accepted (clock drift).
	totpSkew = 1
	// RecoveryCodeCount is how many one-time codes enrolment hands out.
	RecoveryCodeCount = 10
	// recoveryAlphabet has 32 symbols and no look-alikes (no 0/O, 1/I).
	recoveryAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

var (
	// ErrInvalidSecondFactor means the code (or recovery code) was wrong.
	ErrInvalidSecondFactor = errors.New("invalid verification code")
	// ErrTOTPAlreadyEnabled is returned by setup when two-factor is already on.
	ErrTOTPAlreadyEnabled = errors.New("two-factor authentication is already enabled")

	totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)
)

// GenerateTOTPSecret returns a new 160-bit secret, base32 encoded.
func GenerateTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return totpEncoding.EncodeToString(raw), nil
}

func totpCodeForStep(secret string, step int64) (string, bool) {
	key, err := totpEncoding.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || step < 0 {
		return "", false
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, value%1_000_000), true
}

// TOTPCode returns the code an authenticator shows at the given time.
func TOTPCode(secret string, at time.Time) string {
	code, _ := totpCodeForStep(secret, at.Unix()/totpPeriod)
	return code
}

// VerifyTOTP checks a code against the steps around now. A step at or before lastStep is
// refused (a code works once). It returns the step that matched.
func VerifyTOTP(secret, code string, now time.Time, lastStep int64) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	current := now.Unix() / totpPeriod
	for step := current + totpSkew; step >= current-totpSkew; step-- {
		if step <= lastStep {
			continue
		}
		want, ok := totpCodeForStep(secret, step)
		if ok && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// TOTPURI is the otpauth:// URI authenticator apps read from the QR code.
func TOTPURI(issuer, account, secret string) string {
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(totpPeriod))
	// Some apps read "+" literally.
	query := strings.ReplaceAll(q.Encode(), "+", "%20")
	return "otpauth://totp/" + url.PathEscape(issuer) + ":" + url.PathEscape(account) + "?" + query
}

func normalizeRecoveryCode(code string) string {
	code = strings.ToUpper(code)
	code = strings.NewReplacer("-", "", " ", "").Replace(code)
	return code
}

func hashRecoveryCode(k CredentialKeys, code string) string {
	mac := hmac.New(sha256.New, k.recovery)
	mac.Write([]byte(normalizeRecoveryCode(code)))
	return hex.EncodeToString(mac.Sum(nil))
}

func newRecoveryCode() (string, error) {
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	out := make([]byte, 10)
	for i, b := range raw {
		out[i] = recoveryAlphabet[int(b)&31]
	}
	return string(out[:5]) + "-" + string(out[5:]), nil
}

// replaceRecoveryCodes deletes the user's codes and stores a fresh set inside tx. It
// returns the plain codes, which are never stored.
func replaceRecoveryCodes(tx *gorm.DB, k CredentialKeys, userID uint) ([]string, error) {
	if !k.Ready() {
		return nil, ErrNoEncryptionKey
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.RecoveryCode{}).Error; err != nil {
		return nil, err
	}
	codes := make([]string, 0, RecoveryCodeCount)
	for i := 0; i < RecoveryCodeCount; i++ {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)
		if err := tx.Create(&models.RecoveryCode{UserID: userID, CodeHash: hashRecoveryCode(k, code)}).Error; err != nil {
			return nil, err
		}
	}
	return codes, nil
}

// BeginTOTPSetup stores a new, not yet active secret (encrypted) and returns it. Calling it
// again before enabling replaces the secret. It needs ENCRYPTION_KEY (ErrNoEncryptionKey).
func BeginTOTPSetup(db *gorm.DB, cfg *config.Config, userID uint) (string, error) {
	keys := CredentialKeysOf(*cfg)
	if !keys.Ready() {
		return "", ErrNoEncryptionKey
	}
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		return "", err
	}
	if user.TOTPEnabled {
		return "", ErrTOTPAlreadyEnabled
	}
	secret, err := GenerateTOTPSecret()
	if err != nil {
		return "", err
	}
	enc, err := EncryptCredential(keys, secret)
	if err != nil {
		return "", err
	}
	err = db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]any{"totp_secret": enc, "totp_enabled": false, "totp_last_step": 0}).Error
	return secret, err
}

// EnableTOTP turns two-factor on when code is valid for the pending secret, and returns
// the recovery codes (shown once).
func EnableTOTP(db *gorm.DB, cfg *config.Config, userID uint, code string, now time.Time) ([]string, error) {
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	if user.TOTPEnabled {
		return nil, ErrTOTPAlreadyEnabled
	}
	if user.TOTPSecret == "" {
		return nil, ErrInvalidSecondFactor
	}
	keys := CredentialKeysOf(*cfg)
	secret, err := DecryptCredential(keys, user.TOTPSecret)
	if err != nil {
		return nil, err
	}
	step, ok := VerifyTOTP(secret, code, now, user.TOTPLastStep)
	if !ok {
		return nil, ErrInvalidSecondFactor
	}
	var codes []string
	err = db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.User{}).Where("id = ? AND totp_enabled = ? AND totp_secret = ?", userID, false, user.TOTPSecret).
			Updates(map[string]any{"totp_enabled": true, "totp_last_step": step})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrTOTPAlreadyEnabled
		}
		var err error
		codes, err = replaceRecoveryCodes(tx, keys, userID)
		return err
	})
	return codes, err
}

// CheckSecondFactor verifies the second login step: an authenticator code or, if none is
// given, a recovery code. Each works once. usedRecovery tells which one matched.
func CheckSecondFactor(db *gorm.DB, cfg *config.Config, user models.User, code, recovery string, now time.Time) (ok bool, usedRecovery bool) {
	keys := CredentialKeysOf(*cfg)
	if !user.TOTPEnabled || user.TOTPSecret == "" || !keys.Ready() {
		return false, false
	}
	if strings.TrimSpace(code) != "" {
		secret, err := DecryptCredential(keys, user.TOTPSecret)
		if err != nil {
			return false, false
		}
		step, valid := VerifyTOTP(secret, code, now, user.TOTPLastStep)
		if !valid {
			return false, false
		}
		// Conditional update: of two concurrent requests with the same code only one wins.
		res := db.Model(&models.User{}).Where("id = ? AND totp_last_step < ?", user.ID, step).
			Update("totp_last_step", step)
		return res.Error == nil && res.RowsAffected == 1, false
	}
	if normalizeRecoveryCode(recovery) == "" {
		return false, false
	}
	res := db.Model(&models.RecoveryCode{}).
		Where("user_id = ? AND code_hash = ? AND used_at IS NULL", user.ID, hashRecoveryCode(keys, recovery)).
		Update("used_at", now)
	if res.Error == nil && res.RowsAffected == 1 {
		return true, true
	}
	return false, false
}

// RegenerateRecoveryCodes replaces all recovery codes with a new set.
func RegenerateRecoveryCodes(db *gorm.DB, cfg *config.Config, userID uint) ([]string, error) {
	var codes []string
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		codes, err = replaceRecoveryCodes(tx, CredentialKeysOf(*cfg), userID)
		return err
	})
	return codes, err
}

// RecoveryCodesRemaining counts the unused recovery codes.
func RecoveryCodesRemaining(db *gorm.DB, userID uint) int {
	var n int64
	db.Model(&models.RecoveryCode{}).Where("user_id = ? AND used_at IS NULL", userID).Count(&n)
	return int(n)
}

// RevokeOtherSessions ends the user's sessions except keep (the one making the change). With
// no current session id (a token from before sessions existed) it only ends the stored rows.
func RevokeOtherSessions(db *gorm.DB, userID uint, keep string, now time.Time) error {
	q := db.Model(&models.Session{}).Where("user_id = ? AND revoked_at IS NULL", userID)
	if keep != "" {
		q = q.Where("public_id <> ?", keep)
	}
	return q.Update("revoked_at", now).Error
}

// DisableTOTP removes the secret and the recovery codes.
func DisableTOTP(db *gorm.DB, userID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", userID).
			Updates(map[string]any{"totp_secret": "", "totp_enabled": false, "totp_last_step": 0}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", userID).Delete(&models.RecoveryCode{}).Error
	})
}
