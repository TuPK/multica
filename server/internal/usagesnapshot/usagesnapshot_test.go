package usagesnapshot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

const queriesFile = "../../pkg/db/queries/platform_usage_daily.sql"

// queryBlock returns the text of the named sqlc query in the queries file.
func queryBlock(t *testing.T, name string) string {
	t.Helper()
	src, err := os.ReadFile(queriesFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, block := range strings.Split(string(src), "-- name: ")[1:] {
		if strings.HasPrefix(block, name+" ") {
			return block
		}
	}
	t.Fatalf("query %s not found in %s", name, queriesFile)
	return ""
}

var workspaceFilter = regexp.MustCompile(`\b(\w+\.)?workspace_id\s*=\s*sqlc\.arg\('workspace_id'\)|\b(\w+\.)?workspace_id\s*=\s*\$1\b`)

// Every count has one definition: a column, the sqlc query that produces it,
// and the bounds that query applies.
func TestEveryCountNamesTheWorkspaceFilterAndItsDayBounds(t *testing.T) {
	wantColumns := []string{"members", "agents", "runtimes", "issues_total", "issues_created", "tasks_run", "storage_bytes"}
	var got []string
	for _, m := range Metrics {
		got = append(got, m.Column)
	}
	if strings.Join(got, ",") != strings.Join(wantColumns, ",") {
		t.Fatalf("metric columns = %v, want %v", got, wantColumns)
	}
	for _, m := range Metrics {
		t.Run(m.Column, func(t *testing.T) {
			q := queryBlock(t, m.Query)
			if !workspaceFilter.MatchString(q) {
				t.Errorf("%s has no workspace_id filter:\n%s", m.Query, q)
			}
			if !strings.Contains(q, "< sqlc.arg('day_end')") {
				t.Errorf("%s has no exclusive upper bound on day_end", m.Query)
			}
			hasStart := strings.Contains(q, ">= sqlc.arg('day_start')")
			if m.Bounds == BoundsWindow && !hasStart {
				t.Errorf("%s counts one day and needs >= day_start", m.Query)
			}
			if m.Bounds == BoundsEnd && hasStart {
				t.Errorf("%s is a snapshot at the end of the day and must not use day_start", m.Query)
			}
		})
	}
}

func TestMetricsMatchTheContractDefinitions(t *testing.T) {
	byColumn := map[string]Metric{}
	for _, m := range Metrics {
		byColumn[m.Column] = m
	}
	for column, bounds := range map[string]Bounds{
		"members": BoundsWindow, "agents": BoundsEnd, "runtimes": BoundsEnd, "issues_total": BoundsEnd,
		"storage_bytes": BoundsEnd, "issues_created": BoundsWindow, "tasks_run": BoundsWindow,
	} {
		if byColumn[column].Bounds != bounds {
			t.Errorf("%s bounds = %v, want %v", column, byColumn[column].Bounds, bounds)
		}
	}
	for column, wants := range map[string][]string{
		"members":        {"FROM member"},
		"agents":         {"FROM agent", "archived_at IS NULL OR", "archived_at >= sqlc.arg('day_end')"},
		"runtimes":       {"FROM agent_runtime"},
		"issues_total":   {"FROM issue"},
		"issues_created": {"FROM issue"},
		"tasks_run":      {"FROM agent_task_queue", "status = 'completed'", "completed_at >= sqlc.arg('day_start')"},
		"storage_bytes":  {"FROM attachment", "sum(size_bytes)"},
	} {
		q := queryBlock(t, byColumn[column].Query)
		for _, want := range wants {
			if !strings.Contains(q, want) {
				t.Errorf("%s: query does not contain %q:\n%s", column, want, q)
			}
		}
	}
}

func TestWindowIsHalfOpenInUTC(t *testing.T) {
	// A day given in a zone ahead of UTC still names its UTC date.
	zone := time.FixedZone("UTC+7", 7*3600)
	day := time.Date(2026, 11, 15, 3, 0, 0, 0, zone) // 2026-11-14 20:00 UTC
	w := DayWindow(day)
	if want := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC); !w.Start.Equal(want) {
		t.Fatalf("start = %v, want %v", w.Start, want)
	}
	if want := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC); !w.End.Equal(want) {
		t.Fatalf("end = %v, want %v", w.End, want)
	}
	for _, tc := range []struct {
		name string
		at   time.Time
		in   bool
	}{
		{"one second before the day", time.Date(2026, 11, 13, 23, 59, 59, 0, time.UTC), false},
		{"first instant of the day", time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC), true},
		{"last second of the day", time.Date(2026, 11, 14, 23, 59, 59, 0, time.UTC), true},
		{"first instant of the next day", time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC), false},
		{"last second in a zone ahead of UTC", time.Date(2026, 11, 15, 6, 59, 59, 0, zone), true},
		{"midnight in a zone ahead of UTC", time.Date(2026, 11, 15, 7, 0, 0, 0, zone), false},
	} {
		if got := w.Contains(tc.at); got != tc.in {
			t.Errorf("%s: Contains(%v) = %v, want %v", tc.name, tc.at, got, tc.in)
		}
	}
}

func TestDayForRunTime(t *testing.T) {
	d := func(day, hour, min, sec int) time.Time { return time.Date(2026, 11, day, hour, min, sec, 0, time.UTC) }
	for _, tc := range []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"01:00 UTC on D computes D-1", d(15, 1, 0, 0), d(14, 0, 0, 0)},
		{"00:59:59 UTC on D computes D-2", d(15, 0, 59, 59), d(13, 0, 0, 0)},
		{"00:00 UTC on D computes D-2", d(15, 0, 0, 0), d(13, 0, 0, 0)},
		{"23:59 UTC on D computes D-1", d(15, 23, 59, 0), d(14, 0, 0, 0)},
		{"01:00 UTC given in a zone ahead of UTC", d(15, 8, 0, 0).In(time.FixedZone("UTC+7", 7*3600)), d(14, 0, 0, 0)},
		{"month boundary", time.Date(2026, 12, 1, 1, 0, 0, 0, time.UTC), time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)},
	} {
		if got := DayFor(tc.now); !got.Equal(tc.want) {
			t.Errorf("%s: DayFor(%v) = %v, want %v", tc.name, tc.now, got, tc.want)
		}
	}
}

func TestUpsertReplacesTheRowAndNeverAdds(t *testing.T) {
	q := queryBlock(t, "UpsertPlatformUsageDaily")
	if !strings.Contains(q, "ON CONFLICT (workspace_id, day) DO UPDATE") {
		t.Errorf("upsert has no ON CONFLICT (workspace_id, day) DO UPDATE:\n%s", q)
	}
	for _, m := range Metrics {
		if want := fmt.Sprintf("%s = EXCLUDED.%s", m.Column, m.Column); !strings.Contains(q, want) {
			t.Errorf("upsert does not set %q", want)
		}
	}
	if !strings.Contains(q, "computed_at = now()") {
		t.Error("upsert does not refresh computed_at")
	}
	// "x = platform_usage_daily.x + EXCLUDED.x" would add a rerun to the row.
	if regexp.MustCompile(`=\s*[\w.]+\s*[-+*]`).MatchString(q) {
		t.Errorf("upsert computes a value from the old row; a rerun must replace it:\n%s", q)
	}
}

func TestMigrationsHaveNoKeysAndOneConcurrentIndex(t *testing.T) {
	up, err := os.ReadFile("../../migrations/689_platform_usage_daily.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"PRIMARY KEY", "REFERENCES", "FOREIGN KEY", "CHECK", "ON DELETE"} {
		if strings.Contains(strings.ToUpper(string(up)), banned) {
			t.Errorf("689 contains %s", banned)
		}
	}
	for _, col := range []string{"workspace_id uuid NOT NULL", "day date NOT NULL", "members int NOT NULL DEFAULT 0",
		"agents int NOT NULL DEFAULT 0", "runtimes int NOT NULL DEFAULT 0", "issues_total int NOT NULL DEFAULT 0",
		"issues_created int NOT NULL DEFAULT 0", "tasks_run int NOT NULL DEFAULT 0",
		"storage_bytes bigint NOT NULL DEFAULT 0", "computed_at timestamptz NOT NULL DEFAULT now()"} {
		if !strings.Contains(string(up), col) {
			t.Errorf("689 lacks column %q", col)
		}
	}
	idx, err := os.ReadFile("../../migrations/690_platform_usage_daily_uidx.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	want := "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS platform_usage_daily_uidx ON platform_usage_daily (workspace_id, day);"
	if strings.TrimSpace(string(idx)) != want {
		t.Errorf("690 = %q, want %q", strings.TrimSpace(string(idx)), want)
	}
}

// fakeStore applies the same replace-on-conflict rule as the upsert statement.
type fakeStore struct {
	mu          sync.Mutex
	ids         []string
	counts      map[string]Counts
	countErr    map[string]error
	upsertErr   map[int]error // keyed by the 0-based call number
	rows        map[string]Row
	upsertSizes []int
	windows     []Window
}

func newFakeStore(n int) *fakeStore {
	s := &fakeStore{counts: map[string]Counts{}, countErr: map[string]error{}, upsertErr: map[int]error{}, rows: map[string]Row{}}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("ws-%05d", i)
		s.ids = append(s.ids, id)
		s.counts[id] = Counts{Members: int32(i%7 + 1), Agents: 2, Runtimes: 1, IssuesTotal: 30, IssuesCreated: 4, TasksRun: 9, StorageBytes: 1 << 20}
	}
	return s
}

func (s *fakeStore) WorkspaceIDs(_ context.Context, after string, limit int) ([]string, error) {
	var out []string
	for _, id := range s.ids {
		if id > after && len(out) < limit {
			out = append(out, id)
		}
	}
	return out, nil
}

func (s *fakeStore) Counts(_ context.Context, id string, w Window) (Counts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.windows = append(s.windows, w)
	if err := s.countErr[id]; err != nil {
		return Counts{}, err
	}
	return s.counts[id], nil
}

func (s *fakeStore) Upsert(_ context.Context, rows []Row) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	call := len(s.upsertSizes)
	s.upsertSizes = append(s.upsertSizes, len(rows))
	if err := s.upsertErr[call]; err != nil {
		return err // the whole batch rolls back
	}
	for _, r := range rows {
		s.rows[r.WorkspaceID+"|"+r.Day.Format("2006-01-02")] = r // replace, never add
	}
	return nil
}

func at(day, hour, min int) time.Time { return time.Date(2026, 11, day, hour, min, 0, 0, time.UTC) }

func TestRunComputesThePreviousUTCDay(t *testing.T) {
	for _, tc := range []struct {
		name    string
		now     time.Time
		wantDay string
	}{
		{"01:00 UTC computes yesterday", at(15, 1, 0), "2026-11-14"},
		{"00:59 UTC computes the day before yesterday", at(15, 0, 59), "2026-11-13"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newFakeStore(3)
			res, err := New(s).Run(context.Background(), tc.now)
			if err != nil {
				t.Fatal(err)
			}
			if got := res.Day.Format("2006-01-02"); got != tc.wantDay {
				t.Fatalf("day = %s, want %s", got, tc.wantDay)
			}
			if len(s.rows) != 3 {
				t.Fatalf("rows = %d, want 3", len(s.rows))
			}
			for key := range s.rows {
				if !strings.HasSuffix(key, "|"+tc.wantDay) {
					t.Errorf("row %s is not for %s", key, tc.wantDay)
				}
			}
			for _, w := range s.windows {
				if w.End.Sub(w.Start) != 24*time.Hour || w.Start.Format("2006-01-02") != tc.wantDay {
					t.Errorf("window %v..%v is not the UTC day %s", w.Start, w.End, tc.wantDay)
				}
			}
		})
	}
}

func TestRerunForTheSameDayReplacesTheRow(t *testing.T) {
	s := newFakeStore(2)
	j := New(s)
	if _, err := j.Run(context.Background(), at(15, 1, 0)); err != nil {
		t.Fatal(err)
	}
	first := s.rows["ws-00001|2026-11-14"]

	if _, err := j.Run(context.Background(), at(15, 1, 30)); err != nil {
		t.Fatal(err)
	}
	if len(s.rows) != 2 {
		t.Fatalf("rows after a rerun = %d, want 2", len(s.rows))
	}
	if got := s.rows["ws-00001|2026-11-14"]; got.Counts != first.Counts {
		t.Fatalf("rerun changed the row: %+v, then %+v", first.Counts, got.Counts)
	}

	// New numbers replace the old ones; they are not added to them.
	c := s.counts["ws-00001"]
	c.TasksRun, c.StorageBytes = 5, 2<<20
	s.counts["ws-00001"] = c
	if _, err := j.Run(context.Background(), at(15, 2, 0)); err != nil {
		t.Fatal(err)
	}
	got := s.rows["ws-00001|2026-11-14"].Counts
	if got.TasksRun != 5 || got.StorageBytes != 2<<20 {
		t.Fatalf("row = %+v, want tasks_run 5 and storage 2 MiB", got)
	}
}

func TestOneFailingWorkspaceIsSkippedAndTheRunReportsIt(t *testing.T) {
	s := newFakeStore(4)
	s.countErr["ws-00001"] = errors.New("count boom")
	res, err := New(s).Run(context.Background(), at(15, 1, 0))
	if err == nil || !strings.Contains(err.Error(), "1 of 4") {
		t.Fatalf("err = %v, want a failure naming 1 of 4 workspaces", err)
	}
	if res.Written != 3 || res.Skipped != 1 {
		t.Fatalf("written %d skipped %d, want 3 and 1", res.Written, res.Skipped)
	}
	if _, ok := s.rows["ws-00001|2026-11-14"]; ok {
		t.Error("the failing workspace has a row")
	}
	for _, id := range []string{"ws-00000", "ws-00002", "ws-00003"} {
		if _, ok := s.rows[id+"|2026-11-14"]; !ok {
			t.Errorf("%s has no row", id)
		}
	}

	// The next attempt of the same day fills the gap.
	delete(s.countErr, "ws-00001")
	if _, err := New(s).Run(context.Background(), at(15, 1, 5)); err != nil {
		t.Fatal(err)
	}
	if len(s.rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(s.rows))
	}
}

func TestBatchesHoldAtMost500WorkspacesEach(t *testing.T) {
	s := newFakeStore(1203)
	res, err := New(s).Run(context.Background(), at(15, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{500, 500, 203}; fmt.Sprint(s.upsertSizes) != fmt.Sprint(want) {
		t.Fatalf("batch sizes = %v, want %v", s.upsertSizes, want)
	}
	if res.Written != 1203 || len(s.rows) != 1203 {
		t.Fatalf("written %d, rows %d, want 1203", res.Written, len(s.rows))
	}
	if MaxBatch != 500 {
		t.Fatalf("MaxBatch = %d, want 500", MaxBatch)
	}

	// A larger batch size is clamped.
	s = newFakeStore(1203)
	j := New(s)
	j.BatchSize = 5000
	if _, err := j.Run(context.Background(), at(15, 1, 0)); err != nil {
		t.Fatal(err)
	}
	for _, n := range s.upsertSizes {
		if n > 500 {
			t.Fatalf("batch of %d, want at most 500", n)
		}
	}
}

func TestAFailedBatchRollsBackAndTheOthersStillRun(t *testing.T) {
	s := newFakeStore(1203)
	s.upsertErr[1] = errors.New("commit boom")
	res, err := New(s).Run(context.Background(), at(15, 1, 0))
	if err == nil {
		t.Fatal("want an error so the scheduler retries the day")
	}
	if res.Written != 703 || res.Skipped != 500 {
		t.Fatalf("written %d skipped %d, want 703 and 500", res.Written, res.Skipped)
	}
	if len(s.rows) != 703 {
		t.Fatalf("rows = %d, want 703", len(s.rows))
	}
}

func TestRunStopsWhenTheContextIsCanceled(t *testing.T) {
	s := newFakeStore(10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(s).Run(ctx, at(15, 1, 0)); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(s.rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(s.rows))
	}
}
