// Package domain is the vocabulary of the imports module: the steps of the CSV and vCard import
// wizard and the rules for the uploaded file. It imports nothing but the standard library
// (docs/adr/0012-module-layout.md).
package domain

import (
	"fmt"
	"strings"
)

type UserID uint

type Kind string

const (
	CSV Kind = "csv"
	VCF Kind = "vcf"
)

type NoFileError struct{}

func (NoFileError) Error() string { return "no file uploaded" }

type TooLargeError struct{ MaxMB int64 }

func (e TooLargeError) Error() string {
	return fmt.Sprintf("File too large. Maximum size is %d MB", e.MaxMB)
}

type WrongTypeError struct{ Kind Kind }

func (e WrongTypeError) Error() string {
	return fmt.Sprintf("File must be a %s file", strings.ToUpper(string(e.Kind)))
}

// InvalidFileError carries why a file with the right name and size could not be read.
type InvalidFileError struct{ Reason string }

func (e InvalidFileError) Error() string { return e.Reason }

// CheckFile applies the rules every upload must pass before its content is read.
func CheckFile(kind Kind, filename string, size, maxBytes int64) error {
	if size > maxBytes {
		return TooLargeError{MaxMB: maxBytes / (1024 * 1024)}
	}
	if !strings.HasSuffix(strings.ToLower(filename), "."+string(kind)) {
		return WrongTypeError{Kind: kind}
	}
	return nil
}

type Mapping struct {
	CSVColumn    string
	ContactField string
	Group        int
}

type Upload struct {
	SessionID         string
	Headers           []string
	SuggestedMappings []Mapping
	RowCount          int
	SampleData        [][]string
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

type Row struct {
	Index            int
	ParsedContact    map[string]any
	ValidationErrors []string
	Warnings         []string
	DuplicateMatch   *Duplicate
	SuggestedAction  string
}

type Preview struct {
	SessionID      string
	Rows           []Row
	TotalRows      int
	ValidRows      int
	DuplicateCount int
	ErrorCount     int
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
	TotalProcessed int
	Created        int
	Updated        int
	Skipped        int
	Errors         []string
	SkippedRows    []SkippedRow
}
