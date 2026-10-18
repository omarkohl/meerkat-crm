package models

import "time"

// IcsFeed is a user's subscribable calendar feed. Only the SHA-256 hash of the
// token is stored; the URL carrying the plaintext token is shown once. A user has
// at most one feed: regenerating replaces the hash, revoking deletes the row.
type IcsFeed struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"not null;uniqueIndex"`
	TokenHash string `gorm:"not null;uniqueIndex" json:"-"`
	// LastFetchedAt is when the feed was last read through its URL; it is written at
	// most once a minute (see the tokens module).
	LastFetchedAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
