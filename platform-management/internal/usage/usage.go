// Package usage reads the usage numbers the console shows (platform spec 5.13):
// the newest row of platform_usage_daily for each workspace, their totals for
// the overview, and the current counts of one workspace for its detail page.
// Multica's nightly job writes the table; this package only reads it.
package usage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
)

// TotalsWindow is how far back the overview looks for a workspace's newest row.
// A workspace whose newest row is older counts as having none.
const TotalsWindow = 7 * 24 * time.Hour

const dayLayout = "2006-01-02"

// Snapshot is the newest usage row of one workspace. The zero value is a
// workspace with no row: a null day and zeros. The JSON names are the console
// API's.
type Snapshot struct {
	Day           *string `json:"usage_day"`
	Members       int64   `json:"usage_members"`
	Agents        int64   `json:"usage_agents"`
	Runtimes      int64   `json:"usage_runtimes"`
	IssuesTotal   int64   `json:"usage_issues_total"`
	IssuesCreated int64   `json:"usage_issues_created"`
	TasksRun      int64   `json:"usage_tasks_run"`
	StorageBytes  int64   `json:"usage_storage_bytes"`
}

// Current is the live count of one workspace, computed when the detail page is
// asked for. It is numbers only.
type Current struct {
	Members      int64 `json:"members"`
	Agents       int64 `json:"agents"`
	Runtimes     int64 `json:"runtimes"`
	IssuesTotal  int64 `json:"issues_total"`
	StorageBytes int64 `json:"storage_bytes"`
}

// Totals is the sum of the newest rows of every workspace that has one.
type Totals struct {
	// Workspaces is how many workspaces have a row in the window.
	Workspaces int64
	Snapshot
}

// Map returns the totals under the overview's JSON names.
func (t Totals) Map() map[string]any {
	return map[string]any{
		"usage_workspaces":     t.Workspaces,
		"usage_day":            t.Day,
		"usage_members":        t.Members,
		"usage_agents":         t.Agents,
		"usage_runtimes":       t.Runtimes,
		"usage_issues_total":   t.IssuesTotal,
		"usage_issues_created": t.IssuesCreated,
		"usage_tasks_run":      t.TasksRun,
		"usage_storage_bytes":  t.StorageBytes,
	}
}

func dayText(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.UTC().Format(dayLayout)
	return &s
}

func uuidText(u pgtype.UUID) string {
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// snapshotsByWorkspace keys the newest rows by workspace id. A workspace with no
// row is not in the map.
func snapshotsByWorkspace(rows []db.ListLatestUsageDailyRow) map[string]Snapshot {
	out := make(map[string]Snapshot, len(rows))
	for _, r := range rows {
		out[uuidText(r.WorkspaceID)] = Snapshot{
			Day: dayText(r.Day), Members: int64(r.Members), Agents: int64(r.Agents), Runtimes: int64(r.Runtimes),
			IssuesTotal: int64(r.IssuesTotal), IssuesCreated: int64(r.IssuesCreated), TasksRun: int64(r.TasksRun),
			StorageBytes: r.StorageBytes,
		}
	}
	return out
}

// Reader reads usage over a Postgres pool.
type Reader struct {
	q    *db.Queries
	pool db.DBTX
}

// NewReader returns a Reader over pool.
func NewReader(pool db.DBTX) *Reader { return &Reader{q: db.New(pool), pool: pool} }

// DayFlow is one day's issues created and tasks run of one workspace. They are
// flows: the day's count, not a running total.
type DayFlow struct {
	Day           time.Time
	IssuesCreated int64
	TasksRun      int64
}

// Flows returns the daily rows of the workspace from since on, oldest first.
// A day with no row is absent.
func (r *Reader) Flows(ctx context.Context, workspaceID string, since time.Time) ([]DayFlow, error) {
	var id pgtype.UUID
	if err := id.Scan(workspaceID); err != nil || !id.Valid {
		return nil, fmt.Errorf("invalid workspace id %q", workspaceID)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT day, issues_created, tasks_run FROM platform_usage_daily
		WHERE workspace_id = $1 AND day >= $2::date
		ORDER BY day`, id, since.UTC().Format(dayLayout))
	if err != nil {
		return nil, fmt.Errorf("read daily flows: %w", err)
	}
	defer rows.Close()
	var out []DayFlow
	for rows.Next() {
		var d pgtype.Date
		var f DayFlow
		if err := rows.Scan(&d, &f.IssuesCreated, &f.TasksRun); err != nil {
			return nil, fmt.Errorf("scan daily flow: %w", err)
		}
		f.Day = d.Time
		out = append(out, f)
	}
	return out, rows.Err()
}

// AutopilotRuns returns the autopilot runs the workspace has used in the period
// that holds now (spec R6 section 3b.4). A workspace with no period row has used
// none. Periods are UTC months and carry no foreign key, so a missing row is zero.
func (r *Reader) AutopilotRuns(ctx context.Context, workspaceID string, now time.Time) (int64, error) {
	var id pgtype.UUID
	if err := id.Scan(workspaceID); err != nil || !id.Valid {
		return 0, fmt.Errorf("invalid workspace id %q", workspaceID)
	}
	var used int64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(used_count), 0)::bigint FROM autopilot_quota_period
		WHERE workspace_id = $1 AND period_start <= $2 AND period_end > $2`,
		id, now.UTC()).Scan(&used)
	if err != nil {
		return 0, fmt.Errorf("read autopilot runs: %w", err)
	}
	return used, nil
}

// Latest returns the newest row of each listed workspace. A workspace with no
// row is absent from the map; its zero Snapshot is the "no usage" value.
func (r *Reader) Latest(ctx context.Context, workspaceIDs []string) (map[string]Snapshot, error) {
	if len(workspaceIDs) == 0 {
		return map[string]Snapshot{}, nil
	}
	ids := make([]pgtype.UUID, len(workspaceIDs))
	for i, s := range workspaceIDs {
		if err := ids[i].Scan(s); err != nil || !ids[i].Valid {
			return nil, fmt.Errorf("invalid workspace id %q", s)
		}
	}
	rows, err := r.q.ListLatestUsageDaily(ctx, ids)
	if err != nil {
		return nil, err
	}
	return snapshotsByWorkspace(rows), nil
}

// Totals sums the newest row of each workspace that has one since now less
// TotalsWindow.
func (r *Reader) Totals(ctx context.Context, now time.Time) (Totals, error) {
	since := pgtype.Date{Time: now.UTC().Add(-TotalsWindow), Valid: true}
	row, err := r.q.SumLatestUsageDaily(ctx, since)
	if err != nil {
		return Totals{}, err
	}
	return Totals{
		Workspaces: row.Workspaces,
		Snapshot: Snapshot{
			Day: dayText(row.Day), Members: row.Members, Agents: row.Agents, Runtimes: row.Runtimes,
			IssuesTotal: row.IssuesTotal, IssuesCreated: row.IssuesCreated, TasksRun: row.TasksRun,
			StorageBytes: row.StorageBytes,
		},
	}, nil
}

// Current counts one workspace now.
func (r *Reader) Current(ctx context.Context, workspaceID string) (Current, error) {
	var id pgtype.UUID
	if err := id.Scan(workspaceID); err != nil || !id.Valid {
		return Current{}, fmt.Errorf("invalid workspace id %q", workspaceID)
	}
	row, err := r.q.GetCurrentWorkspaceCounts(ctx, id)
	if err != nil {
		return Current{}, err
	}
	return Current{
		Members: row.Members, Agents: row.Agents, Runtimes: row.Runtimes,
		IssuesTotal: row.IssuesTotal, StorageBytes: row.StorageBytes,
	}, nil
}
