package billingapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
)

// PgxWorkspaces is the WorkspaceReader over the shared workspace table.
type PgxWorkspaces struct{ q *db.Queries }

// NewPgxWorkspaces returns a WorkspaceReader over pool.
func NewPgxWorkspaces(pool db.DBTX) *PgxWorkspaces { return &PgxWorkspaces{q: db.New(pool)} }

// Slug implements WorkspaceReader.
func (w *PgxWorkspaces) Slug(ctx context.Context, workspaceID string) (string, bool, error) {
	var id pgtype.UUID
	if err := id.Scan(workspaceID); err != nil || !id.Valid {
		return "", false, fmt.Errorf("invalid uuid %q", workspaceID)
	}
	slug, err := w.q.GetWorkspaceSlug(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return slug, err == nil, err
}
