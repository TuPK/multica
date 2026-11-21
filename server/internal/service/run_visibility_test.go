package service

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// A run takes the visibility of the person who started it (spec 7.1 rule 12,
// D55, O-R4-3): the claim gate refuses a run whose originator cannot see the
// issue's project, and the task stays queued.

// runVisibilityFixture is a workspace with one restricted project, an issue in
// it and an agent that can run there.
type runVisibilityFixture struct {
	principalFixture
	projectID string
	issueID   string
	agentID   string
	runtimeID string
	ownerID   string
}

func newRunVisibilityFixture(t *testing.T) runVisibilityFixture {
	t.Helper()
	f, owner := newPrincipalFixture(t)
	projectID := f.Project(t, "restricted run project", testutil.Cols{"visibility": "private"})
	issueID := f.Issue(t, "restricted run issue", testutil.Cols{"project_id": projectID})
	runtimeID := f.Runtime(t, "run-visibility-runtime", testutil.Cols{"owner_id": owner})
	agentID := f.Agent(t, "run-visibility-agent", runtimeID, testutil.Cols{"owner_id": owner})
	fixture := runVisibilityFixture{principalFixture: f, projectID: projectID, issueID: issueID, agentID: agentID, runtimeID: runtimeID, ownerID: owner}
	// A run reads a private project only when both its member and its agent's
	// owner can see it, so the owner is a member of the project.
	fixture.grant(t, owner)
	return fixture
}

// grant gives userID access to the restricted project.
func (f runVisibilityFixture) grant(t *testing.T, userID string) {
	t.Helper()
	f.InsertNoID(t, "project_member", testutil.Cols{
		"project_id": f.projectID, "workspace_id": f.WorkspaceID, "user_id": userID,
	}, "project_id = $1 AND user_id = $2", f.projectID, userID)
}

// revoke removes userID from the restricted project's members.
func (f runVisibilityFixture) revoke(t *testing.T, userID string) {
	t.Helper()
	f.Exec(t, `DELETE FROM project_member WHERE project_id = $1 AND user_id = $2`, f.projectID, userID)
}

// queuedTask queues a task on the restricted issue. originator "" leaves
// originator_user_id NULL.
func (f runVisibilityFixture) queuedTask(t *testing.T, originator string, over ...testutil.Cols) string {
	t.Helper()
	cols := testutil.Cols{
		"runtime_id":        f.runtimeID,
		"issue_id":          f.issueID,
		"status":            "queued",
		"originator_source": "direct_human",
	}
	if originator != "" {
		cols["originator_user_id"] = originator
		cols["accountable_user_id"] = originator
	}
	for _, o := range over {
		for k, v := range o {
			cols[k] = v
		}
	}
	return f.Task(t, f.agentID, cols)
}

func (f runVisibilityFixture) refusal(t *testing.T, taskID string) *claimRefusal {
	t.Helper()
	task, err := f.q.GetAgentTask(context.Background(), util.MustParseUUID(taskID))
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	refusal, err := f.svc.TaskSvc.claimRefusalFor(context.Background(), f.q, util.MustParseUUID(f.WorkspaceID), task)
	if err != nil {
		t.Fatalf("claimRefusalFor: %v", err)
	}
	return refusal
}

// taskRow returns the columns a refusal must not move.
func (f runVisibilityFixture) taskRow(t *testing.T, taskID string) db.AgentTaskQueue {
	t.Helper()
	task, err := f.q.GetAgentTask(context.Background(), util.MustParseUUID(taskID))
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	return task
}

func TestClaimRefusedWhenOriginatorCannotSee(t *testing.T) {
	f := newRunVisibilityFixture(t)
	outsider := f.member(t, "outsider")
	taskID := f.queuedTask(t, outsider)
	before := f.taskRow(t, taskID)

	r := f.refusal(t, taskID)
	if r == nil {
		t.Fatal("a run whose originator cannot see the project was admitted")
	}
	if r.Reason != VerdictReasonProjectNotVisible {
		t.Errorf("reason = %q, want %q", r.Reason, VerdictReasonProjectNotVisible)
	}

	after := f.taskRow(t, taskID)
	if after.Status != "queued" || after.Status != before.Status ||
		after.OriginatorUserID != before.OriginatorUserID || after.DispatchedAt.Valid || after.StartedAt.Valid {
		t.Errorf("task changed by the refusal: before %+v, after %+v", before, after)
	}
}

func TestTaskClaimedAfterGrant(t *testing.T) {
	f := newRunVisibilityFixture(t)
	user := f.member(t, "later-grantee")
	taskID := f.queuedTask(t, user)

	if f.refusal(t, taskID) == nil {
		t.Fatal("before the grant the run was admitted")
	}
	f.grant(t, user)
	if r := f.refusal(t, taskID); r != nil {
		t.Errorf("after the grant the run is still refused with %q", r.Reason)
	}
}

func TestClaimAdmitsOwnerAndOpenProjects(t *testing.T) {
	f := newRunVisibilityFixture(t)
	if r := f.refusal(t, f.queuedTask(t, f.UserID)); r != nil {
		t.Errorf("owner's run refused with %q", r.Reason)
	}
	open := f.Project(t, "open run project")
	openIssue := f.Issue(t, "open run issue", testutil.Cols{"project_id": open})
	member := f.member(t, "plain")
	if r := f.refusal(t, f.queuedTask(t, member, testutil.Cols{"issue_id": openIssue})); r != nil {
		t.Errorf("run on a workspace project refused with %q", r.Reason)
	}
	noProject := f.Issue(t, "no project issue")
	if r := f.refusal(t, f.queuedTask(t, member, testutil.Cols{"issue_id": noProject})); r != nil {
		t.Errorf("run on an issue without a project refused with %q", r.Reason)
	}
}

func TestClaimRefusedWhenOriginatorLeftWorkspace(t *testing.T) {
	f := newRunVisibilityFixture(t)
	user := f.member(t, "leaver")
	f.grant(t, user)
	taskID := f.queuedTask(t, user)
	f.Exec(t, `DELETE FROM member WHERE workspace_id = $1 AND user_id = $2`, f.WorkspaceID, user)
	if r := f.refusal(t, taskID); r == nil || r.Reason != VerdictReasonProjectNotVisible {
		t.Errorf("a run of someone who left the workspace: refusal = %+v", r)
	}
}

// With no originator the run is bounded by its agent's owner alone, so it
// reads the project only while that owner is a member.
func TestNullOriginatorUserID(t *testing.T) {
	f := newRunVisibilityFixture(t)
	f.revoke(t, f.ownerID)
	if r := f.refusal(t, f.queuedTask(t, "", testutil.Cols{"originator_source": nil})); r == nil || r.Reason != VerdictReasonProjectNotVisible {
		t.Errorf("NULL originator on a restricted project: refusal = %+v, want %q", r, VerdictReasonProjectNotVisible)
	}
	open := f.Project(t, "open null originator")
	openIssue := f.Issue(t, "open null originator issue", testutil.Cols{"project_id": open})
	if r := f.refusal(t, f.queuedTask(t, "", testutil.Cols{"originator_source": nil, "issue_id": openIssue})); r != nil {
		t.Errorf("NULL originator on a workspace project refused with %q", r.Reason)
	}
}

// autopilotTask queues a task of an autopilot run whose originator is NULL, for
// an autopilot created by creator.
func (f runVisibilityFixture) autopilotTask(t *testing.T, creatorType, creatorID string) string {
	t.Helper()
	apID := f.Insert(t, "autopilot", testutil.Cols{
		"workspace_id":    f.WorkspaceID,
		"title":           "run visibility autopilot",
		"assignee_type":   "agent",
		"assignee_id":     f.agentID,
		"status":          "active",
		"execution_mode":  "create_issue",
		"created_by_type": creatorType,
		"created_by_id":   creatorID,
	})
	runID := f.Insert(t, "autopilot_run", testutil.Cols{"autopilot_id": apID, "source": "schedule", "status": "running"})
	return f.queuedTask(t, "", testutil.Cols{"originator_source": nil, "autopilot_run_id": runID})
}

func TestAutopilotRunUsesCreator(t *testing.T) {
	f := newRunVisibilityFixture(t)
	creator := f.member(t, "autopilot-creator")

	taskID := f.autopilotTask(t, "member", creator)
	if r := f.refusal(t, taskID); r == nil || r.Reason != VerdictReasonProjectNotVisible {
		t.Fatalf("creator cannot see the project: refusal = %+v, want %q", r, VerdictReasonProjectNotVisible)
	}
	if got := f.taskRow(t, taskID); got.Status != "queued" {
		t.Errorf("refused autopilot task status = %q, want queued", got.Status)
	}

	f.grant(t, creator)
	if r := f.refusal(t, taskID); r != nil {
		t.Errorf("creator can see the project but the run is refused with %q", r.Reason)
	}

	f.Exec(t, `DELETE FROM member WHERE workspace_id = $1 AND user_id = $2`, f.WorkspaceID, creator)
	if r := f.refusal(t, taskID); r == nil || r.Reason != VerdictReasonProjectNotVisible {
		t.Errorf("creator left the workspace: refusal = %+v, want %q", r, VerdictReasonProjectNotVisible)
	}
}

// A creator that is an agent is no person: the run sees no restricted project.
func TestAutopilotRunWithAgentCreatorSeesNoRestrictedProject(t *testing.T) {
	f := newRunVisibilityFixture(t)
	f.revoke(t, f.ownerID)
	taskID := f.autopilotTask(t, "agent", f.agentID)
	if r := f.refusal(t, taskID); r == nil || r.Reason != VerdictReasonProjectNotVisible {
		t.Errorf("agent-created autopilot: refusal = %+v, want %q", r, VerdictReasonProjectNotVisible)
	}
}

// The originator is the actor when there is one; the autopilot creator is
// used only when the originator is NULL.
func TestAutopilotRunPrefersOriginatorOverCreator(t *testing.T) {
	f := newRunVisibilityFixture(t)
	creator := f.member(t, "ap-creator-granted")
	f.grant(t, creator)
	outsider := f.member(t, "ap-originator-outsider")

	taskID := f.autopilotTask(t, "member", creator)
	f.Exec(t, `UPDATE agent_task_queue SET originator_user_id = $2, accountable_user_id = $2, originator_source = 'direct_human' WHERE id = $1`, taskID, outsider)
	if r := f.refusal(t, taskID); r == nil {
		t.Error("the originator cannot see the project but the creator's grant admitted the run")
	}
}

// Visibility is read only: refusing or admitting never writes the creator into
// originator_user_id (migration 129: the column decides the Composio overlay).
func TestOriginatorUserIDNotOverwritten(t *testing.T) {
	f := newRunVisibilityFixture(t)
	creator := f.member(t, "not-overwritten")
	f.grant(t, creator)
	taskID := f.autopilotTask(t, "member", creator)

	for _, step := range []string{"admitted", "refused"} {
		if step == "refused" {
			f.Exec(t, `DELETE FROM project_member WHERE project_id = $1 AND user_id = $2`, f.projectID, creator)
		}
		f.refusal(t, taskID)
		if got := f.taskRow(t, taskID); got.OriginatorUserID.Valid {
			t.Errorf("%s: originator_user_id = %s, want NULL", step, util.UUIDToString(got.OriginatorUserID))
		}
	}

	if _, err := runRefusedForVisibility(context.Background(), f.q, util.MustParseUUID(f.WorkspaceID), f.taskRow(t, taskID)); err != nil {
		t.Fatalf("runRefusedForVisibility: %v", err)
	}
	if got := f.taskRow(t, taskID); got.OriginatorUserID.Valid || got.AccountableUserID.Valid {
		t.Errorf("the visibility check wrote a person into the task: %+v", got)
	}
}
