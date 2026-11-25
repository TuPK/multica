package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

const (
	// outcomeCursorName is the name of the one cursor row the transition reader owns.
	outcomeCursorName = "outcome"
	// outcomeBatchSize bounds the rows one transaction reads or settles.
	outcomeBatchSize = 200
	// defaultOutcomeSettleWindow is settle_days = 7 (spec 2.4). The workspace
	// setting, 1 to 60 days, replaces it when the setting exists.
	defaultOutcomeSettleWindow = 7 * 24 * time.Hour
	// outcomeSkipLabel keeps an issue out of outcomes (spec 2.4).
	outcomeSkipLabel = "wc:skip"
	// TODO(A6): TestOutcomeSampledFollowsTrackRecord replaces this fixed mode and the
	// fixed sampled = true with the mode the track record gives (spec 3.3). Until
	// then every outcome is verified in full.
	outcomeVerificationMode = "full"
)

// OutcomeService opens a work outcome when an issue enters done, closes the current
// one on reopen or cancel, and settles outcomes once their settle time has passed.
// The scheduler runs ProcessTransitions and Settle from one job. Both return without
// writing while durable_outcomes is off, and the cursor does not move while it is
// off, so turning the flag back on resumes from the stopped transition.
type OutcomeService struct {
	Queries   *db.Queries
	TxStarter TxStarter
	Flags     *featureflag.Service
	// Now is the clock for settle_at. Nil means time.Now.
	Now func() time.Time
}

func (s *OutcomeService) enabled(ctx context.Context) bool {
	return featureflags.DurableOutcomesEnabled(ctx, s.Flags)
}

func (s *OutcomeService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// ProcessTransitions applies the transitions after the cursor, one batch per
// transaction, until none are left. It returns how many transitions it read.
//
// The cursor orders by created_at, which is the start of the writing transaction.
// A transaction that commits after a later one started can therefore land behind
// the cursor and be skipped; see the A2 report for the overlap that would close it.
func (s *OutcomeService) ProcessTransitions(ctx context.Context) (int, error) {
	if !s.enabled(ctx) {
		return 0, nil
	}
	total := 0
	for {
		n, err := s.processBatch(ctx)
		total += n
		if err != nil || n < outcomeBatchSize {
			return total, err
		}
	}
}

func (s *OutcomeService) processBatch(ctx context.Context) (int, error) {
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin outcome batch: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)

	// Before any cursor row exists, every transition is unread. The zero UUID is
	// valid, not NULL: a NULL id would make the row comparison match nothing.
	after := db.ListIssueTransitionsAfterCursorParams{
		AfterAt:  pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true},
		AfterID:  pgtype.UUID{Valid: true},
		RowLimit: outcomeBatchSize,
	}
	cur, err := q.GetDurableOutcomeCursor(ctx, outcomeCursorName)
	switch {
	case err == nil:
		after.AfterAt = cur.CreatedAt
		after.AfterID = cur.TransitionID
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return 0, fmt.Errorf("read outcome cursor: %w", err)
	}

	rows, err := q.ListIssueTransitionsAfterCursor(ctx, after)
	if err != nil {
		return 0, fmt.Errorf("list transitions after cursor: %w", err)
	}
	for _, r := range rows {
		if err := s.apply(ctx, q, r); err != nil {
			return 0, fmt.Errorf("apply transition %s: %w", util.UUIDToString(r.ID), err)
		}
	}
	if len(rows) > 0 {
		last := rows[len(rows)-1]
		if err := q.UpsertDurableOutcomeCursor(ctx, db.UpsertDurableOutcomeCursorParams{
			Name:         outcomeCursorName,
			CreatedAt:    last.CreatedAt,
			TransitionID: last.ID,
		}); err != nil {
			return 0, fmt.Errorf("move outcome cursor: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit outcome batch: %w", err)
	}
	return len(rows), nil
}

// apply reads one transition by lifecycle category (D32). Entering done opens an
// outcome. Leaving done for an unfinished category reopens the current outcome,
// and leaving done for cancelled reverts it. Any other change is not an outcome event.
func (s *OutcomeService) apply(ctx context.Context, q *db.Queries, r db.ListIssueTransitionsAfterCursorRow) error {
	from := ""
	if r.FromStatusKey.Valid {
		from = LifecycleCategoryOf(r.FromCatalogCategory, r.FromStatusKey.String)
	}
	to := LifecycleCategoryOf(r.ToCatalogCategory, r.ToStatusKey)
	switch {
	case to == "done" && from != "done":
		return s.enterDone(ctx, q, r)
	case from == "done" && to == "closed":
		_, err := q.RevertCurrentWorkOutcome(ctx, db.RevertCurrentWorkOutcomeParams{
			WorkspaceID: r.WorkspaceID, IssueID: r.IssueID,
		})
		return err
	case from == "done" && to != "done":
		_, err := q.ReopenCurrentWorkOutcome(ctx, db.ReopenCurrentWorkOutcomeParams{
			WorkspaceID: r.WorkspaceID, IssueID: r.IssueID,
		})
		return err
	}
	return nil
}

func (s *OutcomeService) enterDone(ctx context.Context, q *db.Queries, r db.ListIssueTransitionsAfterCursorRow) error {
	labels, err := q.ListIssueLabelNames(ctx, r.IssueID)
	if err != nil {
		return fmt.Errorf("list labels: %w", err)
	}
	if slices.Contains(labels, outcomeSkipLabel) {
		return nil
	}

	in := WorkClassInput{LabelNames: labels}
	params := db.CreateWorkOutcomeParams{
		WorkspaceID:      r.WorkspaceID,
		IssueID:          r.IssueID,
		DoneTransitionID: r.ID,
		VerificationMode: outcomeVerificationMode,
		Sampled:          true,
		SettleAt:         pgtype.Timestamptz{Time: s.now().Add(defaultOutcomeSettleWindow), Valid: true},
	}
	// The doer is the issue's assignee. A member's outcome keeps no agent id, so it
	// stays out of agent track records (spec 2.4).
	switch r.AssigneeType.String {
	case "agent":
		params.AgentID = r.AssigneeID
	case "member":
		params.MemberID = r.AssigneeID
	case "squad":
		params.SquadID = r.AssigneeID
		in.SquadID = util.UUIDToString(r.AssigneeID)
	}
	params.WorkClass = WorkClassFor(in)

	outcome, err := q.CreateWorkOutcome(ctx, params)
	if err != nil {
		return fmt.Errorf("create outcome: %w", err)
	}
	// A new done supersedes the outcome before it. On a replay the row returned is
	// the existing one, and this update finds nothing left to supersede.
	if err := q.SupersedeCurrentWorkOutcomes(ctx, db.SupersedeCurrentWorkOutcomesParams{
		SupersededBy: outcome.ID,
		WorkspaceID:  r.WorkspaceID,
		IssueID:      r.IssueID,
	}); err != nil {
		return fmt.Errorf("supersede previous outcome: %w", err)
	}
	return nil
}

// Settle decides the verdict of every unsettled outcome whose settle_at is at or
// before now (spec 2.6). It returns how many it settled.
func (s *OutcomeService) Settle(ctx context.Context, now time.Time) (int, error) {
	if !s.enabled(ctx) {
		return 0, nil
	}
	dueAt := pgtype.Timestamptz{Time: now, Valid: true}
	total := 0
	for {
		due, err := s.Queries.ListDueWorkOutcomes(ctx, db.ListDueWorkOutcomesParams{
			DueAt: dueAt, RowLimit: outcomeBatchSize,
		})
		if err != nil {
			return total, fmt.Errorf("list due outcomes: %w", err)
		}
		for _, o := range due {
			// TODO(A3): pass the latest outcome_verification per criterion here.
			// Until checks are written, a sampled outcome has no results and settles
			// as unverified.
			v := DecideVerdict(VerdictInput{Sampled: o.Sampled, Reopened: o.FailCause == "reopened"})
			n, err := s.Queries.SettleWorkOutcome(ctx, db.SettleWorkOutcomeParams{
				ID:        o.ID,
				Verdict:   v.Verdict,
				Severity:  v.Severity,
				FailCause: v.FailCause,
				SettledAt: dueAt,
			})
			if err != nil {
				return total, fmt.Errorf("settle outcome %s: %w", util.UUIDToString(o.ID), err)
			}
			total += int(n)
		}
		if len(due) < outcomeBatchSize {
			return total, nil
		}
	}
}
