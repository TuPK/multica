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

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/lifecycle"
	"github.com/multica-ai/multica/platform-management/internal/pricechange"
)

// JobName is the usage_rating job's name in internal/jobs; it is the lock key.
const JobName = "usage_rating"

// Interval is how often the job looks for closed days to rate. A day is charged
// within an hour of closing, and a late snapshot row is charged on the next run.
const Interval = time.Hour

// batchSize bounds the days one query returns.
const batchSize = 200

// RaterConfig holds the settings the rating reads from the environment.
type RaterConfig struct {
	// RequireVerification is PLATFORM_BUSINESS_DISCOUNT_REQUIRES_VERIFICATION:
	// the Business discount applies only to a verified profile.
	RequireVerification bool
	// VATRate is BILLING_VAT_RATE, for the temporary invoice at the credit limit.
	VATRate billing.Rate
	// Mail sends the 80 percent notice. Nil sends none.
	Mail lifecycle.Sender
	// AppURL is the origin the notice links to, PLATFORM_APP_URL.
	AppURL string
}

// Rater rates closed days into usage_charge, and acts on the credit limit once
// a day is written (spec 7b.3, rule 12).
type Rater struct {
	pool    *pgxpool.Pool
	store   Store
	cfg     RaterConfig
	invoice *Invoicer
	notices *lifecycle.PgxStore
	prices  pricechange.Store
}

// NewRater returns a Rater over pool.
func NewRater(pool *pgxpool.Pool, cfg RaterConfig) *Rater {
	return &Rater{
		pool: pool, store: NewStore(pool), cfg: cfg,
		invoice: NewInvoicer(pool, InvoicerConfig{VATRate: cfg.VATRate}), notices: lifecycle.NewPgxStore(pool),
		prices: pricechange.NewPgxStore(pool),
	}
}

// Run rates every closed day (before the UTC day of now) that has a usage
// snapshot row and no charge rows yet, and returns how many charge rows it
// wrote. A rerun, or a run in parallel with another, writes nothing twice: the
// unique index and the insert's ON CONFLICT keep one row per workspace, day
// and meter.
func (r *Rater) Run(ctx context.Context, now time.Time) (int64, error) {
	today := midnightUTC(now)
	var total int64
	// A failing credit limit step must not stop the rating of other workspaces.
	var limitErrs []error
	for {
		batch, err := r.store.DaysToRate(ctx, today, batchSize)
		if err != nil {
			return total, fmt.Errorf("list days to rate: %w", err)
		}
		for _, c := range batch {
			n, err := r.rateDay(ctx, c)
			if err != nil {
				return total, fmt.Errorf("rate workspace %s day %s: %w", uuidText(c.WorkspaceID), c.Day.Time.Format(dayLayout), err)
			}
			total += n
			if n > 0 {
				if err := r.checkCreditLimit(ctx, c.WorkspaceID, now); err != nil {
					limitErrs = append(limitErrs, fmt.Errorf("credit limit of workspace %s: %w", uuidText(c.WorkspaceID), err))
				}
			}
		}
		// Every day in a full batch is charged now, so the next query starts
		// past them. A short batch is the last one.
		if len(batch) < batchSize {
			return total, errors.Join(limitErrs...)
		}
	}
}

// Job returns Run as a job function for internal/jobs.
func (r *Rater) Job(now func() time.Time) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := r.Run(ctx, now())
		return err
	}
}

// rateDay rates one candidate day from the plan, account and workspace facts
// the query read, and writes its rows.
func (r *Rater) rateDay(ctx context.Context, c db.ListUsageDaysToRateRow) (int64, error) {
	// The prices of the day, not of now: a change effective after 00:00 UTC of
	// the day does not touch it (spec 7b.4, rule 9d).
	prices, err := pricechange.PriceOnDay(ctx, r.prices, c.PlanKey, c.Day.Time)
	if err != nil {
		return 0, fmt.Errorf("plan %s prices on the day: %w", c.PlanKey, err)
	}
	var rates Rates
	if err := json.Unmarshal(prices[pricechange.FieldUsageRates], &rates); err != nil {
		return 0, fmt.Errorf("plan %s usage_rates: %w", c.PlanKey, err)
	}
	var terms Terms
	if err := json.Unmarshal(prices[pricechange.FieldUsageTerms], &terms); err != nil {
		return 0, fmt.Errorf("plan %s usage_terms: %w", c.PlanKey, err)
	}
	discount := BusinessDiscountPct(terms, c.AccountKind, c.Verified, r.cfg.RequireVerification)
	rows, err := RateDay(Input{
		Day:         c.Day.Time,
		PlanKey:     c.PlanKey,
		Rates:       rates,
		DiscountPct: discount,
		Status:      c.WorkspaceStatus,
		Quantities: map[string]int64{
			MeterTasksRun:     int64(c.TasksRun),
			MeterStorageBytes: c.StorageBytes,
			MeterMembers:      int64(c.Members),
		},
	})
	if err != nil {
		return 0, err
	}
	return r.store.WriteDay(ctx, c.WorkspaceID, c.Day.Time, rows)
}

func midnightUTC(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// noticePercent is the share of the credit limit at which the owners are told.
const noticePercent = 80

// checkCreditLimit reads what the workspace has used and not been invoiced for.
// At the credit limit it issues a temporary invoice for all of it, with no
// minimum. At 80 percent it emails the owners once in the month. The limit
// never blocks usage.
func (r *Rater) checkCreditLimit(ctx context.Context, ws pgtype.UUID, now time.Time) error {
	q := db.New(r.pool)
	terms, err := q.GetUsageTerms(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var t Terms
	if err := json.Unmarshal(terms.UsageTerms, &t); err != nil {
		return fmt.Errorf("plan %s usage_terms: %w", terms.PlanKey, err)
	}
	limit := t.CreditLimit[terms.AccountKind]
	if limit <= 0 {
		return nil
	}
	accrued, err := q.GetUsageAccrual(ctx, ws)
	if err != nil {
		return err
	}
	switch {
	case accrued >= limit:
		_, err := r.invoice.invoice(ctx, ws, now, limit)
		return err
	case accrued*100 >= limit*noticePercent && r.cfg.Mail != nil:
		return r.noticeEighty(ctx, ws, accrued, limit, now)
	}
	return nil
}
