package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// readyReport is a shadow run whose candidate passes spec 9.6: a not-losing lower bound
// of at least 0.5 and at least as many wins as losses.
func readyReport() ShadowReport {
	return ShadowReport{Pairs: 20, Shadow: 14, Tie: 2, Primary: 4, NotLosingLower: 0.6}
}

// TestPromotionReadyTable checks spec 9.6's rule for a finished run: the lower bound of
// not losing reaches 0.5, and wins are at least losses.
func TestPromotionReadyTable(t *testing.T) {
	cases := []struct {
		name   string
		report ShadowReport
		want   bool
	}{
		{"clear win", ShadowReport{Shadow: 14, Tie: 2, Primary: 4, NotLosingLower: 0.6}, true},
		{"bound exactly half with wins equal to losses", ShadowReport{Shadow: 5, Primary: 5, NotLosingLower: 0.5}, true},
		{"bound just under half", ShadowReport{Shadow: 14, Primary: 4, NotLosingLower: 0.49}, false},
		{"bound high but more losses than wins", ShadowReport{Shadow: 4, Primary: 6, Tie: 9, NotLosingLower: 0.7}, false},
		{"no compared pairs", ShadowReport{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PromotionReady(c.report); got != c.want {
				t.Errorf("PromotionReady(%+v) = %v, want %v", c.report, got, c.want)
			}
		})
	}
}

// runtimeChangeEnv is an organisation with a seat on runtime A, a candidate runtime B, and
// a completed shadow run for the seat. The flag is on, so seat_model_changed is written.
type runtimeChangeEnv struct {
	*orgEnv
	fromRuntime string
	toRuntime   string
	cfg         db.ShadowConfig
	svc         *RuntimeChangeService
}

func newRuntimeChangeEnv(t *testing.T, autoPromote bool) runtimeChangeEnv {
	t.Helper()
	e := newOrgEnv(t)
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableSeats, featureflag.Rule{Default: true})
	flags := featureflag.NewService(provider)
	e.changes.Flags = flags
	from := e.fx.Runtime(t, "seat runtime", testutil.Cols{"provider": "anthropic"})
	e.fx.Exec(t, `UPDATE agent SET runtime_id = $2, model = 'claude-seat', home_squad_id = $3 WHERE id = $1`, util.UUIDToString(e.subject), from, util.UUIDToString(e.squad))
	to := e.fx.Runtime(t, "candidate runtime", testutil.Cols{"provider": "openai"})
	cfgID := e.fx.Insert(t, "shadow_config", testutil.Cols{
		"workspace_id": util.UUIDToString(e.ws), "agent_id": util.UUIDToString(e.subject),
		"candidate_runtime_id": to, "candidate_model": "gpt-candidate", "purpose": ShadowPurposeModelTrial,
		"created_by": util.UUIDToString(e.owner), "state": "completed", "auto_promote": autoPromote,
	})
	cfg, err := e.q.GetShadowConfig(context.Background(), db.GetShadowConfigParams{
		ID: util.MustParseUUID(cfgID), WorkspaceID: e.ws,
	})
	if err != nil {
		t.Fatalf("load shadow config: %v", err)
	}
	return runtimeChangeEnv{
		orgEnv: e, fromRuntime: from, toRuntime: to, cfg: cfg,
		svc: &RuntimeChangeService{Queries: e.q, Proposals: e.props, Changes: e.changes, Flags: flags},
	}
}

// seat reads the seat's runtime and model.
func (re runtimeChangeEnv) seat(t *testing.T) (string, string) {
	t.Helper()
	a, err := re.q.GetAgent(context.Background(), re.subject)
	if err != nil {
		t.Fatalf("load seat: %v", err)
	}
	return util.UUIDToString(a.RuntimeID), a.Model.String
}

// runtimeChangeProposals counts the runtime_change proposals filed in the workspace.
func (re runtimeChangeEnv) runtimeChangeProposals(t *testing.T) int {
	t.Helper()
	return re.fx.Count(t, `SELECT COUNT(*) FROM proposal WHERE workspace_id = $1 AND kind = 'org_change' AND payload->>'type' = 'runtime_change'`, util.UUIDToString(re.ws))
}

// TestCompletedCreatesProposal: a finished run that passes is a proposal for the owner
// (spec 9.6), and the seat keeps its runtime until the owner approves it.
func TestCompletedCreatesProposal(t *testing.T) {
	re := newRuntimeChangeEnv(t, false)

	if err := re.svc.OnShadowCompleted(context.Background(), re.cfg, readyReport()); err != nil {
		t.Fatalf("OnShadowCompleted: %v", err)
	}

	if n := re.runtimeChangeProposals(t); n != 1 {
		t.Fatalf("runtime_change proposals = %d, want 1", n)
	}
	if rt, model := re.seat(t); rt != re.fromRuntime || model != "claude-seat" {
		t.Errorf("seat moved before approval: runtime %s model %q", rt, model)
	}
}

// TestAutoPromoteAppliesAndProbation: with auto_promote the owner's standing decision
// applies the change at once, opens probation, and writes seat_model_changed.
func TestAutoPromoteAppliesAndProbation(t *testing.T) {
	re := newRuntimeChangeEnv(t, true)
	ctx := context.Background()

	if err := re.svc.OnShadowCompleted(ctx, re.cfg, readyReport()); err != nil {
		t.Fatalf("OnShadowCompleted: %v", err)
	}

	rt, model := re.seat(t)
	if rt != re.toRuntime || model != "gpt-candidate" {
		t.Fatalf("seat after auto-promote: runtime %s model %q, want %s gpt-candidate", rt, model, re.toRuntime)
	}
	app, err := re.q.GetOrgChangeApplicationByProposal(ctx, db.GetOrgChangeApplicationByProposalParams{
		WorkspaceID: re.ws, ProposalID: mustProposalOf(t, re, ctx),
	})
	if err != nil {
		t.Fatalf("application row: %v", err)
	}
	if app.ChangeType != OrgChangeRuntime || !app.ProbationEndsAt.Valid {
		t.Errorf("application = type %q, probation %v; want runtime_change with probation", app.ChangeType, app.ProbationEndsAt.Valid)
	}
	if n := re.fx.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = $2 AND subject_id = $3`,
		util.UUIDToString(re.ws), GovernanceKindSeatModelChanged, util.UUIDToString(re.subject)); n != 1 {
		t.Errorf("seat_model_changed events = %d, want 1", n)
	}
}

// mustProposalOf is the one runtime_change proposal the run filed.
func mustProposalOf(t *testing.T, re runtimeChangeEnv, ctx context.Context) pgtype.UUID {
	t.Helper()
	var id string
	if err := re.fx.Pool.QueryRow(ctx, `SELECT id::text FROM proposal WHERE workspace_id = $1 AND kind = 'org_change' AND payload->>'type' = 'runtime_change' ORDER BY created_at DESC LIMIT 1`, util.UUIDToString(re.ws)).Scan(&id); err != nil {
		t.Fatalf("runtime_change proposal: %v", err)
	}
	return util.MustParseUUID(id)
}

// TestRuntimeChangeRevertRestoresSeat: reverting the applied change puts the seat's
// runtime and model back.
func TestRuntimeChangeRevertRestoresSeat(t *testing.T) {
	re := newRuntimeChangeEnv(t, true)
	ctx := context.Background()
	if err := re.svc.OnShadowCompleted(ctx, re.cfg, readyReport()); err != nil {
		t.Fatalf("OnShadowCompleted: %v", err)
	}
	app, err := re.q.GetOrgChangeApplicationByProposal(ctx, db.GetOrgChangeApplicationByProposalParams{
		WorkspaceID: re.ws, ProposalID: mustProposalOf(t, re, ctx),
	})
	if err != nil {
		t.Fatalf("application row: %v", err)
	}

	if _, err := re.changes.RevertOrgChange(ctx, re.q, re.ws, app.ID, re.boss()); err != nil {
		t.Fatalf("RevertOrgChange: %v", err)
	}

	if rt, model := re.seat(t); rt != re.fromRuntime || model != "claude-seat" {
		t.Errorf("seat after revert: runtime %s model %q, want %s claude-seat", rt, model, re.fromRuntime)
	}
}

// TestProbationWorseReverts: a worse verdict on a runtime change files the change back to
// the runtime and model the seat had, which is what probation proposes to restore.
func TestProbationWorseReverts(t *testing.T) {
	re := newRuntimeChangeEnv(t, true)
	ctx := context.Background()
	if err := re.svc.OnShadowCompleted(ctx, re.cfg, readyReport()); err != nil {
		t.Fatalf("OnShadowCompleted: %v", err)
	}
	app, err := re.q.GetOrgChangeApplicationByProposal(ctx, db.GetOrgChangeApplicationByProposalParams{
		WorkspaceID: re.ws, ProposalID: mustProposalOf(t, re, ctx),
	})
	if err != nil {
		t.Fatalf("application row: %v", err)
	}

	raw, err := rollbackPayload(ctx, re.q, app)
	if err != nil {
		t.Fatalf("rollbackPayload: %v", err)
	}
	var got runtimeChangePayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != OrgChangeRuntime || got.ToRuntimeID != re.fromRuntime || got.ToModel != "claude-seat" {
		t.Errorf("rollback payload = %+v, want runtime_change back to %s / claude-seat", got, re.fromRuntime)
	}
}

// TestAutoPromoteOwnerOnly: only an owner or an admin may turn auto-promotion on.
func TestAutoPromoteOwnerOnly(t *testing.T) {
	sf := newShadowFixture(t, true)
	user := sf.User(t, "Non owner", testutil.UniqueEmail("non-owner"))
	sf.Member(t, sf.WorkspaceID, user, "member")
	in := ShadowStartInput{
		AgentID: util.MustParseUUID(sf.seat), CandidateRuntimeID: util.MustParseUUID(sf.cand),
		CandidateModel: "gpt-candidate", Purpose: ShadowPurposeModelTrial, SampleRate: 1, TargetPairs: 20,
		AutoPromote: true,
	}
	actor := GovernanceActor{Type: GovernanceActorMember, ID: util.MustParseUUID(user)}
	if _, err := sf.svc.Start(context.Background(), sf.ws, actor, in); !errors.Is(err, ErrGovernanceActorNotBoss) {
		t.Errorf("a member starting auto-promote: err = %v, want ErrGovernanceActorNotBoss", err)
	}
}

// TestRuntimeChangeValidationRefusesRunningRun: a runtime change may name only a completed
// run of the same seat, so a proposal cannot run ahead of its evidence.
func TestRuntimeChangeValidationRefusesRunningRun(t *testing.T) {
	re := newRuntimeChangeEnv(t, false)
	ctx := context.Background()
	re.fx.Exec(t, `UPDATE shadow_config SET state = 'running' WHERE id = $1`, util.UUIDToString(re.cfg.ID))
	payload := `{"type":"runtime_change","agent_id":"` + util.UUIDToString(re.subject) +
		`","to_runtime_id":"` + re.toRuntime + `","to_model":"gpt-candidate","shadow_config_id":"` +
		util.UUIDToString(re.cfg.ID) + `","report":{"pairs":20},"rationale":"r"}`

	if _, _, err := re.changes.ValidateOrgChange(ctx, re.q, re.ws, []byte(payload)); err == nil {
		t.Error("a runtime change against a running shadow run must be refused")
	}
}
