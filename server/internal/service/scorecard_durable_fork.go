package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// durableScorecardRead replaces the Phase 4 done rows of a scorecard period with
// the durable ones: the done time is the first transition into done in the period,
// and the inputs of the A5 metrics come from the transitions and outcomes. It runs
// only while durable_outcomes is on (ScorecardInputsFor).
func durableScorecardRead(ctx context.Context, q *db.Queries, ws pgtype.UUID, scope ScorecardScope, start, end time.Time, in *ScorecardInputs) error {
	ps := pgtype.Timestamptz{Time: start, Valid: true}
	pe := pgtype.Timestamptz{Time: end, Valid: true}

	doneRows, err := q.DurableScorecardDoneIssues(ctx, db.DurableScorecardDoneIssuesParams{
		WorkspaceID: ws, PeriodStart: ps, PeriodEnd: pe, AgentID: scope.AgentID, SquadID: scope.SquadID,
	})
	if err != nil {
		return fmt.Errorf("durable scorecard done issues: %w", err)
	}
	trRows, err := q.DurableScorecardTransitions(ctx, db.DurableScorecardTransitionsParams{
		WorkspaceID: ws, PeriodStart: ps, PeriodEnd: pe, AgentID: scope.AgentID, SquadID: scope.SquadID,
	})
	if err != nil {
		return fmt.Errorf("durable scorecard transitions: %w", err)
	}
	transitions := make([]durableTransition, 0, len(trRows))
	for _, r := range trRows {
		tr := durableTransition{
			IssueID:    util.UUIDToString(r.IssueID),
			ToCategory: LifecycleCategoryOf(r.ToCatalogCategory, r.ToStatusKey),
			At:         r.CreatedAt.Time,
		}
		if r.FromStatusKey.Valid {
			tr.FromCategory = LifecycleCategoryOf(r.FromCatalogCategory, r.FromStatusKey.String)
		}
		transitions = append(transitions, tr)
	}
	histories := durableHistories(transitions, start, end)

	var d ScorecardDurableInputs
	for _, h := range histories {
		d.DoneEntries += h.DoneEntries
		d.Reopens += h.Reopens
	}
	d.CycleDays = durableCycleDays(histories)

	ids := make([]pgtype.UUID, 0, len(doneRows))
	for _, r := range doneRows {
		doneAt := r.DoneAt.Time
		in.Done = append(in.Done, ScorecardIssueRow{
			CreatedAt:     r.CreatedAt.Time,
			DoneAt:        doneAt,
			ReopenCount:   int64(r.ReopenCount),
			CompletedRuns: r.CompletedRuns,
			HumanAfter:    r.LastMemberCommentAt.Valid && r.LastMemberCommentAt.Time.After(doneAt),
		})
		if !r.HasHistory {
			d.Unhistoried++
		}
		ids = append(ids, r.ID)
	}

	if len(ids) > 0 {
		outcomes, err := q.DurableScorecardOutcomes(ctx, db.DurableScorecardOutcomesParams{
			WorkspaceID: ws, PeriodStart: ps, PeriodEnd: pe, IssueIds: ids,
		})
		if err != nil {
			return fmt.Errorf("durable scorecard outcomes: %w", err)
		}
		for _, o := range outcomes {
			d.Outcomes.Population++
			if verifiedOutcome(o.Verdict, o.FailCause, o.Sampled, o.Settled) {
				d.Outcomes.Verified++
				if o.Verdict == "pass" {
					d.Outcomes.Pass++
				}
			}
		}
	}
	in.Durable = &d
	return nil
}

// durableDeliveryReadings are the delivery.cycle_time_days and delivery.rework_rate
// readings of one squad scope and day while durable_outcomes is on. Cycle time runs
// from the first start to the first done of the day, and rework is the day's
// transitions out of done over its transitions into done, in percent like the other
// built-in rates. A read failure writes no reading, as the other built-in readings do.
func durableDeliveryReadings(ctx context.Context, q *db.Queries, ws pgtype.UUID, sc builtinScope, yesterday, today time.Time) []builtinReading {
	rows, err := q.DurableSquadTransitions(ctx, db.DurableSquadTransitionsParams{
		WorkspaceID: ws,
		PeriodStart: pgtype.Timestamptz{Time: yesterday, Valid: true},
		PeriodEnd:   pgtype.Timestamptz{Time: today, Valid: true},
		SquadIds:    sc.squadIDs,
	})
	if err != nil {
		return nil
	}
	transitions := make([]durableTransition, 0, len(rows))
	for _, r := range rows {
		tr := durableTransition{
			IssueID:    util.UUIDToString(r.IssueID),
			ToCategory: LifecycleCategoryOf(r.ToCatalogCategory, r.ToStatusKey),
			At:         r.CreatedAt.Time,
		}
		if r.FromStatusKey.Valid {
			tr.FromCategory = LifecycleCategoryOf(r.FromCatalogCategory, r.FromStatusKey.String)
		}
		transitions = append(transitions, tr)
	}
	histories := durableHistories(transitions, yesterday, today)

	var doneEntries, reopens int64
	for _, h := range histories {
		doneEntries += h.DoneEntries
		reopens += h.Reopens
	}
	var out []builtinReading
	add := func(key string, value float64, ok bool) {
		if ok {
			out = append(out, builtinReading{key: key, scope: sc, value: value, observedAt: yesterday})
		}
	}
	if days := durableCycleDays(histories); len(days) > 0 {
		var sum float64
		for _, d := range days {
			sum += d
		}
		add("delivery.cycle_time_days", sum/float64(len(days)), true)
	}
	rate, ok := ratePercent(float64(reopens), float64(doneEntries))
	add("delivery.rework_rate", rate, ok)
	return out
}
