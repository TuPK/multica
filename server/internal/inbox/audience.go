package inbox

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/projectaccess"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// IssueAudience answers whether a member may be told about, or subscribed to,
// one issue (spec 7.1 rule 4). A background writer has no request and so no
// Scope: it computes the recipient's own. A failure to decide counts as "cannot
// see", so a notification is lost rather than leaked, and the caller never
// fails because of it.
type IssueAudience struct {
	ctx         context.Context
	queries     *db.Queries
	workspaceID pgtype.UUID
	issueID     pgtype.UUID
	projectID   pgtype.UUID
	loaded      bool
	verdicts    map[pgtype.UUID]bool
}

// NewIssueAudience loads the issue's project once. Use one audience per issue.
func NewIssueAudience(ctx context.Context, queries *db.Queries, workspaceID, issueID pgtype.UUID) *IssueAudience {
	a := &IssueAudience{ctx: ctx, queries: queries, workspaceID: workspaceID, issueID: issueID, verdicts: map[pgtype.UUID]bool{}}
	if !workspaceID.Valid || !issueID.Valid {
		return a
	}
	issue, err := queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{ID: issueID, WorkspaceID: workspaceID})
	if err != nil {
		slog.Warn("audience: load issue failed; recipients treated as not seeing it", "issue_id", issueID, "error", err)
		return a
	}
	a.projectID, a.loaded = issue.ProjectID, true
	return a
}

// Sees reports whether the member sees the issue.
func (a *IssueAudience) Sees(userID pgtype.UUID) bool {
	if !a.loaded || !userID.Valid {
		return false
	}
	if seen, ok := a.verdicts[userID]; ok {
		return seen
	}
	seen := false
	hidden, err := projectaccess.ForPrincipals(a.ctx, a.queries, a.workspaceID, userID)
	if err == nil {
		seen = hidden.CanSeeProject(a.projectID)
	} else {
		slog.Warn("audience: compute recipient scope failed; recipient treated as not seeing the issue",
			"issue_id", a.issueID, "user_id", userID, "error", err)
	}
	a.verdicts[userID] = seen
	return seen
}
