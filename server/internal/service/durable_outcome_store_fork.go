package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// ErrOutcomesOff: a write that needs durable_outcomes was asked while the flag is off.
var ErrOutcomesOff = errors.New("durable outcomes is off")

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
	// Verification gives the current outcomes their checks (A3). Nil leaves them
	// unchecked, and they settle as unverified.
	Verification *VerificationService
	// TrackRecords gives a new outcome its verification mode (A6). Nil reads the track
	// record through Queries.
	TrackRecords *TrackRecordService
	// Dossier files the lesson task of a seat whose outcome settles fail or partial (spec
	// 9.2). Nil files none.
	Dossier *DossierService
	// testApplyErr is a test-only seam to inject a deterministic apply error for a transition.
	testApplyErr func(r db.ListIssueTransitionsAfterCursorRow) error
}

func (s *OutcomeService) trackRecords() *TrackRecordService {
	if s.TrackRecords != nil {
		return s.TrackRecords
	}
	return &TrackRecordService{Queries: s.Queries, TxStarter: s.TxStarter, Flags: s.Flags}
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

// ProcessTransitions applies unapplied transitions after the safe sequence cursor,
// one page per transaction, until none are left. It returns how many transitions it read.
//
// Each transition carries a monotonic seq assigned at insert. last_seq advances past
// a sequence only once every transaction that could hold a lower sequence has ended
// (tracked via Postgres snapshot xmin/xmax), so a long-running transaction that commits
// late is never skipped regardless of how old its created_at is (V2.2).
// durable_outcome_applied keeps every transition idempotent.
func (s *OutcomeService) ProcessTransitions(ctx context.Context) (int, error) {
	if !s.enabled(ctx) {
		return 0, nil
	}
	cursor, found, err := s.readCursor(ctx)
	if err != nil {
		return 0, err
	}
	// Retry recorded failures once per run before reading new transitions.
	var retryAfter int64
	for {
		page, err := s.applyPage(ctx, retryAfter, outcomeKey{}, false, true)
		if err != nil {
			return 0, err
		}
		if page.read < outcomeBatchSize {
			break
		}
		retryAfter = page.lastSeq
	}
	afterSeq := cursor.LastSeq
	total := 0
	for {
		page, err := s.applyPage(ctx, afterSeq, cursor, found, false)
		if err != nil {
			return total, err
		}
		total += page.read
		cursor = page.cursor
		if page.read > 0 {
			found = true
		}
		if page.read < outcomeBatchSize {
			return total, nil
		}
		afterSeq = page.lastSeq
	}
}

// outcomeKey tracks the sequence cursor and its snapshot barrier, plus the last
// processed (created_at, id) for observability.
type outcomeKey struct {
	At          pgtype.Timestamptz
	ID          pgtype.UUID
	LastSeq     int64
	PendingSeq  int64
	PendingXmax int64
}

// readCursor returns the stored cursor. found is false before the first run that
// read a transition.
func (s *OutcomeService) readCursor(ctx context.Context) (outcomeKey, bool, error) {
	cur, err := s.Queries.GetDurableOutcomeCursor(ctx, outcomeCursorName)
	switch {
	case err == nil:
		return outcomeKey{
			At:          cur.CreatedAt,
			ID:          cur.TransitionID,
			LastSeq:     cur.LastSeq,
			PendingSeq:  cur.PendingSeq,
			PendingXmax: cur.PendingXmax,
		}, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return outcomeKey{}, false, nil
	default:
		return outcomeKey{}, false, fmt.Errorf("read outcome cursor: %w", err)
	}
}

// outcomePage is the result of one transaction over a page of transitions.
type outcomePage struct {
	read    int        // transitions read in this page
	lastSeq int64      // seq of the last transition read, where the next page starts
	cursor  outcomeKey // the cursor after this page
}

// applyPage reads one page of unapplied transitions after afterSeq and applies each
// transition not yet in durable_outcome_applied, updating the sequence cursor once
// snapshot bounds confirm no uncommitted lower sequence can still arrive.
func (s *OutcomeService) applyPage(ctx context.Context, afterSeq int64, cursor outcomeKey, found bool, retry bool) (outcomePage, error) {
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return outcomePage{}, fmt.Errorf("begin outcome page: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)

	snapBefore, err := q.GetCurrentSnapshotBounds(ctx)
	if err != nil {
		return outcomePage{}, fmt.Errorf("read snapshot before transitions: %w", err)
	}
	rows, err := q.ListIssueTransitionsAfterCursor(ctx, db.ListIssueTransitionsAfterCursorParams{
		AfterSeq:    afterSeq,
		RetryFailed: retry,
		RowLimit:    outcomeBatchSize,
	})
	if err != nil {
		return outcomePage{}, fmt.Errorf("list transitions: %w", err)
	}
	snapAfter, err := q.GetCurrentSnapshotBounds(ctx)
	if err != nil {
		return outcomePage{}, fmt.Errorf("read snapshot after transitions: %w", err)
	}

	page := outcomePage{read: len(rows), lastSeq: afterSeq, cursor: cursor}
	for _, r := range rows {
		if r.Seq > page.lastSeq {
			page.lastSeq = r.Seq
		}
		if r.Seq > page.cursor.PendingSeq {
			page.cursor.PendingSeq = r.Seq
			page.cursor.PendingXmax = snapAfter.Xmax
			page.cursor.At = r.CreatedAt
			page.cursor.ID = r.ID
		} else if !page.cursor.At.Valid {
			page.cursor.At = r.CreatedAt
			page.cursor.ID = r.ID
		}

		if _, err := tx.Exec(ctx, "SAVEPOINT outcome_transition"); err != nil {
			return outcomePage{}, err
		}
		claimed, err := q.ClaimOutcomeTransition(ctx, r.ID)
		if err == nil && claimed != 0 {
			err = s.apply(ctx, q, r)
		}
		if err != nil {
			if _, rollbackErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT outcome_transition"); rollbackErr != nil {
				return outcomePage{}, fmt.Errorf("rollback transition %s: %w", util.UUIDToString(r.ID), rollbackErr)
			}
			_, recordErr := tx.Exec(ctx, `INSERT INTO durable_outcome_applied (transition_id, status, attempts, last_error)
				VALUES ($1, 'failed', 1, $2)
				ON CONFLICT (transition_id) DO UPDATE SET
				attempts = durable_outcome_applied.attempts + 1,
				status = CASE WHEN durable_outcome_applied.attempts + 1 >= 3 THEN 'dead_letter' ELSE 'failed' END,
				last_error = EXCLUDED.last_error
				WHERE durable_outcome_applied.status = 'failed'`, r.ID, err.Error())
			if recordErr != nil {
				return outcomePage{}, fmt.Errorf("record transition failure %s: %w", util.UUIDToString(r.ID), recordErr)
			}
			slog.Error("outcome transition failed", "transition_id", util.UUIDToString(r.ID), "error", err)
		}
		if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT outcome_transition"); err != nil {
			return outcomePage{}, err
		}
	}

	// Only advance last_seq once this is the final page of the run (no unapplied
	// rows remain after afterSeq) and every transaction active when pending_seq was
	// observed has ended.
	if !retry && page.read < outcomeBatchSize && page.cursor.PendingSeq > page.cursor.LastSeq {
		noConcurrentWriters := snapBefore.Xmin == snapBefore.Xmax && snapAfter.Xmin == snapAfter.Xmax
		if noConcurrentWriters || snapBefore.Xmin >= page.cursor.PendingXmax {
			page.cursor.LastSeq = page.cursor.PendingSeq
		}
	}

	if !retry && (page.read > 0 || (found && page.cursor != cursor)) {
		if err := q.UpsertDurableOutcomeCursor(ctx, db.UpsertDurableOutcomeCursorParams{
			Name:         outcomeCursorName,
			CreatedAt:    page.cursor.At,
			TransitionID: page.cursor.ID,
			LastSeq:      page.cursor.LastSeq,
			PendingSeq:   page.cursor.PendingSeq,
			PendingXmax:  page.cursor.PendingXmax,
		}); err != nil {
			return outcomePage{}, fmt.Errorf("move outcome cursor: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return outcomePage{}, fmt.Errorf("commit outcome page: %w", err)
	}
	return page, nil
}

// apply reads one transition by lifecycle category (D32). Entering done opens an
// outcome. Leaving done for an unfinished category reopens the current outcome,
// and leaving done for cancelled reverts it. Any other change is not an outcome event.
func (s *OutcomeService) apply(ctx context.Context, q *db.Queries, r db.ListIssueTransitionsAfterCursorRow) error {
	if s.testApplyErr != nil {
		if err := s.testApplyErr(r); err != nil {
			return err
		}
	}
	from := ""
	if r.FromStatusKey.Valid {
		from = LifecycleCategoryOf(r.FromCatalogCategory, r.FromStatusKey.String)
	}
	to := LifecycleCategoryOf(r.ToCatalogCategory, r.ToStatusKey)
	switch {
	case to == "done" && from != "done":
		return s.enterDone(ctx, q, r)
	case from == "done" && to == "closed":
		if _, err := q.RevertCurrentWorkOutcome(ctx, db.RevertCurrentWorkOutcomeParams{
			WorkspaceID: r.WorkspaceID, IssueID: r.IssueID,
		}); err != nil {
			return err
		}
		return s.recordCurrentTrack(ctx, q, r.WorkspaceID, r.IssueID)
	case from == "done" && to != "done":
		_, err := q.ReopenCurrentWorkOutcome(ctx, db.ReopenCurrentWorkOutcomeParams{
			WorkspaceID: r.WorkspaceID, IssueID: r.IssueID,
		})
		return err
	}
	return nil
}

func (s *OutcomeService) enterDone(ctx context.Context, q *db.Queries, r db.ListIssueTransitionsAfterCursorRow) error {
	if r.OutcomesDisabled {
		return nil
	}
	settleWindow := defaultOutcomeSettleWindow
	var settings struct {
		SettleDays int `json:"settle_days"`
	}
	if json.Unmarshal(r.WorkspaceSettings, &settings) == nil && settings.SettleDays >= 1 && settings.SettleDays <= 60 {
		settleWindow = time.Duration(settings.SettleDays) * 24 * time.Hour
	}
	labels := r.LabelNames
	if slices.Contains(labels, outcomeSkipLabel) {
		return nil
	}

	settleAt := s.now().Add(settleWindow)
	// A metric criterion whose observation window (e.g. Reach's 30-day gap window) extends
	// past the default settle_days must keep the outcome open until that deadline so hourly
	// metric checks can evaluate it before settlement (spec 2.5, 11.3).
	if criteria, err := q.ListIssueAcceptanceCriteria(ctx, db.ListIssueAcceptanceCriteriaParams{
		WorkspaceID: r.WorkspaceID, IssueID: r.IssueID,
	}); err == nil {
		for _, c := range criteria {
			if c.ArchivedAt.Valid || c.CheckKind != CheckKindMetric {
				continue
			}
			var spec metricCheckSpec
			if json.Unmarshal(c.CheckSpec, &spec) == nil && spec.WithinDays > 0 {
				deadline := c.CreatedAt.Time.Add(time.Duration(spec.WithinDays) * 24 * time.Hour)
				if deadline.After(settleAt) {
					settleAt = deadline
				}
			}
		}
	}

	in := WorkClassInput{LabelNames: labels}
	params := db.CreateWorkOutcomeParams{
		ID:               dbid.NewV7(),
		WorkspaceID:      r.WorkspaceID,
		IssueID:          r.IssueID,
		DoneTransitionID: r.ID,
		SettleAt:         pgtype.Timestamptz{Time: settleAt, Valid: true},
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

	// Only an agent has a track record, so a member's or squad's outcome is checked in full.
	// The model and provider are the agent's now: the outcome keeps them when the agent
	// changes later (spec 2.3). The sample is chosen from the id the row is created with
	// (spec 3.3).
	params.VerificationMode = VerificationModeFull
	if params.AgentID.Valid {
		mode, err := s.trackRecords().ModeFor(ctx, r.WorkspaceID, params.AgentID, params.WorkClass)
		if err != nil {
			return err
		}
		params.VerificationMode = mode
		m, err := q.GetAgentRuntimeModel(ctx, db.GetAgentRuntimeModelParams{ID: params.AgentID, WorkspaceID: r.WorkspaceID})
		switch {
		case err == nil:
			params.RuntimeProvider = m.Provider
			params.Model = m.Model
			params.SquadID = m.HomeSquadID
		case errors.Is(err, pgx.ErrNoRows):
			// The agent is gone; the outcome keeps no model. Failing here would stall the cursor.
		default:
			return fmt.Errorf("read agent model: %w", err)
		}
	}
	params.Sampled = Sampled(util.UUIDToString(params.ID), params.VerificationMode)

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

// recordCurrentTrack recomputes the pair of the issue's current outcome, when it has one.
// A revert settles that outcome (spec 2.4), so its pair moves too.
func (s *OutcomeService) recordCurrentTrack(ctx context.Context, q *db.Queries, ws, issue pgtype.UUID) error {
	o, err := q.GetCurrentWorkOutcomeForIssue(ctx, db.GetCurrentWorkOutcomeForIssueParams{WorkspaceID: ws, IssueID: issue})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read current outcome: %w", err)
	}
	return recordTrack(ctx, q, o.WorkspaceID, o.AgentID, o.WorkClass, s.now())
}

// ScheduleChecks gives the current outcomes their checks (spec 2.5). It is a no-op while
// durable_outcomes is off, and when no VerificationService is wired.
func (s *OutcomeService) ScheduleChecks(ctx context.Context) (int, error) {
	if !s.enabled(ctx) || s.Verification == nil {
		return 0, nil
	}
	return s.Verification.ScheduleChecks(ctx)
}

// CheckMetrics decides the metric criteria (spec 2.5). The hourly job calls it; it is a
// no-op while durable_outcomes is off.
func (s *OutcomeService) CheckMetrics(ctx context.Context, now time.Time) (int, error) {
	if !s.enabled(ctx) || s.Verification == nil {
		return 0, nil
	}
	return s.Verification.CheckMetrics(ctx, now)
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
			n, err := s.settleOne(ctx, o, dueAt)
			if err != nil {
				return total, err
			}
			total += n
		}
		if len(due) < outcomeBatchSize {
			return total, nil
		}
	}
}

// settleOne decides one outcome from the newest check of each criterion, writes the
// verdict, and for a fail or partial leaves the system note on the issue. All of it is
// one transaction, so a verdict never settles without its note.
func (s *OutcomeService) settleOne(ctx context.Context, o db.WorkOutcome, at pgtype.Timestamptz) (int, error) {
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin settle: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)

	fresh, err := q.LockWorkOutcomeForSettle(ctx, db.LockWorkOutcomeForSettleParams{
		ID: o.ID, WorkspaceID: o.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("lock outcome %s: %w", util.UUIDToString(o.ID), err)
	}
	if fresh.SettledAt.Valid {
		return 0, nil
	}
	o = fresh

	results, notMet, err := s.criterionResults(ctx, q, o)
	if err != nil {
		return 0, err
	}
	v := DecideVerdict(VerdictInput{Sampled: o.Sampled, Reopened: o.FailCause == "reopened", Results: results})
	n, err := q.SettleWorkOutcome(ctx, db.SettleWorkOutcomeParams{
		ID:        o.ID,
		Verdict:   v.Verdict,
		Severity:  v.Severity,
		FailCause: v.FailCause,
		SettledAt: at,
	})
	if err != nil {
		return 0, fmt.Errorf("settle outcome %s: %w", util.UUIDToString(o.ID), err)
	}
	if n == 1 {
		// A settled result moves its pair's record and mode at once, so a high fail
		// takes effect with the verdict (spec 3.2).
		if err := recordTrack(ctx, q, o.WorkspaceID, o.AgentID, o.WorkClass, s.now()); err != nil {
			return 0, err
		}
		// A settled high fail drops its squad one level in this transaction (spec 3.4).
		settled := o
		settled.Verdict, settled.Severity = v.Verdict, v.Severity
		if err := (&EnvelopeService{Now: s.now, Flags: s.Flags}).OnHighFail(ctx, q, settled); err != nil {
			return 0, err
		}
	}
	if n == 1 && (v.Verdict == "fail" || v.Verdict == "partial") {
		if err := s.postSettledNote(ctx, q, o, v, notMet); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit settle: %w", err)
	}
	if n == 1 && s.Dossier != nil {
		// The lesson task comes after the verdict commits. A failure is logged, not returned,
		// because the verdict stands either way.
		settled := o
		settled.Verdict = v.Verdict
		if err := s.Dossier.OnOutcomeSettled(ctx, settled); err != nil {
			slog.Warn("seat lesson task: outcome settled", "outcome_id", util.UUIDToString(o.ID), "error", err)
		}
	}
	return int(n), nil
}

// criterionResults reads the newest check of each open criterion of the outcome's issue.
// A criterion with no check is unable and does not count (spec 2.6). An artifact that is
// still unpublished at settle time is a fail: the document it needs is missing. notMet
// names the criteria that failed or were partly met, for the note.
func (s *OutcomeService) criterionResults(ctx context.Context, q *db.Queries, o db.WorkOutcome) ([]CriterionResult, []string, error) {
	criteria, err := q.ListIssueAcceptanceCriteria(ctx, db.ListIssueAcceptanceCriteriaParams{WorkspaceID: o.WorkspaceID, IssueID: o.IssueID})
	if err != nil {
		return nil, nil, fmt.Errorf("list criteria: %w", err)
	}
	latest, err := q.ListLatestCriterionVerifications(ctx, o.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("list checks: %w", err)
	}
	byCriterion := make(map[string]db.OutcomeVerification, len(latest))
	for _, v := range latest {
		byCriterion[util.UUIDToString(v.CriterionID)] = v
	}
	// A jury's juror checks are latest until the panel decides; only a decided panel
	// counts (durable spec 8.3).
	panels, err := q.ListJuryPanelsForOutcome(ctx, db.ListJuryPanelsForOutcomeParams{WorkspaceID: o.WorkspaceID, OutcomeID: o.ID})
	if err != nil {
		return nil, nil, fmt.Errorf("list jury panels: %w", err)
	}
	undecided := map[string]bool{}
	for _, p := range panels {
		if p.State != juryStateUnanimous && p.State != juryStateResolved {
			undecided[util.UUIDToString(p.CriterionID)] = true
		}
	}
	var results []CriterionResult
	var notMet []string
	for _, c := range criteria {
		if c.ArchivedAt.Valid {
			continue
		}
		if v, ok := byCriterion[util.UUIDToString(c.ID)]; ok && !undecided[util.UUIDToString(c.ID)] {
			results = append(results, CriterionResult{Result: v.Result, Severity: v.Severity})
			if v.Result == "fail" || v.Result == "partial" {
				notMet = append(notMet, c.Text)
			}
			continue
		}
		if c.CheckKind == CheckKindArtifact {
			if _, err := q.InsertOutcomeVerification(ctx, verificationParams(o.WorkspaceID, o.ID, VerificationInput{
				CriterionID: c.ID, Result: "fail", Severity: "medium",
				Note: "No published document of the required kind when the outcome settled.",
			}, verifierTypeSystem, pgtype.UUID{}, pgtype.UUID{})); err != nil {
				return nil, nil, fmt.Errorf("record missing artifact: %w", err)
			}
			results = append(results, CriterionResult{Result: "fail", Severity: "medium"})
			notMet = append(notMet, c.Text)
			continue
		}
		results = append(results, CriterionResult{Result: "unable"})
	}
	return results, notMet, nil
}

// postSettledNote leaves the system comment on the issue that tells the doer a fail or
// partial was settled. A doer that is an agent is mentioned, so the mention path reaches
// it. The issue is not reopened.
func (s *OutcomeService) postSettledNote(ctx context.Context, q *db.Queries, o db.WorkOutcome, v VerdictResult, notMet []string) error {
	content := fmt.Sprintf("Outcome settled: %s", v.Verdict)
	if v.FailCause != "" {
		content += " (" + v.FailCause + ")"
	}
	content += "."
	if len(notMet) > 0 {
		content += " Not met: " + strings.Join(notMet, "; ") + "."
	}
	if o.AgentID.Valid {
		if agent, err := q.GetAgent(ctx, o.AgentID); err == nil {
			content = fmt.Sprintf("[@%s](mention://agent/%s) %s", agent.Name, util.UUIDToString(agent.ID), content)
		}
	}
	if _, err := q.CreateComment(ctx, db.CreateCommentParams{
		ID:          dbid.NewV7(),
		IssueID:     o.IssueID,
		WorkspaceID: o.WorkspaceID,
		AuthorType:  "system",
		AuthorID:    pgtype.UUID{Valid: true},
		Content:     content,
		Type:        "system",
		ParentID:    pgtype.UUID{},
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The issue (or workspace) was deleted before settlement (no FK cascade).
			// Skip the note so one deleted issue never poisons the global Settle sweep.
			return nil
		}
		return fmt.Errorf("post settled note: %w", err)
	}
	return nil
}
