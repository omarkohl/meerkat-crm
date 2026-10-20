package models

import (
	"fmt"
	"strings"
	"time"
)

// ImportableContactFields defines the valid target fields for import
var ImportableContactFields = []string{
	// Scalars
	"firstname", "lastname", "middle_name", "prefix", "suffix", "nickname", "gender",
	"birthday", "anniversary", "organization", "department", "job_title", "role",
	"how_we_met", "food_preference", "work_information", "contact_information", "circles", "note",
	// Multi-value values
	"email", "phone", "url", "impp",
	"address_street", "address_city", "address_region", "address_postal", "address_country",
	// Multi-value labels/types
	"email_label", "phone_label", "url_label", "impp_label", "address_label",
}

// ColumnMapping represents how a CSV column maps to a contact field
type ColumnMapping struct {
	CSVColumn    string `json:"csv_column" validate:"required"`
	ContactField string `json:"contact_field"` // Empty means "ignore this column"
	Group        int    `json:"group"`         // Multi-value entry index (0-based); ties value+label/parts of one entry together
}

// ImportUploadResponse is returned after CSV upload
type ImportUploadResponse struct {
	SessionID         string          `json:"session_id"`
	Headers           []string        `json:"headers"`
	SuggestedMappings []ColumnMapping `json:"suggested_mappings"`
	RowCount          int             `json:"row_count"`
	SampleData        [][]string      `json:"sample_data"` // First few rows for preview
}

// ImportPreviewRequest is sent to request a preview with mappings
type ImportPreviewRequest struct {
	SessionID string          `json:"session_id" validate:"required"`
	Mappings  []ColumnMapping `json:"mappings" validate:"required,dive"`
}

// DuplicateMatch describes a potential duplicate contact
type DuplicateMatch struct {
	ExistingContactID uint   `json:"existing_contact_id"`
	ExistingFirstname string `json:"existing_firstname"`
	ExistingLastname  string `json:"existing_lastname"`
	ExistingEmail     string `json:"existing_email"`
	ExistingPhone     string `json:"existing_phone"`
	MatchReason       string `json:"match_reason"`     // "email", "name", "phone", or "vcard_uid"
	ExistingDeleted   bool   `json:"existing_deleted"` // true if the matched contact is soft-deleted
}

// ImportRowPreview represents one row in the import preview
type ImportRowPreview struct {
	RowIndex         int                    `json:"row_index"`
	ParsedContact    map[string]interface{} `json:"parsed_contact"`    // Parsed field values
	ValidationErrors []string               `json:"validation_errors"` // Any validation issues
	Warnings         []string               `json:"warnings"`          // Values dropped so the row can import
	DuplicateMatch   *DuplicateMatch        `json:"duplicate_match"`   // Potential duplicate, if any
	SuggestedAction  string                 `json:"suggested_action"`  // "add", "skip", or "update"
}

// ImportPreviewResponse contains the full preview data
type ImportPreviewResponse struct {
	SessionID      string             `json:"session_id"`
	Rows           []ImportRowPreview `json:"rows"`
	TotalRows      int                `json:"total_rows"`
	ValidRows      int                `json:"valid_rows"`
	DuplicateCount int                `json:"duplicate_count"`
	ErrorCount     int                `json:"error_count"`
}

// RowImportAction specifies what to do with each row
type RowImportAction struct {
	RowIndex int    `json:"row_index" validate:"min=0"`
	Action   string `json:"action" validate:"required,oneof=skip add update"`
}

// ImportConfirmRequest is sent to execute the import
type ImportConfirmRequest struct {
	SessionID string            `json:"session_id" validate:"required"`
	Actions   []RowImportAction `json:"actions" validate:"required,dive"`
}

// ImportResult summarizes what happened during import
type ImportResult struct {
	TotalProcessed int      `json:"total_processed"`
	Created        int      `json:"created"`
	Updated        int      `json:"updated"`
	Skipped        int      `json:"skipped"`
	Errors         []string `json:"errors"`
	// SkippedRows lists every row counted in Skipped, so the result can say which.
	SkippedRows  []SkippedRow `json:"skipped_rows"`
	PhotosQueued int          `json:"photos_queued"`
}

// ImportPhotoStatus is the progress of the photos of a confirmed vCard import.
type ImportPhotoStatus struct {
	Phase  string // "importing_photos" or "done"
	Done   int
	Total  int
	Saved  int
	Failed int
}

// SkippedRow names a row that was not imported and why.
type SkippedRow struct {
	Row    int    `json:"row"` // 1-based, as the preview shows it
	Name   string `json:"name"`
	Reason string `json:"reason"` // "skipped" (chosen in the preview), "invalid" or "failed"
	Detail string `json:"detail,omitempty"`
}

// SkipRow records a row the import did not apply.
func (r *ImportResult) SkipRow(preview *ImportRowPreview, reason, detail string) {
	r.Skipped++
	name := func(key string) string {
		v, _ := preview.ParsedContact[key].(string)
		return v
	}
	r.SkippedRows = append(r.SkippedRows, SkippedRow{
		Row:    preview.RowIndex + 1,
		Name:   strings.TrimSpace(name("firstname") + " " + name("lastname")),
		Reason: reason,
		Detail: detail,
	})
}

// SkipChosenRow records a row whose action was "skip": chosen by the user, or
// forced because the row failed validation.
func (r *ImportResult) SkipChosenRow(preview *ImportRowPreview) {
	if len(preview.ValidationErrors) > 0 {
		r.SkipRow(preview, "invalid", strings.Join(preview.ValidationErrors, ", "))
		return
	}
	r.SkipRow(preview, "skipped", "")
}

// FailRow records a row that could not be applied, as an error and a skipped row.
func (r *ImportResult) FailRow(preview *ImportRowPreview, detail string) {
	r.Errors = append(r.Errors, fmt.Sprintf("Row %d: %s", preview.RowIndex+1, detail))
	r.SkipRow(preview, "failed", detail)
}

// ImportSession stores temporary import data server-side (not persisted to DB)
type ImportSession struct {
	ID        string
	UserID    uint
	Headers   []string
	Rows      [][]string
	CreatedAt time.Time
	ExpiresAt time.Time
	// Cached preview data (set after PreviewImport is called)
	Mappings      []ColumnMapping
	PreviewRows   []ImportRowPreview
	PreviewCached bool
}
