package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// shadowTestTicks is one USD: the cost of one task in the shadow run's cost tests.
const shadowTestTicks = 10_000_000_000

// usageOf records one task's usage, so its cost is one USD.
func (sf shadowFixture) usageOf(t *testing.T, taskID pgtype.UUID) {
	t.Helper()
	sf.Insert(t, "task_usage", testutil.Cols{
		"task_id": util.UUIDToString(taskID), "provider": "anthropic", "model": "claude-sonnet-4.6",
		"cost_usd_ticks": int64(shadowTestTicks), "updated_at": testutil.Raw("now()"),
	})
}

// completeTask runs a queued or dispatched task to completion with an output, through the
// production completion path, so the shadow settlement hook sees it.
func (sf shadowFixture) completeTask(t *testing.T, taskID string, output string) {
	t.Helper()
	sf.Exec(t, `UPDATE agent_task_queue SET status = 'running', started_at = now() WHERE id = $1`, taskID)
	raw, err := json.Marshal(map[string]string{"output": output})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sf.svc.Tasks.CompleteTask(context.Background(), util.MustParseUUID(taskID), raw, "", "", "", false, "", ""); err != nil {
		t.Fatalf("complete task %s: %v", taskID, err)
	}
}

// TestShadowJurorJudgesBlind: once the primary and the shadow have completed, a juror from the
// jury pool gets both answers labelled A and B, without any model name. Its VERDICT line is the
// pair's comparison, and both sides' real cost is written with it.
func TestShadowJurorJudgesBlind(t *testing.T) {
	sf := newShadowFixture(t, true)
	sf.svc.Tasks.Shadow = sf.svc
	ctx := context.Background()
	cfg := sf.start(t)

	juror := sf.Agent(t, "juror", sf.Runtime(t, "juror runtime", testutil.Cols{"provider": "google"}))
	pool, err := json.Marshal(map[string]any{"jury_pool": []string{juror}})
	if err != nil {
		t.Fatal(err)
	}
	sf.Exec(t, `UPDATE workspace SET settings = $2::jsonb WHERE id = $1`, sf.WorkspaceID, string(pool))

	primary := sf.primaryTask(t)
	if err := sf.svc.OnPrimaryClaimed(ctx, primary); err != nil {
		t.Fatalf("OnPrimaryClaimed: %v", err)
	}
	pair, err := sf.q.GetShadowPairForPrimary(ctx, primary.ID)
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	sf.usageOf(t, primary.ID)
	sf.completeTask(t, util.UUIDToString(primary.ID), "The primary's answer.")

	shadowTaskID := util.UUIDToString(pair.ShadowTaskID)
	sf.usageOf(t, pair.ShadowTaskID)
	sf.completeTask(t, shadowTaskID, "The candidate's answer.")

	var jurorTask string
	if err := sf.Pool.QueryRow(ctx, `SELECT id::text FROM agent_task_queue
		WHERE context->>'durable_kind' = 'verification' AND context->>'shadow_pair_id' = $1`,
		util.UUIDToString(pair.ID)).Scan(&jurorTask); err != nil {
		t.Fatalf("juror task: %v", err)
	}

	task, err := sf.q.GetAgentTask(ctx, util.MustParseUUID(jurorTask))
	if err != nil {
		t.Fatalf("load juror task: %v", err)
	}
	blocks := shadowJurorBriefing(ctx, sf.q, task)
	if len(blocks) != 1 {
		t.Fatalf("juror briefing blocks = %d, want 1", len(blocks))
	}
	body := blocks[0].Body
	if !strings.Contains(body, "The primary's answer.") || !strings.Contains(body, "The candidate's answer.") {
		t.Fatalf("the juror does not see both answers:\n%s", body)
	}
	for _, name := range []string{"gpt-candidate", "claude-seat", sf.cand, sf.runtime} {
		if strings.Contains(body, name) {
			t.Errorf("the juror briefing names %q; it must be blind", name)
		}
	}

	sf.completeTask(t, jurorTask, "Answer B is the better one.\nVERDICT: B")

	got, err := sf.q.GetShadowPair(ctx, db.GetShadowPairParams{ID: pair.ID, WorkspaceID: sf.ws})
	if err != nil {
		t.Fatalf("reload pair: %v", err)
	}
	want, err := ResolveShadowVerdict("B", ShadowPrimaryIsA(util.UUIDToString(pair.ID)))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Comparison.Valid || Comparison(got.Comparison.String) != want {
		t.Fatalf("stored comparison = %v, want %q", got.Comparison, want)
	}
	if got.PrimaryCostMicros != 1_000_000 || got.ShadowCostMicros != 1_000_000 {
		t.Errorf("costs = %d and %d micros, want the real 1000000 each", got.PrimaryCostMicros, got.ShadowCostMicros)
	}
	report, err := sf.svc.Report(ctx, sf.ws, cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Pairs != 1 || report.PrimaryCostAvgMicros != 1_000_000 {
		t.Errorf("report = %+v, want one pair with a real primary cost", report)
	}
}

// TestShadowJurorUnableWithoutPool: with no jury pool the pair is unable, not stuck.
func TestShadowJurorUnableWithoutPool(t *testing.T) {
	sf := newShadowFixture(t, true)
	sf.svc.Tasks.Shadow = sf.svc
	ctx := context.Background()
	sf.start(t)
	primary := sf.primaryTask(t)
	if err := sf.svc.OnPrimaryClaimed(ctx, primary); err != nil {
		t.Fatalf("OnPrimaryClaimed: %v", err)
	}
	pair, err := sf.q.GetShadowPairForPrimary(ctx, primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	sf.completeTask(t, util.UUIDToString(primary.ID), "primary")
	sf.completeTask(t, util.UUIDToString(pair.ShadowTaskID), "shadow")

	got, err := sf.q.GetShadowPair(ctx, db.GetShadowPairParams{ID: pair.ID, WorkspaceID: sf.ws})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Comparison.Valid || Comparison(got.Comparison.String) != ComparisonUnable {
		t.Fatalf("comparison = %v, want unable with no juror", got.Comparison)
	}
}

// TestShadowHeldByBudget: a shadow run at its budget's hold is refused, like the other held
// classes, and waits in the queue.
func TestShadowHeldByBudget(t *testing.T) {
	f := newBudgetFixture(t)
	ctx := context.Background()
	f.company(t, BudgetModeHold, 1_000_000)
	issue := f.Issue(t, "shadow budget issue")
	id := f.queued(t, testutil.Cols{"issue_id": issue, "context": testutil.Raw(`'{"durable_kind":"shadow"}'::jsonb`)})
	f.usage(t, id, 10_000_000_000)
	if err := f.acct.AccountTask(ctx, util.MustParseUUID(id)); err != nil {
		t.Fatal(err)
	}
	task, err := f.q.GetAgentTask(ctx, util.MustParseUUID(id))
	if err != nil {
		t.Fatal(err)
	}
	refusal, err := f.tasks.claimRefusalFor(WithShadowCapableClaim(ctx, true), f.q, f.ws, task)
	if err != nil {
		t.Fatalf("claim gate: %v", err)
	}
	if refusal == nil || refusal.Reason != VerdictReasonBudgetHold {
		t.Fatalf("a shadow run at the budget's limit: refusal %+v, want %q", refusal, VerdictReasonBudgetHold)
	}
}

// TestShadowCostEstimateWarns: the estimate is the seat's recent average cost times the sample
// rate and the target, and it warns when that exceeds what the workspace budget has left.
func TestShadowCostEstimateWarns(t *testing.T) {
	ctx := context.Background()
	t.Run("over the remaining budget", func(t *testing.T) {
		f := newBudgetFixture(t)
		f.company(t, BudgetModeHold, 1_000_000)
		seed := f.queued(t, testutil.Cols{"status": "completed", "completed_at": testutil.Raw("now() - interval '1 day'")})
		f.usage(t, seed, 10_000_000_000)
		svc := &ShadowService{Queries: f.q}
		est, err := svc.Estimate(ctx, f.ws, util.MustParseUUID(f.agent), 1, 20)
		if err != nil {
			t.Fatalf("Estimate: %v", err)
		}
		if est.Micros != 20_000_000 || !est.Warn || est.RemainingMicros != 1_000_000 {
			t.Errorf("estimate = %+v, want 20000000 micros, a warning, and 1000000 left", est)
		}
	})
	t.Run("within the budget", func(t *testing.T) {
		f := newBudgetFixture(t)
		f.company(t, BudgetModeHold, 100_000_000)
		seed := f.queued(t, testutil.Cols{"status": "completed", "completed_at": testutil.Raw("now() - interval '1 day'")})
		f.usage(t, seed, 10_000_000_000)
		svc := &ShadowService{Queries: f.q}
		est, err := svc.Estimate(ctx, f.ws, util.MustParseUUID(f.agent), 1, 20)
		if err != nil {
			t.Fatalf("Estimate: %v", err)
		}
		if est.Warn {
			t.Errorf("estimate %+v warns within a 100 USD budget", est)
		}
	})
}
