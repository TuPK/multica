package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/runtimeapps"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// TestShadowSelectionDeterministic: the same issue and config always make the same
// choice (spec 9.4), the rate bounds the share chosen, and rate 1 chooses every issue.
func TestShadowSelectionDeterministic(t *testing.T) {
	const config = "cfg-0001"
	first := ShadowSelected("issue-1", config, 0.3)
	for i := 0; i < 5; i++ {
		if got := ShadowSelected("issue-1", config, 0.3); got != first {
			t.Fatalf("call %d chose %v, first call chose %v", i, got, first)
		}
	}
	if !ShadowSelected("any-issue", config, 1) {
		t.Error("rate 1 must choose every issue")
	}
	chosen := 0
	const n = 4000
	for i := 0; i < n; i++ {
		if ShadowSelected(fmt.Sprintf("issue-%d", i), config, 0.3) {
			chosen++
		}
	}
	if share := float64(chosen) / n; math.Abs(share-0.3) > 0.03 {
		t.Errorf("share chosen = %.3f, want about 0.30", share)
	}
}

// TestComparisonBlindAB: the juror's A and B are assigned per pair, so both orders occur
// across pairs, and the verdict maps back to the right side whichever label is primary's.
func TestComparisonBlindAB(t *testing.T) {
	primaryA, primaryB := 0, 0
	for i := 0; i < 200; i++ {
		if ShadowPrimaryIsA(fmt.Sprintf("pair-%d", i)) {
			primaryA++
		} else {
			primaryB++
		}
	}
	if primaryA == 0 || primaryB == 0 {
		t.Fatalf("label order never varies across pairs: primary=A %d, primary=B %d", primaryA, primaryB)
	}

	cases := []struct {
		verdict  string
		primaryA bool
		want     Comparison
	}{
		{"A", true, ComparisonPrimary},
		{"B", true, ComparisonShadow},
		{"A", false, ComparisonShadow},
		{"B", false, ComparisonPrimary},
		{"tie", true, ComparisonTie},
		{"unable", false, ComparisonUnable},
	}
	for _, c := range cases {
		got, err := ResolveShadowVerdict(c.verdict, c.primaryA)
		if err != nil {
			t.Fatalf("verdict %q primaryA=%v: %v", c.verdict, c.primaryA, err)
		}
		if got != c.want {
			t.Errorf("verdict %q primaryA=%v = %q, want %q", c.verdict, c.primaryA, got, c.want)
		}
	}
	if _, err := ResolveShadowVerdict("both", true); err == nil {
		t.Error("an unknown verdict must be refused")
	}
}

// TestShadowReportNotLosingBound: the report's bound is the Wilson lower limit of
// (shadow + tie) / n (spec 9.4). 12 of 20 not losing gives 0.3865 at 95%.
func TestShadowReportNotLosingBound(t *testing.T) {
	var results []ShadowPairResult
	add := func(c Comparison, n int, primaryCost, shadowCost int64) {
		for i := 0; i < n; i++ {
			results = append(results, ShadowPairResult{Comparison: c, PrimaryCostMicros: primaryCost, ShadowCostMicros: shadowCost})
		}
	}
	add(ComparisonShadow, 8, 100, 200)
	add(ComparisonTie, 4, 100, 200)
	add(ComparisonPrimary, 8, 100, 200)
	add(ComparisonUnable, 3, 0, 0)

	r := BuildShadowReport(results)

	if r.Pairs != 20 || r.Shadow != 8 || r.Tie != 4 || r.Primary != 8 || r.Unable != 3 {
		t.Fatalf("counts = %+v, want 20 compared pairs with 8 shadow, 4 tie, 8 primary, 3 unable", r)
	}
	if math.Abs(r.NotLosingLower-0.3865) > 0.001 {
		t.Errorf("NotLosingLower = %.4f, want 0.3865", r.NotLosingLower)
	}
	if r.PrimaryCostAvgMicros != 100 || r.ShadowCostAvgMicros != 200 {
		t.Errorf("average costs = %d/%d, want 100/200", r.PrimaryCostAvgMicros, r.ShadowCostAvgMicros)
	}
}

// TestShadowTaskClassification: a shadow task is the shadow run class, a budget that
// holds routine holds it, and the effect policy forbids it everything but read (D18).
func TestShadowTaskClassification(t *testing.T) {
	if got := durableRunClassOf([]byte(`{"durable_kind":"shadow","shadow_pair_id":"p"}`)); got != RunClassShadow {
		t.Fatalf("run class = %q, want %q", got, RunClassShadow)
	}
	if !heldRunClasses()[RunClassShadow] {
		t.Error("the shadow class must be held by a budget and by a paused company")
	}
	if !budgetHoldsClass([]string{string(RunClassRoutine)}, RunClassShadow) {
		t.Error("a routine-only budget must hold shadow runs")
	}
	if !taskClassForbids(runClassShadow, connectedapps.EffectWrite) {
		t.Error("a shadow run must be refused write effects")
	}
	if !taskClassForbids(runClassShadow, connectedapps.EffectWriteInternal) {
		t.Error("a shadow run must be refused write_internal effects")
	}
	if taskClassForbids(runClassShadow, connectedapps.EffectRead) {
		t.Error("a shadow run may read")
	}
}

// shadowFixture is a company with a seat (the primary agent), a candidate runtime, and
// an issue. The shadow service is wired with the flag set by the test.
type shadowFixture struct {
	principalFixture
	ws       pgtype.UUID
	owner    GovernanceActor
	runtime  string
	seat     string
	cand     string
	issue    string
	svc      *ShadowService
	provider *featureflag.StaticProvider
}

func newShadowFixture(t *testing.T, flagOn bool) shadowFixture {
	t.Helper()
	f, _ := companyWorkspace(t)
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableSeats, featureflag.Rule{Default: flagOn})
	runtime := f.Runtime(t, "shadow primary runtime")
	candidate := f.Runtime(t, "shadow candidate runtime", testutil.Cols{"provider": "openai"})
	sf := shadowFixture{
		principalFixture: f,
		ws:               util.MustParseUUID(f.WorkspaceID),
		owner:            GovernanceActor{Type: GovernanceActorMember, ID: util.MustParseUUID(f.UserID)},
		runtime:          runtime,
		seat:             f.Agent(t, "seat", runtime),
		cand:             candidate,
		issue:            f.Issue(t, "shadow issue"),
		provider:         provider,
	}
	sf.svc = &ShadowService{
		Queries:   f.q,
		TxStarter: f.Pool,
		Tasks:     &TaskService{Queries: f.q, TxStarter: f.Pool, Bus: events.New()},
		Flags:     featureflag.NewService(provider),
	}
	t.Cleanup(func() {
		f.Cleanup(t, `DELETE FROM shadow_pair WHERE config_id IN (SELECT id FROM shadow_config WHERE workspace_id = $1)`, f.WorkspaceID)
		f.Cleanup(t, `DELETE FROM shadow_config WHERE workspace_id = $1`, f.WorkspaceID)
	})
	return sf
}

// start opens a shadow run on the seat at rate 1, so every primary task is chosen.
func (sf shadowFixture) start(t *testing.T) db.ShadowConfig {
	t.Helper()
	cfg, err := sf.svc.Start(context.Background(), sf.ws, sf.owner, ShadowStartInput{
		AgentID: util.MustParseUUID(sf.seat), CandidateRuntimeID: util.MustParseUUID(sf.cand),
		CandidateModel: "gpt-candidate", Purpose: ShadowPurposeModelTrial, SampleRate: 1, TargetPairs: 20,
	})
	if err != nil {
		t.Fatalf("start shadow: %v", err)
	}
	return cfg
}

// primaryTask is a claimed task of the seat on the issue, as the claim path hands it to
// the shadow hook.
func (sf shadowFixture) primaryTask(t *testing.T) db.AgentTaskQueue {
	t.Helper()
	id := sf.Task(t, sf.seat, testutil.Cols{"issue_id": sf.issue, "status": "running", "runtime_id": sf.runtime})
	task, err := sf.q.GetAgentTask(context.Background(), util.MustParseUUID(id))
	if err != nil {
		t.Fatalf("load primary task: %v", err)
	}
	return task
}

// TestShadowFlagOffStopsWrites: with durable_seats off, a claimed primary writes no pair
// and enqueues no shadow task, even when the run is running (spec 0 rule 5).
func TestShadowFlagOffStopsWrites(t *testing.T) {
	sf := newShadowFixture(t, true)
	sf.start(t)
	task := sf.primaryTask(t)
	// The run is running when the owner turns the flag off.
	sf.provider.Set(featureflags.DurableSeats, featureflag.Rule{Default: false})

	if err := sf.svc.OnPrimaryClaimed(context.Background(), task); err != nil {
		t.Fatalf("OnPrimaryClaimed: %v", err)
	}
	if n := sf.Count(t, `SELECT COUNT(*) FROM shadow_pair WHERE workspace_id = $1`, sf.WorkspaceID); n != 0 {
		t.Errorf("shadow_pair rows with the flag off = %d, want 0", n)
	}
	if n := sf.Count(t, `SELECT COUNT(*) FROM agent_task_queue WHERE context->>'durable_kind' = 'shadow'`); n != 0 {
		t.Errorf("shadow tasks with the flag off = %d, want 0", n)
	}
	if _, err := sf.svc.Start(context.Background(), sf.ws, sf.owner, ShadowStartInput{
		AgentID: util.MustParseUUID(sf.seat), CandidateRuntimeID: util.MustParseUUID(sf.cand),
		CandidateModel: "gpt-candidate", Purpose: ShadowPurposeModelTrial, SampleRate: 1, TargetPairs: 20,
	}); !errors.Is(err, ErrDurableSeatsOff) {
		t.Errorf("starting a run with the flag off: err = %v, want ErrDurableSeatsOff", err)
	}
}

// TestShadowOneTaskPerPrimary: a chosen primary gets one pair and one shadow task on the
// shadow agent; a second claim of the same primary changes nothing. The shadow agent
// runs the candidate runtime and model, and is linked to the seat.
func TestShadowOneTaskPerPrimary(t *testing.T) {
	sf := newShadowFixture(t, true)
	sf.start(t)
	task := sf.primaryTask(t)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if err := sf.svc.OnPrimaryClaimed(ctx, task); err != nil {
			t.Fatalf("OnPrimaryClaimed call %d: %v", i, err)
		}
	}

	if n := sf.Count(t, `SELECT COUNT(*) FROM shadow_pair WHERE primary_task_id = $1`, util.UUIDToString(task.ID)); n != 1 {
		t.Errorf("pairs for the primary = %d, want 1", n)
	}
	if n := sf.Count(t, `SELECT COUNT(*) FROM agent_task_queue WHERE context->>'durable_kind' = 'shadow' AND context->>'shadow_pair_id' IS NOT NULL`); n != 1 {
		t.Errorf("shadow tasks = %d, want 1", n)
	}
	var shadowAgent string
	err := sf.Pool.QueryRow(ctx, `
		SELECT a.id::text FROM agent a
		WHERE a.shadow_of_agent_id = $1 AND a.runtime_id = $2 AND a.model = 'gpt-candidate'`,
		sf.seat, sf.cand).Scan(&shadowAgent)
	if err != nil {
		t.Fatalf("shadow agent on the candidate runtime: %v", err)
	}
	if shadowAgent == sf.seat {
		t.Error("the shadow task must run on a shadow agent, not the seat")
	}
}

// TestShadowVerdictAndReport: verdicts recorded on pairs make the run's report, through
// the same blind mapping the juror uses.
func TestShadowVerdictAndReport(t *testing.T) {
	sf := newShadowFixture(t, true)
	cfg := sf.start(t)
	ctx := context.Background()
	task := sf.primaryTask(t)
	if err := sf.svc.OnPrimaryClaimed(ctx, task); err != nil {
		t.Fatalf("OnPrimaryClaimed: %v", err)
	}
	pair, err := sf.q.GetShadowPairForPrimary(ctx, task.ID)
	if err != nil {
		t.Fatalf("load pair: %v", err)
	}

	if err := sf.svc.RecordVerdict(ctx, sf.ws, pair.ID, "B"); err != nil {
		t.Fatalf("RecordVerdict: %v", err)
	}
	got, err := sf.q.GetShadowPairForPrimary(ctx, task.ID)
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

	report, err := sf.svc.Report(ctx, sf.ws, cfg.ID)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if report.Pairs != 1 {
		t.Errorf("report pairs = %d, want 1", report.Pairs)
	}
}

func TestShadowEnqueueFailureRollsBackAndRetries(t *testing.T) {
	sf := newShadowFixture(t, true)
	cfg := sf.start(t)
	primary := sf.primaryTask(t)
	ctx := context.Background()
	if _, err := sf.Pool.Exec(ctx, `UPDATE issue SET triage_state = 'pending' WHERE id = $1`, sf.issue); err != nil {
		t.Fatal(err)
	}
	if err := sf.svc.OnPrimaryClaimed(ctx, primary); !errors.Is(err, ErrIssueInTriage) {
		t.Fatalf("enqueue error = %v, want triage rejection", err)
	}
	if n := sf.Count(t, `SELECT count(*) FROM shadow_pair WHERE primary_task_id = $1`, util.UUIDToString(primary.ID)); n != 0 {
		t.Fatalf("pairs after failed enqueue = %d, want 0", n)
	}
	if n := sf.Count(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND agent_id IN (SELECT id FROM agent WHERE shadow_of_agent_id = $2)`, sf.issue, sf.seat); n != 0 {
		t.Fatalf("shadow tasks after failed enqueue = %d, want 0", n)
	}
	if _, err := sf.Pool.Exec(ctx, `UPDATE issue SET triage_state = NULL WHERE id = $1`, sf.issue); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := sf.svc.OnPrimaryClaimed(ctx, primary); err != nil {
			t.Fatalf("retry %d: %v", i, err)
		}
	}
	if n := sf.Count(t, `SELECT count(*) FROM shadow_pair p JOIN agent_task_queue t ON t.id = p.shadow_task_id WHERE p.primary_task_id = $1 AND t.context->>'durable_kind' = 'shadow' AND t.context->>'shadow_config_id' = $2 AND t.context->>'shadow_pair_id' = p.id::text`, util.UUIDToString(primary.ID), util.UUIDToString(cfg.ID)); n != 1 {
		t.Fatalf("linked, marked pairs after retry = %d, want 1", n)
	}
	if n := sf.Count(t, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1 AND agent_id IN (SELECT id FROM agent WHERE shadow_of_agent_id = $2)`, sf.issue, sf.seat); n != 1 {
		t.Fatalf("shadow tasks after retry = %d, want 1", n)
	}
}

func TestShadowOutputIsNotPostedAsComment(t *testing.T) {
	sf := newShadowFixture(t, true)
	sf.Cleanup(t, `DELETE FROM comment WHERE issue_id = $1`, sf.issue)
	sf.start(t)
	primary := sf.primaryTask(t)
	ctx := context.Background()
	if err := sf.svc.OnPrimaryClaimed(ctx, primary); err != nil {
		t.Fatal(err)
	}
	pair, err := sf.q.GetShadowPairForPrimary(ctx, primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	shadow, err := sf.q.GetAgentTask(ctx, pair.ShadowTaskID)
	if err != nil {
		t.Fatal(err)
	}
	sf.completeTask(t, util.UUIDToString(primary.ID), "ordinary output")
	sf.completeTask(t, util.UUIDToString(shadow.ID), "shadow output")
	if n := sf.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1 AND source_task_id = $2 AND author_type = 'agent'`, sf.issue, primary.ID); n != 1 {
		t.Errorf("ordinary task comments = %d, want 1", n)
	}
	if n := sf.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1 AND source_task_id = $2 AND author_type = 'agent' AND author_id = $3`, sf.issue, shadow.ID, shadow.AgentID); n != 0 {
		t.Errorf("shadow task comments = %d, want 0", n)
	}
}

func TestShadowTaskHasNoGatewayOverlayOrAppMetadata(t *testing.T) {
	sf := newShadowFixture(t, true)
	ctx := context.Background()
	sf.Exec(t, `UPDATE issue SET creator_type = 'member', creator_id = $2 WHERE id = $1`, sf.issue, sf.UserID)
	sf.Exec(t, `UPDATE agent SET composio_toolkit_allowlist = ARRAY['notion'] WHERE id = $1`, sf.seat)
	builder := &stubOverlayBuilder{resp: json.RawMessage(`{"mcpServers":{"composio":{"type":"http","url":"https://mcp.example/session"}}}`), apps: []runtimeapps.ConnectedApp{{Provider: "composio", ServerName: "composio", ToolkitSlug: "notion", ToolkitName: "Notion"}}}
	sf.svc.Tasks.Composio = builder
	// ComposioMCPApps enables the gateway overlay.
	sf.svc.Tasks.FeatureFlags = composioMCPAppsTestFlags(true)
	sf.start(t)
	primary := sf.primaryTask(t)
	if err := sf.svc.OnPrimaryClaimed(ctx, primary); err != nil {
		t.Fatal(err)
	}
	pair, err := sf.q.GetShadowPairForPrimary(ctx, primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	issue, err := sf.q.GetIssue(ctx, util.MustParseUUID(sf.issue))
	if err != nil {
		t.Fatal(err)
	}
	shadow, err := sf.q.GetAgentTask(ctx, pair.ShadowTaskID)
	if err != nil {
		t.Fatal(err)
	}
	check := func(id pgtype.UUID, want bool) {
		t.Helper()
		var overlay, apps []byte
		if err := sf.Pool.QueryRow(ctx, `SELECT runtime_mcp_overlay, runtime_connected_apps FROM agent_task_queue WHERE id = $1`, id).Scan(&overlay, &apps); err != nil {
			t.Fatal(err)
		}
		if (len(overlay) > 0) != want || (len(apps) > 0) != want {
			t.Errorf("task %s: overlay=%s apps=%s, want present=%v", util.UUIDToString(id), overlay, apps, want)
		}
	}
	check(shadow.ID, false)
	sf.Exec(t, `UPDATE agent_task_queue SET status = 'cancelled' WHERE id = $1`, shadow.ID)
	ordinary, err := sf.svc.Tasks.EnqueueVerificationTask(ctx, issue, shadow.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	check(ordinary.ID, true)
}

// TestShadowStartConcurrentSingleRunning: two concurrent Start calls for the same seat
// yield exactly one running shadow_config; the other returns ErrShadowAlreadyRunning.
func TestShadowStartConcurrentSingleRunning(t *testing.T) {
	sf := newShadowFixture(t, true)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		seatID := sf.Agent(t, fmt.Sprintf("concurrent-seat-%d", i), sf.runtime)
		in := ShadowStartInput{
			AgentID: util.MustParseUUID(seatID), CandidateRuntimeID: util.MustParseUUID(sf.cand),
			CandidateModel: "gpt-candidate", Purpose: ShadowPurposeModelTrial, SampleRate: 1, TargetPairs: 20,
		}
		var ready, done sync.WaitGroup
		ready.Add(2)
		done.Add(2)
		release := make(chan struct{})
		errs := make([]error, 2)
		for g := 0; g < 2; g++ {
			go func(idx int) {
				defer done.Done()
				ready.Done()
				<-release
				_, errs[idx] = sf.svc.Start(ctx, sf.ws, sf.owner, in)
			}(g)
		}
		ready.Wait()
		close(release)
		done.Wait()

		var okCount, alreadyCount int
		for _, err := range errs {
			switch {
			case err == nil:
				okCount++
			case errors.Is(err, ErrShadowAlreadyRunning):
				alreadyCount++
			default:
				t.Fatalf("attempt %d: unexpected error: %v", i, err)
			}
		}
		if okCount != 1 || alreadyCount != 1 {
			t.Fatalf("attempt %d: ok=%d already_running=%d errs=%v, want 1 and 1", i, okCount, alreadyCount, errs)
		}
		if n := sf.Count(t, `SELECT count(*) FROM shadow_config WHERE workspace_id = $1 AND agent_id = $2 AND state = 'running'`, sf.WorkspaceID, seatID); n != 1 {
			t.Fatalf("attempt %d: running configs = %d, want 1", i, n)
		}
	}
}

// TestShadowStartAllowedAfterStopped: a stopped shadow run does not block starting a new one
// on the same seat.
func TestShadowStartAllowedAfterStopped(t *testing.T) {
	sf := newShadowFixture(t, true)
	ctx := context.Background()

	first := sf.start(t)
	if _, err := sf.q.SetShadowConfigState(ctx, db.SetShadowConfigStateParams{
		ID: first.ID, WorkspaceID: sf.ws, State: "stopped",
	}); err != nil {
		t.Fatalf("stop first run: %v", err)
	}

	second := sf.start(t)
	if second.ID == first.ID {
		t.Fatalf("second run reused first run id %s", util.UUIDToString(first.ID))
	}
	if n := sf.Count(t, `SELECT count(*) FROM shadow_config WHERE workspace_id = $1 AND agent_id = $2 AND state = 'running'`, sf.WorkspaceID, sf.seat); n != 1 {
		t.Fatalf("running configs = %d, want 1", n)
	}
	if n := sf.Count(t, `SELECT count(*) FROM shadow_config WHERE workspace_id = $1 AND agent_id = $2 AND state = 'stopped'`, sf.WorkspaceID, sf.seat); n != 1 {
		t.Fatalf("stopped configs = %d, want 1", n)
	}
}
