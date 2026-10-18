// Package persistence implements the Monica import wizard port on top of the import services
// and the existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md). Fetching, mapping and applying the snapshot stay in
// services: they share the contact merge rules with the CSV and vCard imports.
package persistence

import (
	"context"

	"meerkat/clock"
	"meerkat/config"
	"meerkat/models"
	"meerkat/monicaimport/internal/app"
	"meerkat/monicaimport/internal/domain"
	"meerkat/services"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

type Wizard struct {
	db       *gorm.DB
	cfg      *config.Config
	sessions *services.MonicaImportManager
}

func New(db *gorm.DB, cfg *config.Config) *Wizard {
	return &Wizard{db: db, cfg: cfg, sessions: services.NewMonicaImportManager()}
}

var _ app.Wizard = (*Wizard)(nil)

func (w *Wizard) SetClock(c clock.Clock) { w.sessions.SetClock(c) }

func (w *Wizard) Connect(ctx context.Context, actor domain.UserID, baseURL, apiToken string) (domain.Connection, error) {
	go w.sessions.CleanupExpired()
	resp, appErr := w.sessions.Connect(ctx, uint(actor), models.MonicaConnectRequest{BaseURL: baseURL, APIToken: apiToken}, w.cfg.MonicaBlockPrivateURLs)
	if appErr != nil {
		return domain.Connection{}, appErr
	}
	return domain.Connection{SessionID: resp.SessionID, Totals: counts(resp.Totals), EstimatedSeconds: resp.EstimatedFetchSeconds}, nil
}

func (w *Wizard) StartFetch(_ context.Context, actor domain.UserID, o domain.FetchOptions, log *zerolog.Logger) error {
	appErr := w.sessions.StartFetch(w.db, uint(actor), models.MonicaFetchRequest{
		SessionID: o.SessionID, IncludeRelationships: o.IncludeRelationships, IncludeExtras: o.IncludeExtras,
		CollapseReciprocalRelationships: o.CollapseReciprocalRelationships,
	}, log)
	if appErr != nil {
		return appErr
	}
	return nil
}

func (w *Wizard) Status(actor domain.UserID, session string) (domain.Status, error) {
	s, appErr := w.sessions.Status(uint(actor), session)
	if appErr != nil {
		return domain.Status{}, appErr
	}
	out := domain.Status{SessionID: s.SessionID, Phase: s.Phase, Done: s.PhaseDone, Total: s.PhaseTotal, Error: s.Error}
	if s.Result != nil {
		r := result(s.Result)
		out.Result = &r
	}
	return out, nil
}

func (w *Wizard) Preview(actor domain.UserID, session string) (domain.Preview, error) {
	p, appErr := w.sessions.Preview(uint(actor), session)
	if appErr != nil {
		return domain.Preview{}, appErr
	}
	out := domain.Preview{
		SessionID: p.SessionID, TotalRows: p.TotalRows, ValidRows: p.ValidRows, DuplicateCount: p.DuplicateCount,
		ErrorCount: p.ErrorCount, Totals: related(p.Totals),
	}
	for _, r := range p.Rows {
		row := domain.Row{
			Index: r.RowIndex, ParsedContact: r.ParsedContact, ValidationErrors: r.ValidationErrors, Warnings: r.Warnings,
			SuggestedAction: r.SuggestedAction, Related: related(r.Related), HasPhoto: r.HasPhoto,
		}
		if d := r.DuplicateMatch; d != nil {
			row.DuplicateMatch = &domain.Duplicate{
				ExistingContactID: d.ExistingContactID, ExistingFirstname: d.ExistingFirstname, ExistingLastname: d.ExistingLastname,
				ExistingEmail: d.ExistingEmail, ExistingPhone: d.ExistingPhone, MatchReason: d.MatchReason, ExistingDeleted: d.ExistingDeleted,
			}
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

func (w *Wizard) Confirm(_ context.Context, actor domain.UserID, session string, actions []domain.Action, log *zerolog.Logger) (domain.Result, error) {
	req := models.MonicaConfirmRequest{SessionID: session}
	for _, a := range actions {
		req.Actions = append(req.Actions, models.RowImportAction{RowIndex: a.RowIndex, Action: a.Action})
	}
	res, appErr := w.sessions.Confirm(w.db, uint(actor), req, w.cfg, log)
	if appErr != nil {
		return domain.Result{}, appErr
	}
	return result(res), nil
}

func (w *Wizard) Cancel(actor domain.UserID, session string) error {
	if _, appErr := w.sessions.Status(uint(actor), session); appErr != nil {
		return appErr
	}
	w.sessions.Delete(session)
	return nil
}

func counts(c models.MonicaEntityCounts) domain.EntityCounts {
	return domain.EntityCounts{
		Contacts: c.Contacts, Activities: c.Activities, Notes: c.Notes, Reminders: c.Reminders,
		Calls: c.Calls, Tasks: c.Tasks, Gifts: c.Gifts, Debts: c.Debts,
	}
}

func related(c models.MonicaRowRelatedCounts) domain.RelatedCounts {
	return domain.RelatedCounts{Activities: c.Activities, Notes: c.Notes, Reminders: c.Reminders, Relationships: c.Relationships, ExtraNotes: c.ExtraNotes}
}

func result(r *models.MonicaImportResult) domain.Result {
	out := domain.Result{
		TotalProcessed: r.TotalProcessed, Created: r.Created, Updated: r.Updated, Skipped: r.Skipped, Errors: r.Errors,
		ActivitiesCreated: r.ActivitiesCreated, NotesCreated: r.NotesCreated, RemindersCreated: r.RemindersCreated,
		RelationshipsCreated: r.RelationshipsCreated, RelationshipsCollapsed: r.RelationshipsCollapsed,
		PhotosQueued: r.PhotosQueued, PhotosSaved: r.PhotosSaved, PhotosFailed: r.PhotosFailed,
	}
	for _, s := range r.SkippedRows {
		out.SkippedRows = append(out.SkippedRows, domain.SkippedRow{Row: s.Row, Name: s.Name, Reason: s.Reason, Detail: s.Detail})
	}
	return out
}
