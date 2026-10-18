package models

import (
	"time"
)

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
