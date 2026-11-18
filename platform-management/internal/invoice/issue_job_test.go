package invoice

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests for the issue jobs (spec 7.3 rules 1 to 3, 5, 6). They run
// against PLATFORM_TEST_DATABASE_URL and only the fake provider; the real
// SePay adapter is never called. Each test writes under its own random order
// ids and deletes the billing orders and records it created.

const fullBuyerJSON = `{"name":"Cong ty A","tax_code":"0312345678","address":"1 Pho A","email":"a@example.com"}`

type issueFixture struct {
	t      *testing.T
	pool   *pgxpool.Pool
	store  *PgxStore
	fake   *Fake
	wsID   string
	start  time.Time
	orders []string
}

func newIssueFixture(t *testing.T) *issueFixture {
	t.Helper()
	pool := testdb.Open(t)
	var wsID string
	if err := pool.QueryRow(context.Background(), `SELECT gen_random_uuid()::text`).Scan(&wsID); err != nil {
		t.Fatal(err)
	}
	f := &issueFixture{
		t:     t,
		pool:  pool,
		store: NewPgxStore(pool),
		fake:  NewFake(),
		wsID:  wsID,
		start: time.Now().UTC().Add(-24 * time.Hour),
	}
	t.Cleanup(func() {
		ctx := context.Background()
		for _, id := range f.orders {
			pool.Exec(ctx, `DELETE FROM einvoice_issuance WHERE order_id = $1`, id)
			pool.Exec(ctx, `DELETE FROM audit_event WHERE target_type = 'einvoice' AND target_id = $1`, id)
			pool.Exec(ctx, `DELETE FROM billing_order WHERE id = $1`, id)
		}
	})
	return f
}

// order inserts a billing order and returns its id.
func (f *issueFixture) order(provider string, amount int64, status string, paidAt time.Time, buyer string) string {
	f.t.Helper()
	var id string
	err := f.pool.QueryRow(context.Background(), `INSERT INTO billing_order
		(id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount, currency, status, provider,
		 transfer_code, buyer, paid_at, expires_at)
		VALUES (gen_random_uuid(), $1, 'subscribe', 'pro', 'month', 1, $2, 0, 'VND', $3, $4,
		        gen_random_uuid()::text, $5::jsonb, $6, now() + interval '1 day')
		RETURNING id::text`, f.wsID, amount, status, provider, buyer, paidAt).Scan(&id)
	if err != nil {
		f.t.Fatalf("insert billing order: %v", err)
	}
	f.orders = append(f.orders, id)
	return id
}

// paidOrder is an eligible order: paid after the start mark, amount above zero.
func (f *issueFixture) paidOrder(buyer string) string {
	return f.order("sepay", 150000, "paid", time.Now().UTC().Add(-time.Hour), buyer)
}

func (f *issueFixture) service(maxAttempts int) *Service {
	return NewService(f.store, f.fake, Config{StartAt: f.start, MaxAttempts: maxAttempts})
}

func (f *issueFixture) get(orderID string, revision int) Record {
	f.t.Helper()
	r, err := f.store.Get(context.Background(), orderID, revision)
	if err != nil {
		f.t.Fatalf("get record %s/%d: %v", orderID, revision, err)
	}
	return r
}

func (f *issueFixture) countRows(orderID string) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM einvoice_issuance WHERE order_id = $1`, orderID).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *issueFixture) orderStatus(orderID string) string {
	f.t.Helper()
	var s string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status FROM billing_order WHERE id = $1`, orderID).Scan(&s); err != nil {
		f.t.Fatal(err)
	}
	return s
}

// makeDue moves a pending record's next attempt into the past so the next
// Issue run claims it, without waiting for the backoff.
func (f *issueFixture) makeDue(orderID string, revision int) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE einvoice_issuance
		SET next_attempt_at = now() - interval '1 second', locked_until = NULL
		WHERE order_id = $1 AND revision = $2`, orderID, revision); err != nil {
		f.t.Fatal(err)
	}
}

func (f *issueFixture) mustEnqueue(s *Service) {
	f.t.Helper()
	if _, err := s.Enqueue(context.Background()); err != nil {
		f.t.Fatalf("enqueue: %v", err)
	}
}

func (f *issueFixture) mustIssue(s *Service) {
	f.t.Helper()
	if _, err := s.Issue(context.Background()); err != nil {
		f.t.Fatalf("issue: %v", err)
	}
}

// TestEnqueueCreatesOneRecordPerPaidOrder: a paid order after the start mark
// with an amount gets one pending record that carries the buyer. A zero-amount
// order, an admin grant, an order paid before the start mark and an unpaid
// order get none.
func TestEnqueueCreatesOneRecordPerPaidOrder(t *testing.T) {
	f := newIssueFixture(t)
	now := time.Now().UTC()
	eligible := f.paidOrder(fullBuyerJSON)
	zero := f.order("sepay", 0, "paid", now.Add(-time.Hour), fullBuyerJSON)
	grant := f.order("admin_grant", 150000, "paid", now.Add(-time.Hour), fullBuyerJSON)
	early := f.order("sepay", 150000, "paid", f.start.Add(-time.Hour), fullBuyerJSON)
	unpaid := f.order("sepay", 150000, "pending", now.Add(-time.Hour), fullBuyerJSON)

	f.mustEnqueue(f.service(8))

	rec := f.get(eligible, 1)
	if rec.State != StatePending || rec.Revision != 1 || rec.Amount != 150000 || rec.Currency != "VND" {
		t.Fatalf("eligible record = state %q revision %d amount %d currency %q", rec.State, rec.Revision, rec.Amount, rec.Currency)
	}
	if rec.Provider != "fake" {
		t.Fatalf("provider = %q, want fake", rec.Provider)
	}
	var gotBuyer, wantBuyer map[string]string
	if err := json.Unmarshal(rec.Buyer, &gotBuyer); err != nil {
		t.Fatalf("buyer is not JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(fullBuyerJSON), &wantBuyer); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotBuyer, wantBuyer) {
		t.Fatalf("buyer was not copied from billing_order: got %v", gotBuyer)
	}
	for name, id := range map[string]string{"zero amount": zero, "admin grant": grant, "before start": early, "unpaid": unpaid} {
		if n := f.countRows(id); n != 0 {
			t.Errorf("%s: %d records, want 0", name, n)
		}
	}
}

// TestEnqueueTwiceIsIdempotent: a second enqueue run adds no record.
func TestEnqueueTwiceIsIdempotent(t *testing.T) {
	f := newIssueFixture(t)
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(8)
	f.mustEnqueue(s)
	f.mustEnqueue(s)
	if n := f.countRows(id); n != 1 {
		t.Fatalf("records after two enqueues = %d, want 1", n)
	}
}

// TestIssueTwiceConcurrentlyMakesOneInvoice: two issue runs at once issue one
// invoice at the provider. The claim takes the lock on the record so the second
// run sees nothing due.
func TestIssueTwiceConcurrentlyMakesOneInvoice(t *testing.T) {
	f := newIssueFixture(t)
	ctx := context.Background()
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(8)
	f.mustEnqueue(s)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = s.Issue(ctx)
		}(i)
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
	}
	if got := f.fake.IssueCalls(); got != 1 {
		t.Fatalf("provider issue calls = %d, want 1", got)
	}
	if rec := f.get(id, 1); rec.State != StateIssued {
		t.Fatalf("state = %q, want issued", rec.State)
	}
}

// TestProviderFailureRetriesAndOrderStaysPaid: a transient failure leaves the
// record pending with a later attempt time, and the billing order stays paid.
// The next due run issues it.
func TestProviderFailureRetriesAndOrderStaysPaid(t *testing.T) {
	f := newIssueFixture(t)
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(8)
	f.mustEnqueue(s)

	f.fake.FailNextIssue(&ProviderError{Kind: KindTransient, Code: "HTTP_503"}, false)
	f.mustIssue(s)

	rec := f.get(id, 1)
	if rec.State != StatePending || rec.Attempts != 1 || rec.LastErrorCode != "HTTP_503" {
		t.Fatalf("after failure: state %q attempts %d last error %q", rec.State, rec.Attempts, rec.LastErrorCode)
	}
	if !rec.NextAttemptAt.After(time.Now().Add(30 * time.Second)) {
		t.Fatalf("next attempt %v is not after the first backoff", rec.NextAttemptAt)
	}
	if st := f.orderStatus(id); st != "paid" {
		t.Fatalf("order status = %q, want paid", st)
	}

	f.makeDue(id, 1)
	f.mustIssue(s)
	if rec := f.get(id, 1); rec.State != StateIssued {
		t.Fatalf("after retry: state = %q, want issued", rec.State)
	}
}

// TestMissingFieldsMakeNeedsInfoWithNoProviderCall: a buyer without a required
// field becomes needs_info with the field name, and the provider is not called.
// The armed failure would be consumed by any call and so would show in the count.
func TestMissingFieldsMakeNeedsInfoWithNoProviderCall(t *testing.T) {
	f := newIssueFixture(t)
	const noTaxCode = `{"name":"Cong ty A","address":"1 Pho A","email":"a@example.com"}`
	id := f.paidOrder(noTaxCode)
	s := f.service(8)
	f.mustEnqueue(s)

	f.fake.FailNextIssue(&ProviderError{Kind: KindTransient, Code: "HTTP_503"}, false)
	f.mustIssue(s)

	rec := f.get(id, 1)
	if rec.State != StateNeedsInfo {
		t.Fatalf("state = %q, want needs_info", rec.State)
	}
	if len(rec.MissingFields) != 1 || rec.MissingFields[0] != "tax_code" {
		t.Fatalf("missing fields = %v, want [tax_code]", rec.MissingFields)
	}
	if got := f.fake.IssueCalls(); got != 0 {
		t.Fatalf("provider issue calls = %d, want 0", got)
	}
}

// TestExhaustedAttemptsBecomeFailed: when the attempts reach the configured
// maximum, a transient failure makes the record failed and the order stays paid.
func TestExhaustedAttemptsBecomeFailed(t *testing.T) {
	f := newIssueFixture(t)
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(2)
	f.mustEnqueue(s)

	f.fake.FailNextIssue(&ProviderError{Kind: KindTransient, Code: "HTTP_503"}, false)
	f.mustIssue(s)
	f.makeDue(id, 1)
	f.fake.FailNextIssue(&ProviderError{Kind: KindTransient, Code: "HTTP_503"}, false)
	f.mustIssue(s)

	rec := f.get(id, 1)
	if rec.State != StateFailed || rec.Attempts != 2 {
		t.Fatalf("state %q attempts %d, want failed after 2", rec.State, rec.Attempts)
	}
	if st := f.orderStatus(id); st != "paid" {
		t.Fatalf("order status = %q, want paid", st)
	}
}

// TestPermanentJobErrorFailsRecord: a permanent rejection makes the record
// failed on the first attempt.
func TestPermanentJobErrorFailsRecord(t *testing.T) {
	f := newIssueFixture(t)
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(8)
	f.mustEnqueue(s)

	f.fake.FailNextIssue(&ProviderError{Kind: KindPermanent, Code: "EINVOICE_BAD_DATA"}, false)
	f.mustIssue(s)

	rec := f.get(id, 1)
	if rec.State != StateFailed || rec.Attempts != 1 || rec.LastErrorCode != "EINVOICE_BAD_DATA" {
		t.Fatalf("state %q attempts %d last error %q, want failed on attempt 1", rec.State, rec.Attempts, rec.LastErrorCode)
	}
}

// TestJobLooksUpBeforeReissue: after an unknown outcome the invoice already
// exists at the provider. The retry looks it up by reference code and does not
// issue a second one.
func TestJobLooksUpBeforeReissue(t *testing.T) {
	f := newIssueFixture(t)
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(8)
	f.mustEnqueue(s)

	f.fake.FailNextIssue(&ProviderError{Kind: KindUnknownOutcome, Code: "TIMEOUT"}, true)
	f.mustIssue(s)
	if rec := f.get(id, 1); rec.State != StatePending {
		t.Fatalf("after timeout: state = %q, want pending", rec.State)
	}

	f.makeDue(id, 1)
	f.mustIssue(s)
	rec := f.get(id, 1)
	if rec.State != StateIssued || rec.InvoiceNumber == "" {
		t.Fatalf("after retry: state %q number %q, want issued with a number", rec.State, rec.InvoiceNumber)
	}
	if got := f.fake.IssueCalls(); got != 1 {
		t.Fatalf("provider issue calls = %d, want 1 (lookup must come first)", got)
	}
}

// TestRetryAfterTimeoutLooksUpBeforeReissue: a record that failed after a
// timeout may already exist at the provider. A staff retry must keep the
// attempt count, so the next run looks the invoice up by reference code first
// and does not issue a second one (spec 7.3 rule 5, AC58).
func TestRetryAfterTimeoutLooksUpBeforeReissue(t *testing.T) {
	f := newIssueFixture(t)
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(1)
	f.mustEnqueue(s)

	f.fake.FailNextIssue(&ProviderError{Kind: KindUnknownOutcome, Code: "TIMEOUT"}, true)
	f.mustIssue(s)
	if rec := f.get(id, 1); rec.State != StateFailed {
		t.Fatalf("after timeout with one attempt: state = %q, want failed", rec.State)
	}

	var staff string
	if err := f.pool.QueryRow(context.Background(), `SELECT gen_random_uuid()::text`).Scan(&staff); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Retry(context.Background(), id, 1, staff); err != nil {
		t.Fatalf("retry: %v", err)
	}

	f.makeDue(id, 1)
	f.mustIssue(s)
	rec := f.get(id, 1)
	if rec.State != StateIssued || rec.InvoiceNumber == "" {
		t.Fatalf("after retry: state %q number %q, want issued with a number", rec.State, rec.InvoiceNumber)
	}
	if got := f.fake.IssueCalls(); got != 1 {
		t.Fatalf("provider issue calls = %d, want 1 (retry must look up before reissue)", got)
	}
}

// TestIssueSendsReferenceCodeFromOrderAndRevision: the client reference code is
// order_id and revision, and the amount, currency and buyer are forwarded.
func TestIssueSendsReferenceCodeFromOrderAndRevision(t *testing.T) {
	f := newIssueFixture(t)
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(8)
	f.mustEnqueue(s)
	f.mustIssue(s)

	got := f.fake.LastIssue()
	if got.ReferenceCode != ReferenceCode(id, 1) {
		t.Fatalf("reference code = %q, want %q", got.ReferenceCode, ReferenceCode(id, 1))
	}
	if got.Amount != 150000 || got.Currency != "VND" || got.Buyer.TaxCode != "0312345678" {
		t.Fatalf("request = amount %d currency %q tax code %q", got.Amount, got.Currency, got.Buyer.TaxCode)
	}
}

// TestEinvoiceOffWhenProviderEmpty: with no provider configured the jobs do
// nothing and create no record (spec 7.3 rule 9).
func TestEinvoiceOffWhenProviderEmpty(t *testing.T) {
	f := newIssueFixture(t)
	ctx := context.Background()
	id := f.paidOrder(fullBuyerJSON)
	s := NewService(f.store, nil, Config{StartAt: f.start, MaxAttempts: 8})

	if _, err := s.Enqueue(ctx); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := s.Issue(ctx); err != nil {
		t.Fatalf("issue: %v", err)
	}
	if n := f.countRows(id); n != 0 {
		t.Fatalf("records = %d, want 0 when the provider is off", n)
	}
}
