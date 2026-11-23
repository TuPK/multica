package quotaalert

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/lifecycle"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

type fakeSource struct {
	ws     []string
	counts map[string]map[string]int64
	limits map[string]map[string]*int64
}

func (f *fakeSource) Workspaces(context.Context) ([]string, error) { return f.ws, nil }

func (f *fakeSource) Counts(_ context.Context, ws string, _ time.Time) (map[string]int64, error) {
	return f.counts[ws], nil
}

func (f *fakeSource) Limits(_ context.Context, ws string, _ time.Time) (map[string]*int64, error) {
	return f.limits[ws], nil
}

type fakeNotices struct {
	recorded map[string]bool
}

func (f *fakeNotices) Contact(context.Context, string) (lifecycle.Contact, error) {
	return lifecycle.Contact{Slug: "acme", Emails: []string{"owner@example.test"}}, nil
}

func (f *fakeNotices) InsertNotice(_ context.Context, n lifecycle.Notice) (bool, error) {
	key := n.WorkspaceID + "|" + n.Key
	if f.recorded[key] {
		return false, nil
	}
	f.recorded[key] = true
	return true, nil
}

func (f *fakeNotices) DeleteNotice(_ context.Context, ws, key string) error {
	delete(f.recorded, ws+"|"+key)
	return nil
}

type fakeMail struct {
	sent []lifecycle.Message
	err  error
}

func (f *fakeMail) Send(_ context.Context, m lifecycle.Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

func i64(v int64) *int64 { return &v }

var june = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

func agentsAt(used int64) *fakeSource {
	return &fakeSource{
		ws:     []string{"w1"},
		counts: map[string]map[string]int64{"w1": {plan.GateAgents: used}},
		limits: map[string]map[string]*int64{"w1": {plan.GateAgents: i64(10)}},
	}
}

// TestWarningAtEightyThenOneHundredOncePerMonth: a use of 8 of 10 sends the 80
// warning once; reaching 10 sends the 100 warning once; the next month repeats.
func TestWarningAtEightyThenOneHundredOncePerMonth(t *testing.T) {
	src := agentsAt(8)
	mail := &fakeMail{}
	job := Job{Source: src, Notices: &fakeNotices{recorded: map[string]bool{}}, Mail: mail, AppURL: "https://app.example.test"}

	if n, err := job.Run(context.Background(), june); err != nil || n != 1 {
		t.Fatalf("at 8 of 10: sent %d, err %v; want 1", n, err)
	}
	if n, _ := job.Run(context.Background(), june); n != 0 {
		t.Fatalf("second run in the same month sent %d, want 0", n)
	}
	src.counts["w1"][plan.GateAgents] = 10
	if n, _ := job.Run(context.Background(), june); n != 1 {
		t.Fatalf("at 10 of 10: sent %d, want only the 100 warning", n)
	}
	if n, _ := job.Run(context.Background(), june); n != 0 {
		t.Fatalf("repeat at 10 sent %d, want 0", n)
	}
	july := june.AddDate(0, 1, 0)
	if n, _ := job.Run(context.Background(), july); n != 2 {
		t.Fatalf("next month at 10 of 10: sent %d, want both warnings again", n)
	}
	if len(mail.sent) != 4 {
		t.Fatalf("emails = %d, want 4", len(mail.sent))
	}
	if want := "Delegora: acme reached 80% of its agents limit"; mail.sent[0].Subject != want {
		t.Fatalf("first subject = %q, want %q", mail.sent[0].Subject, want)
	}
}

// TestAutopilotRunsWarnAtEighty: the autopilot runs of the period warn like the
// other counted limits, with the autopilot_runs gate in the subject (spec R6
// section 3c.5, rule 1).
func TestAutopilotRunsWarnAtEighty(t *testing.T) {
	src := &fakeSource{
		ws:     []string{"w1"},
		counts: map[string]map[string]int64{"w1": {plan.GateAutopilotRuns: 8}},
		limits: map[string]map[string]*int64{"w1": {plan.GateAutopilotRuns: i64(10)}},
	}
	mail := &fakeMail{}
	job := Job{Source: src, Notices: &fakeNotices{recorded: map[string]bool{}}, Mail: mail}

	if n, err := job.Run(context.Background(), june); err != nil || n != 1 {
		t.Fatalf("at 8 of 10 autopilot runs: sent %d, err %v; want 1", n, err)
	}
	if want := "Delegora: acme reached 80% of its autopilot_runs limit"; len(mail.sent) != 1 || mail.sent[0].Subject != want {
		t.Fatalf("mail = %+v, want subject %q", mail.sent, want)
	}
}

// TestNoLimitNoWarning: a gate the plan leaves unlimited, or a zero-limit gate,
// never warns.
func TestNoLimitNoWarning(t *testing.T) {
	src := &fakeSource{
		ws:     []string{"w1"},
		counts: map[string]map[string]int64{"w1": {plan.GateAgents: 500, plan.GateRuntimes: 3}},
		limits: map[string]map[string]*int64{"w1": {plan.GateAgents: nil, plan.GateRuntimes: i64(0)}},
	}
	job := Job{Source: src, Notices: &fakeNotices{recorded: map[string]bool{}}, Mail: &fakeMail{}}
	if n, err := job.Run(context.Background(), june); err != nil || n != 0 {
		t.Fatalf("sent %d, err %v; want 0", n, err)
	}
}

// TestFailedSendIsRetriedNextRun: when the email fails the record is removed, so
// the next run sends it.
func TestFailedSendIsRetriedNextRun(t *testing.T) {
	src := agentsAt(9)
	notices := &fakeNotices{recorded: map[string]bool{}}
	mail := &fakeMail{err: errors.New("provider down")}
	job := Job{Source: src, Notices: notices, Mail: mail}

	if _, err := job.Run(context.Background(), june); err == nil {
		t.Fatal("a failed send returned no error")
	}
	mail.err = nil
	if n, err := job.Run(context.Background(), june); err != nil || n != 1 {
		t.Fatalf("retry sent %d, err %v; want 1", n, err)
	}
}

func TestThresholds(t *testing.T) {
	cases := []struct {
		used, limit int64
		want        []int64
	}{
		{7, 10, nil},
		{8, 10, []int64{80}},
		{9, 10, []int64{80}},
		{10, 10, []int64{80, 100}},
		{11, 10, []int64{80, 100}},
		{5, 0, nil},
	}
	for _, c := range cases {
		if got := thresholds(c.used, c.limit); !slices.Equal(got, c.want) {
			t.Errorf("thresholds(%d, %d) = %v, want %v", c.used, c.limit, got, c.want)
		}
	}
}

// syncNotices and syncMail make the fakes safe for concurrent runs. The real
// notice store relies on its unique key for this; the fake's lock stands in for it.
type syncNotices struct {
	mu sync.Mutex
	fakeNotices
}

func (s *syncNotices) InsertNotice(ctx context.Context, n lifecycle.Notice) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fakeNotices.InsertNotice(ctx, n)
}

type syncMail struct {
	mu   sync.Mutex
	sent []lifecycle.Message
}

func (s *syncMail) Send(_ context.Context, m lifecycle.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, m)
	return nil
}

// Spec R6 section 3c.5 rule 1: a warning goes out once per (workspace, limit,
// threshold, month) however many runs overlap or follow one another.
func TestQuotaAlertTwiceAndParallelSendsOne(t *testing.T) {
	src := agentsAt(8)
	notices := &syncNotices{fakeNotices: fakeNotices{recorded: map[string]bool{}}}
	mail := &syncMail{}
	job := Job{Source: src, Notices: notices, Mail: mail}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = job.Run(context.Background(), june)
		}()
	}
	wg.Wait()
	if len(mail.sent) != 1 {
		t.Fatalf("parallel runs sent %d emails, want 1", len(mail.sent))
	}
	if n, err := job.Run(context.Background(), june); err != nil || n != 0 {
		t.Fatalf("a later run sent %d (err %v), want 0", n, err)
	}
}

// Spec R6 section 3c.5 rule 1: the alert reads workspaces that are not being
// deleted; a workspace in pending_deletion is not in the list.
func TestQuotaAlertSkipsPendingDeletionWorkspace(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	var active, pending string
	if err := pool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ('Quota Alert Active', 'qa-' || substr(md5(random()::text), 1, 12)) RETURNING id::text`).Scan(&active); err != nil {
		t.Fatalf("insert active workspace: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ('Quota Alert Pending', 'qa-' || substr(md5(random()::text), 1, 12)) RETURNING id::text`).Scan(&pending); err != nil {
		t.Fatalf("insert pending workspace: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO platform_workspace (workspace_id, status, reason, deletion_scheduled_at) VALUES ($1, 'pending_deletion', 'scheduled', now())`, pending); err != nil {
		t.Fatalf("mark pending workspace: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM platform_workspace WHERE workspace_id = ANY($1::uuid[])`, []string{active, pending})
		pool.Exec(ctx, `DELETE FROM workspace WHERE id = ANY($1::uuid[])`, []string{active, pending})
	})

	ids, err := NewPgxSource(pool).Workspaces(ctx)
	if err != nil {
		t.Fatalf("Workspaces: %v", err)
	}
	if !slices.Contains(ids, active) {
		t.Fatalf("active workspace missing from the alert source")
	}
	if slices.Contains(ids, pending) {
		t.Fatalf("workspace in pending_deletion is in the alert source")
	}
}

// Spec R6 section 3c.5 rule 1: the alert only sends warnings. It records notices
// and reads the limits, and leaves the limits it read as they were.
func TestQuotaAlertDoesNotChangeEnforcement(t *testing.T) {
	src := agentsAt(10)
	notices := &fakeNotices{recorded: map[string]bool{}}
	mail := &fakeMail{}

	if _, err := (Job{Source: src, Notices: notices, Mail: mail}).Run(context.Background(), june); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := src.limits["w1"]; len(got) != 1 || got[plan.GateAgents] == nil || *got[plan.GateAgents] != 10 {
		t.Fatalf("limits changed by the alert: %v", got)
	}
	if len(mail.sent) != 2 || len(notices.recorded) != 2 {
		t.Fatalf("sent %d emails and recorded %d notices, want the 80 and 100 warnings only", len(mail.sent), len(notices.recorded))
	}
}
