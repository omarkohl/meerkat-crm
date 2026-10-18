// Package domain is the vocabulary of the monicaimport module: the steps of the import from a
// Monica instance. It imports nothing but the standard library
// (docs/adr/0012-module-layout.md).
package domain

import "errors"

type UserID uint

var ErrNoSession = errors.New("session_id is missing")

type EntityCounts struct {
	Contacts, Activities, Notes, Reminders, Calls, Tasks, Gifts, Debts int
}

type Connection struct {
	SessionID        string
	Totals           EntityCounts
	EstimatedSeconds int
}

type FetchOptions struct {
	SessionID                       string
	IncludeRelationships            bool
	IncludeExtras                   bool
	CollapseReciprocalRelationships bool
}

type Duplicate struct {
	ExistingContactID uint
	ExistingFirstname string
	ExistingLastname  string
	ExistingEmail     string
	ExistingPhone     string
	MatchReason       string
	ExistingDeleted   bool
}

type RelatedCounts struct {
	Activities, Notes, Reminders, Relationships, ExtraNotes int
}

type Row struct {
	Index            int
	ParsedContact    map[string]any
	ValidationErrors []string
	Warnings         []string
	DuplicateMatch   *Duplicate
	SuggestedAction  string
	Related          RelatedCounts
	HasPhoto         bool
}

type Preview struct {
	SessionID      string
	Rows           []Row
	TotalRows      int
	ValidRows      int
	DuplicateCount int
	ErrorCount     int
	Totals         RelatedCounts
}

type Action struct {
	RowIndex int
	Action   string
}

type SkippedRow struct {
	Row    int
	Name   string
	Reason string
	Detail string
}

type Result struct {
	TotalProcessed         int
	Created                int
	Updated                int
	Skipped                int
	Errors                 []string
	SkippedRows            []SkippedRow
	ActivitiesCreated      int
	NotesCreated           int
	RemindersCreated       int
	RelationshipsCreated   int
	RelationshipsCollapsed int
	PhotosQueued           int
	PhotosSaved            int
	PhotosFailed           int
}

type Status struct {
	SessionID string
	Phase     string
	Done      int
	Total     int
	Error     string
	Result    *Result
}
