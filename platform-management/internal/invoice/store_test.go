package invoice

import (
	"context"
	"errors"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Each test writes under its own random
// order id, so rows from other runs on the shared database never count, and
// it deletes the rows it wrote. The application never deletes an invoice
// record (spec 7.3 rule 2); the cleanup below removes only test fixtures.
//
// Records are addressed by their natural key (order_id, revision); the table
// has no surrogate id (spec 7.5).

type recordFixture struct {
	t       *testing.T
	store   *PgxStore
	pool    *pgxpool.Pool
	orderID string
	wsID    string
}

func newRecordFixture(t *testing.T) *recordFixture {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()
	var orderID, wsID string
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text, gen_random_uuid()::text`).Scan(&orderID, &wsID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM einvoice_issuance WHERE order_id = $1`, orderID)
	})
	return &recordFixture{t: t, store: NewPgxStore(pool), pool: pool, orderID: orderID, wsID: wsID}
}

func (f *recordFixture) newRecord() NewRecord {
	return NewRecord{
		OrderID:     f.orderID,
		WorkspaceID: f.wsID,
		Provider:    "sepay",
		Currency:    "VND",
		Amount:      150000,
		VATAmount:   0,
		Buyer:       []byte(`{"name":"Cong ty A"}`),
	}
}

func (f *recordFixture) insert() Record {
	f.t.Helper()
	r, err := f.store.Insert(context.Background(), f.newRecord())
	if err != nil {
		f.t.Fatalf("insert: %v", err)
	}
	return r
}

// setRow changes fields the store does not yet write, so a test can place a
// row in a state that a later step (P43.3, P43.4) will reach through the store.
func (f *recordFixture) setRow(revision int, sql string, args ...any) {
	f.t.Helper()
	args = append([]any{f.orderID, revision}, args...)
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

// TestRevisionUniqueRejectsSecondLiveRecord: two concurrent inserts of the
// same (order_id, revision) cannot both succeed; the unique index is the
// backstop when two workers race past the max+1 read.
func TestRevisionUniqueRejectsSecondLiveRecord(t *testing.T) {
	f := newRecordFixture(t)
	ctx := context.Background()
	const insertSQL = `INSERT INTO einvoice_issuance
		(order_id, revision, workspace_id, state, provider, currency, amount, vat_amount, buyer, created_at, updated_at)
		VALUES ($1, 1, $2, 'pending', 'sepay', 'VND', 0, 0, '{}'::jsonb, now(), now())`

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := f.pool.Exec(ctx, insertSQL, f.orderID, f.wsID)
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	failures := 0
	for _, err := range errs {
		if err == nil {
			continue
		}
		failures++
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Errorf("second insert failed with %v, want unique violation 23505", err)
		}
	}
	if failures != 1 {
		t.Fatalf("concurrent inserts at revision 1: %d failed, want exactly 1", failures)
	}
}

// TestInsertUsesMaxPlusOne: the store assigns revision = max + 1 in the insert
// statement, so revisions for one order run 1, 2, 3 and never repeat.
func TestInsertUsesMaxPlusOne(t *testing.T) {
	f := newRecordFixture(t)

	for want := 1; want <= 3; want++ {
		r := f.insert()
		if r.Revision != want {
			t.Errorf("insert %d: revision = %d, want %d", want, r.Revision, want)
		}
		if r.State != StatePending {
			t.Errorf("insert %d: state = %q, want %q", want, r.State, StatePending)
		}
	}
}

// TestStoreHasNoDeleteQuery: the invoice queries never remove a row.
func TestStoreHasNoDeleteQuery(t *testing.T) {
	const path = "../../queries/einvoice.sql"
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if regexp.MustCompile(`(?i)\bDELETE\b`).Match(src) {
		t.Fatalf("%s contains a DELETE statement; invoice records are never deleted (spec 7.3 rule 2)", path)
	}
}

// TestIssuedRecordNumberNeverChanges: an issued record keeps its first invoice
// number; a second issue of the same record is refused and changes nothing.
func TestIssuedRecordNumberNeverChanges(t *testing.T) {
	f := newRecordFixture(t)
	ctx := context.Background()
	r := f.insert()

	issued := Issued{
		ProviderInvoiceID: "prov-1",
		InvoiceNumber:     "INV-0001",
		LookupURL:         "https://example.test/lookup/1",
		IssuedAt:          time.Now(),
	}
	if err := f.store.MarkIssued(ctx, r.OrderID, r.Revision, issued); err != nil {
		t.Fatalf("first issue: %v", err)
	}

	second := issued
	second.InvoiceNumber = "INV-9999"
	if err := f.store.MarkIssued(ctx, r.OrderID, r.Revision, second); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("second issue: err = %v, want ErrStateConflict", err)
	}

	got, err := f.store.Get(ctx, r.OrderID, r.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateIssued {
		t.Errorf("state = %q, want %q", got.State, StateIssued)
	}
	if got.InvoiceNumber != "INV-0001" {
		t.Errorf("invoice_number = %q, want the first number INV-0001", got.InvoiceNumber)
	}
}

// TestDueIndexQueryOnlyReturnsPendingDue: the due query returns pending rows
// whose next_attempt_at has passed and nothing else.
func TestDueIndexQueryOnlyReturnsPendingDue(t *testing.T) {
	f := newRecordFixture(t)
	ctx := context.Background()
	now := time.Now()
	past := now.Add(-time.Minute)

	due := f.insert() // revision 1: pending, due
	f.setRow(due.Revision, `UPDATE einvoice_issuance SET next_attempt_at = $3 WHERE order_id = $1 AND revision = $2`, past)

	future := f.insert() // revision 2: pending, not yet due
	f.setRow(future.Revision, `UPDATE einvoice_issuance SET next_attempt_at = $3 WHERE order_id = $1 AND revision = $2`, now.Add(time.Hour))

	issued := f.insert() // revision 3: issued, past due time must not matter
	f.setRow(issued.Revision, `UPDATE einvoice_issuance SET state = 'issued', next_attempt_at = $3 WHERE order_id = $1 AND revision = $2`, past)

	needsInfo := f.insert() // revision 4
	f.setRow(needsInfo.Revision, `UPDATE einvoice_issuance SET state = 'needs_info', next_attempt_at = $3 WHERE order_id = $1 AND revision = $2`, past)

	failed := f.insert() // revision 5
	f.setRow(failed.Revision, `UPDATE einvoice_issuance SET state = 'failed', next_attempt_at = $3 WHERE order_id = $1 AND revision = $2`, past)

	dueRows, err := f.store.ListDue(ctx, now, 1000)
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	for _, r := range dueRows {
		if r.State != StatePending {
			t.Errorf("ListDue returned state %q for revision %d; want only pending", r.State, r.Revision)
		}
		if r.NextAttemptAt.After(now) {
			t.Errorf("ListDue returned revision %d due at %v, after now %v", r.Revision, r.NextAttemptAt, now)
		}
	}
	found := map[int]bool{}
	for _, r := range dueRows {
		if r.OrderID == f.orderID {
			found[r.Revision] = true
		}
	}
	if !found[due.Revision] {
		t.Errorf("ListDue did not return the pending due revision %d", due.Revision)
	}
	for name, r := range map[string]Record{"future": future, "issued": issued, "needs_info": needsInfo, "failed": failed} {
		if found[r.Revision] {
			t.Errorf("ListDue returned %s revision %d", name, r.Revision)
		}
	}
}

// TestReplacedOriginalBecomesCancelled: replacement never cancels at the
// provider (spec 7.3 rule 6). Locally the issued original is marked cancelled
// with the admin's reason, and a new pending revision carries the corrected
// buyer. Only an issued record can be replaced.
func TestReplacedOriginalBecomesCancelled(t *testing.T) {
	f := newRecordFixture(t)
	ctx := context.Background()
	original := f.insert()

	if _, err := f.store.ReplaceIssued(ctx, original.OrderID, original.Revision, []byte(`{}`), "admin-1", "wrong buyer"); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("replace of a pending record: err = %v, want ErrStateConflict", err)
	}

	if err := f.store.MarkIssued(ctx, original.OrderID, original.Revision, Issued{
		ProviderInvoiceID: "prov-1",
		InvoiceNumber:     "INV-0002",
		LookupURL:         "https://example.test/lookup/2",
		IssuedAt:          time.Now(),
	}); err != nil {
		t.Fatalf("issue: %v", err)
	}

	buyer := []byte(`{"name":"Cong ty B"}`)
	next, err := f.store.ReplaceIssued(ctx, original.OrderID, original.Revision, buyer, "admin-1", "wrong buyer")
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if next.Revision != 2 || next.State != StatePending {
		t.Errorf("replacement = revision %d state %q, want revision 2 state %q", next.Revision, next.State, StatePending)
	}
	if string(next.Buyer) != string(buyer) {
		t.Errorf("replacement buyer = %s, want %s", next.Buyer, buyer)
	}

	got, err := f.store.Get(ctx, original.OrderID, original.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateCancelled {
		t.Errorf("original state = %q, want %q", got.State, StateCancelled)
	}
	if got.CancelReason != "wrong buyer" || got.CancelledBy != "admin-1" || got.CancelledAt == nil {
		t.Errorf("original cancel fields = reason %q by %q at %v; want the admin's reason, actor and a time", got.CancelReason, got.CancelledBy, got.CancelledAt)
	}
	if got.InvoiceNumber != "INV-0002" {
		t.Errorf("original invoice_number = %q, want INV-0002 kept", got.InvoiceNumber)
	}
}
