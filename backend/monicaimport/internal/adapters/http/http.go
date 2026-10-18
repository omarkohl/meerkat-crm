// Package httpadapter exposes the Monica import as huma operations. It maps HTTP to the use
// cases and their results and errors back to the responses the API always sent
// (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"net/http"

	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/monicaimport/internal/app"
	"meerkat/monicaimport/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

const base = "/contacts/import/monica"

func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	tags := []string{"monica-import"}
	reg := func(id, method, path, summary string, status int) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: base + path, Summary: summary, Tags: tags, DefaultStatus: status}
	}
	api.RegisterOn(a, group, reg("connect-monica-import", http.MethodPost, "/connect", "Check the Monica URL and API token; answers what the account holds and a session", 0), h.connect)
	api.RegisterOn(a, group, reg("fetch-monica-import", http.MethodPost, "/fetch", "Start fetching the account in the background", http.StatusAccepted), h.fetch)
	api.RegisterOn(a, group, reg("monica-import-status", http.MethodGet, "/status", "Progress of the fetch and the import, for polling", 0), h.status)
	api.RegisterOn(a, group, reg("monica-import-preview", http.MethodGet, "/preview", "The contacts of the fetched account with duplicate detection", 0), h.preview)
	api.RegisterOn(a, group, reg("confirm-monica-import", http.MethodPost, "/confirm", "Import the fetched contacts with the chosen action per contact", 0), h.confirm)
	api.RegisterOn(a, group, reg("cancel-monica-import", http.MethodDelete, "/session", "Cancel a session and drop its API token from memory", 0), h.cancel)
}

type handlers struct{ uc *app.UseCases }

type MonicaEntityCounts struct {
	Contacts   int `json:"contacts"`
	Activities int `json:"activities"`
	Notes      int `json:"notes"`
	Reminders  int `json:"reminders"`
	Calls      int `json:"calls"`
	Tasks      int `json:"tasks"`
	Gifts      int `json:"gifts"`
	Debts      int `json:"debts"`
}

type MonicaConnectBody struct {
	BaseURL  string `json:"base_url" validate:"required,max=500"`
	APIToken string `json:"api_token" validate:"required,max=4000" doc:"Held in memory for the session only"`
}

type MonicaConnectInput struct{ Body MonicaConnectBody }

func (i *MonicaConnectInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type MonicaConnectResponse struct {
	SessionID             string             `json:"session_id"`
	Totals                MonicaEntityCounts `json:"totals"`
	EstimatedFetchSeconds int                `json:"estimated_fetch_seconds"`
}

type MonicaConnectOutput struct{ Body MonicaConnectResponse }

type MonicaFetchBody struct {
	SessionID                       string `json:"session_id" validate:"required"`
	IncludeRelationships            bool   `json:"include_relationships" doc:"One extra request per contact"`
	IncludeExtras                   bool   `json:"include_extras" doc:"Import calls, tasks, gifts and debts as notes"`
	CollapseReciprocalRelationships bool   `json:"collapse_reciprocal_relationships" doc:"Import one row per relationship pair"`
}

type MonicaFetchInput struct{ Body MonicaFetchBody }

func (i *MonicaFetchInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type MonicaFetchOutput struct {
	Body struct {
		SessionID string `json:"session_id"`
	}
}

type MonicaSessionQuery struct {
	SessionID string `query:"session_id" doc:"The session from connect"`
}

type MonicaSkippedRow struct {
	Row    int    `json:"row" doc:"1-based, as the preview shows it"`
	Name   string `json:"name"`
	Reason string `json:"reason" doc:"skipped, invalid or failed"`
	Detail string `json:"detail,omitempty"`
}

type MonicaImportResult struct {
	TotalProcessed         int                `json:"total_processed"`
	Created                int                `json:"created"`
	Updated                int                `json:"updated"`
	Skipped                int                `json:"skipped"`
	Errors                 []string           `json:"errors" nullable:"false"`
	SkippedRows            []MonicaSkippedRow `json:"skipped_rows" nullable:"false"`
	ActivitiesCreated      int                `json:"activities_created"`
	NotesCreated           int                `json:"notes_created"`
	RemindersCreated       int                `json:"reminders_created"`
	RelationshipsCreated   int                `json:"relationships_created"`
	RelationshipsCollapsed int                `json:"relationships_collapsed" doc:"Reciprocal halves skipped because the other direction was imported"`
	PhotosQueued           int                `json:"photos_queued"`
	PhotosSaved            int                `json:"photos_saved"`
	PhotosFailed           int                `json:"photos_failed"`
}

type MonicaStatusResponse struct {
	SessionID  string              `json:"session_id"`
	Phase      string              `json:"phase" doc:"connecting, fetching_contacts, fetching_activities, fetching_notes, fetching_reminders, fetching_extras, fetching_relationships, building_preview, ready, importing, importing_photos, done or failed"`
	PhaseDone  int                 `json:"phase_done"`
	PhaseTotal int                 `json:"phase_total"`
	Error      string              `json:"error,omitempty"`
	Result     *MonicaImportResult `json:"result,omitempty" doc:"Set from the importing_photos phase on"`
}

type MonicaStatusOutput struct{ Body MonicaStatusResponse }

type MonicaDuplicateMatch struct {
	ExistingContactID uint   `json:"existing_contact_id"`
	ExistingFirstname string `json:"existing_firstname"`
	ExistingLastname  string `json:"existing_lastname"`
	ExistingEmail     string `json:"existing_email"`
	ExistingPhone     string `json:"existing_phone"`
	MatchReason       string `json:"match_reason" doc:"email, name, phone or vcard_uid"`
	ExistingDeleted   bool   `json:"existing_deleted"`
}

type MonicaRelatedCounts struct {
	Activities    int `json:"activities"`
	Notes         int `json:"notes"`
	Reminders     int `json:"reminders"`
	Relationships int `json:"relationships"`
	ExtraNotes    int `json:"extra_notes" doc:"Calls, tasks, gifts and debts converted to notes"`
}

type MonicaRowPreview struct {
	RowIndex         int                                `json:"row_index"`
	ParsedContact    map[string]any                     `json:"parsed_contact"`
	ValidationErrors []string                           `json:"validation_errors" nullable:"false"`
	Warnings         []string                           `json:"warnings" nullable:"false"`
	DuplicateMatch   api.Nullable[MonicaDuplicateMatch] `json:"duplicate_match" doc:"The existing contact this row matches, null when none"`
	SuggestedAction  string                             `json:"suggested_action" doc:"add, skip or update"`
	Related          MonicaRelatedCounts                `json:"related"`
	HasPhoto         bool                               `json:"has_photo"`
}

type MonicaPreviewResponse struct {
	SessionID      string              `json:"session_id"`
	Rows           []MonicaRowPreview  `json:"rows" nullable:"false"`
	TotalRows      int                 `json:"total_rows"`
	ValidRows      int                 `json:"valid_rows"`
	DuplicateCount int                 `json:"duplicate_count"`
	ErrorCount     int                 `json:"error_count"`
	Totals         MonicaRelatedCounts `json:"totals"`
}

type MonicaPreviewOutput struct{ Body MonicaPreviewResponse }

type MonicaRowAction struct {
	RowIndex int    `json:"row_index" validate:"min=0"`
	Action   string `json:"action" validate:"required,oneof=skip add update"`
}

type MonicaConfirmBody struct {
	SessionID string            `json:"session_id" validate:"required"`
	Actions   []MonicaRowAction `json:"actions" validate:"required,dive"`
}

type MonicaConfirmInput struct{ Body MonicaConfirmBody }

func (i *MonicaConfirmInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type MonicaConfirmOutput struct{ Body MonicaImportResult }

type MonicaCancelOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

func actorOf(ctx context.Context) (domain.UserID, error) {
	id, ok := api.UserID(ctx)
	if !ok {
		return 0, apperrors.ErrUnauthorized("Authentication required")
	}
	return domain.UserID(id), nil
}

func (h *handlers) connect(ctx context.Context, in *MonicaConnectInput) (*MonicaConnectOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	log := api.Logger(ctx)
	c, err := h.uc.Connect(ctx, actor, in.Body.BaseURL, in.Body.APIToken)
	if err != nil {
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) {
			log.Warn().Str("code", appErr.Code).Msg("Monica connect failed")
		}
		return nil, toAppError(err)
	}
	log.Info().Str("session_id", c.SessionID).Int("contacts", c.Totals.Contacts).Msg("Monica import session connected")
	return &MonicaConnectOutput{Body: MonicaConnectResponse{
		SessionID: c.SessionID, EstimatedFetchSeconds: c.EstimatedSeconds,
		Totals: MonicaEntityCounts{
			Contacts: c.Totals.Contacts, Activities: c.Totals.Activities, Notes: c.Totals.Notes, Reminders: c.Totals.Reminders,
			Calls: c.Totals.Calls, Tasks: c.Totals.Tasks, Gifts: c.Totals.Gifts, Debts: c.Totals.Debts,
		},
	}}, nil
}

func (h *handlers) fetch(ctx context.Context, in *MonicaFetchInput) (*MonicaFetchOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	err = h.uc.StartFetch(ctx, actor, domain.FetchOptions{
		SessionID: in.Body.SessionID, IncludeRelationships: in.Body.IncludeRelationships, IncludeExtras: in.Body.IncludeExtras,
		CollapseReciprocalRelationships: in.Body.CollapseReciprocalRelationships,
	}, api.Logger(ctx))
	if err != nil {
		return nil, toAppError(err)
	}
	out := &MonicaFetchOutput{}
	out.Body.SessionID = in.Body.SessionID
	return out, nil
}

func (h *handlers) status(ctx context.Context, in *MonicaSessionQuery) (*MonicaStatusOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	s, err := h.uc.Status(actor, in.SessionID)
	if err != nil {
		return nil, toAppError(err)
	}
	out := &MonicaStatusOutput{Body: MonicaStatusResponse{SessionID: s.SessionID, Phase: s.Phase, PhaseDone: s.Done, PhaseTotal: s.Total, Error: s.Error}}
	if s.Result != nil {
		r := resultResponse(*s.Result)
		out.Body.Result = &r
	}
	return out, nil
}

func (h *handlers) preview(ctx context.Context, in *MonicaSessionQuery) (*MonicaPreviewOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.uc.Preview(actor, in.SessionID)
	if err != nil {
		return nil, toAppError(err)
	}
	out := &MonicaPreviewOutput{Body: MonicaPreviewResponse{
		SessionID: p.SessionID, TotalRows: p.TotalRows, ValidRows: p.ValidRows, DuplicateCount: p.DuplicateCount,
		ErrorCount: p.ErrorCount, Totals: relatedResponse(p.Totals), Rows: make([]MonicaRowPreview, len(p.Rows)),
	}}
	for i, r := range p.Rows {
		row := MonicaRowPreview{
			RowIndex: r.Index, ParsedContact: r.ParsedContact, ValidationErrors: nonNil(r.ValidationErrors), Warnings: nonNil(r.Warnings),
			SuggestedAction: r.SuggestedAction, Related: relatedResponse(r.Related), HasPhoto: r.HasPhoto,
		}
		if d := r.DuplicateMatch; d != nil {
			row.DuplicateMatch.Value = &MonicaDuplicateMatch{
				ExistingContactID: d.ExistingContactID, ExistingFirstname: d.ExistingFirstname, ExistingLastname: d.ExistingLastname,
				ExistingEmail: d.ExistingEmail, ExistingPhone: d.ExistingPhone, MatchReason: d.MatchReason, ExistingDeleted: d.ExistingDeleted,
			}
		}
		out.Body.Rows[i] = row
	}
	return out, nil
}

func (h *handlers) confirm(ctx context.Context, in *MonicaConfirmInput) (*MonicaConfirmOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	actions := make([]domain.Action, len(in.Body.Actions))
	for i, a := range in.Body.Actions {
		actions[i] = domain.Action{RowIndex: a.RowIndex, Action: a.Action}
	}
	r, err := h.uc.Confirm(ctx, actor, in.Body.SessionID, actions, api.Logger(ctx))
	if err != nil {
		return nil, toAppError(err)
	}
	return &MonicaConfirmOutput{Body: resultResponse(r)}, nil
}

func (h *handlers) cancel(ctx context.Context, in *MonicaSessionQuery) (*MonicaCancelOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.Cancel(actor, in.SessionID); err != nil {
		return nil, toAppError(err)
	}
	out := &MonicaCancelOutput{}
	out.Body.Status = "cancelled"
	return out, nil
}

func relatedResponse(c domain.RelatedCounts) MonicaRelatedCounts {
	return MonicaRelatedCounts{Activities: c.Activities, Notes: c.Notes, Reminders: c.Reminders, Relationships: c.Relationships, ExtraNotes: c.ExtraNotes}
}

func resultResponse(r domain.Result) MonicaImportResult {
	out := MonicaImportResult{
		TotalProcessed: r.TotalProcessed, Created: r.Created, Updated: r.Updated, Skipped: r.Skipped, Errors: nonNil(r.Errors),
		SkippedRows: make([]MonicaSkippedRow, len(r.SkippedRows)), ActivitiesCreated: r.ActivitiesCreated, NotesCreated: r.NotesCreated,
		RemindersCreated: r.RemindersCreated, RelationshipsCreated: r.RelationshipsCreated, RelationshipsCollapsed: r.RelationshipsCollapsed,
		PhotosQueued: r.PhotosQueued, PhotosSaved: r.PhotosSaved, PhotosFailed: r.PhotosFailed,
	}
	for i, s := range r.SkippedRows {
		out.SkippedRows[i] = MonicaSkippedRow{Row: s.Row, Name: s.Name, Reason: s.Reason, Detail: s.Detail}
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// toAppError passes on errors that carry their own status (they come from the wizard).
func toAppError(err error) *apperrors.AppError {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	if errors.Is(err, domain.ErrNoSession) {
		return apperrors.ErrMissingField("session_id")
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}
