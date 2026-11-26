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
