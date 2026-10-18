// Package httpadapter exposes the admin use cases as huma operations. It maps HTTP to the use
// cases and their results and errors back to the responses the API always sent
// (docs/adr/0012-module-layout.md). The backup download sends a file, not JSON, so it is a
// plain Gin handler. Every route is mounted on a group behind the admin middleware, with its full /admin path.
package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"meerkat/admin/internal/app"
	"meerkat/admin/internal/domain"
	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/logger"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

func Register(a huma.API, admin gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	backups, audit, system := []string{"backups"}, []string{"audit-log"}, []string{"system-status"}
	api.RegisterOn(a, admin, huma.Operation{
		OperationID: "get-system-status", Method: http.MethodGet, Path: "/admin/system-status",
		Summary: "Operational warnings for administrators", Tags: system,
	}, h.status)
	api.RegisterOn(a, admin, huma.Operation{
		OperationID: "list-audit-log", Method: http.MethodGet, Path: "/admin/audit-log",
		Summary: "Security events, newest first", Tags: audit,
	}, h.auditLog)
	api.RegisterOn(a, admin, huma.Operation{
		OperationID: "list-backups", Method: http.MethodGet, Path: "/admin/backups",
		Summary: "The server backups, newest first, with the schedule settings", Tags: backups,
	}, h.listBackups)
	api.RegisterOn(a, admin, huma.Operation{
		OperationID: "create-backup", Method: http.MethodPost, Path: "/admin/backups", DefaultStatus: http.StatusCreated,
		Summary: "Make a backup now and prune old ones down to the retention count", Tags: backups,
	}, h.createBackup)
	api.RegisterOn(a, admin, huma.Operation{
		OperationID: "delete-backup", Method: http.MethodDelete, Path: "/admin/backups/{name}",
		Summary: "Delete one backup", Tags: backups,
	}, h.deleteBackup)
	if admin != nil { // cmd/openapi describes operations only; the download is not one
		admin.GET("/admin/backups/:name", h.download)
	}
}

type handlers struct{ uc *app.UseCases }

type SystemStatusOutput struct {
	Body struct {
		PrivateURLsAllowed bool     `json:"private_urls_allowed"`
		UserCount          int64    `json:"user_count"`
		PrivateURLWarning  bool     `json:"private_url_warning" doc:"Outbound features may reach private addresses on an instance with more than one user"`
		Features           []string `json:"features" doc:"The outbound features that still allow private addresses"`
		Setting            string   `json:"setting" doc:"The setting that fixes the warning"`
	}
}

type AuditEntryResponse struct {
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UserID    *uint     `json:"user_id" nullable:"true"`
	Username  string    `json:"username"`
	Event     string    `json:"event"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	Detail    string    `json:"detail" doc:"Machine-readable ;-separated key:value segments"`
}

// AuditLogInput values are strings on purpose: a page or limit that does not parse falls
// back to its default, as before.
type AuditLogInput struct {
	Page     string `query:"page" doc:"Page number, from 1"`
	Limit    string `query:"limit" doc:"Page size, 1 to 100, default 25"`
	Event    string `query:"event" doc:"Only this event, exact match"`
	Username string `query:"username" doc:"Only usernames containing this, case-insensitive"`
}

type AuditLogOutput struct {
	Body struct {
		Entries       []AuditEntryResponse `json:"entries"`
		Total         int64                `json:"total"`
		Page          int                  `json:"page"`
		Limit         int                  `json:"limit"`
		Events        []string             `json:"events" doc:"Every event, in the order the UI shows them"`
		RetentionDays int                  `json:"retention_days" doc:"Entries older than this are purged daily; 0 keeps them"`
	}
}

type BackupResponse struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

type BackupListOutput struct {
	Body struct {
		Backups        []BackupResponse `json:"backups"`
		Enabled        bool             `json:"enabled" doc:"Whether the scheduled backup runs"`
		Time           string           `json:"time" doc:"Time of day of the scheduled backup"`
		RetentionCount int              `json:"retention_count"`
		Directory      string           `json:"directory" doc:"Where the backups are stored on the server"`
	}
}

type BackupCreatedOutput struct{ Body BackupResponse }

type BackupPath struct {
	Name string `path:"name" doc:"The backup's file name"`
}

type MessageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func actorOf(ctx context.Context) (domain.UserID, error) {
	id, ok := api.UserID(ctx)
	if !ok {
		return 0, apperrors.ErrUnauthorized("Authentication required")
	}
	return domain.UserID(id), nil
}

func (h *handlers) status(ctx context.Context, _ *struct{}) (*SystemStatusOutput, error) {
	s, err := h.uc.SystemStatus(ctx)
	if err != nil {
		return nil, toAppError(err)
	}
	out := &SystemStatusOutput{}
	out.Body.PrivateURLsAllowed, out.Body.UserCount, out.Body.PrivateURLWarning = s.PrivateURLsAllowed, s.UserCount, s.PrivateURLWarning
	out.Body.Features, out.Body.Setting = s.Features, "BLOCK_PRIVATE_URLS"
	return out, nil
}

func (h *handlers) auditLog(ctx context.Context, in *AuditLogInput) (*AuditLogOutput, error) {
	p, err := h.uc.ListAuditLog(ctx, domain.AuditFilter{Event: in.Event, Username: in.Username, Page: atoiOr0(in.Page), Limit: atoiOr0(in.Limit)})
	if err != nil {
		return nil, toAppError(err)
	}
	out := &AuditLogOutput{}
	out.Body.Entries = make([]AuditEntryResponse, len(p.Entries))
	for i, e := range p.Entries {
		var user *uint
		if e.UserID != nil {
			id := uint(*e.UserID)
			user = &id
		}
		out.Body.Entries[i] = AuditEntryResponse{ID: e.ID, CreatedAt: e.CreatedAt, UserID: user, Username: e.Username,
			Event: e.Event, IP: e.IP, UserAgent: e.UserAgent, Detail: e.Detail}
	}
	out.Body.Total, out.Body.Page, out.Body.Limit, out.Body.Events, out.Body.RetentionDays = p.Total, p.Page, p.Limit, p.Events, p.RetentionDays
	return out, nil
}

func (h *handlers) listBackups(ctx context.Context, _ *struct{}) (*BackupListOutput, error) {
	l, err := h.uc.ListBackups(ctx)
	if err != nil {
		return nil, toAppError(err)
	}
	out := &BackupListOutput{}
	out.Body.Backups = make([]BackupResponse, len(l.Backups))
	for i, b := range l.Backups {
		out.Body.Backups[i] = backupResponse(b)
	}
	out.Body.Enabled, out.Body.Time, out.Body.RetentionCount, out.Body.Directory = l.Settings.Enabled, l.Settings.Time, l.Settings.RetentionCount, l.Settings.Directory
	return out, nil
}

func (h *handlers) createBackup(ctx context.Context, _ *struct{}) (*BackupCreatedOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	api.LiftWriteDeadline(ctx)
	log := api.Logger(ctx)
	b, err := h.uc.CreateBackup(ctx, actor, api.Now(ctx), domain.Client{IP: api.ClientIP(ctx), UserAgent: api.UserAgent(ctx)})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create backup")
		return nil, toAppError(err)
	}
	if b.PruneErr != nil {
		log.Warn().Err(b.PruneErr).Msg("Failed to prune old backups")
	}
	log.Info().Str("backup", b.Name).Int64("bytes", b.Size).Msg("Backup created")
	return &BackupCreatedOutput{Body: backupResponse(b.Backup)}, nil
}

func (h *handlers) deleteBackup(ctx context.Context, in *BackupPath) (*MessageOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.DeleteBackup(ctx, actor, in.Name, domain.Client{IP: api.ClientIP(ctx), UserAgent: api.UserAgent(ctx)}); err != nil {
		return nil, toAppError(err)
	}
	api.Logger(ctx).Info().Str("backup", in.Name).Msg("Backup deleted")
	out := &MessageOutput{}
	out.Body.Message = "Backup deleted"
	return out, nil
}

func (h *handlers) download(c *gin.Context) {
	v, _ := c.Get("userID")
	actor, ok := v.(uint)
	if !ok {
		apperrors.AbortWithError(c, apperrors.ErrUnauthorized("Authentication required"))
		return
	}
	name := c.Param("name")
	path, err := h.uc.OpenBackup(c.Request.Context(), domain.UserID(actor), name, domain.Client{IP: c.ClientIP(), UserAgent: c.Request.UserAgent()})
	if err != nil {
		logger.FromContext(c).Debug().Err(err).Msg("Backup download refused")
		apperrors.AbortWithError(c, toAppError(err))
		return
	}
	api.LiftWriteDeadlineOn(c)
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.FileAttachment(path, name)
}

func backupResponse(b domain.Backup) BackupResponse {
	return BackupResponse{Name: b.Name, Size: b.Size, CreatedAt: b.CreatedAt}
}

func atoiOr0(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// toAppError words a use-case error the way the Gin handlers did.
func toAppError(err error) *apperrors.AppError {
	if errors.Is(err, domain.ErrBackupNotFound) {
		return apperrors.ErrNotFound("Backup")
	}
	var f *app.Failure
	if errors.As(err, &f) {
		switch f.Stage {
		case app.StageListBackups:
			return apperrors.ErrInternal("Failed to list backups").WithError(f.Err)
		case app.StageCreateBackup:
			return apperrors.ErrInternal("Failed to create backup").WithError(f.Err)
		case app.StageDeleteBackup:
			return apperrors.ErrInternal("Failed to delete backup").WithError(f.Err)
		case app.StageAuditCount:
			return apperrors.ErrDatabase("count").WithError(f.Err)
		case app.StageAuditQuery:
			return apperrors.ErrDatabase("query").WithError(f.Err)
		case app.StageCountUsers:
			return apperrors.ErrDatabase("count users").WithError(f.Err)
		}
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}
