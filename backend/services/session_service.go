package services

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"meerkat/config"
	"meerkat/logger"
	"meerkat/models"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

const (
	maxUserAgentLen   = 255
	maxAuditDetailLen = 1000
	// sessionRetentionAfterEnd is how long an expired or revoked session row is kept.
	sessionRetentionAfterEnd = 24 * time.Hour
)

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// IssueSession records a new login session and returns the JWT that belongs to it. The
// token carries the session id ("sid") and the user's token version ("tv").
func IssueSession(db *gorm.DB, user models.User, cfg *config.Config, ip, userAgent string, now time.Time) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	session := models.Session{
		UserID:     user.ID,
		PublicID:   hex.EncodeToString(raw),
		UserAgent:  truncate(userAgent, maxUserAgentLen),
		IP:         truncate(ip, 64),
		LastSeenAt: now,
		ExpiresAt:  now.Add(time.Hour * time.Duration(cfg.JWTExpiryHours)),
	}
	token, err := signToken(user, cfg, session.PublicID, now)
	if err != nil {
		return "", err
	}
	if err := db.Create(&session).Error; err != nil {
		return "", err
	}
	return token, nil
}

func activeSessions(db *gorm.DB, userID uint, now time.Time) *gorm.DB {
	return db.Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, now)
}

// ListSessions returns the user's live sessions, newest first.
func ListSessions(db *gorm.DB, userID uint, now time.Time) ([]models.Session, error) {
	var sessions []models.Session
	err := activeSessions(db, userID, now).Order("created_at DESC, id DESC").Find(&sessions).Error
	return sessions, err
}

// RevokeSession revokes one of the user's live sessions. It reports false when there is no
// such session (unknown, someone else's, already revoked or expired).
func RevokeSession(db *gorm.DB, userID uint, publicID string, now time.Time) (bool, error) {
	res := activeSessions(db, userID, now).Model(&models.Session{}).Where("public_id = ?", publicID).Update("revoked_at", now)
	return res.RowsAffected > 0, res.Error
}

// RevokeAllSessions signs the user out everywhere: sessions are revoked and the token
// version is raised, which also kills tokens issued before sessions existed.
func RevokeAllSessions(db *gorm.DB, userID uint, now time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Raw SQL: the field is read-only for gorm so that Save() cannot overwrite it.
		if err := tx.Exec("UPDATE users SET token_version = token_version + 1 WHERE id = ?", userID).Error; err != nil {
			return err
		}
		return tx.Model(&models.Session{}).Where("user_id = ? AND revoked_at IS NULL", userID).
			Update("revoked_at", now).Error
	})
}

// SessionIDFromToken returns the "sid" claim of a token signed with our key, or "". It is
// used on logout, which is a public route, and ignores expiry on purpose.
func SessionIDFromToken(tokenString string, cfg *config.Config) string {
	token, err := jwt.Parse(tokenString, func(*jwt.Token) (any, error) { return []byte(cfg.JWTSecretKey), nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithoutClaimsValidation())
	if err != nil || !token.Valid {
		return ""
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		sid, _ := claims["sid"].(string)
		return sid
	}
	return ""
}

// PurgeSessions deletes sessions that ended (expired or were revoked) more than a day ago.
func PurgeSessions(db *gorm.DB, now time.Time) (int64, error) {
	cutoff := now.Add(-sessionRetentionAfterEnd)
	res := db.Where("expires_at < ? OR revoked_at < ?", cutoff, cutoff).Delete(&models.Session{})
	return res.RowsAffected, res.Error
}

// PurgeAuditLog deletes audit entries older than retentionDays. 0 or less keeps everything.
func PurgeAuditLog(db *gorm.DB, retentionDays int, now time.Time) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	res := db.Where("created_at < ?", now.AddDate(0, 0, -retentionDays)).Delete(&models.AuditLog{})
	return res.RowsAffected, res.Error
}

// WriteUserAudit records an event about a user, with their username, the client address and
// user agent. Like WriteAudit it never fails the caller.
func WriteUserAudit(db *gorm.DB, userID uint, event, detail, ip, userAgent string) {
	entry := models.AuditLog{UserID: &userID, Event: event, IP: ip, UserAgent: userAgent, Detail: detail}
	var name string
	if err := db.Model(&models.User{}).Where("id = ?", userID).Limit(1).Pluck("username", &name).Error; err == nil {
		entry.Username = name
	}
	WriteAudit(db, entry)
}

// WriteAudit stores an audit entry. It never fails the caller: an error is only logged.
func WriteAudit(db *gorm.DB, entry models.AuditLog) {
	entry.UserAgent = truncate(entry.UserAgent, maxUserAgentLen)
	entry.Detail = truncate(entry.Detail, maxAuditDetailLen)
	if err := db.Create(&entry).Error; err != nil {
		logger.Warn().Err(err).Str("event", entry.Event).Msg("Failed to write audit log entry")
	}
}
