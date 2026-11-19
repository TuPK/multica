package entitlement

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// PgxStatus is the StatusReader over platform_workspace.
type PgxStatus struct{ q *db.Queries }

// NewPgxStatus returns a StatusReader over pool.
func NewPgxStatus(pool db.DBTX) *PgxStatus { return &PgxStatus{q: db.New(pool)} }

// Status implements StatusReader: a workspace with no row is active.
func (s *PgxStatus) Status(ctx context.Context, workspaceID string) (string, error) {
	var id pgtype.UUID
	if err := id.Scan(workspaceID); err != nil || !id.Valid {
		return "", fmt.Errorf("invalid workspace id %q", workspaceID)
	}
	row, err := s.q.GetPlatformWorkspace(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "active", nil
	}
	if err != nil {
		return "", err
	}
	return row.Status, nil
}

// NewPgxLoader returns the Loader that reads the platform's tables.
func NewPgxLoader(pool db.DBTX) Loader {
	return Loader{
		Plans:         plan.NewPgxReader(pool),
		Subscriptions: subscription.NewPgxReader(pool),
		Status:        NewPgxStatus(pool),
		Tenants:       NewPgxTenants(pool),
	}
}

// PgxTenants is the TenantReader over tenant_workspace.
type PgxTenants struct{ q *db.Queries }

// NewPgxTenants returns a TenantReader over pool.
func NewPgxTenants(pool db.DBTX) *PgxTenants { return &PgxTenants{q: db.New(pool)} }

// SiblingIDs implements TenantReader.
func (t *PgxTenants) SiblingIDs(ctx context.Context, workspaceID string) ([]string, error) {
	var id pgtype.UUID
	if err := id.Scan(workspaceID); err != nil || !id.Valid {
		return nil, fmt.Errorf("invalid workspace id %q", workspaceID)
	}
	rows, err := t.q.ListTenantWorkspaceIDs(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = fmt.Sprintf("%x-%x-%x-%x-%x", r.Bytes[0:4], r.Bytes[4:6], r.Bytes[6:8], r.Bytes[8:10], r.Bytes[10:16])
	}
	return out, nil
}
