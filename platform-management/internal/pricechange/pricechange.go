// Package pricechange schedules, applies and cancels a plan's price changes
// (spec 7b.4, decision D46). A plan's prices change only through a
// plan_price_change row with an effective date; the background job
// plan_price_apply applies a due row once, and tells the plan's subscribers by
// email before an increase.
//
// Every write is one transaction that locks the plan row first, so two admins,
// or two job runs, serialise on the plan. The caller passes the time; nothing
// here reads the clock.
package pricechange

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/lifecycle"
)

const (
	// JobName is the job's name in internal/jobs; it is the lock key.
	JobName = "plan_price_apply"
	// Interval is how often the job runs.
	Interval = time.Hour
	// BatchSize bounds the changes one run applies or announces.
	BatchSize = 100
)

// States of a plan_price_change row.
const (
	StateScheduled = "scheduled"
	StateApplied   = "applied"
	StateCanceled  = "canceled"
)

// The price fields of a plan: the only fields a change may carry.
const (
	FieldSeatPriceMonth = "seat_price_month"
	FieldSeatPriceYear  = "seat_price_year"
	FieldUsageRates     = "usage_rates"
	FieldUsageTerms     = "usage_terms"
)

// PriceFields lists the price fields.
var PriceFields = []string{FieldSeatPriceMonth, FieldSeatPriceYear, FieldUsageRates, FieldUsageTerms}

// Kinds of billing_notice written here.
const (
	KindPriceChange         = "price_change"
	KindPriceChangeCanceled = "price_change_canceled"
)

// ErrPending is what Tx.Insert returns when the plan already has a scheduled
// change.
var ErrPending = errors.New("pricechange: the plan already has a scheduled change")

// Prices maps a price field to its JSON value. A seat price is a number or
// null; the usage fields are objects.
type Prices map[string]json.RawMessage

// Settings are the limits of a change (spec 7b.4 rules 5 and 7, section 5.1).
// A zero field takes its default.
type Settings struct {
	// NoticeDays is PLATFORM_PRICE_NOTICE_DAYS.
	NoticeDays int
	// ConfirmPct is PLATFORM_PRICE_CONFIRM_PCT.
	ConfirmPct int
	// Max is PLATFORM_PRICE_MAX: the largest price of one unit.
	Max int64
}

// Default values of Settings.
const (
	DefaultNoticeDays       = 30
	DefaultConfirmPct       = 50
	DefaultMax        int64 = 1_000_000_000
)

func (s Settings) withDefaults() Settings {
	if s.NoticeDays == 0 {
		s.NoticeDays = DefaultNoticeDays
	}
	if s.ConfirmPct == 0 {
		s.ConfirmPct = DefaultConfirmPct
	}
	if s.Max == 0 {
		s.Max = DefaultMax
	}
	return s
}

// Change is one row of plan_price_change.
type Change struct {
	ID            string     `json:"id"`
	PlanKey       string     `json:"plan_key"`
	State         string     `json:"state"`
	EffectiveFrom time.Time  `json:"effective_from"`
	Old           Prices     `json:"old"`
	New           Prices     `json:"new"`
	Reason        string     `json:"reason"`
	NoticeSentAt  *time.Time `json:"notice_sent_at"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	AppliedAt     *time.Time `json:"applied_at"`
	CanceledBy    string     `json:"canceled_by"`
	CanceledAt    *time.Time `json:"canceled_at"`
}

// PlanRow is the part of a plan row a change reads. Prices holds all four
// price fields.
type PlanRow struct {
	Key, Name, BillingModel string
	Prices                  Prices
	Version                 int64
}

// Workspace is a workspace with a paid subscription (active or past_due) on a
// plan.
type Workspace struct {
	ID              string
	PendingDeletion bool
}

// Tx is the writes of one operation, inside one transaction with its audit row.
type Tx interface {
	// LockPlan reads the plan row and locks it.
	LockPlan(ctx context.Context, key string) (PlanRow, bool, error)
	// Scheduled returns the plan's scheduled change.
	Scheduled(ctx context.Context, key string) (Change, bool, error)
	// Insert stores a scheduled change. It returns ErrPending when the plan
	// has one.
	Insert(ctx context.Context, c Change) (Change, error)
	// MarkApplied moves the change to applied when it is still scheduled and
	// its effective_from is at or before now. found == false, and nothing
	// written, otherwise.
	MarkApplied(ctx context.Context, id string, now time.Time) (Change, bool, error)
	// SetPrices writes the given price fields to the plan and moves its version
	// up by one.
	SetPrices(ctx context.Context, key string, p Prices) error
	// Cancel moves the plan's scheduled change to canceled. found == false when
	// there is none.
	Cancel(ctx context.Context, key, by string, now time.Time) (Change, bool, error)

	audit.Writer
}

// Store is everything the service reads and writes.
type Store interface {
	// InTx runs fn in one transaction: it commits when fn returns nil and rolls
	// back otherwise.
	InTx(ctx context.Context, fn func(Tx) error) error
	// PaidWorkspaces lists the workspaces whose subscription on the plan is
	// active or past_due.
	PaidWorkspaces(ctx context.Context, planKey string) ([]Workspace, error)
	// History returns the plan's changes, newest first.
	History(ctx context.Context, planKey string) ([]Change, error)
	// Due returns up to limit scheduled changes whose effective_from is at or
	// before now, the oldest first.
	Due(ctx context.Context, now time.Time, limit int) ([]Change, error)
	// NoticeDue returns up to limit scheduled changes whose notice has not been
	// sent.
	NoticeDue(ctx context.Context, limit int) ([]Change, error)
	// MarkNoticeSent records that every subscriber was told.
	MarkNoticeSent(ctx context.Context, id string, at time.Time) error
	// Contact returns the workspace's name, slug and owners' addresses.
	Contact(ctx context.Context, workspaceID string) (lifecycle.Contact, error)
	// InsertNotice records a billing_notice row and reports whether it is new.
	InsertNotice(ctx context.Context, n lifecycle.Notice) (bool, error)
	DeleteNotice(ctx context.Context, workspaceID, key string) error
}

// Invalidator clears the process's plan policy cache.
type Invalidator interface{ InvalidateAll() }

// Actor is the staff member behind a request. The job has none.
type Actor struct{ ID, Label string }

// Request is a staff member's price change.
type Request struct {
	EffectiveFrom      time.Time `json:"effective_from"`
	New                Prices    `json:"new"`
	Reason             string    `json:"reason"`
	ConfirmLargeChange bool      `json:"confirm_large_change"`
}

// Result is what Schedule did.
type Result struct {
	// Change is the stored change; the zero value for a dry run.
	Change Change `json:"price_change"`
	// Applied is true when the change took effect in the request.
	Applied bool `json:"applied"`
	// Affected is the number of paid subscriptions on the plan.
	Affected int `json:"affected_subscribers"`
	// NoticeDate is the UTC date the notice goes out, "" when none goes out.
	NoticeDate string `json:"notice_date"`
}

// Error is a refusal the caller sees: an HTTP status, a stable code and a
// message. Detail holds extra fields of the JSON error body. Anything else a
// Service returns is an internal failure.
type Error struct {
	Status  int
	Code    string
	Message string
	Detail  map[string]any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
