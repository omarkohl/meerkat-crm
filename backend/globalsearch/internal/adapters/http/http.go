// Package httpadapter exposes the global search as a huma operation
// (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/globalsearch/internal/app"
	"meerkat/globalsearch/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

const (
	defaultLimit = 5
	maxLimit     = 50
)

func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "search", Method: http.MethodGet, Path: "/search",
		Summary: "Search contacts, notes and activities at once, best matches first", Tags: []string{"search"},
		Description: "Without type the answer has the three groups, each with its total and the first `limit` results. " +
			"With type only that group is answered, and offset pages through it. Trashed contacts and their notes are never found. " +
			"The matched text comes as segments, never as HTML.",
	}, h.search)
}

type handlers struct{ uc *app.UseCases }

type SearchInput struct {
	Q      string `query:"q" doc:"What to search for, at most 200 characters" maxLength:"200"`
	Type   string `query:"type" doc:"Only this group: contacts, notes or activities" enum:"contacts,notes,activities,"`
	Limit  string `query:"limit" doc:"Results per group, 1 to 50, default 5"`
	Offset string `query:"offset" doc:"Results to skip in the group named by type, default 0; needs type"`
}

type SegmentResponse struct {
	Text  string `json:"text"`
	Match bool   `json:"match" doc:"The searched text was found here"`
}

type ContactResult struct {
	ID           uint              `json:"id"`
	Firstname    string            `json:"firstname"`
	Lastname     string            `json:"lastname"`
	Nickname     string            `json:"nickname"`
	Name         []SegmentResponse `json:"name" doc:"The display name with the match marked"`
	MatchedField string            `json:"matched_field" doc:"Where the text was found: name, email or phone" enum:"name,email,phone,"`
	Snippet      []SegmentResponse `json:"snippet" doc:"The matching value when it is not the name"`
}

type NoteResult struct {
	ID          uint              `json:"id"`
	ContactID   *uint             `json:"contact_id" doc:"Null for a note without a contact"`
	ContactName string            `json:"contact_name"`
	Date        time.Time         `json:"date"`
	Snippet     []SegmentResponse `json:"snippet"`
}

type ActivityResult struct {
	ID      uint              `json:"id"`
	Title   []SegmentResponse `json:"title"`
	Date    time.Time         `json:"date"`
	Snippet []SegmentResponse `json:"snippet"`
}

type ContactGroup struct {
	Total int             `json:"total"`
	Items []ContactResult `json:"items"`
}

type NoteGroup struct {
	Total int          `json:"total"`
	Items []NoteResult `json:"items"`
}

type ActivityGroup struct {
	Total int              `json:"total"`
	Items []ActivityResult `json:"items"`
}

type SearchOutput struct {
	Body struct {
		Query      string         `json:"query"`
		Contacts   *ContactGroup  `json:"contacts,omitempty"`
		Notes      *NoteGroup     `json:"notes,omitempty"`
		Activities *ActivityGroup `json:"activities,omitempty"`
	}
}

func segments(in []domain.Segment) []SegmentResponse {
	out := make([]SegmentResponse, len(in))
	for i, s := range in {
		out[i] = SegmentResponse{Text: s.Text, Match: s.Match}
	}
	return out
}

func intParam(field, value string, def, min, max int) (int, error) {
	if value == "" {
		return def, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < min || n > max {
		return 0, apperrors.ErrInvalidInput(field, field+" must be a number from "+strconv.Itoa(min)+" to "+strconv.Itoa(max))
	}
	return n, nil
}

func (h *handlers) search(ctx context.Context, in *SearchInput) (*SearchOutput, error) {
	id, ok := api.UserID(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized("Authentication required")
	}
	limit, err := intParam("limit", in.Limit, defaultLimit, 1, maxLimit)
	if err != nil {
		return nil, err
	}
	offset, err := intParam("offset", in.Offset, 0, 0, 1_000_000)
	if err != nil {
		return nil, err
	}
	if offset > 0 && in.Type == "" {
		return nil, apperrors.ErrInvalidInput("offset", "offset needs type: only one group can be paged")
	}
	res, err := h.uc.Search(ctx, domain.UserID(id), app.Request{Term: in.Q, Kind: domain.Kind(in.Type), Limit: limit, Offset: offset})
	var failure *app.Failure
	switch {
	case errors.Is(err, app.ErrEmptyTerm):
		return nil, apperrors.ErrInvalidInput("q", "q must not be empty")
	case errors.Is(err, app.ErrUnknownKind):
		return nil, apperrors.ErrInvalidInput("type", "type must be contacts, notes or activities")
	case errors.As(err, &failure):
		return nil, apperrors.ErrDatabase("Search failed").WithError(err)
	case err != nil:
		return nil, err
	}
	out := &SearchOutput{}
	out.Body.Query = res.Term
	if g := res.Contacts; g != nil {
		group := &ContactGroup{Total: g.Total, Items: make([]ContactResult, len(g.Items))}
		for i, c := range g.Items {
			group.Items[i] = ContactResult{
				ID: c.ID, Firstname: c.Firstname, Lastname: c.Lastname, Nickname: c.Nickname,
				Name: segments(c.Name), MatchedField: c.Field, Snippet: segments(c.Snippet),
			}
		}
		out.Body.Contacts = group
	}
	if g := res.Notes; g != nil {
		group := &NoteGroup{Total: g.Total, Items: make([]NoteResult, len(g.Items))}
		for i, n := range g.Items {
			group.Items[i] = NoteResult{ID: n.ID, ContactID: n.ContactID, ContactName: n.ContactName, Date: n.Date, Snippet: segments(n.Snippet)}
		}
		out.Body.Notes = group
	}
	if g := res.Activities; g != nil {
		group := &ActivityGroup{Total: g.Total, Items: make([]ActivityResult, len(g.Items))}
		for i, a := range g.Items {
			group.Items[i] = ActivityResult{ID: a.ID, Title: segments(a.Title), Date: a.Date, Snippet: segments(a.Snippet)}
		}
		out.Body.Activities = group
	}
	return out, nil
}
