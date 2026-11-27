package service

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// shadowQueuedTask is a queued shadow task on the fixture's issue and runtime.
func (sf shadowFixture) shadowQueuedTask(t *testing.T) db.AgentTaskQueue {
	t.Helper()
	id := sf.Task(t, sf.seat, testutil.Cols{
		"issue_id": sf.issue, "status": "queued", "runtime_id": sf.runtime,
		"context": testutil.Raw(`'{"durable_kind":"shadow"}'::jsonb`),
	})
	task, err := sf.q.GetAgentTask(context.Background(), util.MustParseUUID(id))
	if err != nil {
		t.Fatalf("load shadow task: %v", err)
	}
	return task
}

// claimRefusalOf is the claim gate's verdict on a queued task for a claimant with or without
// the shadow capability.
func (sf shadowFixture) claimRefusalOf(t *testing.T, capable bool, task db.AgentTaskQueue) *claimRefusal {
	t.Helper()
	ctx := WithShadowCapableClaim(context.Background(), capable)
	refusal, err := sf.svc.Tasks.claimRefusalFor(ctx, sf.q, sf.ws, task)
	if err != nil {
		t.Fatalf("claim gate: %v", err)
	}
	return refusal
}

// localDirectoryOnIssue gives the fixture's issue a project with one local directory resource.
func (sf shadowFixture) localDirectoryOnIssue(t *testing.T, mode string) {
	t.Helper()
	project := sf.Project(t, "shadow project")
	sf.Exec(t, `UPDATE issue SET project_id = $2 WHERE id = $1`, sf.issue, project)
	sf.Insert(t, "project_resource", testutil.Cols{
		"project_id": project, "workspace_id": sf.WorkspaceID, "resource_type": "local_directory",
		"resource_ref": testutil.Raw(`'{"path":"/home/owner/repo","execution_mode":"` + mode + `"}'::jsonb`),
	})
}

// TestShadowClaimRequiresCapability: a shadow task waits for a daemon that advertised the
// shadow capability; without it the claim gate refuses and the task stays queued (D30).
func TestShadowClaimRequiresCapability(t *testing.T) {
	sf := newShadowFixture(t, true)
	task := sf.shadowQueuedTask(t)

	refusal := sf.claimRefusalOf(t, false, task)
	if refusal == nil || refusal.Reason != VerdictReasonShadowCapability {
		t.Fatalf("a daemon without the capability: refusal %+v, want %q", refusal, VerdictReasonShadowCapability)
	}
	if refusal := sf.claimRefusalOf(t, true, task); refusal != nil {
		t.Errorf("a daemon with the capability: refused with %q", refusal.Reason)
	}
}

// TestShadowInPlaceRefused: a shadow run never runs against a user's working copy, in place.
func TestShadowInPlaceRefused(t *testing.T) {
	sf := newShadowFixture(t, true)
	sf.localDirectoryOnIssue(t, "in_place")
	task := sf.shadowQueuedTask(t)

	refusal := sf.claimRefusalOf(t, true, task)
	if refusal == nil || refusal.Reason != VerdictReasonShadowWorkspace {
		t.Fatalf("an in_place project: refusal %+v, want %q", refusal, VerdictReasonShadowWorkspace)
	}
}

// TestShadowNoWorktreeClaim: a shadow run claims no worktree. A project whose local directory
// runs as a worktree refuses the shadow run, and a project with no local directory admits it.
func TestShadowNoWorktreeClaim(t *testing.T) {
	t.Run("worktree project refused", func(t *testing.T) {
		sf := newShadowFixture(t, true)
		sf.localDirectoryOnIssue(t, "worktree")
		refusal := sf.claimRefusalOf(t, true, sf.shadowQueuedTask(t))
		if refusal == nil || refusal.Reason != VerdictReasonShadowWorkspace {
			t.Fatalf("a worktree project: refusal %+v, want %q", refusal, VerdictReasonShadowWorkspace)
		}
	})
	t.Run("project without a directory admitted", func(t *testing.T) {
		sf := newShadowFixture(t, true)
		if refusal := sf.claimRefusalOf(t, true, sf.shadowQueuedTask(t)); refusal != nil {
			t.Fatalf("no local directory: refused with %q", refusal.Reason)
		}
	})
}

// TestShadowAgentHidden: a shadow agent is not listed beside its seat.
func TestShadowAgentHidden(t *testing.T) {
	sf := newShadowFixture(t, true)
	shadow, err := sf.q.CreateShadowAgent(context.Background(), db.CreateShadowAgentParams{
		Name: "seat (shadow test)", RuntimeID: util.MustParseUUID(sf.cand),
		Model: pgtype.Text{String: "gpt-candidate", Valid: true}, SeatID: util.MustParseUUID(sf.seat), WorkspaceID: sf.ws,
	})
	if err != nil {
		t.Fatalf("create shadow agent: %v", err)
	}
	agents, err := sf.q.ListAgents(context.Background(), sf.ws)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	seatListed := false
	for _, a := range agents {
		if a.ID == shadow.ID {
			t.Fatalf("the shadow agent %s is listed", util.UUIDToString(shadow.ID))
		}
		if util.UUIDToString(a.ID) == sf.seat {
			seatListed = true
		}
	}
	if !seatListed {
		t.Error("the seat itself is not listed")
	}
}

// TestShadowNoWriteMcpOrAppTokens: a shadow agent keeps no MCP overlay, no connected-app
// allowlist and no environment secret of its seat, so it holds no write token (spec 9.4 (b)).
func TestShadowNoWriteMcpOrAppTokens(t *testing.T) {
	sf := newShadowFixture(t, true)
	sf.Exec(t, `UPDATE agent SET mcp_config = '{"servers":{"crm":{}}}'::jsonb,
		composio_toolkit_allowlist = ARRAY['github'], custom_env = '{"API_TOKEN":"secret"}'::jsonb WHERE id = $1`, sf.seat)
	shadow, err := sf.q.CreateShadowAgent(context.Background(), db.CreateShadowAgentParams{
		Name: "seat (shadow tokens)", RuntimeID: util.MustParseUUID(sf.cand),
		Model: pgtype.Text{String: "gpt-candidate", Valid: true}, SeatID: util.MustParseUUID(sf.seat), WorkspaceID: sf.ws,
	})
	if err != nil {
		t.Fatalf("create shadow agent: %v", err)
	}
	var mcpNull, allowNull bool
	var env string
	if err := sf.Pool.QueryRow(context.Background(), `
		SELECT mcp_config IS NULL, composio_toolkit_allowlist IS NULL, custom_env::text
		FROM agent WHERE id = $1`, shadow.ID).Scan(&mcpNull, &allowNull, &env); err != nil {
		t.Fatalf("read shadow agent: %v", err)
	}
	if !mcpNull || !allowNull || env != "{}" {
		t.Errorf("shadow agent carries mcp_null=%v allowlist_null=%v env=%s; want no MCP overlay, no allowlist, no env", mcpNull, allowNull, env)
	}
}

// TestShadowClaimRequiresCapabilitySingle: the single-runtime claim (ClaimTaskForRuntime, the
// legacy per-runtime endpoint) applies the same capability gate as the batch claim.
func TestShadowClaimRequiresCapabilitySingle(t *testing.T) {
	sf := newShadowFixture(t, true)
	shadow := sf.shadowQueuedTask(t)
	ctx := context.Background()
	runtime := util.MustParseUUID(sf.runtime)

	got, err := sf.svc.Tasks.ClaimTaskForRuntime(ctx, runtime)
	if err != nil {
		t.Fatalf("claim without the capability: %v", err)
	}
	if got != nil {
		t.Fatalf("a daemon without the capability claimed task %s", util.UUIDToString(got.ID))
	}
	got, err = sf.svc.Tasks.ClaimTaskForRuntime(WithShadowCapableClaim(ctx, true), runtime)
	if err != nil {
		t.Fatalf("claim with the capability: %v", err)
	}
	if got == nil || got.ID != shadow.ID {
		t.Fatalf("a daemon with the capability got %v, want the shadow task", got)
	}
}

// TestShadowClaimFieldsByPurpose: omit_skills is decided by shadow purpose, not hard-coded true
// for every shadow run. model_trial keeps skills so the candidate model sees the same briefing as
// the seat (spec 9.4); necessity_test omits skills (spec 10.2); non-shadow tasks get zero values.
func TestShadowClaimFieldsByPurpose(t *testing.T) {
	cases := []struct {
		name string
		ctx  []byte
		want ShadowClaimFields
	}{
		{
			name: "non-shadow task",
			ctx:  []byte(`{}`),
			want: ShadowClaimFields{Shadow: false, OmitSkills: false},
		},
		{
			name: "model_trial keeps skills",
			ctx:  []byte(`{"durable_kind":"shadow","shadow_purpose":"model_trial","omit_skills":false}`),
			want: ShadowClaimFields{Shadow: true, OmitSkills: false},
		},
		{
			// Without the claim's omit_skills mark, a shadow task omits skills (the safe default).
			name: "model_trial by purpose alone omits skills",
			ctx:  []byte(`{"durable_kind":"shadow","shadow_purpose":"model_trial"}`),
			want: ShadowClaimFields{Shadow: true, OmitSkills: true},
		},
		{
			name: "necessity_test omits skills",
			ctx:  []byte(`{"durable_kind":"shadow","shadow_purpose":"necessity_test","omit_skills":true}`),
			want: ShadowClaimFields{Shadow: true, OmitSkills: true},
		},
		{
			name: "necessity_test by purpose alone omits skills",
			ctx:  []byte(`{"durable_kind":"shadow","shadow_purpose":"necessity_test"}`),
			want: ShadowClaimFields{Shadow: true, OmitSkills: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ShadowClaimFieldsOf(tc.ctx)
			if got != tc.want {
				t.Errorf("ShadowClaimFieldsOf(%s) = %+v, want %+v", string(tc.ctx), got, tc.want)
			}
		})
	}
}

// TestShadowOnPrimaryClaimedSetsOmitSkillsByPurpose: the enqueued model trial task records
// omit_skills=false, so it keeps the seat's skills. A necessity run's skill policy is
// TestShadowOmitsSkillUnderTest.
func TestShadowOnPrimaryClaimedSetsOmitSkillsByPurpose(t *testing.T) {
	for _, tc := range []struct {
		purpose  string
		wantOmit bool
	}{
		{purpose: ShadowPurposeModelTrial, wantOmit: false},
	} {
		t.Run(tc.purpose, func(t *testing.T) {
			sf := newShadowFixture(t, true)
			ctx := context.Background()
			if _, err := sf.svc.Start(ctx, sf.ws, sf.owner, ShadowStartInput{
				AgentID: util.MustParseUUID(sf.seat), CandidateRuntimeID: util.MustParseUUID(sf.cand),
				CandidateModel: "gpt-candidate", Purpose: tc.purpose, SampleRate: 1, TargetPairs: 10,
			}); err != nil {
				t.Fatalf("start %s shadow: %v", tc.purpose, err)
			}
			primaryID := sf.Task(t, sf.seat, testutil.Cols{"issue_id": sf.issue, "runtime_id": sf.runtime, "status": "dispatched"})
			primary, err := sf.q.GetAgentTask(ctx, util.MustParseUUID(primaryID))
			if err != nil {
				t.Fatalf("load primary: %v", err)
			}
			if err := sf.svc.OnPrimaryClaimed(ctx, primary); err != nil {
				t.Fatalf("OnPrimaryClaimed: %v", err)
			}
			pair, err := sf.q.GetShadowPairForPrimary(ctx, primary.ID)
			if err != nil {
				t.Fatalf("load pair: %v", err)
			}
			shadowTask, err := sf.q.GetAgentTask(ctx, pair.ShadowTaskID)
			if err != nil {
				t.Fatalf("load shadow task: %v", err)
			}
			got := ShadowClaimFieldsOf(shadowTask.Context)
			want := ShadowClaimFields{Shadow: true, OmitSkills: tc.wantOmit}
			if got != want {
				t.Fatalf("purpose %s: ShadowClaimFieldsOf(%s) = %+v, want %+v", tc.purpose, string(shadowTask.Context), got, want)
			}
		})
	}
}
