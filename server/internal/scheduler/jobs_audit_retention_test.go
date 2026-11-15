package scheduler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type fakeAuditDeleter struct {
	calls  []db.DeleteAuditEventsBeforeParams
	remain int64 // rows still deletable
	err    error
}

func (f *fakeAuditDeleter) DeleteAuditEventsBefore(_ context.Context, arg db.DeleteAuditEventsBeforeParams) (int64, error) {
	f.calls = append(f.calls, arg)
	if f.err != nil {
		return 0, f.err
	}
	n := min(f.remain, int64(arg.Limit))
	f.remain -= n
	return n, nil
}

func TestParseAuditRetentionDays(t *testing.T) {
	cases := []struct {
		raw  string
		want int
		bad  bool
	}{
		{"", 365, false},
		{"  ", 365, false},
		{"30", 30, false},
		{"730", 730, false},
		{"29", 0, true},
		{"0", 0, true},
		{"-5", 0, true},
		{"abc", 0, true},
		{"90.5", 0, true},
	}
	for _, c := range cases {
		got, err := ParseAuditRetentionDays(c.raw)
		if c.bad {
			if err == nil {
				t.Errorf("%q accepted as %d", c.raw, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%q = %d, %v; want %d", c.raw, got, err, c.want)
		}
	}
}

func TestAuditRetentionJobSpec(t *testing.T) {
	spec := AuditRetentionJob(&fakeAuditDeleter{}, 365)
	if err := spec.validate(); err != nil {
		t.Fatalf("spec invalid: %v", err)
	}
	if spec.Name != "audit_retention" || spec.Cadence != 24*time.Hour {
		t.Fatalf("name/cadence = %q/%s", spec.Name, spec.Cadence)
	}
}

func TestAuditRetentionJobDeletesOneBoundedBatchOlderThanTheCutoff(t *testing.T) {
	d := &fakeAuditDeleter{remain: 12000}
	before := time.Now()
	res, err := AuditRetentionJob(d, 90).Handler(context.Background(), HandlerInput{})
	after := time.Now()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.calls) != 1 || d.calls[0].Limit != 5000 || res.RowsAffected != 5000 {
		t.Fatalf("calls %+v, rows %d; want one call of 5000", d.calls, res.RowsAffected)
	}
	cutoff := d.calls[0].CreatedAt.Time
	if !d.calls[0].CreatedAt.Valid || cutoff.Before(before.AddDate(0, 0, -90)) || cutoff.After(after.AddDate(0, 0, -90)) {
		t.Fatalf("cutoff %v is not now - 90 days", cutoff)
	}
}

func TestAuditRetentionJobSecondRunDeletesNothingMore(t *testing.T) {
	d := &fakeAuditDeleter{remain: 40}
	job := AuditRetentionJob(d, 365)
	first, _ := job.Handler(context.Background(), HandlerInput{})
	second, err := job.Handler(context.Background(), HandlerInput{})
	if err != nil || first.RowsAffected != 40 || second.RowsAffected != 0 {
		t.Fatalf("first %d, second %d, err %v; want 40 then 0", first.RowsAffected, second.RowsAffected, err)
	}
}

func TestAuditRetentionJobReturnsDeleteErrorForRetry(t *testing.T) {
	boom := errors.New("db down")
	_, err := AuditRetentionJob(&fakeAuditDeleter{err: boom}, 365).Handler(context.Background(), HandlerInput{})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the delete error", err)
	}
}

// Database tier (needs DATABASE_URL). The table is shared with other suites, so
// the assertions are on this test's own rows and never on the run's totals.
func TestAuditRetentionJobDeletesOnlyOldFixtureRowsAndIsIdempotent(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	ws := uuid.NewString()
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM audit_event WHERE workspace_id = $1`, ws)
	})
	insert := func(age time.Duration) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO audit_event (workspace_id, actor_type, action, created_at)
			VALUES ($1, 'system', 'workspace.updated', now() - $2::interval)`,
			ws, fmt.Sprintf("%d seconds", int(age.Seconds()))); err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
	}
	insert(400 * 24 * time.Hour)
	insert(100 * 24 * time.Hour)
	insert(time.Hour)
	count := func(where string) (n int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND `+where, ws).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	job := AuditRetentionJob(db.New(pool), 365)
	// Another suite may have left old rows that the batch reaches first, so run
	// until this test's old row is gone.
	for i := 0; i < 50 && count(`created_at < now() - interval '365 days'`) > 0; i++ {
		if _, err := job.Handler(ctx, HandlerInput{}); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(`created_at < now() - interval '365 days'`); n != 0 {
		t.Fatalf("%d fixture rows older than the cutoff remain", n)
	}
	if n := count(`true`); n != 2 {
		t.Fatalf("%d fixture rows remain, want the 2 newer than the cutoff", n)
	}
	if _, err := job.Handler(ctx, HandlerInput{}); err != nil {
		t.Fatal(err)
	}
	if n := count(`true`); n != 2 {
		t.Fatalf("second run changed the fixture rows: %d remain, want 2", n)
	}
}
