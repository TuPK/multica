// Package tenant holds the rules of the workspace-to-tenant link that the
// Delegora server owns (spec R6 section 5): the lock that serialises changes to a
// tenant's workspaces, and the detach step of workspace deletion.
package tenant

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Gate is the plan gate whose advisory lock serialises every change to the set
// of workspaces of one tenant, whether or not the gate is enforced.
const Gate = "max_workspaces"

// Queries is the part of *db.Queries this package uses.
type Queries interface {
	LockPlanGate(ctx context.Context, arg db.LockPlanGateParams) error
	GetTenantIDByWorkspace(ctx context.Context, workspaceID pgtype.UUID) (pgtype.UUID, error)
	DeleteTenantWorkspaceLink(ctx context.Context, workspaceID pgtype.UUID) error
	CountTenantWorkspaceLinks(ctx context.Context, tenantID pgtype.UUID) (int64, error)
	DeleteTenant(ctx context.Context, id pgtype.UUID) error
	DeleteTenantPolicy(ctx context.Context, tenantID pgtype.UUID) error
}

// Lock takes the transaction-scoped advisory lock of the tenant.
func Lock(ctx context.Context, q Queries, tenantID pgtype.UUID) error {
	return q.LockPlanGate(ctx, db.LockPlanGateParams{Scope: util.UUIDToString(tenantID), Gate: Gate})
}

// Detach removes a workspace from its tenant, and the tenant when no workspace
// is left. It runs in the transaction that deletes the workspace, and takes the
// tenant lock first so it queues behind a creation that is counting the
// tenant's workspaces. A workspace with no link has nothing to detach.
func Detach(ctx context.Context, q Queries, workspaceID pgtype.UUID) error {
	tenantID, err := q.GetTenantIDByWorkspace(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := Lock(ctx, q, tenantID); err != nil {
		return err
	}
	if err := q.DeleteTenantWorkspaceLink(ctx, workspaceID); err != nil {
		return err
	}
	left, err := q.CountTenantWorkspaceLinks(ctx, tenantID)
	if err != nil {
		return err
	}
	if left > 0 {
		return nil
	}
	if err := q.DeleteTenantPolicy(ctx, tenantID); err != nil {
		return err
	}
	return q.DeleteTenant(ctx, tenantID)
}
