package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/projectaccess"
)

// memberSeesProject reports whether userID sees projectID. A user who is not a
// member of the workspace sees no private project.
func memberSeesProject(ctx context.Context, q projectaccess.Querier, workspaceID, userID, projectID pgtype.UUID) (bool, error) {
	hidden, err := projectaccess.ForPrincipals(ctx, q, workspaceID, userID)
	if err != nil {
		return false, err
	}
	return hidden.CanSeeProject(projectID), nil
}

// assigneeSeesProject enforces that a member assigned to an issue of a project
// must see that project. It writes the 400 or 503 and returns false when the
// assignment must not go on.
func (h *Handler) assigneeSeesProject(w http.ResponseWriter, r *http.Request, workspaceID pgtype.UUID, assigneeType pgtype.Text, assigneeID, projectID pgtype.UUID) bool {
	if assigneeType.String != "member" || !assigneeID.Valid || !projectID.Valid {
		return true
	}
	sees, err := memberSeesProject(r.Context(), h.Queries, workspaceID, assigneeID, projectID)
	if err != nil {
		slog.Error("compute assignee project scope", append(logger.RequestAttrs(r), "error", err)...)
		writeErrorCode(w, http.StatusServiceUnavailable, "scope_unavailable", "visibility could not be determined")
		return false
	}
	if !sees {
		writeErrorCode(w, http.StatusBadRequest, "project_member_invalid", "project_member_invalid")
		return false
	}
	return true
}

// assigneeCanSeeProject is assigneeSeesProject for a batch, which skips the
// item instead of answering the request. An undecidable scope fails closed.
func (h *Handler) assigneeCanSeeProject(r *http.Request, workspaceID pgtype.UUID, assigneeType pgtype.Text, assigneeID, projectID pgtype.UUID) bool {
	if assigneeType.String != "member" || !assigneeID.Valid || !projectID.Valid {
		return true
	}
	sees, err := memberSeesProject(r.Context(), h.Queries, workspaceID, assigneeID, projectID)
	return err == nil && sees
}
