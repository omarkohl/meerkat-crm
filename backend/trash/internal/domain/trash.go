// Package domain is the vocabulary of the trash module: deleted contacts and how long they
// stay. It imports nothing but the standard library (docs/adr/0012-module-layout.md).
package domain

import (
	"errors"
	"time"
)

type UserID uint

type ContactID uint

// DefaultRetentionDays is how long a deleted contact stays when none is configured.
const DefaultRetentionDays = 30

// ErrNotInTrash means the contact does not exist, is not deleted, or is another user's.
var ErrNotInTrash = errors.New("contact not in trash")

type TrashedContact struct {
	ID             ContactID
	Firstname      string
	Lastname       string
	Nickname       string
	Email          string
	PhotoThumbnail string
	DeletedAt      time.Time
}

// Retention is the configured number of days, or the default when it is below one.
func Retention(configured int) int {
	if configured >= 1 {
		return configured
	}
	return DefaultRetentionDays
}

// PurgeAt is when a contact deleted at deletedAt is removed for good.
func PurgeAt(deletedAt time.Time, retentionDays int) time.Time {
	return deletedAt.Add(time.Duration(retentionDays) * 24 * time.Hour)
}
