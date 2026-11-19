package subscription

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

// TenantWorkspaces reads what a tenant-wide limit override needs: whether the
// tenant exists and its workspaces.
type TenantWorkspaces interface {
	TenantExists(ctx context.Context, tenantID string) (bool, error)
	WorkspaceIDs(ctx context.Context, tenantID string) ([]string, error)
}

// PgxTenantWorkspaces is the TenantWorkspaces over Postgres.
type PgxTenantWorkspaces struct{ q *db.Queries }

// NewPgxTenantWorkspaces returns a TenantWorkspaces over pool.
func NewPgxTenantWorkspaces(pool db.DBTX) *PgxTenantWorkspaces {
	return &PgxTenantWorkspaces{q: db.New(pool)}
}

// TenantExists implements TenantWorkspaces.
func (p *PgxTenantWorkspaces) TenantExists(ctx context.Context, tenantID string) (bool, error) {
	var id pgtype.UUID
	if err := id.Scan(tenantID); err != nil || !id.Valid {
		return false, nil
	}
	return p.q.TenantExistsForLimits(ctx, id)
}

// WorkspaceIDs implements TenantWorkspaces.
func (p *PgxTenantWorkspaces) WorkspaceIDs(ctx context.Context, tenantID string) ([]string, error) {
	var id pgtype.UUID
	if err := id.Scan(tenantID); err != nil {
		return nil, err
	}
	rows, err := p.q.ListWorkspacesOfTenant(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = uuidText(r)
	}
	return out, nil
}

// postTenantLimitOverride sets one gate on the tenant's workspaces, or on the
// subset named in workspace_ids (M40, D97). The tenant must exist; a named
// workspace must belong to it.
func (c *Console) postTenantLimitOverride(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := trust.CanonicalUUID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var body struct {
		Gate         string   `json:"gate"`
		Value        *int64   `json:"value"`
		WorkspaceIDs []string `json:"workspace_ids"`
		Reason       string   `json:"reason"`
	}
	if perr := decodeBody(w, r, &body); perr != nil {
		c.fail(w, r, "decode limit override", perr)
		return
	}
	exists, err := c.Tenants.TenantExists(r.Context(), tenantID)
	if err != nil {
		c.fail(w, r, "check tenant", err)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	all, err := c.Tenants.WorkspaceIDs(r.Context(), tenantID)
	if err != nil {
		c.fail(w, r, "list tenant workspaces", err)
		return
	}
	targets := all
	if len(body.WorkspaceIDs) > 0 {
		member := map[string]bool{}
		for _, id := range all {
			member[id] = true
		}
		targets = nil
		for _, raw := range body.WorkspaceIDs {
			id, ok := trust.CanonicalUUID(raw)
			if !ok || !member[id] {
				writeFromError(w, &Error{Status: http.StatusBadRequest, Code: CodeWorkspaceNotInTenant, Message: "every workspace_id must belong to the tenant"})
				return
			}
			targets = append(targets, id)
		}
	}
	a, err := c.currentActor(r)
	if err != nil {
		c.fail(w, r, "resolve staff actor", err)
		return
	}
	err = c.Subscriptions.SetLimitOverride(r.Context(), tenantID, targets,
		LimitOverride{Gate: body.Gate, Value: body.Value}, body.Reason, OverrideActor{ID: a.ID, Label: a.Label})
	var serr *Error
	if errors.As(err, &serr) {
		writeFromError(w, serr)
		return
	}
	if err != nil {
		c.fail(w, r, "set limit override", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"gate": body.Gate, "workspace_ids": targets})
}
