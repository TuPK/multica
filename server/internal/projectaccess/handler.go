package projectaccess

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Handler serves the project-access routes. The routes sit behind the
// workspace member and human-actor middleware, like the other project routes.
type Handler struct {
	Queries *db.Queries
	// Scopes computes the viewer's Scope; tests replace it with a failing fake.
	Scopes  Querier
	Service *Service
}

// NewHandler returns a Handler over q whose transactions run on tx.
func NewHandler(q *db.Queries, tx TxStarter) *Handler {
	return &Handler{Queries: q, Scopes: q, Service: &Service{Queries: q, Tx: tx}}
}

const (
	codeProjectNotFound      = "project_not_found"
	codeProjectMemberInvalid = "project_member_invalid"
	codeScopeUnavailable     = "scope_unavailable"
	maxBody                  = 1 << 20
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

// writeProjectNotFound is the one answer for a project that does not exist and
// for one the caller may not see, so the two cannot be told apart.
func writeProjectNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, codeProjectNotFound, "project not found")
}

// request is what every route needs before its own work.
type request struct {
	actor       Actor
	workspaceID pgtype.UUID
	projectID   pgtype.UUID
	scope       Scope
	project     db.GetProjectAccessRow
}

// load resolves the caller, computes the Scope and loads the project. It
// writes the response and returns false when the request cannot go on.
func (h *Handler) load(w http.ResponseWriter, r *http.Request) (request, bool) {
	var req request
	userID, err := util.ParseUUID(r.Header.Get("X-User-ID"))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "user not authenticated")
		return req, false
	}
	workspaceID, err := util.ParseUUID(middleware.ResolveWorkspaceIDFromRequest(r, h.Queries))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_workspace", "workspace id is required")
		return req, false
	}
	scope, err := ForViewer(r.Context(), h.Scopes, workspaceID, userID)
	if errors.Is(err, ErrNotMember) {
		writeError(w, http.StatusNotFound, "workspace_not_found", "workspace not found")
		return req, false
	}
	if err != nil {
		slog.Error("projectaccess: compute scope", "error", err)
		writeError(w, http.StatusServiceUnavailable, codeScopeUnavailable, "visibility could not be determined")
		return req, false
	}
	projectID, err := util.ParseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeProjectNotFound(w)
		return req, false
	}
	project, err := h.Queries.GetProjectAccess(r.Context(), db.GetProjectAccessParams{ID: projectID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !scope.Allows(project.ID)) {
		writeProjectNotFound(w)
		return req, false
	}
	if err != nil {
		slog.Error("projectaccess: load project", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to load project")
		return req, false
	}
	user, err := h.Queries.GetUser(r.Context(), userID)
	if err != nil {
		slog.Error("projectaccess: load user", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to load user")
		return req, false
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	req = request{
		actor: Actor{
			UserID: userID, Email: user.Email,
			IP: ip, UserAgent: r.UserAgent(), RequestID: chimw.GetReqID(r.Context()),
		},
		workspaceID: workspaceID, projectID: projectID, scope: scope, project: project,
	}
	return req, true
}

// loadManager is load for the routes only owner and admin may call. The
// project is looked up first so a hidden project is still a 404.
func (h *Handler) loadManager(w http.ResponseWriter, r *http.Request) (request, bool) {
	req, ok := h.load(w, r)
	if !ok {
		return req, false
	}
	if !req.scope.CanManage() {
		writeError(w, http.StatusForbidden, "forbidden", "insufficient permissions")
		return req, false
	}
	return req, true
}

// parseUserIDs turns the ids of a request body into UUIDs, or writes the
// 400 that names the field.
func parseUserIDs(w http.ResponseWriter, ids []string, field string) ([]pgtype.UUID, bool) {
	out := make([]pgtype.UUID, 0, len(ids))
	for _, s := range ids {
		id, err := util.ParseUUID(s)
		if err != nil {
			writeMemberInvalid(w, field)
			return nil, false
		}
		out = append(out, id)
	}
	return out, true
}

func writeMemberInvalid(w http.ResponseWriter, field string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{
		"error": "every user must be a member of the workspace", "code": codeProjectMemberInvalid, "field": field,
	})
}

// fail answers an error from the Service.
func fail(w http.ResponseWriter, err error) {
	var invalid *MemberInvalidError
	switch {
	case errors.As(err, &invalid):
		writeMemberInvalid(w, invalid.Field)
	case errors.Is(err, ErrProjectNotFound):
		writeProjectNotFound(w)
	default:
		slog.Error("projectaccess: change failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to change project access")
	}
}

// PutAccess serves PUT /api/projects/{id}/access.
func (h *Handler) PutAccess(w http.ResponseWriter, r *http.Request) {
	req, ok := h.loadManager(w, r)
	if !ok {
		return
	}
	var body struct {
		Access    string   `json:"access"`
		MemberIDs []string `json:"member_ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if body.Access != AccessWorkspace && body.Access != AccessRestricted {
		writeError(w, http.StatusBadRequest, "invalid_request", "access must be workspace or restricted")
		return
	}
	ids, ok := parseUserIDs(w, body.MemberIDs, "member_ids")
	if !ok {
		return
	}
	res, err := h.Service.SwitchAccess(r.Context(), req.actor, req.workspaceID, req.projectID, body.Access, ids)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project_id": util.UUIDToString(req.projectID), "access": res.Access, "auto_added": res.AutoAdded})
}

// ListMembers serves GET /api/projects/{id}/members.
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	req, ok := h.load(w, r)
	if !ok {
		return
	}
	rows, err := h.Queries.ListProjectMembers(r.Context(), req.projectID)
	if err != nil {
		fail(w, err)
		return
	}
	type member struct {
		UserID    string  `json:"user_id"`
		Name      string  `json:"name"`
		AddedBy   *string `json:"added_by"`
		CreatedAt string  `json:"created_at"`
	}
	out := make([]member, 0, len(rows))
	for _, m := range rows {
		item := member{UserID: util.UUIDToString(m.UserID), Name: m.Name, CreatedAt: m.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05.000000Z")}
		if m.AddedBy.Valid {
			s := util.UUIDToString(m.AddedBy)
			item.AddedBy = &s
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out})
}

// AddMembers serves POST /api/projects/{id}/members.
func (h *Handler) AddMembers(w http.ResponseWriter, r *http.Request) {
	req, ok := h.loadManager(w, r)
	if !ok {
		return
	}
	var body struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	ids, ok := parseUserIDs(w, body.UserIDs, "user_ids")
	if !ok {
		return
	}
	added, err := h.Service.Grant(r.Context(), req.actor, req.workspaceID, req.projectID, ids)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": len(added)})
}

// RemoveMember serves DELETE /api/projects/{id}/members/{userId}.
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	req, ok := h.loadManager(w, r)
	if !ok {
		return
	}
	userID, err := util.ParseUUID(chi.URLParam(r, "userId"))
	if err != nil {
		writeMemberInvalid(w, "user_id")
		return
	}
	if err := h.Service.Revoke(r.Context(), req.actor, req.workspaceID, req.projectID, userID); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
