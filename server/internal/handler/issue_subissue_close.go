package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// openSubIssuesBlockingClose lists the sub-issues that keep an agent from
// moving parent to target. An agent may not close a parent while its
// sub-issues are open: the stage barrier wakes the parent's assignee only
// while the parent is open, so a closed parent would leave its open
// sub-issues with nobody to advance them. Nothing blocks when the parent is
// already closed, when the target is not a closed status, or when every
// sub-issue is closed. A member's close is their own decision and is not
// checked here; callers decide that by the actor type.
func (h *Handler) openSubIssuesBlockingClose(ctx context.Context, parent db.Issue, target string) ([]string, error) {
	if !isClosedStatus(ctx, h, parent.WorkspaceID, target) || isClosedStatus(ctx, h, parent.WorkspaceID, parent.Status) {
		return nil, nil
	}
	children, err := h.Queries.ListChildIssues(ctx, parent.ID)
	if err != nil {
		return nil, err
	}
	var open []string
	for _, child := range children {
		if !isClosedStatus(ctx, h, child.WorkspaceID, child.Status) {
			open = append(open, h.issueIdentifier(ctx, child))
		}
	}
	return open, nil
}

// writeSubIssuesOpen answers a close refused because sub-issues are open.
func (h *Handler) writeSubIssuesOpen(ctx context.Context, w http.ResponseWriter, parent db.Issue, open []string) {
	writeJSON(w, http.StatusConflict, map[string]any{
		"error": fmt.Sprintf("%s has %d open sub-issue(s): %s. Finish or cancel them before closing it.",
			h.issueIdentifier(ctx, parent), len(open), strings.Join(open, ", ")),
		"code":            "sub_issues_open",
		"open_sub_issues": open,
	})
}

// isClosedStatus reports whether a status is in a terminal lifecycle category.
func isClosedStatus(ctx context.Context, h *Handler, workspaceID pgtype.UUID, status string) bool {
	category := issuestatus.Category(ctx, h.Queries, workspaceID, status)
	return category == issuestatus.CategoryDone || category == issuestatus.CategoryClosed
}

// issueIdentifier renders an issue as its workspace key, e.g. TUPK-31.
func (h *Handler) issueIdentifier(ctx context.Context, issue db.Issue) string {
	return h.getIssuePrefix(ctx, issue.WorkspaceID) + "-" + strconv.Itoa(int(issue.Number))
}
