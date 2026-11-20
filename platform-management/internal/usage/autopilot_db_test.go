package usage

import (
	"context"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// TestAutopilotRunsCountsOnlyThePeriodThatHoldsNow: the runs of a workspace are
// the used count of the period that holds now, not of an earlier one. A
// workspace with no period row has used none. Skips without the test database.
func TestAutopilotRunsCountsOnlyThePeriodThatHoldsNow(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	var ws string
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM autopilot_quota_period WHERE workspace_id = $1::uuid`, ws)
	})
	for _, p := range []struct {
		start, end time.Time
		used       int
	}{
		{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), 9},
		{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), 4},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO autopilot_quota_period (workspace_id, period_start, period_end, used_count)
			VALUES ($1::uuid, $2, $3, $4)`, ws, p.start, p.end, p.used); err != nil {
			t.Fatal(err)
		}
	}

	r := NewReader(pool)
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	got, err := r.AutopilotRuns(ctx, ws, now)
	if err != nil {
		t.Fatal(err)
	}
	if got != 4 {
		t.Fatalf("AutopilotRuns in November = %d, want 4 (the November period)", got)
	}

	other, err := r.AutopilotRuns(ctx, "00000000-0000-0000-0000-000000000000", now)
	if err != nil {
		t.Fatal(err)
	}
	if other != 0 {
		t.Fatalf("AutopilotRuns of a workspace with no period = %d, want 0", other)
	}
}

// TestFlowsReturnTheWorkspaceDaysSinceOldestFirst: the daily rows of one workspace
// from the given day on, oldest first; other workspaces and older days are left
// out. Skips without the test database.
func TestFlowsReturnTheWorkspaceDaysSinceOldestFirst(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	var ws, other string
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text, gen_random_uuid()::text`).Scan(&ws, &other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM platform_usage_daily WHERE workspace_id = ANY($1::uuid[])`, []string{ws, other})
	})
	rows := []struct {
		ws         string
		day        time.Time
		issues, tk int
	}{
		{ws, time.Date(2030, 5, 1, 0, 0, 0, 0, time.UTC), 9, 1},
		{ws, time.Date(2030, 5, 5, 0, 0, 0, 0, time.UTC), 3, 2},
		{ws, time.Date(2030, 5, 6, 0, 0, 0, 0, time.UTC), 0, 4},
		{other, time.Date(2030, 5, 6, 0, 0, 0, 0, time.UTC), 7, 100},
	}
	for _, r := range rows {
		if _, err := pool.Exec(ctx, `INSERT INTO platform_usage_daily (workspace_id, day, issues_created, tasks_run)
			VALUES ($1::uuid, $2, $3, $4)`, r.ws, r.day, r.issues, r.tk); err != nil {
			t.Fatal(err)
		}
	}

	got, err := NewReader(pool).Flows(ctx, ws, time.Date(2030, 5, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := []DayFlow{
		{Day: time.Date(2030, 5, 5, 0, 0, 0, 0, time.UTC), IssuesCreated: 3, TasksRun: 2},
		{Day: time.Date(2030, 5, 6, 0, 0, 0, 0, time.UTC), IssuesCreated: 0, TasksRun: 4},
	}
	if len(got) != len(want) {
		t.Fatalf("Flows = %+v, want %+v", got, want)
	}
	for i := range want {
		if !got[i].Day.Equal(want[i].Day) || got[i].IssuesCreated != want[i].IssuesCreated || got[i].TasksRun != want[i].TasksRun {
			t.Fatalf("Flows[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
