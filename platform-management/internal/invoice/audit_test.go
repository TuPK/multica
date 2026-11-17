package invoice

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Audit tests (spec 7.3 "Kiểm toán", rule I7): each enqueue, issue, replace and
// failed outcome writes one audit row in the transaction of the status change.
// Rows are found by the order id the test created, never by table totals.

const (
	auditRequested = "einvoice.requested"
	auditIssued    = "einvoice.issued"
	auditNeedsInfo = "einvoice.needs_info"
	auditFailed    = "einvoice.failed"
	auditReplaced  = "einvoice.replaced"
)

type auditRow struct {
	WorkspaceID string
	ActorType   string
	ActorID     string
	TargetType  string
	Detail      map[string]any
	Raw         string
}

// auditRows returns the audit rows of one action for an order.
func (f *issueFixture) auditRows(orderID, action string) []auditRow {
	f.t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT COALESCE(workspace_id::text, ''), actor_type,
		COALESCE(actor_id, ''), target_type, detail::text FROM audit_event
		WHERE target_id = $1 AND action = $2 ORDER BY created_at`, orderID, action)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		if err := rows.Scan(&r.WorkspaceID, &r.ActorType, &r.ActorID, &r.TargetType, &r.Raw); err != nil {
			f.t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(r.Raw), &r.Detail); err != nil {
			f.t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *issueFixture) wantOneAudit(orderID, action string) auditRow {
	f.t.Helper()
	rows := f.auditRows(orderID, action)
	if len(rows) != 1 {
		f.t.Fatalf("%s rows for order %s = %d, want 1", action, orderID, len(rows))
	}
	r := rows[0]
	if r.WorkspaceID != f.wsID || r.TargetType != "einvoice" {
		f.t.Fatalf("%s row workspace %q target type %q, want %q and einvoice", action, r.WorkspaceID, r.TargetType, f.wsID)
	}
	return r
}

// noBuyerValues fails when a row holds any value of fullBuyerJSON.
func noBuyerValues(t *testing.T, r auditRow) {
	t.Helper()
	for _, v := range []string{"Cong ty A", "0312345678", "1 Pho A", "a@example.com"} {
		if strings.Contains(r.Raw, v) {
			t.Fatalf("audit detail holds a buyer value: %s", r.Raw)
		}
	}
}

func TestEnqueueWritesOneRequestedAuditRowPerRecord(t *testing.T) {
	f := newIssueFixture(t)
	a, b := f.paidOrder(fullBuyerJSON), f.paidOrder(fullBuyerJSON)
	s := f.service(8)

	f.mustEnqueue(s)
	for _, id := range []string{a, b} {
		r := f.wantOneAudit(id, auditRequested)
		if r.ActorType != "system" {
			t.Fatalf("actor type = %q, want system", r.ActorType)
		}
		noBuyerValues(t, r)
	}

	// A second run finds no order without a record and writes nothing.
	f.mustEnqueue(s)
	for _, id := range []string{a, b} {
		f.wantOneAudit(id, auditRequested)
	}
}

func TestIssuedWritesOneAuditRowInTheSameTransaction(t *testing.T) {
	f := newIssueFixture(t)
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(8)
	f.mustEnqueue(s)
	f.mustIssue(s)

	rec := f.get(id, 1)
	if rec.State != StateIssued {
		t.Fatalf("state = %q, want issued", rec.State)
	}
	r := f.wantOneAudit(id, auditIssued)
	if r.ActorType != "system" {
		t.Fatalf("actor type = %q, want system", r.ActorType)
	}
	if r.Detail["invoice_number"] != rec.InvoiceNumber || r.Detail["provider_invoice_id"] != rec.ProviderInvoiceID || r.Detail["revision"] != float64(1) {
		t.Fatalf("detail = %s, want number %q provider id %q revision 1", r.Raw, rec.InvoiceNumber, rec.ProviderInvoiceID)
	}
	noBuyerValues(t, r)
}

func TestFailedAttemptWritesFailedAuditRow(t *testing.T) {
	f := newIssueFixture(t)
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(8)
	f.mustEnqueue(s)

	f.fake.FailNextIssue(&ProviderError{Kind: KindPermanent, Code: "EINVOICE_BAD_DATA"}, false)
	f.mustIssue(s)

	if st := f.get(id, 1).State; st != StateFailed {
		t.Fatalf("state = %q, want failed", st)
	}
	r := f.wantOneAudit(id, auditFailed)
	if r.ActorType != "system" || r.Detail["error_code"] != "EINVOICE_BAD_DATA" || r.Detail["attempts"] != float64(1) {
		t.Fatalf("actor %q detail %s, want system with error_code EINVOICE_BAD_DATA and attempts 1", r.ActorType, r.Raw)
	}
	noBuyerValues(t, r)
}

func TestMissingFieldsWriteNeedsInfoAuditRowWithFieldNamesOnly(t *testing.T) {
	f := newIssueFixture(t)
	const noTaxCode = `{"name":"Cong ty A","address":"1 Pho A","email":"a@example.com"}`
	id := f.paidOrder(noTaxCode)
	s := f.service(8)
	f.mustEnqueue(s)
	f.mustIssue(s)

	if st := f.get(id, 1).State; st != StateNeedsInfo {
		t.Fatalf("state = %q, want needs_info", st)
	}
	if got := f.fake.IssueCalls(); got != 0 {
		t.Fatalf("provider issue calls = %d, want 0", got)
	}
	r := f.wantOneAudit(id, auditNeedsInfo)
	fields, _ := r.Detail["missing_fields"].([]any)
	if len(fields) != 1 || fields[0] != "tax_code" {
		t.Fatalf("detail = %s, want missing_fields [tax_code]", r.Raw)
	}
	noBuyerValues(t, r)
	if len(f.auditRows(id, auditFailed)) != 0 {
		t.Fatal("needs_info also wrote a failed row")
	}
}

func TestRetryableErrorWritesNoAuditRow(t *testing.T) {
	f := newIssueFixture(t)
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(8)
	f.mustEnqueue(s)

	f.fake.FailNextIssue(&ProviderError{Kind: KindTransient, Code: "HTTP_503"}, false)
	f.mustIssue(s)

	if st := f.get(id, 1).State; st != StatePending {
		t.Fatalf("state = %q, want pending", st)
	}
	for _, action := range []string{auditFailed, auditNeedsInfo, auditIssued} {
		if n := len(f.auditRows(id, action)); n != 0 {
			t.Fatalf("%s rows = %d, want 0 for a retryable error", action, n)
		}
	}
}

func TestReplaceWritesOneAuditRowAndRollsBackWithTheChange(t *testing.T) {
	f := newIssueFixture(t)
	ctx := context.Background()
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(8)
	f.mustEnqueue(s)
	f.mustIssue(s)

	var staffID string
	if err := f.pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&staffID); err != nil {
		t.Fatal(err)
	}
	buyer := Buyer{Name: "Cong ty A", TaxCode: "0312345678", Address: "1 Pho A", Email: "new@example.com"}

	// An actor that is not a UUID is refused and changes nothing.
	if _, err := s.Replace(ctx, id, 1, buyer, "staff-1", "wrong email"); err == nil {
		t.Fatal("replace with a non-UUID actor succeeded")
	}
	if st := f.get(id, 1).State; st != StateIssued || f.countRows(id) != 1 {
		t.Fatalf("a refused replace changed the record: state %q rows %d", st, f.countRows(id))
	}

	if _, err := s.Replace(ctx, id, 1, buyer, staffID, "wrong email"); err != nil {
		t.Fatalf("replace: %v", err)
	}
	r := f.wantOneAudit(id, auditReplaced)
	if r.ActorType != "staff" || r.ActorID != staffID || r.Detail["reason"] != "wrong email" ||
		r.Detail["revision"] != float64(1) || r.Detail["new_revision"] != float64(2) {
		t.Fatalf("actor %q/%q detail %s, want staff %s, revisions 1 to 2 and the reason", r.ActorType, r.ActorID, r.Raw, staffID)
	}
	noBuyerValues(t, r)
	if strings.Contains(r.Raw, "new@example.com") {
		t.Fatalf("audit detail holds the new email: %s", r.Raw)
	}

	// The record is no longer issued: a second replace conflicts and writes nothing.
	if _, err := s.Replace(ctx, id, 1, buyer, staffID, "again"); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("second replace error = %v, want ErrStateConflict", err)
	}
	f.wantOneAudit(id, auditReplaced)
	if f.countRows(id) != 2 {
		t.Fatalf("rows = %d, want 2", f.countRows(id))
	}
}

func TestAuditRowRollsBackWhenTheStateChangeFails(t *testing.T) {
	f := newIssueFixture(t)
	ctx := context.Background()
	id := f.paidOrder(fullBuyerJSON)
	s := f.service(8)
	f.mustEnqueue(s)
	if _, err := f.pool.Exec(ctx, `UPDATE einvoice_issuance SET state = 'failed' WHERE order_id = $1`, id); err != nil {
		t.Fatal(err)
	}

	err := f.store.MarkIssued(ctx, id, 1, Issued{ProviderInvoiceID: "p1", InvoiceNumber: "N1"})
	if !errors.Is(err, ErrStateConflict) {
		t.Fatalf("MarkIssued error = %v, want ErrStateConflict", err)
	}
	if st := f.get(id, 1).State; st != StateFailed {
		t.Fatalf("state = %q, want failed", st)
	}
	if n := len(f.auditRows(id, auditIssued)); n != 0 {
		t.Fatalf("issued rows = %d, want 0", n)
	}

	err = f.store.RecordAttempt(ctx, id, 1, Attempt{State: StateFailed, LastErrorCode: "X"})
	if !errors.Is(err, ErrStateConflict) {
		t.Fatalf("RecordAttempt error = %v, want ErrStateConflict", err)
	}
	if n := len(f.auditRows(id, auditFailed)); n != 0 {
		t.Fatalf("failed rows = %d, want 0", n)
	}
}
