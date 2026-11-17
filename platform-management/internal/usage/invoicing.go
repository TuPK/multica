package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
)

// JobNameInvoicing is the usage_invoicing job's name in internal/jobs.
const JobNameInvoicing = "usage_invoicing"

// The kinds of usage invoice, as the billing.usage_invoiced audit row names them.
const (
	invoiceMonthly = "monthly"
	// invoiceFinal is the last invoice of a workspace that left its usage plan.
	invoiceFinal = "final"
	// invoiceCreditLimit is the temporary invoice issued at the credit limit.
	invoiceCreditLimit = "credit_limit"
)

// InvoicerConfig holds the settings the invoicing reads from the environment.
type InvoicerConfig struct {
	// VATRate is BILLING_VAT_RATE.
	VATRate billing.Rate
	// MinInvoice is PLATFORM_USAGE_MIN_INVOICE: a monthly invoice below it is
	// not issued and its rows carry over.
	MinInvoice int64
}

// Invoicer turns usage_charge rows that are not on an invoice into usage orders
// (spec 7b.3, rules 11 and 12).
type Invoicer struct {
	pool *pgxpool.Pool
	cfg  InvoicerConfig
}

// NewInvoicer returns an Invoicer over pool.
func NewInvoicer(pool *pgxpool.Pool, cfg InvoicerConfig) *Invoicer {
	return &Invoicer{pool: pool, cfg: cfg}
}

// Run issues, for each workspace with uninvoiced rows, the monthly invoice of
// the days before the current UTC month (a usage plan) or the final invoice (a
// workspace that left its usage plan), and returns how many it issued. It
// continues past a failing workspace and returns the failures joined.
//
// Each invoice is made under a lock on the workspace's subscription row, so a
// rerun, or a run in parallel with another, finds the rows already invoiced and
// issues nothing twice.
func (i *Invoicer) Run(ctx context.Context, now time.Time) (int, error) {
	ids, err := db.New(i.pool).ListUsageInvoiceWorkspaces(ctx, pgtype.Date{Time: monthStart(now), Valid: true})
	if err != nil {
		return 0, fmt.Errorf("list workspaces to invoice: %w", err)
	}
	issued := 0
	var errs []error
	for _, ws := range ids {
		if err := ctx.Err(); err != nil {
			return issued, errors.Join(append(errs, err)...)
		}
		ok, err := i.invoice(ctx, ws, now, 0)
		if err != nil {
			errs = append(errs, fmt.Errorf("invoice workspace %s: %w", uuidText(ws), err))
		}
		if ok {
			issued++
		}
	}
	return issued, errors.Join(errs...)
}

// Job returns Run as a job function for internal/jobs.
func (i *Invoicer) Job(now func() time.Time) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := i.Run(ctx, now())
		return err
	}
}

// invoice issues the workspace's invoice in one transaction and reports whether
// it did. A creditLimit above 0 makes it the temporary invoice of rule 12: all
// uninvoiced rows, no minimum.
func (i *Invoicer) invoice(ctx context.Context, ws pgtype.UUID, now time.Time, creditLimit int64) (bool, error) {
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)

	sub, err := q.LockUsageInvoiceSubscription(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	kind, before := invoiceFinal, pgtype.Date{}
	switch {
	case creditLimit > 0:
		kind = invoiceCreditLimit
	case sub.BillingModel == plan.BillingUsage:
		kind, before = invoiceMonthly, pgtype.Date{Time: monthStart(now), Valid: true}
	}
	rows, err := q.ListUninvoicedUsageCharges(ctx, db.ListUninvoicedUsageChargesParams{WorkspaceID: ws, Before: before})
	if err != nil {
		return false, err
	}
	var sum int64
	days := make([]pgtype.Date, len(rows))
	for n, r := range rows {
		sum += r.NetAmount
		days[n] = r.Day
	}
	if sum == 0 || (kind == invoiceMonthly && sum < i.cfg.MinInvoice) {
		return false, nil
	}

	buyer, err := buyerSnapshot(ctx, q, ws)
	if err != nil {
		return false, err
	}
	terms, err := q.GetUsageTerms(ctx, ws)
	if err != nil {
		return false, err
	}
	d, err := invoiceDraft(uuidText(ws), sub.PlanKey, terms.Currency, rows, i.cfg.VATRate, buyer)
	if err != nil {
		return false, err
	}
	code, err := billing.NewTransferCode()
	if err != nil {
		return false, err
	}
	order, err := q.CreateBillingOrder(ctx, db.CreateBillingOrderParams{
		WorkspaceID: ws, Kind: string(d.Kind), PlanKey: d.PlanKey, Interval: d.Interval, Seats: int32(d.Seats),
		Amount: d.Amount, VatAmount: d.VATAmount, Currency: d.Currency, Provider: billing.ProviderManual,
		TransferCode: code, IdempotencyKey: d.IdempotencyKey, Buyer: d.Buyer,
		PeriodStart: pgtype.Timestamptz{Time: *d.PeriodStart, Valid: true},
		PeriodEnd:   pgtype.Timestamptz{Time: *d.PeriodEnd, Valid: true},
		CreatedAt:   pgtype.Timestamptz{Time: now.UTC(), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("an order with the idempotency key %s exists already", d.IdempotencyKey)
	}
	if err != nil {
		return false, err
	}
	attached, err := q.AttachUsageCharges(ctx, db.AttachUsageChargesParams{OrderID: order.ID, WorkspaceID: ws, Days: days})
	if err != nil {
		return false, err
	}
	if attached != int64(len(rows)) {
		return false, fmt.Errorf("invoiced %d rows, expected %d", attached, len(rows))
	}

	w := workspacestatus.NewTx(tx)
	detail := map[string]any{
		"order_id": uuidText(order.ID), "kind": kind, "amount": d.Amount, "vat_amount": d.VATAmount, "lines": len(rows),
	}
	if creditLimit > 0 {
		if err := audit.Record(ctx, w, audit.Entry{
			WorkspaceID: uuidText(ws), ActorType: audit.ActorSystem, Action: audit.ActionBillingUsageCreditLimitReached,
			TargetType: "billing_order", TargetID: uuidText(order.ID),
			Detail: map[string]any{"order_id": uuidText(order.ID), "credit_limit": creditLimit, "accrued_amount": d.Amount},
		}); err != nil {
			return false, err
		}
	}
	if err := audit.Record(ctx, w, audit.Entry{
		WorkspaceID: uuidText(ws), ActorType: audit.ActorSystem, Action: audit.ActionBillingUsageInvoiced,
		TargetType: "billing_order", TargetID: uuidText(order.ID), Detail: detail,
	}); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// invoiceDraft is the order of one invoice: the sum of the rows and its VAT, a
// period of whole months from the earliest to the latest day, and a key named
// for the latest day, so that the same rows never make two orders. rows are
// oldest day first.
func invoiceDraft(workspaceID, planKey, currency string, rows []db.ListUninvoicedUsageChargesRow, vat billing.Rate, buyer json.RawMessage) (billing.Draft, error) {
	if len(rows) == 0 {
		return billing.Draft{}, errors.New("an invoice needs at least one row")
	}
	var sum int64
	for _, r := range rows {
		sum += r.NetAmount
	}
	vatAmount, _, err := billing.WithVAT(sum, vat)
	if err != nil {
		return billing.Draft{}, err
	}
	first, last := rows[0].Day.Time.UTC(), rows[len(rows)-1].Day.Time.UTC()
	start, end := monthStart(first), monthStart(last).AddDate(0, 1, 0)
	return billing.ValidateDraft(billing.Draft{
		WorkspaceID: workspaceID, Kind: billing.KindUsage, PlanKey: planKey, Interval: subscription.IntervalMonth,
		Amount: sum, VATAmount: vatAmount, Currency: currency, Buyer: buyer, PeriodStart: &start, PeriodEnd: &end,
		IdempotencyKey: "usage:" + workspaceID + ":" + last.Format(dayLayout),
	})
}

// buyerSnapshot is the billing profile as an order stores it, {} for a workspace
// with none.
func buyerSnapshot(ctx context.Context, q *db.Queries, ws pgtype.UUID) (json.RawMessage, error) {
	p, err := q.GetBillingProfile(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return json.RawMessage(`{}`), nil
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{
		"name": p.LegalName, "tax_code": p.TaxCode, "address": p.Address, "invoice_email": p.InvoiceEmail,
	})
}

func monthStart(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
}
