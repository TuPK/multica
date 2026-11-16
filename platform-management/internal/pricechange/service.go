package pricechange

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/lifecycle"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

const (
	targetPlan  = "plan"
	systemLabel = "system: " + JobName
)

// Deps are the service's dependencies. Mail may be nil: no notice is sent. Cache
// may be nil. Log defaults to slog.Default.
type Deps struct {
	Store    Store
	Mail     lifecycle.Sender
	Cache    Invalidator
	Settings Settings
	AppURL   string
	Log      *slog.Logger
}

// Service schedules, cancels and applies price changes.
type Service struct{ d Deps }

// New returns a Service.
func New(d Deps) (*Service, error) {
	if d.Store == nil {
		return nil, errors.New("pricechange: a store is required")
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	d.Settings = d.Settings.withDefaults()
	return &Service{d: d}, nil
}

func notFound(code, msg string) *Error {
	return &Error{Status: http.StatusNotFound, Code: code, Message: msg}
}

func conflict(code, msg string, detail map[string]any) *Error {
	return &Error{Status: http.StatusConflict, Code: code, Message: msg, Detail: detail}
}

func (a Actor) entry(e audit.Entry) audit.Entry {
	if a == (Actor{}) {
		e.ActorType, e.ActorLabel = audit.ActorSystem, systemLabel
		return e
	}
	e.ActorType, e.ActorID, e.ActorLabel = audit.ActorStaff, a.ID, a.Label
	return e
}

// record writes the audit row of a schedule, apply or cancel: the plan, the
// effective date, the old and new prices and the reason (spec 7b.4 rule 15). The
// plan's key is the row's target; the detail names it "plan" because the audit
// filter hides every detail key that contains "key".
func record(ctx context.Context, tx Tx, a Actor, action, planName string, c Change) error {
	return audit.Record(ctx, tx, a.entry(audit.Entry{
		Action: action, TargetType: targetPlan, TargetID: c.PlanKey, TargetLabel: planName,
		Detail: map[string]any{
			"plan": c.PlanKey, "effective_from": c.EffectiveFrom.UTC().Format(time.RFC3339),
			"old": c.Old, "new": c.New, "reason": c.Reason,
		},
	}))
}

// applyIn applies a change inside tx, whose plan row the caller has locked. It
// writes nothing and returns false when the change is not scheduled and due.
func applyIn(ctx context.Context, tx Tx, a Actor, p PlanRow, c Change, now time.Time) (Change, bool, error) {
	got, ok, err := tx.MarkApplied(ctx, c.ID, now)
	if err != nil || !ok {
		return Change{}, false, err
	}
	if err := tx.SetPrices(ctx, c.PlanKey, got.New); err != nil {
		return Change{}, false, err
	}
	return got, true, record(ctx, tx, a, audit.ActionPlanPriceApplied, p.Name, got)
}

// Schedule stores a price change for the plan, or with dryRun only reports its
// impact. A change that needs no notice and is already due takes effect in the
// call (spec 7b.4 rules 7 and 8).
func (s *Service) Schedule(ctx context.Context, a Actor, key string, req Request, dryRun bool, now time.Time) (Result, error) {
	if err := plan.ValidateKey(key); err != nil {
		return Result{}, notFound("plan_not_found", "no such plan")
	}
	if e := validateRequest(req); e != nil {
		return Result{}, e
	}
	paid, err := s.d.Store.PaidWorkspaces(ctx, key)
	if err != nil {
		return Result{}, err
	}

	var res Result
	err = s.d.Store.InTx(ctx, func(tx Tx) error {
		res = Result{Affected: len(paid)}
		p, found, err := tx.LockPlan(ctx, key)
		if err != nil {
			return err
		}
		if !found {
			return notFound("plan_not_found", "no such plan")
		}
		if _, pending, err := tx.Scheduled(ctx, key); err != nil {
			return err
		} else if pending {
			return conflict("price_change_pending", "the plan already has a scheduled price change", nil)
		}
		old, verr := validatePrices(p, req, s.d.Settings.Max)
		if verr != nil {
			return verr
		}
		effective := req.EffectiveFrom.UTC()
		if touchesUsage(req.New) && (!atMidnightUTC(effective) || effective.Before(now)) {
			return invalid("effective_from", "a change to usage_rates or usage_terms starts at 00:00 UTC, not in the past")
		}
		if pct := changePct(old, req.New); pct > float64(s.d.Settings.ConfirmPct) && !req.ConfirmLargeChange {
			return conflict("price_change_confirm_required", "the change is large; confirm it with confirm_large_change",
				map[string]any{"change_pct": pct})
		}

		notice := isIncrease(old, req.New) && len(paid) > 0
		if notice {
			min := now.Add(time.Duration(s.d.Settings.NoticeDays) * 24 * time.Hour)
			if effective.Before(min) {
				return conflict("price_notice_too_short", "a price increase needs notice before it takes effect",
					map[string]any{"min_effective_from": min.UTC().Format(time.RFC3339)})
			}
			res.NoticeDate = now.UTC().Format(time.DateOnly)
		} else if effective.Before(now) {
			effective = now
		}
		if dryRun {
			return nil
		}

		c, err := tx.Insert(ctx, Change{
			PlanKey: key, EffectiveFrom: effective, Old: old, New: req.New, Reason: req.Reason, CreatedBy: a.ID,
		})
		if errors.Is(err, ErrPending) {
			return conflict("price_change_pending", "the plan already has a scheduled price change", nil)
		}
		if err != nil {
			return err
		}
		if err := record(ctx, tx, a, audit.ActionPlanPriceScheduled, p.Name, c); err != nil {
			return err
		}
		res.Change = c
		if !effective.After(now) {
			got, applied, err := applyIn(ctx, tx, a, p, c, now)
			if err != nil {
				return err
			}
			if applied {
				res.Change, res.Applied = got, true
			}
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	if res.Applied && s.d.Cache != nil {
		s.d.Cache.InvalidateAll()
	}
	return res, nil
}

// Cancel cancels the plan's scheduled change. When its notice has gone out, the
// plan's subscribers get one correction.
func (s *Service) Cancel(ctx context.Context, a Actor, key string, now time.Time) (Change, error) {
	if err := plan.ValidateKey(key); err != nil {
		return Change{}, notFound("price_change_not_found", "the plan has no scheduled price change")
	}
	var c Change
	err := s.d.Store.InTx(ctx, func(tx Tx) error {
		p, found, err := tx.LockPlan(ctx, key)
		if err != nil {
			return err
		}
		if !found {
			return notFound("plan_not_found", "no such plan")
		}
		var ok bool
		if c, ok, err = tx.Cancel(ctx, key, a.ID, now); err != nil {
			return err
		}
		if !ok {
			return notFound("price_change_not_found", "the plan has no scheduled price change")
		}
		return record(ctx, tx, a, audit.ActionPlanPriceCanceled, p.Name, c)
	})
	if err != nil {
		return Change{}, err
	}
	if c.NoticeSentAt != nil {
		// The cancel is committed; a correction that fails is logged, and a
		// subscriber who got the notice may not get the correction.
		if err := s.announce(ctx, c, KindPriceChangeCanceled); err != nil {
			s.d.Log.Error("pricechange: correction not sent", "plan", key, "error", err)
		}
	}
	return c, nil
}

// History returns the plan's changes, newest first.
func (s *Service) History(ctx context.Context, key string) ([]Change, error) {
	if err := plan.ValidateKey(key); err != nil {
		return []Change{}, nil
	}
	out, err := s.d.Store.History(ctx, key)
	if out == nil {
		out = []Change{}
	}
	return out, err
}

// Run applies the changes due at now, then sends the notices that are due. A
// failure of one change does not stop the others; the errors are joined.
func (s *Service) Run(ctx context.Context, now time.Time) error {
	var errs []error
	due, err := s.d.Store.Due(ctx, now, BatchSize)
	if err != nil {
		return fmt.Errorf("list due price changes: %w", err)
	}
	applied := false
	for _, c := range due {
		ok, err := s.applyDue(ctx, c, now)
		if err != nil {
			errs = append(errs, fmt.Errorf("apply price change %s of plan %s: %w", c.ID, c.PlanKey, err))
			continue
		}
		applied = applied || ok
	}
	if applied && s.d.Cache != nil {
		s.d.Cache.InvalidateAll()
	}
	if s.d.Mail != nil {
		if err := s.sendNotices(ctx, now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// applyDue applies one change in its own transaction: the plan row lock first,
// then the conditional move of the change, so a second or parallel run changes
// nothing.
func (s *Service) applyDue(ctx context.Context, c Change, now time.Time) (applied bool, err error) {
	err = s.d.Store.InTx(ctx, func(tx Tx) error {
		p, found, err := tx.LockPlan(ctx, c.PlanKey)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("the plan does not exist")
		}
		_, applied, err = applyIn(ctx, tx, Actor{}, p, c, now)
		return err
	})
	return applied, err
}

// Job returns Run as a job function for internal/jobs.
func (s *Service) Job(now func() time.Time) func(context.Context) error {
	return func(ctx context.Context) error { return s.Run(ctx, now()) }
}

// PriceOnDay returns the prices in force at 00:00 UTC of day.
//
// TODO(Q9): the lookup of a price by day belongs to slice Q9 (spec 7b.4 rule 9d:
// the `old` of the earliest applied change whose effective_from is after that
// moment, else the plan's current prices). Q12 leaves the signature only.
func PriceOnDay(ctx context.Context, st Store, planKey string, day time.Time) (Prices, error) {
	return nil, errors.New("pricechange: PriceOnDay is slice Q9's")
}
