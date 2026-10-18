// Package httpadapter exposes the import wizard as huma operations. It maps HTTP to the use
// cases and their results and errors back to the responses the API always sent
// (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"

	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/imports/internal/app"
	"meerkat/imports/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	tags := []string{"import"}
	reg := func(id, path, summary string) huma.Operation {
		return huma.Operation{OperationID: id, Method: http.MethodPost, Path: path, Summary: summary, Tags: tags}
	}
	api.RegisterOn(a, group, reg("upload-csv-import", "/contacts/import/upload", "Upload a CSV file; answers the columns and suggested mappings"), h.uploadCSV)
	api.RegisterOn(a, group, reg("preview-csv-import", "/contacts/import/preview", "Apply column mappings and preview the rows with duplicate detection"), h.preview)
	api.RegisterOn(a, group, reg("confirm-csv-import", "/contacts/import/confirm", "Import the previewed CSV rows with the chosen action per row"), h.confirmCSV)
	api.RegisterOn(a, group, reg("upload-vcf-import", "/contacts/import/vcf/upload", "Upload a VCF file; answers the preview directly"), h.uploadVCF)
	api.RegisterOn(a, group, reg("confirm-vcf-import", "/contacts/import/vcf/confirm", "Import the previewed vCards with the chosen action per row, photos included"), h.confirmVCF)
}

type handlers struct{ uc *app.UseCases }

type ColumnMapping struct {
	CSVColumn    string `json:"csv_column" validate:"required"`
	ContactField string `json:"contact_field" doc:"Empty ignores the column"`
	Group        int    `json:"group" doc:"Multi-value entry index (0-based); ties a value to its label or parts"`
}

type UploadResponse struct {
	SessionID         string          `json:"session_id"`
	Headers           []string        `json:"headers" nullable:"false"`
	SuggestedMappings []ColumnMapping `json:"suggested_mappings" nullable:"false"`
	RowCount          int             `json:"row_count"`
	SampleData        [][]string      `json:"sample_data" nullable:"false" doc:"The first few rows"`
}

type DuplicateMatch struct {
	ExistingContactID uint   `json:"existing_contact_id"`
	ExistingFirstname string `json:"existing_firstname"`
	ExistingLastname  string `json:"existing_lastname"`
	ExistingEmail     string `json:"existing_email"`
	ExistingPhone     string `json:"existing_phone"`
	MatchReason       string `json:"match_reason" doc:"email, name, phone or vcard_uid"`
	ExistingDeleted   bool   `json:"existing_deleted" doc:"The matched contact is in the trash"`
}

// MatchOrNull is a DuplicateMatch or null: huma cannot mark a referenced schema nullable.
type MatchOrNull struct{ *DuplicateMatch }

func (MatchOrNull) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{OneOf: []*huma.Schema{r.Schema(reflect.TypeOf(DuplicateMatch{}), true, "DuplicateMatch"), {Type: "null"}}}
}

func (m MatchOrNull) MarshalJSON() ([]byte, error) {
	if m.DuplicateMatch == nil {
		return []byte("null"), nil
	}
	return json.Marshal(m.DuplicateMatch)
}

type RowPreview struct {
	RowIndex         int            `json:"row_index"`
	ParsedContact    map[string]any `json:"parsed_contact"`
	ValidationErrors []string       `json:"validation_errors" nullable:"false"`
	Warnings         []string       `json:"warnings" nullable:"false" doc:"Values dropped so the row can import"`
	DuplicateMatch   MatchOrNull    `json:"duplicate_match" doc:"The existing contact this row matches, null when none"`
	SuggestedAction  string         `json:"suggested_action" doc:"add, skip or update"`
}

type PreviewResponse struct {
	SessionID      string       `json:"session_id"`
	Rows           []RowPreview `json:"rows" nullable:"false"`
	TotalRows      int          `json:"total_rows"`
	ValidRows      int          `json:"valid_rows"`
	DuplicateCount int          `json:"duplicate_count"`
	ErrorCount     int          `json:"error_count"`
}

type RowAction struct {
	RowIndex int    `json:"row_index" validate:"min=0"`
	Action   string `json:"action" validate:"required,oneof=skip add update"`
}

type SkippedRow struct {
	Row    int    `json:"row" doc:"1-based, as the preview shows it"`
	Name   string `json:"name"`
	Reason string `json:"reason" doc:"skipped (chosen in the preview), invalid or failed"`
	Detail string `json:"detail,omitempty"`
}

type ImportResult struct {
	TotalProcessed int          `json:"total_processed"`
	Created        int          `json:"created"`
	Updated        int          `json:"updated"`
	Skipped        int          `json:"skipped"`
	Errors         []string     `json:"errors" nullable:"false"`
	SkippedRows    []SkippedRow `json:"skipped_rows" nullable:"false"`
}

type FileForm struct {
	File huma.FormFile `form:"file" required:"false" doc:"The file; leaving it out is a 400"`
}

type UploadInput struct {
	RawBody huma.MultipartFormFiles[FileForm]
}

type PreviewBody struct {
	SessionID string          `json:"session_id" validate:"required"`
	Mappings  []ColumnMapping `json:"mappings" validate:"required,dive"`
}

type PreviewInput struct{ Body PreviewBody }

func (i *PreviewInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type ConfirmBody struct {
	SessionID string      `json:"session_id" validate:"required"`
	Actions   []RowAction `json:"actions" validate:"required,dive"`
}

type ConfirmInput struct{ Body ConfirmBody }

func (i *ConfirmInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type UploadOutput struct{ Body UploadResponse }

type PreviewOutput struct{ Body PreviewResponse }

type ResultOutput struct{ Body ImportResult }

func actorOf(ctx context.Context) (domain.UserID, error) {
	id, ok := api.UserID(ctx)
	if !ok {
		return 0, apperrors.ErrUnauthorized("Authentication required")
	}
	return domain.UserID(id), nil
}

func file(in *UploadInput) (*app.File, func()) {
	f := in.RawBody.Data().File
	if !f.IsSet {
		return nil, func() {}
	}
	return &app.File{Name: f.Filename, Size: f.Size, Content: f}, func() { _ = f.Close() }
}

func (h *handlers) uploadCSV(ctx context.Context, in *UploadInput) (*UploadOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	f, closeFile := file(in)
	defer closeFile()
	up, err := h.uc.UploadCSV(ctx, actor, f)
	if err != nil {
		return nil, toAppError(err)
	}
	log := api.Logger(ctx)
	log.Info().Str("session_id", up.SessionID).Int("headers", len(up.Headers)).Int("rows", up.RowCount).Msg("CSV uploaded successfully")
	out := &UploadOutput{Body: UploadResponse{
		SessionID: up.SessionID, Headers: up.Headers, RowCount: up.RowCount, SampleData: up.SampleData,
		SuggestedMappings: make([]ColumnMapping, len(up.SuggestedMappings)),
	}}
	for i, m := range up.SuggestedMappings {
		out.Body.SuggestedMappings[i] = ColumnMapping{CSVColumn: m.CSVColumn, ContactField: m.ContactField, Group: m.Group}
	}
	return out, nil
}

func (h *handlers) uploadVCF(ctx context.Context, in *UploadInput) (*PreviewOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	f, closeFile := file(in)
	defer closeFile()
	p, err := h.uc.UploadVCF(ctx, actor, f)
	if err != nil {
		return nil, toAppError(err)
	}
	api.Logger(ctx).Info().Str("session_id", p.SessionID).Int("contacts", p.TotalRows).Int("valid", p.ValidRows).
		Int("duplicates", p.DuplicateCount).Int("errors", p.ErrorCount).Msg("VCF uploaded and parsed successfully")
	return &PreviewOutput{Body: previewResponse(p)}, nil
}

func (h *handlers) preview(ctx context.Context, in *PreviewInput) (*PreviewOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	mappings := make([]domain.Mapping, len(in.Body.Mappings))
	for i, m := range in.Body.Mappings {
		mappings[i] = domain.Mapping{CSVColumn: m.CSVColumn, ContactField: m.ContactField, Group: m.Group}
	}
	p, err := h.uc.Preview(ctx, actor, in.Body.SessionID, mappings)
	if err != nil {
		return nil, toAppError(err)
	}
	api.Logger(ctx).Info().Str("session_id", p.SessionID).Int("total", p.TotalRows).Int("valid", p.ValidRows).
		Int("duplicates", p.DuplicateCount).Int("errors", p.ErrorCount).Msg("Import preview generated")
	return &PreviewOutput{Body: previewResponse(p)}, nil
}

func actions(in *ConfirmInput) []domain.Action {
	out := make([]domain.Action, len(in.Body.Actions))
	for i, a := range in.Body.Actions {
		out[i] = domain.Action{RowIndex: a.RowIndex, Action: a.Action}
	}
	return out
}

func (h *handlers) confirmCSV(ctx context.Context, in *ConfirmInput) (*ResultOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	r, err := h.uc.ConfirmCSV(ctx, actor, in.Body.SessionID, actions(in), api.Logger(ctx))
	if err != nil {
		return nil, toAppError(err)
	}
	return &ResultOutput{Body: resultResponse(r)}, nil
}

func (h *handlers) confirmVCF(ctx context.Context, in *ConfirmInput) (*ResultOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	r, err := h.uc.ConfirmVCF(ctx, actor, in.Body.SessionID, actions(in), api.Logger(ctx))
	if err != nil {
		return nil, toAppError(err)
	}
	return &ResultOutput{Body: resultResponse(r)}, nil
}

func previewResponse(p domain.Preview) PreviewResponse {
	out := PreviewResponse{
		SessionID: p.SessionID, TotalRows: p.TotalRows, ValidRows: p.ValidRows, DuplicateCount: p.DuplicateCount,
		ErrorCount: p.ErrorCount, Rows: make([]RowPreview, len(p.Rows)),
	}
	for i, r := range p.Rows {
		row := RowPreview{
			RowIndex: r.Index, ParsedContact: r.ParsedContact, ValidationErrors: nonNil(r.ValidationErrors), Warnings: nonNil(r.Warnings),
			SuggestedAction: r.SuggestedAction,
		}
		if d := r.DuplicateMatch; d != nil {
			row.DuplicateMatch.DuplicateMatch = &DuplicateMatch{
				ExistingContactID: d.ExistingContactID, ExistingFirstname: d.ExistingFirstname, ExistingLastname: d.ExistingLastname,
				ExistingEmail: d.ExistingEmail, ExistingPhone: d.ExistingPhone, MatchReason: d.MatchReason, ExistingDeleted: d.ExistingDeleted,
			}
		}
		out.Rows[i] = row
	}
	return out
}

func resultResponse(r domain.Result) ImportResult {
	out := ImportResult{
		TotalProcessed: r.TotalProcessed, Created: r.Created, Updated: r.Updated, Skipped: r.Skipped, Errors: nonNil(r.Errors),
		SkippedRows: make([]SkippedRow, len(r.SkippedRows)),
	}
	for i, s := range r.SkippedRows {
		out.SkippedRows[i] = SkippedRow{Row: s.Row, Name: s.Name, Reason: s.Reason, Detail: s.Detail}
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// toAppError words an error the way the Gin handlers did; errors that carry their own status
// come from the wizard and are passed on.
func toAppError(err error) *apperrors.AppError {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	var noFile domain.NoFileError
	var tooLarge domain.TooLargeError
	var wrongType domain.WrongTypeError
	var invalid domain.InvalidFileError
	switch {
	case errors.As(err, &noFile):
		return apperrors.ErrInvalidInput("file", "No file uploaded")
	case errors.As(err, &tooLarge), errors.As(err, &wrongType), errors.As(err, &invalid):
		return apperrors.ErrInvalidInput("file", err.Error())
	}
	return apperrors.ErrInternal("Failed to process file").WithError(err)
}
