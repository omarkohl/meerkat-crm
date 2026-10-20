// Package persistence implements the import wizard port on top of the import services and
// the existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md). Parsing, mapping, duplicate detection and merging stay in
// services: the Monica import and the contact sync share them.
package persistence

import (
	"context"
	"io"

	"meerkat/clock"
	"meerkat/config"
	"meerkat/imports/internal/app"
	"meerkat/imports/internal/domain"
	"meerkat/models"
	"meerkat/services"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

type Wizard struct {
	db       *gorm.DB
	cfg      *config.Config
	sessions *services.ImportSessionManager
}

func New(db *gorm.DB, cfg *config.Config) *Wizard {
	return &Wizard{db: db, cfg: cfg, sessions: services.NewImportSessionManager()}
}

var _ app.Wizard = (*Wizard)(nil)

func (w *Wizard) SetClock(c clock.Clock) { w.sessions.SetClock(c) }

func (w *Wizard) MaxSize(kind domain.Kind) int64 {
	if kind == domain.VCF {
		return services.MaxVCFSize
	}
	return services.MaxCSVSize
}

func (w *Wizard) UploadCSV(_ context.Context, actor domain.UserID, file io.Reader) (domain.Upload, error) {
	go w.sessions.CleanupExpired()
	headers, rows, err := services.ParseCSV(file)
	if err != nil {
		return domain.Upload{}, domain.InvalidFileError{Reason: err.Error()}
	}
	id := w.sessions.CreateCSVSession(uint(actor), headers, rows)
	sample := make([][]string, 0, services.SampleRows)
	for i := 0; i < len(rows) && i < services.SampleRows; i++ {
		sample = append(sample, rows[i])
	}
	suggested := services.SuggestColumnMappings(headers)
	out := domain.Upload{SessionID: id, Headers: headers, RowCount: len(rows), SampleData: sample}
	for _, m := range suggested {
		out.SuggestedMappings = append(out.SuggestedMappings, domain.Mapping{CSVColumn: m.CSVColumn, ContactField: m.ContactField, Group: m.Group})
	}
	return out, nil
}

func (w *Wizard) UploadVCF(ctx context.Context, actor domain.UserID, file io.Reader) (domain.Preview, error) {
	go w.sessions.CleanupExpired()
	contacts, previews, stats, err := services.ParseVCF(file, w.db.WithContext(ctx), uint(actor))
	if err != nil {
		return domain.Preview{}, domain.InvalidFileError{Reason: err.Error()}
	}
	id := w.sessions.CreateVCFSession(uint(actor), contacts, previews)
	return preview(&models.ImportPreviewResponse{
		SessionID: id, Rows: previews, TotalRows: len(previews), ValidRows: stats.ValidCount,
		DuplicateCount: stats.DuplicateCount, ErrorCount: stats.ErrorCount,
	}), nil
}

func (w *Wizard) Preview(ctx context.Context, actor domain.UserID, session string, mappings []domain.Mapping) (domain.Preview, error) {
	req := models.ImportPreviewRequest{SessionID: session}
	for _, m := range mappings {
		req.Mappings = append(req.Mappings, models.ColumnMapping{CSVColumn: m.CSVColumn, ContactField: m.ContactField, Group: m.Group})
	}
	resp, appErr := w.sessions.PreviewCSV(w.db.WithContext(ctx), uint(actor), req)
	if appErr != nil {
		return domain.Preview{}, appErr
	}
	return preview(resp), nil
}

func request(session string, actions []domain.Action) models.ImportConfirmRequest {
	req := models.ImportConfirmRequest{SessionID: session}
	for _, a := range actions {
		req.Actions = append(req.Actions, models.RowImportAction{RowIndex: a.RowIndex, Action: a.Action})
	}
	return req
}

func (w *Wizard) ConfirmCSV(ctx context.Context, actor domain.UserID, session string, actions []domain.Action, log *zerolog.Logger) (domain.Result, error) {
	res, appErr := w.sessions.Confirm(w.db.WithContext(ctx), uint(actor), request(session, actions), log)
	if appErr != nil {
		return domain.Result{}, appErr
	}
	return result(res), nil
}

func (w *Wizard) ConfirmVCF(ctx context.Context, actor domain.UserID, session string, actions []domain.Action, log *zerolog.Logger) (domain.Result, error) {
	res, appErr := w.sessions.ConfirmVCF(w.db.WithContext(ctx), uint(actor), request(session, actions), w.cfg, log)
	if appErr != nil {
		return domain.Result{}, appErr
	}
	return result(res), nil
}

func (w *Wizard) PhotoStatus(actor domain.UserID, session string) (domain.PhotoStatus, error) {
	st, appErr := w.sessions.PhotoStatus(uint(actor), session)
	if appErr != nil {
		return domain.PhotoStatus{}, appErr
	}
	return domain.PhotoStatus{Phase: st.Phase, Done: st.Done, Total: st.Total, Saved: st.Saved, Failed: st.Failed}, nil
}

func preview(p *models.ImportPreviewResponse) domain.Preview {
	out := domain.Preview{
		SessionID: p.SessionID, TotalRows: p.TotalRows, ValidRows: p.ValidRows,
		DuplicateCount: p.DuplicateCount, ErrorCount: p.ErrorCount,
	}
	for _, r := range p.Rows {
		row := domain.Row{
			Index: r.RowIndex, ParsedContact: r.ParsedContact, ValidationErrors: r.ValidationErrors, Warnings: r.Warnings,
			SuggestedAction: r.SuggestedAction,
		}
		if d := r.DuplicateMatch; d != nil {
			row.DuplicateMatch = &domain.Duplicate{
				ExistingContactID: d.ExistingContactID, ExistingFirstname: d.ExistingFirstname, ExistingLastname: d.ExistingLastname,
				ExistingEmail: d.ExistingEmail, ExistingPhone: d.ExistingPhone, MatchReason: d.MatchReason, ExistingDeleted: d.ExistingDeleted,
			}
		}
		out.Rows = append(out.Rows, row)
	}
	return out
}

func result(r *models.ImportResult) domain.Result {
	out := domain.Result{TotalProcessed: r.TotalProcessed, Created: r.Created, Updated: r.Updated, Skipped: r.Skipped, Errors: r.Errors, PhotosQueued: r.PhotosQueued}
	for _, s := range r.SkippedRows {
		out.SkippedRows = append(out.SkippedRows, domain.SkippedRow{Row: s.Row, Name: s.Name, Reason: s.Reason, Detail: s.Detail})
	}
	return out
}
