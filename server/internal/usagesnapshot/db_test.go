package usagesnapshot

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Database tests. They skip unless DATABASE_URL names a database that ends in
// _test. They share that database with the other packages' tests, so they
// assert on their own fixture rows only; the all-workspaces scan is limited to
// the fixture workspace.
func openDB(t *testing.T) (*pgxpool.Pool, *testutil.Fixture) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("explicit disposable DATABASE_URL required")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.Database, "_test") {
		t.Skip("DATABASE_URL must name a database ending in _test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, testutil.New(pool, "", "")
}

type seeded struct {
	workspaceID string
	day         time.Time
}

// seed builds one workspace with known numbers for 2026-11-14 (UTC):
//   - members: 2 (a third joins on the 15th)
//   - agents: 1 (one is archived during the 14th, one is created on the 15th)
//   - runtimes: 2
//   - issues_total 3 and issues_created 2 (one issue is older, one is from the 15th)
//   - tasks_run: 2 (a task completed at 00:00 on the 15th and a failed task do not count)
//   - storage_bytes: 1500 (an attachment from the 15th does not count)
func seed(t *testing.T, pool *pgxpool.Pool) seeded {
	t.Helper()
	f := testutil.New(pool, "", "")
	owner := f.User(t, "Owner", testutil.UniqueEmail("usage-owner"))
	ws := f.Workspace(t, "Usage", "usage-"+uuid.NewString()[:8])
	f = testutil.New(pool, ws, owner)

	day := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	in := func(h int) time.Time { return day.Add(time.Duration(h) * time.Hour) }
	next := day.AddDate(0, 0, 1)

	f.Member(t, ws, owner, "owner", testutil.Cols{"created_at": in(1)})
	m2 := f.User(t, "M2", testutil.UniqueEmail("usage-m2"))
	f.Member(t, ws, m2, "member", testutil.Cols{"created_at": in(2)})
	m3 := f.User(t, "M3", testutil.UniqueEmail("usage-m3"))
	f.Member(t, ws, m3, "member", testutil.Cols{"created_at": next.Add(time.Hour)}) // joins after the day

	rt1 := f.Runtime(t, "rt-1", testutil.Cols{"created_at": in(1)})
	f.Runtime(t, "rt-2", testutil.Cols{"created_at": in(3)})

	a1 := f.Agent(t, "live", rt1, testutil.Cols{"created_at": in(1)})
	f.Agent(t, "archived-in-day", rt1, testutil.Cols{"created_at": in(1), "archived_at": in(12)})
	f.Agent(t, "created-after", rt1, testutil.Cols{"created_at": next.Add(time.Hour)})

	issueA := f.Issue(t, "before", testutil.Cols{"created_at": day.Add(-time.Hour)})
	f.Issue(t, "in-day-1", testutil.Cols{"created_at": in(5)})
	f.Issue(t, "in-day-2", testutil.Cols{"created_at": day.Add(24*time.Hour - time.Second)})
	f.Issue(t, "after", testutil.Cols{"created_at": next})

	f.Task(t, a1, testutil.Cols{"status": "completed", "completed_at": in(6), "issue_id": issueA})
	f.Task(t, a1, testutil.Cols{"status": "completed", "completed_at": day.Add(24*time.Hour - time.Second)})
	f.Task(t, a1, testutil.Cols{"status": "completed", "completed_at": next})
	f.Task(t, a1, testutil.Cols{"status": "failed", "completed_at": in(7)})

	for _, a := range []struct {
		size int64
		at   time.Time
	}{{1000, in(4)}, {500, in(20)}, {7000, next.Add(time.Minute)}} {
		f.Insert(t, "attachment", testutil.Cols{
			"workspace_id": ws, "uploader_type": "member", "uploader_id": owner, "filename": "f",
			"url": "https://example.test/f", "content_type": "text/plain", "size_bytes": a.size, "created_at": a.at,
		})
	}
	return seeded{workspaceID: ws, day: day}
}

func TestDBCountsMatchAFixtureWorkspace(t *testing.T) {
	pool, _ := openDB(t)
	s := seed(t, pool)
	got, err := NewDBStore(pool).Counts(context.Background(), s.workspaceID, DayWindow(s.day))
	if err != nil {
		t.Fatal(err)
	}
	want := Counts{Members: 2, Agents: 1, Runtimes: 2, IssuesTotal: 3, IssuesCreated: 2, TasksRun: 2, StorageBytes: 1500}
	if got != want {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}
}

// fixtureWorkspaces lists only the fixture workspace, so a run writes no rows
// for the other workspaces of the shared database.
type fixtureWorkspaces struct {
	Store
	ids []string
}

func (s fixtureWorkspaces) WorkspaceIDs(_ context.Context, after string, _ int) ([]string, error) {
	if after != "" {
		return nil, nil
	}
	return s.ids, nil
}

func TestDBTwoRunsForTheSameDayLeaveOneRowWithTheSameNumbers(t *testing.T) {
	pool, _ := openDB(t)
	s := seed(t, pool)
	j := New(fixtureWorkspaces{Store: NewDBStore(pool), ids: []string{s.workspaceID}})
	now := s.day.AddDate(0, 0, 1).Add(time.Hour) // 01:00 UTC on the 15th computes the 14th

	for i := 0; i < 2; i++ {
		if _, err := j.Run(context.Background(), now); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var members, agents, runtimes, issuesTotal, issuesCreated, tasksRun int32
	var storage int64
	err := pool.QueryRow(context.Background(), `SELECT count(*) OVER (), members, agents, runtimes, issues_total, issues_created, tasks_run, storage_bytes
		FROM platform_usage_daily WHERE workspace_id = $1 AND day = $2`, s.workspaceID, s.day).
		Scan(&n, &members, &agents, &runtimes, &issuesTotal, &issuesCreated, &tasksRun, &storage)
	if err != nil {
		t.Fatal(err)
	}
	got := Counts{members, agents, runtimes, issuesTotal, issuesCreated, tasksRun, storage}
	want := Counts{Members: 2, Agents: 1, Runtimes: 2, IssuesTotal: 3, IssuesCreated: 2, TasksRun: 2, StorageBytes: 1500}
	if n != 1 || got != want {
		t.Fatalf("rows %d, counts %+v; want one row with %+v", n, got, want)
	}
}

func TestDBWorkspacesWithAndWithoutARowAreListed(t *testing.T) {
	pool, _ := openDB(t)
	s := seed(t, pool)
	other := testutil.New(pool, "", "").Workspace(t, "Empty", "usage-empty-"+uuid.NewString()[:8])
	// Page through the whole listing: the shared database may hold more
	// workspaces than one page.
	have := map[string]bool{}
	after := ""
	for {
		ids, err := NewDBStore(pool).WorkspaceIDs(context.Background(), after, 500)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			have[id] = true
		}
		after = ids[len(ids)-1]
	}
	if !have[s.workspaceID] || !have[other] {
		t.Fatalf("listing lacks %s or %s", s.workspaceID, other)
	}
}
