package models

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
