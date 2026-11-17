package paymentnotice

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Each test writes under its own random
// provider name, so rows from other runs on the shared database never count,
// and it deletes the rows it wrote.

type noticeFixture struct {
	t        *testing.T
	store    *PgxStore
	provider string
}

func newNoticeFixture(t *testing.T) *noticeFixture {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()
	var provider string
	if err := pool.QueryRow(ctx, `SELECT 'test-' || gen_random_uuid()::text`).Scan(&provider); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM payment_notice WHERE provider = $1`, provider)
	})
	return &noticeFixture{t: t, store: NewPgxStore(pool), provider: provider}
}

func (f *noticeFixture) newNotice(eventID string, receivedAt time.Time) NewNotice {
	return NewNotice{
		Provider:        f.provider,
		ProviderEventID: eventID,
		TransferCode:    "ABC123",
		Amount:          150000,
		Currency:        "VND",
		BodySHA256:      "aa",
		ReceivedAt:      receivedAt,
	}
}

func (f *noticeFixture) insert(eventID string, receivedAt time.Time) Notice {
	f.t.Helper()
	n, inserted, err := f.store.Insert(context.Background(), f.newNotice(eventID, receivedAt))
	if err != nil {
		f.t.Fatalf("insert %s: %v", eventID, err)
	}
	if !inserted {
		f.t.Fatalf("insert %s: first insert reported not inserted", eventID)
	}
	return n
}

func TestNoticeInsertIsIdempotentOnProviderEvent(t *testing.T) {
	f := newNoticeFixture(t)
	ctx := context.Background()

	first := f.insert("42", time.Now())

	second, inserted, err := f.store.Insert(ctx, NewNotice{
		Provider:        f.provider,
		ProviderEventID: "42",
		TransferCode:    "OTHER9",
		Amount:          1,
		Currency:        "USD",
		BodySHA256:      "bb",
	})
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if inserted {
		t.Error("second insert of the same provider event reported inserted")
	}
	if second.ID != first.ID {
		t.Errorf("second insert returned id %s, want the first row %s", second.ID, first.ID)
	}
	if second.Amount != 150000 || second.Currency != "VND" || second.BodySHA256 != "aa" {
		t.Errorf("second insert returned the new payload; want the first row's fields, got %+v", second)
	}

	var rows int
	if err := f.store.pool.QueryRow(ctx,
		`SELECT count(*) FROM payment_notice WHERE provider = $1 AND provider_event_id = $2`,
		f.provider, "42").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("rows for one provider event = %d, want 1", rows)
	}
}

func TestNoticeTransitionIsConditionalOnState(t *testing.T) {
	f := newNoticeFixture(t)
	ctx := context.Background()
	n := f.insert("1", time.Now())

	// received -> applied succeeds once; the same move again finds the row
	// no longer in received.
	if err := f.store.Transition(ctx, Change{ID: n.ID, From: StateReceived, To: StateApplied, DecidedBy: "system"}); err != nil {
		t.Fatalf("received -> applied: %v", err)
	}
	if err := f.store.Transition(ctx, Change{ID: n.ID, From: StateReceived, To: StateApplied, DecidedBy: "system"}); !errors.Is(err, ErrStateConflict) {
		t.Errorf("repeat received -> applied: err = %v, want ErrStateConflict", err)
	}
	// applied is terminal.
	if err := f.store.Transition(ctx, Change{ID: n.ID, From: StateApplied, To: StateNeedsReview, ReviewReason: "x"}); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("applied -> needs_review: err = %v, want ErrInvalidTransition", err)
	}
	got, err := f.store.Get(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateApplied {
		t.Errorf("state = %s, want applied", got.State)
	}

	// needs_review carries its reason; resolved records who decided and why.
	m := f.insert("2", time.Now())
	if err := f.store.Transition(ctx, Change{ID: m.ID, From: StateReceived, To: StateNeedsReview, ReviewReason: "amount_mismatch"}); err != nil {
		t.Fatalf("received -> needs_review: %v", err)
	}
	if err := f.store.Transition(ctx, Change{ID: m.ID, From: StateNeedsReview, To: StateResolved, DecidedBy: "staff-1", DecisionReason: "manual match"}); err != nil {
		t.Fatalf("needs_review -> resolved: %v", err)
	}
	got, err = f.store.Get(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateResolved || got.ReviewReason != "amount_mismatch" || got.DecidedBy != "staff-1" ||
		got.DecisionReason != "manual match" || got.DecidedAt.IsZero() {
		t.Errorf("resolved row = %+v", got)
	}
	// resolved is terminal: no move back to review, no second close.
	if err := f.store.Transition(ctx, Change{ID: m.ID, From: StateResolved, To: StateNeedsReview, ReviewReason: "x"}); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("resolved -> needs_review: err = %v, want ErrInvalidTransition", err)
	}
	if err := f.store.Transition(ctx, Change{ID: m.ID, From: StateNeedsReview, To: StateDismissed, DecidedBy: "staff-1", DecisionReason: "refund"}); !errors.Is(err, ErrStateConflict) {
		t.Errorf("dismiss after resolve: err = %v, want ErrStateConflict", err)
	}
}

func TestNoticeHasNoBodyColumn(t *testing.T) {
	f := newNoticeFixture(t)
	rows, err := f.store.pool.Query(context.Background(), `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'payment_notice'
		ORDER BY ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	// The exact column list of spec 7.5. Adding a column, including one that
	// holds the raw body, fails this test until the spec and this list agree.
	want := []string{
		"id", "provider", "provider_event_id", "state", "review_reason",
		"transfer_code", "amount", "currency", "order_id", "body_sha256",
		"received_at", "decided_at", "decided_by", "decision_reason",
	}
	if !reflect.DeepEqual(cols, want) {
		t.Errorf("payment_notice columns = %v, want %v", cols, want)
	}
}

func TestQueueListsOnlyNeedsReviewOldestFirst(t *testing.T) {
	f := newNoticeFixture(t)
	ctx := context.Background()
	base := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

	late := f.insert("late", base.Add(3*time.Hour))
	early := f.insert("early", base.Add(time.Hour))
	mid := f.insert("mid", base.Add(2*time.Hour))
	pending := f.insert("received-only", base)
	applied := f.insert("applied", base)
	for _, n := range []Notice{late, early, mid} {
		if err := f.store.Transition(ctx, Change{ID: n.ID, From: StateReceived, To: StateNeedsReview, ReviewReason: "unknown_code"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.store.Transition(ctx, Change{ID: applied.ID, From: StateReceived, To: StateApplied, DecidedBy: "system"}); err != nil {
		t.Fatal(err)
	}

	queue, err := f.store.ListNeedsReview(ctx, 1000)
	if err != nil {
		t.Fatal(err)
	}
	// The queue is shared by every run on this database; keep only this test's rows.
	var mine []string
	for _, n := range queue {
		if n.Provider != f.provider {
			continue
		}
		if n.State != StateNeedsReview {
			t.Errorf("queue row %s has state %s, want needs_review", n.ID, n.State)
		}
		mine = append(mine, n.ProviderEventID)
	}
	want := []string{"early", "mid", "late"}
	if !reflect.DeepEqual(mine, want) {
		t.Errorf("queue order = %v, want %v (oldest first; not received, not applied: %s, %s)", mine, want, pending.ID, applied.ID)
	}
}
