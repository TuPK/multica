package quotaalert

import (
	"context"
	"fmt"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/entitlement"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/usage"
)

// PgxSource reads the workspaces, counts and limits from the database. Counts
// come from usage.Current and limits from the entitlement policy, the same
// numbers the console's resource page shows.
type PgxSource struct {
	pool db.DBTX
}

// NewPgxSource builds the source over pool.
func NewPgxSource(pool db.DBTX) PgxSource { return PgxSource{pool: pool} }

// Workspaces lists the active workspaces. A workspace with no platform row is
// active.
func (s PgxSource) Workspaces(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT w.id::text FROM workspace w
		LEFT JOIN platform_workspace pw ON pw.workspace_id = w.id
		WHERE COALESCE(pw.status, 'active') = 'active'
		ORDER BY w.id`)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Counts returns the current count of each alerted gate of a workspace, and the
// autopilot runs of the period that holds now.
func (s PgxSource) Counts(ctx context.Context, workspaceID string, now time.Time) (map[string]int64, error) {
	reader := usage.NewReader(s.pool)
	cur, err := reader.Current(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("count workspace: %w", err)
	}
	runs, err := reader.AutopilotRuns(ctx, workspaceID, now)
	if err != nil {
		return nil, fmt.Errorf("count autopilot runs: %w", err)
	}
	return map[string]int64{
		plan.GateAgents:        cur.Agents,
		plan.GateRuntimes:      cur.Runtimes,
		plan.GateIssueCount:    cur.IssuesTotal,
		plan.GateStorageBytes:  cur.StorageBytes,
		plan.GateAutopilotRuns: runs,
	}, nil
}

// Limits returns the effective limit of each gate of a workspace at now.
func (s PgxSource) Limits(ctx context.Context, workspaceID string, now time.Time) (map[string]*int64, error) {
	in, err := entitlement.NewPgxLoader(s.pool).Inputs(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load entitlement: %w", err)
	}
	gates := entitlement.Build(in, now).Gates
	out := make(map[string]*int64, len(gates))
	for name, g := range gates {
		out[name] = g.Limit
	}
	return out, nil
}
