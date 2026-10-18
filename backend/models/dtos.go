package models

import (
	"encoding/json"
	"time"
)

// CardDAVConnectionInput is the DTO for creating/updating the user's CardDAV
// connection. On update, an empty password keeps the stored one (set ClearPassword to remove it)
type CardDAVConnectionInput struct {
	BaseURL         string `json:"base_url" validate:"required,url,max=2048"`
	AddressBookPath string `json:"address_book_path" validate:"required,max=2048"`
	AddressBookName string `json:"address_book_name" validate:"max=200"`
	Username        string `json:"username" validate:"omitempty,max=200"`
	Password        string `json:"password" validate:"omitempty,max=500"`
	ClearPassword   bool   `json:"clear_password"`
	Direction       string `json:"direction" validate:"omitempty,oneof=two_way pull push"`
	SyncEnabled     *bool  `json:"sync_enabled"`
}

// CardDAVDiscoverInput is the DTO for listing address books on a CardDAV server.
// An empty password reuses the stored connection credentials when present.
type CardDAVDiscoverInput struct {
	BaseURL  string `json:"base_url" validate:"required,url,max=2048"`
	Username string `json:"username" validate:"omitempty,max=200"`
	Password string `json:"password" validate:"omitempty,max=500"`
}

// CardDAVConnectionResponse is the DTO returned for a CardDAV connection (no
// secrets). Syncs run in the background, so this is also the polling target
// that reports whether one is in flight and how the last one turned out.
type CardDAVConnectionResponse struct {
	ID              uint       `json:"id"`
	BaseURL         string     `json:"base_url"`
	AddressBookPath string     `json:"address_book_path"`
	AddressBookName string     `json:"address_book_name"`
	Username        string     `json:"username"`
	HasPassword     bool       `json:"has_password"`
	Direction       string     `json:"direction"`
	SyncEnabled     bool       `json:"sync_enabled"`
	SyncRunning     bool       `json:"sync_running"`
	LastSyncedAt    *time.Time `json:"last_synced_at"`
	LastSyncStatus  string     `json:"last_sync_status"`
	LastSyncError   string     `json:"last_sync_error"`
	// LastSyncStats is a ContactSyncStats object, or null before the first run.
	LastSyncStats json.RawMessage `json:"last_sync_stats"`
	CreatedAt     time.Time       `json:"created_at"`
}

// DiscoveredAddressBook describes one address book found on a CardDAV server.
type DiscoveredAddressBook struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Birthday represents a unified birthday entry for contacts and relationships
type Birthday struct {
	Type                  string `json:"type"`                              // "contact" or "relationship"
	Name                  string `json:"name"`                              // Unified display name
	Birthday              string `json:"birthday"`                          // Birthday in YYYY-MM-DD format
	PhotoThumbnail        string `json:"photo_thumbnail,omitempty"`         // Profile picture thumbnail (base64)
	ContactID             uint   `json:"contact_id"`                        // Contact ID (the person or parent contact for relationships)
	RelationshipType      string `json:"relationship_type,omitempty"`       // Relationship type (empty for contacts)
	AssociatedContactName string `json:"associated_contact_name,omitempty"` // Parent contact name (for relationships)
}

// AdminUserResponse - user data returned to admin (no password)
type AdminUserResponse struct {
	ID         uint      `json:"id"`
	Username   string    `json:"username"`
	Email      string    `json:"email"`
	Language   string    `json:"language"`
	DateFormat string    `json:"date_format"`
	IsAdmin    bool      `json:"is_admin"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// TwoFactorEnabled tells an admin whether a reset is needed; never the secret.
	TwoFactorEnabled bool `json:"two_factor_enabled"`
}

// AdminUserUpdateInput - DTO for admin updating a user
type AdminUserUpdateInput struct {
	Username *string `json:"username" validate:"omitempty,min=1,max=50,no_at_sign"`
	Email    *string `json:"email" validate:"omitempty,email"`
	Password *string `json:"password" validate:"omitempty,min=8,strong_password"`
	IsAdmin  *bool   `json:"is_admin"`
}

// AdminUsersListResponse - paginated list of users
type AdminUsersListResponse struct {
	Users      []AdminUserResponse `json:"users"`
	Total      int64               `json:"total"`
	Page       int                 `json:"page"`
	Limit      int                 `json:"limit"`
	TotalPages int                 `json:"total_pages"`
}

// BackupResponse describes one stored backup.
type BackupResponse struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// BackupListResponse is the backup list plus the settings that drive the schedule.
type BackupListResponse struct {
	Backups        []BackupResponse `json:"backups"`
	Enabled        bool             `json:"enabled"`
	Time           string           `json:"time"`
	RetentionCount int              `json:"retention_count"`
	Directory      string           `json:"directory"`
}
