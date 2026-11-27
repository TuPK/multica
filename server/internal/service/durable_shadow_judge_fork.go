package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The judging of a shadow run (durable spec 9.4): when both tasks of a pair have completed,
// a juror from the jury pool gets the two answers labelled A and B, and ends its answer with
// a VERDICT line. The verdict is written once, with both sides' cost, and the run completes
// when it has target_pairs compared pairs (spec 9.6 then files the runtime change).

const (
	// shadowVerdictPrefix starts the line a juror ends its answer with.
	shadowVerdictPrefix = "VERDICT:"
	// shadowEstimateWindow is the spend history the cost estimate reads (spec 9.5).
	shadowEstimateWindow = 30 * 24 * time.Hour
	// shadowEstimateRows bounds the tasks the estimate sums.
	shadowEstimateRows = 500
	// shadowBudgetUnlimited is the remaining budget when no budget is active.
	shadowBudgetUnlimited = int64(-1)
	// shadowReconcileBatch bounds how many unjudged pairs one reconciliation pass settles.
	shadowReconcileBatch = 100
)

// durableMarkerOf reads the durable kind and shadow pair a task context carries.
func durableMarkerOf(taskContext []byte) (kind, pairID string) {
	var m struct {
		DurableKind  string `json:"durable_kind"`
		ShadowPairID string `json:"shadow_pair_id"`
	}
	if len(taskContext) == 0 || json.Unmarshal(taskContext, &m) != nil {
		return "", ""
	}
	return m.DurableKind, m.ShadowPairID
}

// taskOutputText is the text a task's result carries as its output.
func taskOutputText(result []byte) string {
	var r struct {
		Output string `json:"output"`
	}
	if len(result) == 0 || json.Unmarshal(result, &r) != nil {
		return ""
	}
	return r.Output
}

// parseShadowVerdict reads the verdict a juror ends its answer with: the last VERDICT line,
// one of A, B, tie or unable. A missing or unknown verdict is unable.
func parseShadowVerdict(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(strings.ToUpper(line), shadowVerdictPrefix) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(line[len(shadowVerdictPrefix):])) {
		case "a":
			return "A"
		case "b":
			return "B"
		case "tie":
			return "tie"
		}
		return "unable"
	}
	return "unable"
}

// OnTaskSettled follows a completed task that belongs to a shadow run: the shadow task or the
// primary opens the juror once both are done, and a juror's answer is the pair's verdict.
// It writes nothing while durable_seats is off.
func (s *ShadowService) OnTaskSettled(ctx context.Context, task db.AgentTaskQueue) error {
	if task.Status != "completed" || !s.enabled(ctx) {
		return nil
	}
	kind, pairID := durableMarkerOf(task.Context)
	switch {
	case pairID != "" && kind == shadowKindMarker:
		pair, err := s.pairFor(ctx, task.AgentID, pairID)
		if err != nil {
			return err
		}
		return s.openJuror(ctx, pair)
	case pairID != "" && kind == "verification":
		pair, err := s.pairFor(ctx, task.AgentID, pairID)
		if err != nil {
			return err
		}
		if pair.Comparison.Valid {
			return nil
		}
		output := taskOutputText(task.Result)
		return s.recordVerdict(ctx, pair, parseShadowVerdict(output), "")
	case kind == "":
		pair, err := s.Queries.GetShadowPairForPrimary(ctx, task.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("load pair for primary: %w", err)
		}
		return s.openJuror(ctx, pair)
	}
	return nil
}

// ReconcileUnjudgedPairs finds completed shadow pairs with no comparison and no juror
// task yet (for example because durable_seats was off when the tasks finished) and
// settles them through openJuror. It writes nothing while durable_seats is off, and
// checks for an existing juror task before opening one.
func (s *ShadowService) ReconcileUnjudgedPairs(ctx context.Context) (int, error) {
	if !s.enabled(ctx) {
		return 0, nil
	}
	pairs, err := s.Queries.ListUnjudgedCompletedShadowPairs(ctx, shadowReconcileBatch)
	if err != nil {
		return 0, fmt.Errorf("list unjudged shadow pairs: %w", err)
	}
	settled := 0
	var firstErr error
	for _, pair := range pairs {
		n, err := s.Queries.CountShadowJurorTasksForPair(ctx, util.UUIDToString(pair.ID))
		if err != nil {
			slog.Warn("durable shadow reconcile: count juror tasks failed", "pair_id", util.UUIDToString(pair.ID), "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if n > 0 {
			continue
		}
		if err := s.openJuror(ctx, pair); err != nil {
			slog.Warn("durable shadow reconcile: open juror failed", "pair_id", util.UUIDToString(pair.ID), "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		settled++
	}
	return settled, firstErr
}

// ShadowPrimaryClaimed samples a seat's claimed task into its shadow run (durable spec 9.4). A
// failure is logged: the claim it follows has already succeeded.
func (s *TaskService) ShadowPrimaryClaimed(ctx context.Context, task db.AgentTaskQueue) {
	if s.Shadow == nil {
		return
	}
	if err := s.Shadow.OnPrimaryClaimed(ctx, task); err != nil {
		slog.Warn("durable shadow: sampling a claimed task failed", "task_id", util.UUIDToString(task.ID), "error", err)
	}
}

// onShadowTaskSettled hands a completed task to the shadow run it may belong to. A failure
// leaves the pair for the next settlement; it never fails the completion.
func (s *TaskService) onShadowTaskSettled(ctx context.Context, task db.AgentTaskQueue) {
	if s.Shadow == nil {
		return
	}
	if err := s.Shadow.OnTaskSettled(ctx, task); err != nil {
		slog.Warn("durable shadow: judging a settled task failed", "task_id", util.UUIDToString(task.ID), "error", err)
	}
}

// pairFor loads a pair of the workspace the task's agent belongs to.
func (s *ShadowService) pairFor(ctx context.Context, agentID pgtype.UUID, pairID string) (db.ShadowPair, error) {
	agent, err := s.Queries.GetAgent(ctx, agentID)
	if err != nil {
		return db.ShadowPair{}, fmt.Errorf("load agent of shadow task: %w", err)
	}
	id, err := util.ParseUUID(pairID)
	if err != nil {
		return db.ShadowPair{}, fmt.Errorf("shadow_pair_id: %w", err)
	}
	pair, err := s.Queries.GetShadowPair(ctx, db.GetShadowPairParams{ID: id, WorkspaceID: agent.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ShadowPair{}, ErrShadowPairNotFound
	}
	if err != nil {
		return db.ShadowPair{}, fmt.Errorf("load pair: %w", err)
	}
	return pair, nil
}

// openJuror gives a pair its juror once both of its tasks have completed: a verification
// task for a pool agent that is neither the seat nor its shadow, chosen to differ from the
// seat's provider where it can. With no juror available the pair is unable, and the run
// goes on without it.
func (s *ShadowService) openJuror(ctx context.Context, pair db.ShadowPair) error {
	if pair.Comparison.Valid || !pair.ShadowTaskID.Valid {
		return nil
	}
	primary, err := s.Queries.GetAgentTask(ctx, pair.PrimaryTaskID)
	if err != nil {
		return fmt.Errorf("load primary task: %w", err)
	}
	shadowTask, err := s.Queries.GetAgentTask(ctx, pair.ShadowTaskID)
	if err != nil {
		return fmt.Errorf("load shadow task: %w", err)
	}
	if primary.Status != "completed" || shadowTask.Status != "completed" {
		return nil
	}
	n, err := s.Queries.CountShadowJurorTasksForPair(ctx, util.UUIDToString(pair.ID))
	if err != nil {
		return fmt.Errorf("count jurors: %w", err)
	}
	if n > 0 {
		return nil
	}
	cfg, err := s.Queries.GetShadowConfig(ctx, db.GetShadowConfigParams{ID: pair.ConfigID, WorkspaceID: pair.WorkspaceID})
	if err != nil {
		return fmt.Errorf("load shadow run: %w", err)
	}
	seat, err := s.Queries.GetAgent(ctx, cfg.AgentID)
	if err != nil {
		return fmt.Errorf("load seat: %w", err)
	}
	issue, err := s.Queries.GetIssue(ctx, pair.IssueID)
	if err != nil {
		return fmt.Errorf("load issue: %w", err)
	}
	provider := ""
	if seat.RuntimeID.Valid {
		if provider, err = runtimeProvider(ctx, s.Queries, seat.RuntimeID); err != nil {
			return err
		}
	}
	jury := &JuryService{Queries: s.Queries}
	pool, err := jury.candidates(ctx, pair.WorkspaceID, issue, map[string]bool{
		util.UUIDToString(seat.ID): true, util.UUIDToString(shadowTask.AgentID): true,
	})
	if err != nil {
		return err
	}
	doer := JurorCandidate{AgentID: util.UUIDToString(seat.ID), Provider: provider, Model: seat.Model.String}
	picked, _ := PickJurors(pool, doer, 1)
	if len(picked) == 0 {
		return s.recordVerdict(ctx, pair, "unable", "no juror was available to compare the two answers")
	}
	juror, err := util.ParseUUID(picked[0].AgentID)
	if err != nil {
		return fmt.Errorf("juror id: %w", err)
	}
	if s.Tasks == nil {
		return errors.New("durable shadow: no task service to give a juror its check")
	}
	task, err := s.Tasks.EnqueueVerificationTask(ctx, issue, juror)
	if err != nil {
		return fmt.Errorf("enqueue juror task: %w", err)
	}
	marked, err := s.Queries.SetShadowJurorTaskContext(ctx, db.SetShadowJurorTaskContextParams{
		ID: task.ID, ShadowPairID: util.UUIDToString(pair.ID),
	})
	if err != nil {
		return fmt.Errorf("mark juror task: %w", err)
	}
	if marked == 0 {
		return fmt.Errorf("juror task %s was claimed before it was marked", util.UUIDToString(task.ID))
	}
	return nil
}

// recordVerdict writes a pair's comparison and both sides' cost, once, and completes the
// run when it has its target of compared pairs.
func (s *ShadowService) recordVerdict(ctx context.Context, pair db.ShadowPair, verdict, reason string) error {
	cmp, err := ResolveShadowVerdict(verdict, ShadowPrimaryIsA(util.UUIDToString(pair.ID)))
	if err != nil {
		return err
	}
	primaryCost, err := TaskCostMicros(ctx, s.Queries, pair.PrimaryTaskID)
	if err != nil {
		return fmt.Errorf("primary cost: %w", err)
	}
	var shadowCost int64
	if pair.ShadowTaskID.Valid {
		if shadowCost, err = TaskCostMicros(ctx, s.Queries, pair.ShadowTaskID); err != nil {
			return fmt.Errorf("shadow cost: %w", err)
		}
	}
	written := int64(0)
	err = s.inTx(ctx, func(q *db.Queries) error {
		n, err := q.SetShadowPairComparison(ctx, db.SetShadowPairComparisonParams{
			ID: pair.ID, WorkspaceID: pair.WorkspaceID,
			Comparison: pgtype.Text{String: string(cmp), Valid: true}, ComparisonReason: reason,
		})
		if err != nil {
			return fmt.Errorf("write comparison: %w", err)
		}
		written = n
		if n == 0 {
			return nil
		}
		_, err = q.SetShadowPairCosts(ctx, db.SetShadowPairCostsParams{
			ID: pair.ID, WorkspaceID: pair.WorkspaceID,
			PrimaryCostMicros: primaryCost, ShadowCostMicros: shadowCost,
		})
		return err
	})
	if err != nil || written == 0 {
		return err
	}
	return s.finishIfComplete(ctx, pair.WorkspaceID, pair.ConfigID)
}

// finishIfComplete completes a running shadow run that has its target of compared pairs, and
// hands its report to the runtime change (spec 9.6). The run is completed under its row lock,
// so two verdicts that land together complete it once.
func (s *ShadowService) finishIfComplete(ctx context.Context, ws, configID pgtype.UUID) error {
	cfg, err := s.Queries.GetShadowConfig(ctx, db.GetShadowConfigParams{ID: configID, WorkspaceID: ws})
	if err != nil {
		return fmt.Errorf("load shadow run: %w", err)
	}
	if cfg.State != "running" {
		return nil
	}
	report, err := s.Report(ctx, ws, configID)
	if err != nil {
		return err
	}
	if report.Pairs < int(cfg.TargetPairs) {
		return nil
	}
	completed := false
	err = s.inTx(ctx, func(q *db.Queries) error {
		locked, err := q.LockShadowConfig(ctx, db.LockShadowConfigParams{ID: cfg.ID, WorkspaceID: ws})
		if err != nil {
			return fmt.Errorf("lock shadow run: %w", err)
		}
		if locked.State != "running" {
			return nil
		}
		if _, err := q.SetShadowConfigState(ctx, db.SetShadowConfigStateParams{ID: cfg.ID, WorkspaceID: ws, State: "completed"}); err != nil {
			return fmt.Errorf("complete shadow run: %w", err)
		}
		completed = true
		return nil
	})
	if err != nil || !completed || s.Runtime == nil {
		return err
	}
	return s.Runtime.OnShadowCompleted(ctx, cfg, report)
}

// ShadowEstimate is the cost of a shadow run before it starts (spec 9.5): the average cost of
// the seat's recent tasks, times the sample rate and the target pairs. Warn is set when the
// estimate exceeds what the workspace's tightest active budget has left; the caller then asks
// for confirmation. RemainingMicros is -1 when no budget is active.
type ShadowEstimate struct {
	Micros          int64
	RemainingMicros int64
	Warn            bool
}

// Estimate prices a shadow run for a seat at a sample rate and a target. The budget it is checked
// against is the seat's own: the active budgets that cover the seat's squad, the company budget
// among them, never a budget of another squad (spec 9.5).
func (s *ShadowService) Estimate(ctx context.Context, ws, seatID pgtype.UUID, rate float64, target int) (ShadowEstimate, error) {
	seat, err := s.Queries.GetAgent(ctx, seatID)
	if err != nil {
		return ShadowEstimate{}, fmt.Errorf("load seat: %w", err)
	}
	ids, err := s.Queries.ListSeatCompletedTaskIDs(ctx, db.ListSeatCompletedTaskIDsParams{
		AgentID: seatID, Since: pgtype.Timestamptz{Time: time.Now().Add(-shadowEstimateWindow), Valid: true},
		RowLimit: shadowEstimateRows,
	})
	if err != nil {
		return ShadowEstimate{}, fmt.Errorf("list seat tasks: %w", err)
	}
	var total int64
	for _, id := range ids {
		c, err := TaskCostMicros(ctx, s.Queries, id)
		if err != nil {
			return ShadowEstimate{}, fmt.Errorf("task cost: %w", err)
		}
		total += c
	}
	var average int64
	if len(ids) > 0 {
		average = total / int64(len(ids))
	}
	est := int64(math.Round(float64(average) * rate * float64(target)))

	budgets, err := s.Queries.ListWorkspaceBudgetStates(ctx, db.ListWorkspaceBudgetStatesParams{
		WorkspaceID: ws, Now: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return ShadowEstimate{}, fmt.Errorf("list budgets: %w", err)
	}
	remaining := shadowBudgetUnlimited
	scope := budgetScope{squadID: seat.HomeSquadID}
	for _, b := range budgets {
		if b.Status != "active" || !budgetCovers(b.ScopeType, b.ScopeID, scope) {
			continue
		}
		if left := b.AmountMicros - b.SpentMicros; remaining == shadowBudgetUnlimited || left < remaining {
			remaining = left
		}
	}
	return ShadowEstimate{Micros: est, RemainingMicros: remaining, Warn: remaining != shadowBudgetUnlimited && est > remaining}, nil
}

// shadowJurorBriefing is the briefing of a juror: the two answers, labelled A and B, with no
// model names (spec 9.4). Which side is A is fixed per pair, so the juror cannot learn the
// seat from the order.
func shadowJurorBriefing(ctx context.Context, q *db.Queries, task db.AgentTaskQueue) []BriefingBlock {
	kind, pairID := durableMarkerOf(task.Context)
	if kind != "verification" || pairID == "" {
		return nil
	}
	agent, err := q.GetAgent(ctx, task.AgentID)
	if err != nil {
		return nil
	}
	id, err := util.ParseUUID(pairID)
	if err != nil {
		return nil
	}
	pair, err := q.GetShadowPair(ctx, db.GetShadowPairParams{ID: id, WorkspaceID: agent.WorkspaceID})
	if err != nil || !pair.ShadowTaskID.Valid {
		return nil
	}
	primary, err := q.GetAgentTask(ctx, pair.PrimaryTaskID)
	if err != nil {
		return nil
	}
	shadowTask, err := q.GetAgentTask(ctx, pair.ShadowTaskID)
	if err != nil {
		return nil
	}
	a, b := taskOutputText(primary.Result), taskOutputText(shadowTask.Result)
	if !ShadowPrimaryIsA(util.UUIDToString(pair.ID)) {
		a, b = b, a
	}
	body := fmt.Sprintf("Two answers to the same task are below, labelled A and B. Judge which one answers the task better. "+
		"End your answer with one line: %s A, %s B, %s tie, or %s unable.\n\n### Answer A\n\n%s\n\n### Answer B\n\n%s",
		shadowVerdictPrefix, shadowVerdictPrefix, shadowVerdictPrefix, shadowVerdictPrefix, a, b)
	return []BriefingBlock{{Title: "Blind comparison", Body: body}}
}
