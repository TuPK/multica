package service

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// seedTrack writes the 30-day track record row of an agent's general work, as a
// recompute would have left it. Cleanup removes it.
func seedTrack(t *testing.T, vf verificationFixture, agent, mode string, nPass int, score float64) {
	t.Helper()
	vf.Exec(t, `INSERT INTO track_record (workspace_id, agent_id, work_class, window_days, model,
		n_pass, verification_mode, score_lower, mode_changed_at)
		VALUES ($1, $2, 'general', 30, '', $3, $4, $5, now() - interval '30 days')`,
		vf.WorkspaceID, agent, nPass, mode, score)
	vf.Cleanup(t, `DELETE FROM track_record WHERE agent_id = $1`, agent)
	vf.Cleanup(t, `DELETE FROM governance_event WHERE workspace_id = $1 AND kind = 'verification_mode_changed'`, vf.WorkspaceID)
}

// trackRow is the stored track record of one window of an agent's general work.
type trackRow struct {
	NPass, NPartial, NFail, NUnverified, NHighFail int
	ScoreLower                                     float64
	VerificationMode                               string
	ModePinned                                     bool
	ModeChanged                                    bool
}

func readTrack(t *testing.T, f principalFixture, agent string, window int) trackRow {
	t.Helper()
	var r trackRow
	f.QueryRow(t, `SELECT n_pass, n_partial, n_fail, n_unverified, n_high_fail, score_lower::float8,
		verification_mode, mode_pinned, mode_changed_at IS NOT NULL
		FROM track_record WHERE workspace_id = $1 AND agent_id = $2 AND work_class = 'general'
		AND window_days = $3 AND model = ''`, f.WorkspaceID, agent, window).
		Scan(&r.NPass, &r.NPartial, &r.NFail, &r.NUnverified, &r.NHighFail, &r.ScoreLower,
			&r.VerificationMode, &r.ModePinned, &r.ModeChanged)
	return r
}

// settleAs moves an outcome to a settled verdict, settled days ago.
func settleAs(t *testing.T, f principalFixture, outcome, verdict, severity string, days int) {
	t.Helper()
	f.Exec(t, `UPDATE work_outcome SET verdict = $2, severity = $3, settled_at = now() - make_interval(days => $4::int)
		WHERE id = $1`, outcome, verdict, severity, days)
}

// TestRecomputeCountsSettledOutcomesPerWindow: the 30-day row counts the settled
// outcomes of the last 30 days, and the 90-day row adds the older ones. Pending
// outcomes are not counted, unverified ones are counted apart and do not reach the score.
func TestRecomputeCountsSettledOutcomesPerWindow(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	agent := util.UUIDToString(util.MustParseUUID(vf.doer))
	vf.Cleanup(t, `DELETE FROM track_record WHERE agent_id = $1`, agent)

	settle := func(verdict, severity string, days int) {
		o := vf.currentOutcome(t, vf.doneIssue(t, vf.doer))
		settleAs(t, vf.principalFixture, util.UUIDToString(o.ID), verdict, severity, days)
	}
	settle("pass", "none", 1)
	settle("pass", "none", 2)
	settle("partial", "low", 3)
	settle("fail", "low", 4)
	settle("unverified", "none", 5)
	settle("pass", "none", 60)                     // counts in the 90-day window only
	vf.currentOutcome(t, vf.doneIssue(t, vf.doer)) // pending: never counted

	if err := recordTrack(ctx, vf.q, vf.ws, util.MustParseUUID(vf.doer), "general", time.Now()); err != nil {
		t.Fatal(err)
	}

	r30 := readTrack(t, vf.principalFixture, agent, 30)
	if r30.NPass != 2 || r30.NPartial != 1 || r30.NFail != 1 || r30.NUnverified != 1 {
		t.Fatalf("30-day counts = %+v, want pass 2, partial 1, fail 1, unverified 1", r30)
	}
	if want := WilsonLower(2, 1, 1); math.Abs(r30.ScoreLower-want) > 1e-6 {
		t.Fatalf("30-day score = %.6f, want %.6f", r30.ScoreLower, want)
	}
	r90 := readTrack(t, vf.principalFixture, agent, 90)
	if r90.NPass != 3 || r90.NPartial != 1 || r90.NFail != 1 {
		t.Fatalf("90-day counts = %+v, want pass 3, partial 1, fail 1", r90)
	}
}

// TestHighFailRecomputesImmediately: a settled fail of severity high moves the pair to
// full in the same settlement, without the nightly job.
func TestHighFailRecomputesImmediately(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	agent := vf.doer
	seedTrack(t, vf, agent, "sampled", 20, 0.9)

	issue := vf.doneIssue(t, agent)
	c := vf.addCriterion(t, issue, "The report has a header.", CheckKindManual, map[string]any{})
	o := vf.currentOutcome(t, issue)
	outcomeID := util.UUIDToString(o.ID)
	// Settlement only checks a sampled outcome, and this one is sampled by the seeded pair.
	vf.Exec(t, `UPDATE work_outcome SET sampled = true, settle_at = now() - interval '1 minute' WHERE id = $1`, outcomeID)
	if _, err := vf.checks.Record(ctx, vf.ws, vf.member, o.ID, VerificationInput{
		CriterionID: c.ID, Result: "fail", Severity: "high", Note: "missing header",
	}); err != nil {
		t.Fatalf("record check: %v", err)
	}

	if _, err := vf.outcomes.Settle(ctx, time.Now()); err != nil {
		t.Fatalf("settle: %v", err)
	}

	got := readTrack(t, vf.principalFixture, agent, 30)
	if got.VerificationMode != "full" || got.NHighFail != 1 {
		t.Fatalf("after settling a high fail the 30-day row is %+v, want full with one high fail", got)
	}
	if n := vf.Count(t, `SELECT count(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'verification_mode_changed'
		AND subject_id = $2 AND actor_type = 'track_record'`, vf.WorkspaceID, agent); n != 1 {
		t.Fatalf("verification_mode_changed events = %d, want one", n)
	}
}

// TestOutcomeSampledFollowsTrackRecord: a new outcome takes the mode its pair has, and
// it is sampled exactly when the deterministic choice of its id says so.
func TestOutcomeSampledFollowsTrackRecord(t *testing.T) {
	vf := newVerificationFixture(t)
	seedTrack(t, vf, vf.doer, "sampled", 20, 0.9)

	issue := vf.doneIssue(t, vf.doer)
	o := vf.currentOutcome(t, issue)
	if o.VerificationMode != "sampled" {
		t.Fatalf("verification_mode = %q, want sampled from the track record", o.VerificationMode)
	}
	if want := Sampled(util.UUIDToString(o.ID), "sampled"); o.Sampled != want {
		t.Fatalf("sampled = %v, want %v from the id", o.Sampled, want)
	}
}

// TestModeForNewPairStartsFull: a pair with no track record verifies in full.
func TestModeForNewPairStartsFull(t *testing.T) {
	vf := newVerificationFixture(t)
	svc := &TrackRecordService{Queries: vf.q, TxStarter: vf.Pool}
	mode, err := svc.ModeFor(context.Background(), vf.ws, util.MustParseUUID(vf.doer), "general")
	if err != nil {
		t.Fatal(err)
	}
	if mode != "full" {
		t.Fatalf("ModeFor = %q, want full", mode)
	}
}

// TestOutcomeKeepsModelAtCreation: an outcome keeps the runtime provider and model its
// agent had when it was created. A later model change shows only on the next outcome.
func TestOutcomeKeepsModelAtCreation(t *testing.T) {
	f, _ := companyWorkspace(t)
	svc, _ := outcomeService(t, f, true)
	runtime := f.Runtime(t, "model runtime", testutil.Cols{"provider": "codex"})
	agent := f.Agent(t, "model doer", runtime, testutil.Cols{"model": "model-v1"})
	issue := outcomeIssue(t, f, "todo")
	f.Exec(t, `UPDATE issue SET assignee_type = 'agent', assignee_id = $2 WHERE id = $1`, issue, agent)

	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	runOutcomeJob(t, svc)
	first := readOutcomeModel(t, f, issue)

	f.Exec(t, `UPDATE agent SET model = 'model-v2' WHERE id = $1`, agent)
	f.Exec(t, `UPDATE issue SET status = 'in_progress' WHERE id = $1`, issue)
	f.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, issue)
	runOutcomeJob(t, svc)
	second := readOutcomeModel(t, f, issue)

	if first != "codex/model-v1" {
		t.Fatalf("first outcome model = %q, want codex/model-v1", first)
	}
	if second != "codex/model-v2" {
		t.Fatalf("second outcome model = %q, want codex/model-v2", second)
	}
	if got := readOutcomeModelByID(t, f, issue, true); got != first {
		t.Fatalf("superseded outcome model changed to %q, want %q", got, first)
	}
}

// readOutcomeModel returns "provider/model" of the issue's current outcome.
func readOutcomeModel(t *testing.T, f principalFixture, issue string) string {
	t.Helper()
	return readOutcomeModelByID(t, f, issue, false)
}

func readOutcomeModelByID(t *testing.T, f principalFixture, issue string, superseded bool) string {
	t.Helper()
	cond := "superseded_by IS NULL"
	if superseded {
		cond = "superseded_by IS NOT NULL"
	}
	var provider, model string
	f.QueryRow(t, fmt.Sprintf(`SELECT runtime_provider, model FROM work_outcome
		WHERE issue_id = $1 AND %s ORDER BY created_at LIMIT 1`, cond), issue).Scan(&provider, &model)
	return provider + "/" + model
}

// TestPinnedModeSurvivesRecompute: a pinned mode holds through recomputes; unpinning
// leaves the mode and the next recompute moves it. Each mode change writes one event,
// by the member who pinned and then by the track record.
func TestPinnedModeSurvivesRecompute(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	agent := vf.doer
	vf.Cleanup(t, `DELETE FROM track_record WHERE agent_id = $1`, agent)
	svc := &TrackRecordService{Queries: vf.q, TxStarter: vf.Pool, Flags: vf.outcomes.Flags}
	spot := "spot"

	if _, err := svc.Pin(ctx, vf.ws, vf.member, util.MustParseUUID(agent), "general", &spot); err != nil {
		t.Fatalf("pin spot: %v", err)
	}
	if got := readTrack(t, vf.principalFixture, agent, 30); got.VerificationMode != "spot" || !got.ModePinned {
		t.Fatalf("after pin = %+v, want pinned spot", got)
	}
	if err := recordTrack(ctx, vf.q, vf.ws, util.MustParseUUID(agent), "general", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := readTrack(t, vf.principalFixture, agent, 30); got.VerificationMode != "spot" {
		t.Fatalf("pinned mode after recompute = %q, want spot", got.VerificationMode)
	}

	if _, err := svc.Pin(ctx, vf.ws, vf.member, util.MustParseUUID(agent), "general", nil); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	if err := recordTrack(ctx, vf.q, vf.ws, util.MustParseUUID(agent), "general", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := readTrack(t, vf.principalFixture, agent, 30); got.VerificationMode != "sampled" || got.ModePinned {
		// No outcome in 90 days: spot falls one step to sampled once it is unpinned.
		t.Fatalf("after unpin and recompute = %+v, want sampled and unpinned", got)
	}

	rows := vf.Count(t, `SELECT count(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'verification_mode_changed' AND subject_id = $2`, vf.WorkspaceID, agent)
	if rows != 2 {
		t.Fatalf("mode change events = %d, want 2 (the pin, then the recompute)", rows)
	}
	var actors string
	vf.QueryRow(t, `SELECT string_agg(actor_type, ',' ORDER BY created_at, id) FROM governance_event
		WHERE workspace_id = $1 AND kind = 'verification_mode_changed' AND subject_id = $2`, vf.WorkspaceID, agent).Scan(&actors)
	if actors != "member,track_record" {
		t.Fatalf("event actors = %q, want member,track_record", actors)
	}
}

// TestRecomputeAllRefreshesPairsWithRecentOutcomes: the nightly job moves a pair whose
// 30-day record meets the bar, and leaves its outcomes alone.
func TestRecomputeAllRefreshesPairsWithRecentOutcomes(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	agent := vf.doer
	vf.Cleanup(t, `DELETE FROM track_record WHERE agent_id = $1`, agent)
	for i := 0; i < 16; i++ {
		o := vf.currentOutcome(t, vf.doneIssue(t, agent))
		settleAs(t, vf.principalFixture, util.UUIDToString(o.ID), "pass", "none", 1)
	}

	flags := featureflag.NewStaticProvider()
	flags.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: true})
	svc := &TrackRecordService{Queries: vf.q, TxStarter: vf.Pool, Flags: featureflag.NewService(flags)}
	if _, err := svc.RecomputeAll(ctx); err != nil {
		t.Fatalf("recompute all: %v", err)
	}

	got := readTrack(t, vf.principalFixture, agent, 30)
	if got.NPass != 16 || got.VerificationMode != "sampled" {
		t.Fatalf("30-day row = %+v, want 16 passes moved to sampled", got)
	}
}

// TestDurableTrackRecordFlagOffWritesNothing: while durable_outcomes is off the nightly
// job writes no track record row.
func TestDurableTrackRecordFlagOffWritesNothing(t *testing.T) {
	vf := newVerificationFixture(t)
	agent := vf.doer
	vf.Cleanup(t, `DELETE FROM track_record WHERE agent_id = $1`, agent)
	settleAs(t, vf.principalFixture, util.UUIDToString(vf.currentOutcome(t, vf.doneIssue(t, agent)).ID), "pass", "none", 1)

	flags := featureflag.NewStaticProvider()
	flags.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: false})
	svc := &TrackRecordService{Queries: vf.q, TxStarter: vf.Pool, Flags: featureflag.NewService(flags)}
	if n, err := svc.RecomputeAll(context.Background()); err != nil || n != 0 {
		t.Fatalf("recompute all with the flag off = %d, %v; want 0, nil", n, err)
	}
	if n := vf.Count(t, `SELECT count(*) FROM track_record WHERE agent_id = $1`, agent); n != 0 {
		t.Fatalf("track record rows = %d, want none while the flag is off", n)
	}
}
