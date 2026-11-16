package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/db"
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
}

// Rater rates closed days into usage_charge.
type Rater struct {
	store Store
	cfg   RaterConfig
}

// NewRater returns a Rater over pool.
func NewRater(pool *pgxpool.Pool, cfg RaterConfig) *Rater {
	return &Rater{store: NewStore(pool), cfg: cfg}
}

// Run rates every closed day (before the UTC day of now) that has a usage
// snapshot row and no charge rows yet, and returns how many charge rows it
// wrote. A rerun, or a run in parallel with another, writes nothing twice: the
// unique index and the insert's ON CONFLICT keep one row per workspace, day
// and meter.
func (r *Rater) Run(ctx context.Context, now time.Time) (int64, error) {
	today := midnightUTC(now)
	var total int64
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
		}
		// Every day in a full batch is charged now, so the next query starts
		// past them. A short batch is the last one.
		if len(batch) < batchSize {
			return total, nil
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
	var rates Rates
	if err := json.Unmarshal(c.UsageRates, &rates); err != nil {
		return 0, fmt.Errorf("plan %s usage_rates: %w", c.PlanKey, err)
	}
	var terms Terms
	if err := json.Unmarshal(c.UsageTerms, &terms); err != nil {
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
