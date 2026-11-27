package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

func runOf(c Comparison, n int) []Comparison {
	out := make([]Comparison, n)
	for i := range out {
		out[i] = c
	}
	return out
}

func joinRuns(runs ...[]Comparison) []Comparison {
	var out []Comparison
	for _, r := range runs {
		out = append(out, r...)
	}
	return out
}

func TestNecessityVerdictTable(t *testing.T) {
	const minPairs = 20
	cases := []struct {
		name        string
		comparisons []Comparison
		margin      float64
		want        string
	}{
		{
			// Spec 10.5 item 2 asks for not_needed here. At margin 0.05 that is not reachable:
			// the 95% upper bound for d at n=40 is 0.0876, so the verdict is inconclusive at 2×minPairs.
			name:        "40 ties at default margin 0.05 is inconclusive",
			comparisons: runOf(ComparisonTie, 40),
			margin:      0.05,
			want:        "inconclusive",
		},
		{
			name:        "40 ties with margin 0.10 is not_needed",
			comparisons: runOf(ComparisonTie, 40),
			margin:      0.10,
			want:        "not_needed",
		},
		{
			name:        "80 ties at margin 0.05 is not_needed",
			comparisons: runOf(ComparisonTie, 80),
			margin:      0.05,
			want:        "not_needed",
		},
		{
			name:        "40 pairs with 30% primary wins is needed",
			comparisons: joinRuns(runOf(ComparisonPrimary, 12), runOf(ComparisonTie, 28)),
			margin:      0.05,
			want:        "needed",
		},
		{
			name:        "exactly minPairs decisive pairs is needed",
			comparisons: joinRuns(runOf(ComparisonPrimary, 12), runOf(ComparisonTie, 8)),
			margin:      0.05,
			want:        "needed",
		},
		{
			name:        "19 ties is below minPairs, running",
			comparisons: runOf(ComparisonTie, 19),
			margin:      0.05,
			want:        "running",
		},
		{
			name:        "25 undecided pairs between minPairs and 2×minPairs is running",
			comparisons: joinRuns(runOf(ComparisonPrimary, 2), runOf(ComparisonShadow, 2), runOf(ComparisonTie, 21)),
			margin:      0.05,
			want:        "running",
		},
		{
			name:        "40 undecided pairs at 2×minPairs is inconclusive",
			comparisons: joinRuns(runOf(ComparisonPrimary, 4), runOf(ComparisonShadow, 3), runOf(ComparisonTie, 33)),
			margin:      0.05,
			want:        "inconclusive",
		},
		{
			// Unable pairs must not count toward n. Counted, n would be 50 and the verdict inconclusive.
			name:        "unable pairs are excluded from n: 30 ties + 20 unable is running",
			comparisons: joinRuns(runOf(ComparisonTie, 30), runOf(ComparisonUnable, 20)),
			margin:      0.05,
			want:        "running",
		},
		{
			// Counted, the 12 primary wins among 40 pairs would make this needed.
			name:        "unable pairs are excluded from n: 12 primary + 28 unable is running",
			comparisons: joinRuns(runOf(ComparisonPrimary, 12), runOf(ComparisonUnable, 28)),
			margin:      0.05,
			want:        "running",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NecessityVerdict(tc.comparisons, tc.margin, minPairs); got != tc.want {
				t.Errorf("NecessityVerdict = %q, want %q", got, tc.want)
			}
		})
	}
}

// necessityEnv is an organisation whose seat sits in a squad and holds two skills: the one
// under test and another. The seat's runtime is set, and durable_seats is on.
type necessityEnv struct {
	*orgEnv
	seat    string
	runtime string
	subject string
	other   string
	ns      *NecessityService
	shadow  *ShadowService
	flags   *featureflag.Service
}

func newNecessityEnv(t *testing.T, flagOn bool) necessityEnv {
	t.Helper()
	e := newOrgEnv(t)
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableSeats, featureflag.Rule{Default: flagOn})
	flags := featureflag.NewService(provider)
	seat := util.UUIDToString(e.subject)
	runtime := e.fx.Runtime(t, "necessity runtime")
	e.fx.Exec(t, `UPDATE agent SET runtime_id = $2, model = 'claude-seat', home_squad_id = $3 WHERE id = $1`,
		seat, runtime, util.UUIDToString(e.squad))
	ne := necessityEnv{orgEnv: e, seat: seat, runtime: runtime, flags: flags}
	ne.subject = ne.skill(t, "under test")
	ne.other = ne.skill(t, "kept")
	ne.link(t, seat, ne.subject)
	ne.link(t, seat, ne.other)
	ne.shadow = &ShadowService{
		Queries:   e.q,
		TxStarter: e.fx.Pool,
		Tasks:     &TaskService{Queries: e.q, TxStarter: e.fx.Pool, Bus: events.New()},
		Flags:     flags,
	}
	ne.ns = &NecessityService{Queries: e.q, TxStarter: e.fx.Pool, Shadow: ne.shadow, Proposals: e.props, Flags: flags}
	ne.shadow.Necessity = ne.ns
	t.Cleanup(func() {
		e.fx.Cleanup(t, `DELETE FROM shadow_pair WHERE config_id IN (SELECT id FROM shadow_config WHERE workspace_id = $1)`, util.UUIDToString(e.ws))
		e.fx.Cleanup(t, `DELETE FROM shadow_config WHERE workspace_id = $1`, util.UUIDToString(e.ws))
		e.fx.Cleanup(t, `DELETE FROM necessity_test WHERE workspace_id = $1`, util.UUIDToString(e.ws))
	})
	return ne
}

func (ne necessityEnv) skill(t *testing.T, label string) string {
	t.Helper()
	return ne.fx.Insert(t, "skill", testutil.Cols{"workspace_id": util.UUIDToString(ne.ws), "name": "necessity " + label + " " + ne.fx.WorkspaceID[:8]})
}

// link gives a seat a skill. agent_skill has no id column, so the row is written directly.
func (ne necessityEnv) link(t *testing.T, agentID, skillID string) {
	t.Helper()
	ne.fx.Exec(t, `INSERT INTO agent_skill (agent_id, skill_id) VALUES ($1, $2)`, agentID, skillID)
}

func (ne necessityEnv) actor() GovernanceActor {
	return GovernanceActor{Type: GovernanceActorMember, ID: ne.owner}
}

// start opens a necessity test on the subject skill and returns it with its shadow run.
func (ne necessityEnv) start(t *testing.T) (db.NecessityTest, db.ShadowConfig) {
	t.Helper()
	test, err := ne.ns.Start(context.Background(), ne.ws, ne.actor(), util.MustParseUUID(ne.seat), util.MustParseUUID(ne.subject))
	if err != nil {
		t.Fatalf("start necessity test: %v", err)
	}
	cfg, err := ne.q.GetShadowConfig(context.Background(), db.GetShadowConfigParams{ID: test.ShadowConfigID, WorkspaceID: ne.ws})
	if err != nil {
		t.Fatalf("load shadow run: %v", err)
	}
	return test, cfg
}

// comparedPairs writes n compared pairs of one kind on a run, each with its own primary task.
func (ne necessityEnv) comparedPairs(t *testing.T, configID string, comparison string, n int) {
	t.Helper()
	issue := ne.fx.Issue(t, "necessity pair issue")
	for i := 0; i < n; i++ {
		task := ne.fx.Task(t, ne.seat, testutil.Cols{"issue_id": issue, "status": "completed", "runtime_id": ne.runtime})
		ne.fx.Insert(t, "shadow_pair", testutil.Cols{
			"workspace_id": util.UUIDToString(ne.ws), "config_id": configID, "issue_id": issue,
			"primary_task_id": task, "comparison": comparison,
		})
	}
}

// necessityProposals counts the skill removals filed for the seat's subject skill.
func (ne necessityEnv) necessityProposals(t *testing.T) int {
	t.Helper()
	return ne.fx.Count(t, `SELECT COUNT(*) FROM proposal WHERE workspace_id = $1 AND kind = 'org_change'
		AND payload->>'type' = 'skill_change' AND payload->'remove_skill_ids' @> to_jsonb(ARRAY[$2::text])`,
		util.UUIDToString(ne.ws), ne.subject)
}

// TestShadowOmitsSkillUnderTest: a necessity run's shadow agent holds the seat's other skills
// and not the one under test, and its task keeps the skills the claim gives it. A model trial's
// shadow task still omits its skills (D30).
func TestShadowOmitsSkillUnderTest(t *testing.T) {
	ne := newNecessityEnv(t, true)
	test, cfg := ne.start(t)
	ne.fx.Exec(t, `UPDATE shadow_config SET sample_rate = 1 WHERE id = $1`, util.UUIDToString(cfg.ID))
	issue := ne.fx.Issue(t, "necessity claim issue")
	primaryID := ne.fx.Task(t, ne.seat, testutil.Cols{"issue_id": issue, "status": "running", "runtime_id": ne.runtime})
	primary, err := ne.q.GetAgentTask(context.Background(), util.MustParseUUID(primaryID))
	if err != nil {
		t.Fatalf("load primary task: %v", err)
	}
	if err := ne.shadow.OnPrimaryClaimed(context.Background(), primary); err != nil {
		t.Fatalf("OnPrimaryClaimed: %v", err)
	}

	cfg, err = ne.q.GetShadowConfig(context.Background(), db.GetShadowConfigParams{ID: cfg.ID, WorkspaceID: ne.ws})
	if err != nil || !cfg.ShadowAgentID.Valid {
		t.Fatalf("the shadow run has no shadow agent: %v", err)
	}
	if n := ne.fx.Count(t, `SELECT COUNT(*) FROM agent_skill WHERE agent_id = $1 AND skill_id = $2`, util.UUIDToString(cfg.ShadowAgentID), ne.subject); n != 0 {
		t.Fatalf("the shadow agent holds the skill under test")
	}
	if n := ne.fx.Count(t, `SELECT COUNT(*) FROM agent_skill WHERE agent_id = $1 AND skill_id = $2`, util.UUIDToString(cfg.ShadowAgentID), ne.other); n != 1 {
		t.Fatalf("the shadow agent lacks the seat's other skill")
	}

	var shadowTask string
	if err := ne.fx.Pool.QueryRow(context.Background(), `SELECT shadow_task_id::text FROM shadow_pair WHERE config_id = $1`, util.UUIDToString(cfg.ID)).Scan(&shadowTask); err != nil {
		t.Fatalf("load shadow task: %v", err)
	}
	shadowRow, err := ne.q.GetAgentTask(context.Background(), util.MustParseUUID(shadowTask))
	if err != nil {
		t.Fatalf("load shadow task row: %v", err)
	}
	if fields := ShadowClaimFieldsOf(shadowRow.Context); fields.OmitSkills {
		t.Fatalf("a necessity shadow task omits skills; it must keep the shadow agent's")
	}
	if fields := ShadowClaimFieldsOf([]byte(`{"durable_kind":"shadow"}`)); !fields.OmitSkills {
		t.Fatalf("a model trial shadow task keeps its skills; D30 says it omits them")
	}
	_ = test
}

// TestNotNeededFilesOneProposal: a necessity run whose comparisons show no loss without the skill
// completes as not_needed and files exactly one skill_change proposal removing it (spec 10.2).
func TestNotNeededFilesOneProposal(t *testing.T) {
	ne := newNecessityEnv(t, true)
	test, cfg := ne.start(t)
	ne.fx.Exec(t, `UPDATE necessity_test SET margin = 0.10 WHERE id = $1`, util.UUIDToString(test.ID))
	ne.comparedPairs(t, util.UUIDToString(cfg.ID), "tie", 40)

	if err := ne.shadow.finishIfComplete(context.Background(), ne.ws, cfg.ID); err != nil {
		t.Fatalf("finishIfComplete: %v", err)
	}
	got, err := ne.q.GetNecessityTest(context.Background(), db.GetNecessityTestParams{ID: test.ID, WorkspaceID: ne.ws})
	if err != nil || got.State != "not_needed" || !got.ProposalID.Valid {
		t.Fatalf("necessity test after 40 ties: state %q proposal %v err %v, want not_needed with a proposal", got.State, got.ProposalID.Valid, err)
	}
	if n := ne.necessityProposals(t); n != 1 {
		t.Fatalf("skill removal proposals = %d, want 1", n)
	}
}

// TestNoSkillRemovalWithoutApproval: a not_needed verdict removes nothing. The seat keeps the
// skill until the owner approves the proposal.
func TestNoSkillRemovalWithoutApproval(t *testing.T) {
	ne := newNecessityEnv(t, true)
	test, cfg := ne.start(t)
	ne.fx.Exec(t, `UPDATE necessity_test SET margin = 0.10 WHERE id = $1`, util.UUIDToString(test.ID))
	ne.comparedPairs(t, util.UUIDToString(cfg.ID), "tie", 40)
	if err := ne.shadow.finishIfComplete(context.Background(), ne.ws, cfg.ID); err != nil {
		t.Fatalf("finishIfComplete: %v", err)
	}
	if n := ne.fx.Count(t, `SELECT COUNT(*) FROM agent_skill WHERE agent_id = $1 AND skill_id = $2`, ne.seat, ne.subject); n != 1 {
		t.Fatalf("the seat lost its skill without an approval (links = %d)", n)
	}
}

// TestWeeklyMaxTwoPerSquad: the weekly job starts at most two necessity tests per squad at a
// time, and a second run in the same week starts nothing more while two are running.
func TestWeeklyMaxTwoPerSquad(t *testing.T) {
	ne := newNecessityEnv(t, true)
	ctx := context.Background()
	runtime := ne.runtime
	for i := 0; i < 5; i++ {
		agent := ne.fx.Agent(t, fmt.Sprintf("weekly seat %d", i), runtime, testutil.Cols{
			"home_squad_id": util.UUIDToString(ne.squad), "model": "claude-seat",
		})
		ne.link(t, agent, ne.skill(t, fmt.Sprintf("weekly %d", i)))
		for k := 0; k < 40; k++ {
			ne.fx.Task(t, agent, testutil.Cols{"status": "completed", "runtime_id": runtime})
		}
	}
	started, err := ne.ns.WeeklySchedule(ctx, ne.ws)
	if err != nil {
		t.Fatalf("WeeklySchedule: %v", err)
	}
	running := ne.fx.Count(t, `SELECT COUNT(*) FROM necessity_test WHERE workspace_id = $1 AND state = 'running'
		AND agent_id IN (SELECT id FROM agent WHERE home_squad_id = $2)`, util.UUIDToString(ne.ws), util.UUIDToString(ne.squad))
	if started != 2 || running != 2 {
		t.Fatalf("weekly started %d, running %d in the squad; want 2 and 2", started, running)
	}
	again, err := ne.ns.WeeklySchedule(ctx, ne.ws)
	if err != nil || again != 0 {
		t.Fatalf("second weekly run started %d (err %v), want 0 while two run", again, err)
	}
}

// TestNecessityFlagOffWritesNothing: while durable_seats is off no test starts and the weekly
// job writes nothing.
func TestNecessityFlagOffWritesNothing(t *testing.T) {
	ne := newNecessityEnv(t, false)
	if _, err := ne.ns.Start(context.Background(), ne.ws, ne.actor(), util.MustParseUUID(ne.seat), util.MustParseUUID(ne.subject)); !errors.Is(err, ErrDurableSeatsOff) {
		t.Fatalf("Start with the flag off: %v, want ErrDurableSeatsOff", err)
	}
	if n, err := ne.ns.WeeklySchedule(context.Background(), ne.ws); err != nil || n != 0 {
		t.Fatalf("WeeklySchedule with the flag off: %d, %v; want 0", n, err)
	}
	if n := ne.fx.Count(t, `SELECT COUNT(*) FROM necessity_test WHERE workspace_id = $1`, util.UUIDToString(ne.ws)); n != 0 {
		t.Fatalf("flag off wrote %d necessity tests", n)
	}
}

// TestFlowStageCandidateThreshold: a gate is a merge-or-drop candidate at 95% first-round
// approval on at least 30 first-round verdicts (spec 10.4).
func TestFlowStageCandidateThreshold(t *testing.T) {
	cases := []struct {
		name string
		s    StageStat
		want bool
	}{
		{"29 of 30 approved first time", StageStat{Passes: 30, ApprovedFirst: 29, Resends: 1}, true},
		{"28 of 30 is below 95%", StageStat{Passes: 30, ApprovedFirst: 28, Resends: 2}, false},
		{"29 of 29 is too few verdicts", StageStat{Passes: 29, ApprovedFirst: 29}, false},
		{"no verdicts", StageStat{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FlowStageCandidate(c.s); got != c.want {
				t.Errorf("FlowStageCandidate(%+v) = %v, want %v", c.s, got, c.want)
			}
		})
	}
}

// TestFlowStageCandidatesCountFirstRound: only round 0 verdicts of a gate count toward its stage;
// a later round's verdict does not.
func TestFlowStageCandidatesCountFirstRound(t *testing.T) {
	ne := newNecessityEnv(t, true)
	ctx := context.Background()
	ws := util.UUIDToString(ne.ws)
	for i := 0; i < 29; i++ {
		ne.fx.Issue(t, "gate pass", testutil.Cols{"flow_step": "collect", "flow_task": "gate", "flow_round": 0, "outcome": "approved"})
	}
	ne.fx.Issue(t, "gate resend", testutil.Cols{"flow_step": "collect", "flow_task": "gate", "flow_round": 0, "outcome": "rejected"})
	ne.fx.Issue(t, "gate later round", testutil.Cols{"flow_step": "collect", "flow_task": "gate", "flow_round": 1, "outcome": "approved"})

	stats, err := ne.ns.FlowStageCandidates(ctx, ne.ws)
	if err != nil {
		t.Fatalf("FlowStageCandidates: %v", err)
	}
	var found *StageStat
	for i := range stats {
		if stats[i].Step == "collect" && stats[i].Task == "gate" {
			found = &stats[i]
		}
	}
	if found == nil || found.Passes != 30 || found.ApprovedFirst != 29 || found.Resends != 1 {
		t.Fatalf("collect/gate stats = %+v, want 30 passes, 29 approved first, 1 resend (workspace %s)", found, ws)
	}
	if !FlowStageCandidate(*found) {
		t.Fatalf("collect/gate at 29 of 30 should be a candidate")
	}
}
