// Package workspaceprovision holds the database work that creates a workspace
// with its owner, built-in issue statuses and audit row. The caller owns the
// transaction and every side effect after commit.
package workspaceprovision

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Params describes the workspace to create. Record writes the audit entry in
// the caller's transaction; Create fills in the entry's workspace and target.
type Params struct {
	Name        string
	Slug        string
	Description pgtype.Text
	Context     pgtype.Text
	IssuePrefix string
	OwnerID     pgtype.UUID
	Record      func(audit.Entry) error
}

// StepError names the step of Create that failed: "create", "owner",
// "statuses" or "audit".
type StepError struct {
	Step string
	Err  error
}

func (e *StepError) Error() string { return "workspace provision " + e.Step + ": " + e.Err.Error() }

func (e *StepError) Unwrap() error { return e.Err }

// Create inserts the workspace, its owner member, the built-in issue statuses
// and the workspace.created audit row through qtx. It does not commit.
func Create(ctx context.Context, qtx *db.Queries, p Params) (db.Workspace, error) {
	ws, err := qtx.CreateWorkspace(ctx, db.CreateWorkspaceParams{
		Name:        p.Name,
		Slug:        p.Slug,
		Description: p.Description,
		Context:     p.Context,
		IssuePrefix: p.IssuePrefix,
	})
	if err != nil {
		return db.Workspace{}, &StepError{Step: "create", Err: err}
	}
	if _, err := qtx.CreateMember(ctx, db.CreateMemberParams{
		WorkspaceID: ws.ID,
		UserID:      p.OwnerID,
		Role:        "owner",
	}); err != nil {
		return db.Workspace{}, &StepError{Step: "owner", Err: err}
	}
	// Seed the built-in statuses in the same transaction, so a workspace is
	// never visible without its status catalog.
	if err := issuestatus.Ensure(ctx, qtx, ws.ID); err != nil {
		return db.Workspace{}, &StepError{Step: "statuses", Err: err}
	}
	id := uuidString(ws.ID)
	if err := p.Record(audit.Entry{
		WorkspaceID: id,
		Action:      audit.ActionWorkspaceCreated,
		TargetType:  "workspace",
		TargetID:    id,
		TargetLabel: ws.Name,
	}); err != nil {
		return db.Workspace{}, &StepError{Step: "audit", Err: err}
	}
	return ws, nil
}
