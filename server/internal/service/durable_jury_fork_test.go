package service

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var (
	juryDoer = JurorCandidate{AgentID: "agent-doer", Provider: "anthropic", Model: "claude-x"}
)

func jurorIDs(jurors []JurorCandidate) []string {
	ids := make([]string, 0, len(jurors))
	for _, j := range jurors {
		ids = append(ids, j.AgentID)
	}
	return ids
}

func TestPickJurorsDiversity(t *testing.T) {
	t.Run("three providers picks two of other providers", func(t *testing.T) {
		pool := []JurorCandidate{
			{AgentID: "agent-a", Provider: "openai", Model: "gpt-4"},
			{AgentID: "agent-b", Provider: "google", Model: "gemini"},
			{AgentID: "agent-c", Provider: "mistral", Model: "mistral-l"},
			{AgentID: "agent-doer", Provider: "anthropic", Model: "claude-x"},
		}

		jurors, diversity := PickJurors(pool, juryDoer, 2)

		if len(jurors) != 2 {
			t.Fatalf("want 2 jurors, got %d: %v", len(jurors), jurorIDs(jurors))
		}
		seen := map[string]bool{}
		for _, j := range jurors {
			if j.Provider == juryDoer.Provider {
				t.Errorf("juror %s shares provider %q with doer", j.AgentID, j.Provider)
			}
			if seen[j.Provider] {
				t.Errorf("jurors share provider %q", j.Provider)
			}
			seen[j.Provider] = true
		}
		if diversity != "provider" {
			t.Errorf("diversity = %q, want %q", diversity, "provider")
		}
	})

	t.Run("two providers needs model_only", func(t *testing.T) {
		pool := []JurorCandidate{
			{AgentID: "agent-a", Provider: "openai", Model: "gpt-4"},
			{AgentID: "agent-b", Provider: "anthropic", Model: "claude-y"},
			{AgentID: "agent-c", Provider: "anthropic", Model: "claude-z"},
		}

		jurors, diversity := PickJurors(pool, juryDoer, 2)

		if len(jurors) != 2 {
			t.Fatalf("want 2 jurors, got %d: %v", len(jurors), jurorIDs(jurors))
		}
		if diversity != "model_only" {
			t.Errorf("diversity = %q, want %q", diversity, "model_only")
		}
	})

	t.Run("only same model available is none but still fills", func(t *testing.T) {
		pool := []JurorCandidate{
			{AgentID: "agent-a", Provider: "anthropic", Model: "claude-x"},
		}

		jurors, diversity := PickJurors(pool, juryDoer, 2)

		if got := jurorIDs(jurors); !reflect.DeepEqual(got, []string{"agent-a"}) {
			t.Errorf("jurors = %v, want [agent-a]", got)
		}
		if diversity != "none" {
			t.Errorf("diversity = %q, want %q", diversity, "none")
		}
	})
}

func TestPickJurorsSkipsDoer(t *testing.T) {
	pool := []JurorCandidate{
		{AgentID: "agent-doer", Provider: "openai", Model: "gpt-4"},
		{AgentID: "agent-b", Provider: "google", Model: "gemini"},
		{AgentID: "agent-c", Provider: "mistral", Model: "mistral-l"},
	}
	doer := JurorCandidate{AgentID: "agent-doer", Provider: "anthropic", Model: "claude-x"}

	jurors, _ := PickJurors(pool, doer, 3)

	for _, j := range jurors {
		if j.AgentID == doer.AgentID {
			t.Fatalf("doer %s was picked as its own juror: %v", doer.AgentID, jurorIDs(jurors))
		}
	}
	if len(jurors) != 2 {
		t.Errorf("want the 2 non-doer candidates, got %v", jurorIDs(jurors))
	}
}

func TestPickJurorsNoEligible(t *testing.T) {
	pool := []JurorCandidate{juryDoer}

	jurors, diversity := PickJurors(pool, juryDoer, 2)

	if len(jurors) != 0 {
		t.Errorf("want no jurors, got %v", jurorIDs(jurors))
	}
	if diversity != "none" {
		t.Errorf("diversity = %q, want %q", diversity, "none")
	}
}

func TestPickJurorsDeterministic(t *testing.T) {
	pool := []JurorCandidate{
		{AgentID: "agent-c", Provider: "mistral", Model: "mistral-l"},
		{AgentID: "agent-a", Provider: "openai", Model: "gpt-4"},
		{AgentID: "agent-b", Provider: "google", Model: "gemini"},
		{AgentID: "agent-doer", Provider: "anthropic", Model: "claude-x"},
	}
	reversed := []JurorCandidate{pool[3], pool[2], pool[1], pool[0]}

	first, firstDiversity := PickJurors(pool, juryDoer, 2)
	second, secondDiversity := PickJurors(pool, juryDoer, 2)
	reordered, reorderedDiversity := PickJurors(reversed, juryDoer, 2)

	if !reflect.DeepEqual(first, second) || firstDiversity != secondDiversity {
		t.Errorf("same input gave different results: %v/%q then %v/%q", jurorIDs(first), firstDiversity, jurorIDs(second), secondDiversity)
	}
	if !reflect.DeepEqual(first, reordered) || firstDiversity != reorderedDiversity {
		t.Errorf("pool order changed the result: %v/%q then %v/%q", jurorIDs(first), firstDiversity, jurorIDs(reordered), reorderedDiversity)
	}
}

func TestPickJurorsSizeCap(t *testing.T) {
	pool := []JurorCandidate{
		{AgentID: "agent-a", Provider: "openai", Model: "gpt-4"},
		{AgentID: "agent-b", Provider: "google", Model: "gemini"},
		{AgentID: "agent-c", Provider: "mistral", Model: "mistral-l"},
		{AgentID: "agent-d", Provider: "cohere", Model: "command"},
	}

	cases := []struct {
		name string
		size int
		want int
	}{
		{"size zero", 0, 0},
		{"size two", 2, 2},
		{"size three", 3, 3},
		{"size larger than pool", 10, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jurors, _ := PickJurors(pool, juryDoer, tc.size)
			if len(jurors) != tc.want {
				t.Errorf("size %d: got %d jurors, want %d: %v", tc.size, len(jurors), tc.want, jurorIDs(jurors))
			}
		})
	}
}

// juryFixture is a verification fixture whose workspace has three agents on three
// providers, none of them the doer's, and a fake asker that records each question put
// to the person. Tests set jury_pool themselves.
type juryFixture struct {
	verificationFixture
	asker  *fakeJuryAsker
	jurors []string
}

type fakeJuryAsker struct {
	panels  []string
	reasons []string
}

func (a *fakeJuryAsker) Ask(_ context.Context, _ *db.Queries, panel db.JuryPanel, reason string) error {
	a.panels = append(a.panels, util.UUIDToString(panel.ID))
	a.reasons = append(a.reasons, reason)
	return nil
}

func newJuryFixture(t *testing.T) juryFixture {
	t.Helper()
	vf := newVerificationFixture(t)
	jf := juryFixture{verificationFixture: vf, asker: &fakeJuryAsker{}}
	for _, provider := range []string{"openai", "google", "mistral"} {
		rt := vf.Runtime(t, "jury runtime "+provider, testutil.Cols{"provider": provider})
		jf.jurors = append(jf.jurors, vf.Agent(t, "juror "+provider, rt, testutil.Cols{"model": provider + "-model"}))
	}
	vf.checks.Jury = &JuryService{Queries: vf.checks.Queries, TxStarter: vf.checks.TxStarter, Tasks: vf.checks.Tasks, Asker: jf.asker}
	return jf
}

// setPool writes the workspace's jury_pool setting.
func (jf juryFixture) setPool(t *testing.T, agents ...string) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"jury_pool": agents})
	if err != nil {
		t.Fatal(err)
	}
	jf.Exec(t, `UPDATE workspace SET settings = $2::jsonb WHERE id = $1`, jf.WorkspaceID, string(raw))
}

// juryCase makes a done, high-priority issue with one jury criterion and runs the check
// job on its outcome. It returns the issue, the criterion and the outcome.
func (jf juryFixture) juryCase(t *testing.T, priority string) (string, db.AcceptanceCriterion, db.WorkOutcome) {
	t.Helper()
	issue := jf.doneIssue(t, jf.doer)
	jf.Cleanup(t, `DELETE FROM jury_panel WHERE outcome_id IN (SELECT id FROM work_outcome WHERE issue_id = $1)`, issue)
	jf.Exec(t, `UPDATE issue SET priority = $2 WHERE id = $1`, issue, priority)
	c := jf.addCriterion(t, issue, "The export has a header.", CheckKindAgentJudgment, map[string]any{"rubric": "Check the header.", "jury": true})
	o := jf.currentOutcome(t, issue)
	jf.schedule(t, o)
	return issue, c, o
}

func (jf juryFixture) schedule(t *testing.T, o db.WorkOutcome) {
	t.Helper()
	if err := jf.checks.Schedule(context.Background(), o); err != nil {
		t.Fatalf("schedule: %v", err)
	}
}

func (jf juryFixture) panelFor(t *testing.T, o db.WorkOutcome, c db.AcceptanceCriterion) db.JuryPanel {
	t.Helper()
	p, err := jf.checks.Queries.GetJuryPanelForCriterion(context.Background(), db.GetJuryPanelForCriterionParams{
		WorkspaceID: jf.ws, OutcomeID: o.ID, CriterionID: c.ID,
	})
	if err != nil {
		t.Fatalf("panel for criterion: %v", err)
	}
	return p
}

// jurorTask is the task an agent was given on a panel, found by the panel id it carries.
func (jf juryFixture) jurorTask(t *testing.T, panel db.JuryPanel, agent pgtype.UUID) string {
	t.Helper()
	var id string
	err := jf.Pool.QueryRow(context.Background(), `
		SELECT id::text FROM agent_task_queue
		WHERE context->>'jury_panel_id' = $1 AND agent_id = $2`,
		util.UUIDToString(panel.ID), util.UUIDToString(agent)).Scan(&id)
	if err != nil {
		t.Fatalf("juror task for %s: %v", util.UUIDToString(agent), err)
	}
	return id
}

// vote records one juror's check through the agent path, as the agent's CLI would.
func (jf juryFixture) vote(t *testing.T, panel db.JuryPanel, agent pgtype.UUID, result string) error {
	t.Helper()
	actor := GovernanceActor{Type: GovernanceActorAgent, ID: agent}
	in := VerificationInput{
		CriterionID: panel.CriterionID, Result: result,
		TaskID: util.MustParseUUID(jf.jurorTask(t, panel, agent)),
	}
	_, err := jf.checks.Record(context.Background(), jf.ws, actor, panel.OutcomeID, in)
	return err
}

func TestJuryUnanimous(t *testing.T) {
	jf := newJuryFixture(t)
	jf.setPool(t, jf.jurors...)
	_, c, o := jf.juryCase(t, "high")

	panel := jf.panelFor(t, o, c)
	if panel.State != "open" || panel.Diversity != "provider" || len(panel.JurorAgentIds) != 2 {
		t.Fatalf("panel = state %q diversity %q jurors %d, want open/provider/2", panel.State, panel.Diversity, len(panel.JurorAgentIds))
	}
	for _, j := range panel.JurorAgentIds {
		if err := jf.vote(t, panel, j, "pass"); err != nil {
			t.Fatalf("juror vote: %v", err)
		}
	}

	if got := jf.panelFor(t, o, c).State; got != "unanimous" {
		t.Fatalf("state = %q, want unanimous", got)
	}
	if len(jf.asker.panels) != 0 {
		t.Errorf("unanimous panel asked the person %d times", len(jf.asker.panels))
	}
	var jury int
	for _, v := range verificationsOf(t, jf.principalFixture, util.UUIDToString(o.ID)) {
		if v.VerifierType == "jury" && v.Result == "pass" && v.CriterionID == util.UUIDToString(c.ID) {
			jury++
		}
	}
	if jury != 1 {
		t.Errorf("jury checks = %d, want exactly one pass", jury)
	}
}

func TestJurySplitCreatesOneAsk(t *testing.T) {
	jf := newJuryFixture(t)
	jf.setPool(t, jf.jurors...)
	_, c, o := jf.juryCase(t, "high")

	panel := jf.panelFor(t, o, c)
	if err := jf.vote(t, panel, panel.JurorAgentIds[0], "pass"); err != nil {
		t.Fatalf("first vote: %v", err)
	}
	if err := jf.vote(t, panel, panel.JurorAgentIds[1], "fail"); err != nil {
		t.Fatalf("second vote: %v", err)
	}
	jf.schedule(t, o)
	jf.schedule(t, o)

	if got := jf.panelFor(t, o, c).State; got != "split" {
		t.Fatalf("state = %q, want split", got)
	}
	if len(jf.asker.panels) != 1 || jf.asker.panels[0] != util.UUIDToString(panel.ID) {
		t.Fatalf("asks = %v, want exactly one for panel %s", jf.asker.panels, util.UUIDToString(panel.ID))
	}
	if jf.asker.reasons[0] != "verification" {
		t.Errorf("ask reason = %q, want verification", jf.asker.reasons[0])
	}
}

func TestJurorUnableReplacedOnce(t *testing.T) {
	t.Run("first unable is replaced and the panel completes", func(t *testing.T) {
		jf := newJuryFixture(t)
		jf.setPool(t, jf.jurors...)
		_, c, o := jf.juryCase(t, "high")

		panel := jf.panelFor(t, o, c)
		unable := panel.JurorAgentIds[0]
		if err := jf.vote(t, panel, unable, "unable"); err != nil {
			t.Fatalf("unable vote: %v", err)
		}
		panel = jf.panelFor(t, o, c)
		if len(panel.JurorAgentIds) != 3 || panel.State != "open" {
			t.Fatalf("after one unable: jurors %d state %q, want 3 open", len(panel.JurorAgentIds), panel.State)
		}
		replacement := panel.JurorAgentIds[2]
		if err := jf.vote(t, panel, panel.JurorAgentIds[1], "pass"); err != nil {
			t.Fatalf("second juror: %v", err)
		}
		if err := jf.vote(t, panel, replacement, "pass"); err != nil {
			t.Fatalf("replacement: %v", err)
		}
		if got := jf.panelFor(t, o, c).State; got != "unanimous" {
			t.Fatalf("state = %q, want unanimous", got)
		}
	})

	t.Run("a second unable escalates and is not replaced again", func(t *testing.T) {
		jf := newJuryFixture(t)
		jf.setPool(t, jf.jurors...)
		_, c, o := jf.juryCase(t, "high")

		panel := jf.panelFor(t, o, c)
		if err := jf.vote(t, panel, panel.JurorAgentIds[0], "unable"); err != nil {
			t.Fatalf("first unable: %v", err)
		}
		panel = jf.panelFor(t, o, c)
		if err := jf.vote(t, panel, panel.JurorAgentIds[1], "unable"); err != nil {
			t.Fatalf("second unable: %v", err)
		}
		panel = jf.panelFor(t, o, c)
		if len(panel.JurorAgentIds) != 3 {
			t.Errorf("jurors = %d, want 3: one replacement only", len(panel.JurorAgentIds))
		}
		if panel.State != "escalated" {
			t.Fatalf("state = %q, want escalated", panel.State)
		}
		if len(jf.asker.panels) != 1 {
			t.Errorf("asks = %d, want 1", len(jf.asker.panels))
		}
	})
}

func TestJuryTimeout24h(t *testing.T) {
	jf := newJuryFixture(t)
	jf.setPool(t, jf.jurors...)
	_, c, o := jf.juryCase(t, "high")
	panel := jf.panelFor(t, o, c)

	base := time.Now()
	jf.checks.Jury.Now = func() time.Time { return base.Add(23 * time.Hour) }
	if err := jf.checks.Jury.Timeout(context.Background()); err != nil {
		t.Fatalf("timeout at 23h: %v", err)
	}
	if got := jf.panelFor(t, o, c).State; got != "open" {
		t.Fatalf("state at 23h = %q, want open", got)
	}

	jf.checks.Jury.Now = func() time.Time { return base.Add(25 * time.Hour) }
	if err := jf.checks.Jury.Timeout(context.Background()); err != nil {
		t.Fatalf("timeout at 25h: %v", err)
	}
	closed := jf.panelFor(t, o, c)
	if closed.State != "timed_out" || !closed.ClosedAt.Valid {
		t.Fatalf("state = %q closed %v, want timed_out with closed_at", closed.State, closed.ClosedAt.Valid)
	}
	if closed.ID != panel.ID {
		t.Fatal("timeout changed the panel row")
	}
}

func TestJuryWithOneAgentEscalates(t *testing.T) {
	t.Run("pool holds only the doer", func(t *testing.T) {
		jf := newJuryFixture(t)
		jf.setPool(t, jf.doer)
		_, c, o := jf.juryCase(t, "high")

		panel := jf.panelFor(t, o, c)
		if panel.State != "escalated" || len(panel.JurorAgentIds) != 0 {
			t.Fatalf("panel = state %q jurors %d, want escalated with none", panel.State, len(panel.JurorAgentIds))
		}
		if len(jf.asker.panels) != 1 {
			t.Errorf("asks = %d, want 1", len(jf.asker.panels))
		}
	})

	t.Run("pool holds one other agent", func(t *testing.T) {
		jf := newJuryFixture(t)
		jf.setPool(t, jf.jurors[0])
		_, c, o := jf.juryCase(t, "high")

		panel := jf.panelFor(t, o, c)
		if panel.State != "escalated" {
			t.Fatalf("state = %q, want escalated", panel.State)
		}
		if len(jf.asker.panels) != 1 {
			t.Errorf("asks = %d, want 1", len(jf.asker.panels))
		}
	})
}

func TestJurorTrackRecordOnOverrule(t *testing.T) {
	jf := newJuryFixture(t)
	jf.Cleanup(t, `DELETE FROM track_record WHERE workspace_id = $1`, jf.WorkspaceID)
	jf.setPool(t, jf.jurors...)
	_, c, o := jf.juryCase(t, "high")

	panel := jf.panelFor(t, o, c)
	agreeing, overruled := panel.JurorAgentIds[0], panel.JurorAgentIds[1]
	if err := jf.vote(t, panel, agreeing, "fail"); err != nil {
		t.Fatalf("agreeing vote: %v", err)
	}
	if err := jf.vote(t, panel, overruled, "pass"); err != nil {
		t.Fatalf("overruled vote: %v", err)
	}
	member := GovernanceActor{Type: GovernanceActorMember, ID: util.MustParseUUID(jf.UserID)}
	if err := jf.checks.Jury.Resolve(context.Background(), jf.ws, member, panel.ID, "fail"); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if got := jf.panelFor(t, o, c).State; got != "resolved" {
		t.Fatalf("state = %q, want resolved", got)
	}
	workClass := "verify:" + o.WorkClass
	track := func(agent pgtype.UUID) db.TrackRecord {
		r, err := jf.checks.Queries.GetTrackRecord(context.Background(), db.GetTrackRecordParams{
			WorkspaceID: jf.ws, AgentID: agent, WorkClass: workClass, WindowDays: trackModeWindow,
		})
		if err != nil {
			t.Fatalf("track record of %s for %q: %v", util.UUIDToString(agent), workClass, err)
		}
		return r
	}
	if r := track(overruled); r.NFail != 1 {
		t.Errorf("overruled juror n_fail = %d, want 1", r.NFail)
	}
	if r := track(agreeing); r.NPass != 1 || r.NFail != 0 {
		t.Errorf("agreeing juror n_pass/n_fail = %d/%d, want 1/0", r.NPass, r.NFail)
	}
}
