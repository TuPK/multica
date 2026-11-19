package planlimit

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// TenantCounter is the query that counts the workspaces of a tenant.
type TenantCounter interface {
	CountTenantWorkspaceLinks(ctx context.Context, tenantID pgtype.UUID) (int64, error)
}

// CountTenantWorkspaces counts the workspaces of the tenant for the
// max_workspaces gate. The gate's lock is scoped by the tenant id: pass the tenant
// id to Admit where a workspace id goes for the other gates.
func CountTenantWorkspaces(q TenantCounter, tenantID pgtype.UUID) CountFunc {
	return func(ctx context.Context) (int64, error) { return q.CountTenantWorkspaceLinks(ctx, tenantID) }
}
