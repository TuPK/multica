package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// ChargeRow is one usage_charge row of the customer's current month.
type ChargeRow struct {
	Day                                   time.Time
	Meter                                 string
	Quantity, UnitPrice, Per, DiscountPct int64
	NetAmount                             int64
}

// UsageInput is what BuildUsage assembles the customer's usage block from.
type UsageInput struct {
	Now      time.Time
	Currency string
	// Rates and DiscountPct price a meter that has no row yet this month.
	Rates       Rates
	DiscountPct int64
	// CreditLimit is 0 when the plan sets none.
	CreditLimit   int64
	Accrued       int64
	UnpaidOrderID string
	// Rows are the rows of the current UTC month, oldest day first.
	Rows []ChargeRow
}

// BuildUsage returns the usage block of spec 7b.3, rule 16, for the UTC month
// holding in.Now. A flow meter (tasks_run) adds up the month's rows; a stock
// meter shows the newest rated day's quantity. Amounts add up the month's rows.
// The price shown is the newest row's, or the plan's when the meter has none.
func BuildUsage(in UsageInput) subscription.Usage {
	start, end := subscription.UsagePeriod(in.Now)
	out := subscription.Usage{
		PeriodStart: start.Format(time.RFC3339), PeriodEnd: end.Format(time.RFC3339), Currency: in.Currency,
		AccruedAmount: in.Accrued, Meters: make([]subscription.UsageMeter, 0, len(Meters)),
	}
	if in.CreditLimit > 0 {
		out.CreditLimit = &in.CreditLimit
	}
	if in.UnpaidOrderID != "" {
		out.UnpaidOrderID = &in.UnpaidOrderID
	}
	for _, meter := range Meters {
		rate, ok := in.Rates[meter]
		if !ok {
			rate = Rate{Per: 1}
		}
		m := subscription.UsageMeter{Key: meter, UnitPrice: rate.Price, Per: rate.Per, DiscountPct: in.DiscountPct}
		for _, r := range in.Rows {
			if r.Meter != meter {
				continue
			}
			if meter == MeterTasksRun {
				m.Quantity += r.Quantity
			} else {
				m.Quantity = r.Quantity
			}
			m.Amount += r.NetAmount
			m.UnitPrice, m.Per, m.DiscountPct = r.UnitPrice, r.Per, r.DiscountPct
		}
		out.Meters = append(out.Meters, m)
	}
	return out
}

// View reads the customer's usage block over a Postgres pool.
type View struct {
	q                   *db.Queries
	requireVerification bool
}

// NewView returns a View over pool. requireVerification is
// PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION.
func NewView(pool db.DBTX, requireVerification bool) *View {
	return &View{q: db.New(pool), requireVerification: requireVerification}
}

// Usage returns the usage block of a workspace on a usage plan, as of now.
func (v *View) Usage(ctx context.Context, workspaceID string, now time.Time) (subscription.Usage, error) {
	ws, err := parseWorkspace(workspaceID)
	if err != nil {
		return subscription.Usage{}, err
	}
	p, err := v.q.GetUsageTerms(ctx, ws)
	if err != nil {
		return subscription.Usage{}, fmt.Errorf("read the plan's usage terms: %w", err)
	}
	var rates Rates
	var terms Terms
	if err := json.Unmarshal(p.UsageRates, &rates); err != nil {
		return subscription.Usage{}, fmt.Errorf("plan %s usage_rates: %w", p.PlanKey, err)
	}
	if err := json.Unmarshal(p.UsageTerms, &terms); err != nil {
		return subscription.Usage{}, fmt.Errorf("plan %s usage_terms: %w", p.PlanKey, err)
	}
	start, end := subscription.UsagePeriod(now)
	rows, err := v.q.ListUsageChargesInRange(ctx, db.ListUsageChargesInRangeParams{
		WorkspaceID: ws, Day: pgtype.Date{Time: start, Valid: true}, Day_2: pgtype.Date{Time: end, Valid: true},
	})
	if err != nil {
		return subscription.Usage{}, err
	}
	accrued, err := v.q.GetUsageAccrual(ctx, ws)
	if err != nil {
		return subscription.Usage{}, err
	}
	in := UsageInput{
		Now: now, Currency: p.Currency, Rates: rates, CreditLimit: terms.CreditLimit[p.AccountKind], Accrued: accrued,
		DiscountPct: BusinessDiscountPct(terms, p.AccountKind, p.Verified, v.requireVerification),
	}
	for _, r := range rows {
		in.Rows = append(in.Rows, ChargeRow{
			Day: r.Day.Time, Meter: r.Meter, Quantity: r.Quantity, UnitPrice: r.UnitPrice, Per: r.Per,
			DiscountPct: int64(r.DiscountPct), NetAmount: r.NetAmount,
		})
	}
	unpaid, err := v.q.GetUnpaidUsageOrderForWorkspace(ctx, ws)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return subscription.Usage{}, err
	}
	if err == nil {
		in.UnpaidOrderID = uuidText(unpaid.ID)
	}
	return BuildUsage(in), nil
}

// MonthRow is one usage_charge row in the console's usage tab. It holds counts
// and money only (isolation rule I6).
type MonthRow struct {
	Day         string  `json:"day"`
	Meter       string  `json:"meter"`
	Quantity    int64   `json:"quantity"`
	UnitPrice   int64   `json:"unit_price"`
	Per         int64   `json:"per"`
	DiscountPct int64   `json:"discount_pct"`
	NetAmount   int64   `json:"net_amount"`
	Note        string  `json:"note"`
	OrderID     *string `json:"order_id"`
}

// UnpaidOrder is a pending usage invoice of the console's usage tab.
type UnpaidOrder struct {
	ID          string  `json:"id"`
	Amount      int64   `json:"amount"`
	VATAmount   int64   `json:"vat_amount"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	PeriodStart *string `json:"period_start"`
	PeriodEnd   *string `json:"period_end"`
}

// Month is the console's usage tab of one workspace.
type Month struct {
	WorkspaceID string `json:"workspace_id"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	// Rows and MissingDays are empty for a workspace that is not on a usage plan.
	Rows []MonthRow `json:"rows"`
	// MissingDays are the days from the start of the month to yesterday that
	// have no platform_usage_daily row.
	MissingDays  []string      `json:"missing_days"`
	UnpaidOrders []UnpaidOrder `json:"unpaid_orders"`
}

// Month returns the usage tab of a workspace for the UTC month holding now.
func (r *Reader) Month(ctx context.Context, workspaceID string, now time.Time) (Month, error) {
	ws, err := parseWorkspace(workspaceID)
	if err != nil {
		return Month{}, err
	}
	start, end := subscription.UsagePeriod(now)
	m := Month{
		WorkspaceID: workspaceID, PeriodStart: start.Format(time.RFC3339), PeriodEnd: end.Format(time.RFC3339),
		Rows: []MonthRow{}, MissingDays: []string{}, UnpaidOrders: []UnpaidOrder{},
	}
	orders, err := r.q.ListUnpaidUsageOrders(ctx, ws)
	if err != nil {
		return Month{}, err
	}
	for _, o := range orders {
		u := UnpaidOrder{
			ID: uuidText(o.ID), Amount: o.Amount, VATAmount: o.VatAmount, Status: o.Status,
			CreatedAt: o.CreatedAt.Time.UTC().Format(time.RFC3339Nano),
		}
		if o.PeriodStart.Valid {
			s := o.PeriodStart.Time.UTC().Format(time.RFC3339Nano)
			u.PeriodStart = &s
		}
		if o.PeriodEnd.Valid {
			s := o.PeriodEnd.Time.UTC().Format(time.RFC3339Nano)
			u.PeriodEnd = &s
		}
		m.UnpaidOrders = append(m.UnpaidOrders, u)
	}
	p, err := r.q.GetUsageTerms(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.BillingModel != plan.BillingUsage) {
		return m, nil
	}
	if err != nil {
		return Month{}, err
	}
	rows, err := r.q.ListUsageChargesInRange(ctx, db.ListUsageChargesInRangeParams{
		WorkspaceID: ws, Day: pgtype.Date{Time: start, Valid: true}, Day_2: pgtype.Date{Time: end, Valid: true},
	})
	if err != nil {
		return Month{}, err
	}
	for _, c := range rows {
		row := MonthRow{
			Day: c.Day.Time.UTC().Format(dayLayout), Meter: c.Meter, Quantity: c.Quantity, UnitPrice: c.UnitPrice,
			Per: c.Per, DiscountPct: int64(c.DiscountPct), NetAmount: c.NetAmount, Note: c.Note,
		}
		if c.OrderID.Valid {
			id := uuidText(c.OrderID)
			row.OrderID = &id
		}
		m.Rows = append(m.Rows, row)
	}
	if yesterday := midnightUTC(now).AddDate(0, 0, -1); !yesterday.Before(start) {
		days, err := r.q.ListMissingUsageDays(ctx, db.ListMissingUsageDaysParams{
			WorkspaceID: ws, Column2: pgtype.Date{Time: start, Valid: true}, Column3: pgtype.Date{Time: yesterday, Valid: true},
		})
		if err != nil {
			return Month{}, err
		}
		for _, d := range days {
			m.MissingDays = append(m.MissingDays, d.Time.UTC().Format(dayLayout))
		}
	}
	return m, nil
}

func parseWorkspace(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil || !u.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid workspace id %q", s)
	}
	return u, nil
}
