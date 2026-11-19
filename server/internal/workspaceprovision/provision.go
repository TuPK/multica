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
	// TenantID, when valid, is the tenant the workspace joins; the caller has
	// checked that the owner may add to it and that its max_workspaces gate
	// admits one more. When not valid, Create makes a tenant of the workspace's
	// own, keyed by the workspace id (spec R6 section 5.9).
	TenantID pgtype.UUID
	Record   func(audit.Entry) error
}

// StepError names the step of Create that failed: "create", "owner",
// "statuses", "tenant" or "audit".
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
	// The workspace's tenant: the one it joins, or a new one keyed by its own id.
	tenantID := p.TenantID
	if !tenantID.Valid {
		if err := qtx.CreateTenant(ctx, db.CreateTenantParams{ID: ws.ID, Name: ws.Name, CreatedBy: p.OwnerID}); err != nil {
			return db.Workspace{}, &StepError{Step: "tenant", Err: err}
		}
		tenantID = ws.ID
	}
	if err := qtx.LinkWorkspaceToTenant(ctx, db.LinkWorkspaceToTenantParams{WorkspaceID: ws.ID, TenantID: tenantID}); err != nil {
		return db.Workspace{}, &StepError{Step: "tenant", Err: err}
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
