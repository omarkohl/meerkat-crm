package models

import "time"

// Session is one login (one JWT). The token carries PublicID as its "sid" claim; a
// session that is revoked or expired is refused even though the JWT itself is valid.
type Session struct {
	ID         uint       `gorm:"primaryKey" json:"-"`
	CreatedAt  time.Time  `json:"created_at"`
	UserID     uint       `gorm:"not null;index" json:"-"`
	PublicID   string     `gorm:"not null;uniqueIndex" json:"id"`
	UserAgent  string     `json:"user_agent"`
	IP         string     `json:"ip"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"-"`
}
