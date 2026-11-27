package handler

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func latestIssueTransition(t *testing.T, issueID string) (actorType, actorID, taskID string) {
	t.Helper()
	dbfx.QueryRow(t, `
		SELECT actor_type, COALESCE(actor_id::text, ''), COALESCE(task_id::text, '')
		FROM issue_status_transition
		WHERE issue_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1`, issueID).Scan(&actorType, &actorID, &taskID)
	return actorType, actorID, taskID
}

func TestProposalStatusTransitionAttributesDecider(t *testing.T) {
	fx := newProposalTestFixture(t)
	waiting := dbfx.Issue(t, fx.title("waiting"), testutil.Cols{
		"status":        "blocked",
		"assignee_type": "agent",
		"assignee_id":   fx.LeaderID,
	})
	dbfx.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, waiting)

	filed := fx.fileProposal(t, "actor-check", map[string]any{
		"evidence": []string{fmt.Sprintf("Waiting on [blocked work](mention://issue/%s)", waiting)},
	})
	dbfx.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, filed.IssueID)

	req := withURLParam(decideRequest(filed.ID, map[string]any{"decision": "approve", "note": "go"}), "id", filed.ID)
	testutil.Call(t, testHandler.DecideProposal, req).Want(http.StatusOK)

	kind, actor, task := latestIssueTransition(t, filed.IssueID)
	if kind != "member" || actor != testUserID || task != "" {
		t.Errorf("proposal issue transition = (%q, %q, %q), want (member, %s, '')", kind, actor, task, testUserID)
	}
	wKind, wActor, wTask := latestIssueTransition(t, waiting)
	if wKind != "member" || wActor != testUserID || wTask != "" {
		t.Errorf("resumed waiting issue transition = (%q, %q, %q), want (member, %s, '')", wKind, wActor, wTask, testUserID)
	}
}

func TestInitiativeStatusTransitionAttributesActor(t *testing.T) {
	fx := newRoomedIniWorkspace(t)
	ini := fx.createInitiative(t, "Actor attribution initiative")
	var issueID string
	dbfx.QueryRow(t, `SELECT issue_id::text FROM initiative WHERE id = $1`, ini.ID).Scan(&issueID)
	dbfx.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, issueID)

	fx.transition(t, testHandler.ForceInitiative, fx.AdminID, ini.ID, map[string]any{
		"to_stage": "define",
		"reason":   "move to define",
	}).Want(http.StatusOK)
	kind, actor, task := latestIssueTransition(t, issueID)
	if kind != "member" || actor != fx.AdminID || task != "" {
		t.Errorf("force stage transition = (%q, %q, %q), want (member, %s, '')", kind, actor, task, fx.AdminID)
	}

	fx.transition(t, testHandler.CloseInitiative, fx.AdminID, ini.ID, map[string]any{
		"reason": "complete",
	}).Want(http.StatusOK)
	cKind, cActor, cTask := latestIssueTransition(t, issueID)
	if cKind != "member" || cActor != fx.AdminID || cTask != "" {
		t.Errorf("close initiative transition = (%q, %q, %q), want (member, %s, '')", cKind, cActor, cTask, fx.AdminID)
	}
}

func TestFlowEngineStatusTransitionAttributesActor(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	reqID := uuidToString(request.ID)
	dbfx.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, reqID)

	var leadID string
	dbfx.QueryRow(t, `SELECT leader_id::text FROM squad WHERE id = $1`, f.SquadID).Scan(&leadID)
	runID := goldenRun(t, reqID, leadID, "todo")

	// Lead agent blocks the request via PlanIssueFlow -> should attribute to (agent, leadID, runID).
	planReq := newRequest("POST", "/api/issues/"+reqID+"/flow/plan", map[string]any{
		"switches": map[string]any{},
		"blocked":  map[string]any{"missing": []string{"invoice"}, "reason": "need invoice"},
	})
	planReq = withURLParams(planReq, "id", reqID)
	planReq.Header.Set("X-Actor-Source", "task_token")
	planReq.Header.Set("X-Agent-ID", leadID)
	planReq.Header.Set("X-Task-ID", runID)
	testutil.Call(t, testHandler.PlanIssueFlow, planReq).Want(http.StatusOK)

	kind, actor, task := latestIssueTransition(t, reqID)
	if kind != "agent" || actor != leadID || task != runID {
		t.Errorf("lead plan transition = (%q, %q, %q), want (agent, %s, %s)", kind, actor, task, leadID, runID)
	}

	// Member resumes the blocked request via ResumeIssueFlow -> should attribute to (member, testUserID, "").
	resumeReq := newRequest("POST", "/api/issues/"+reqID+"/flow/resume", map[string]any{})
	resumeReq = withURLParams(resumeReq, "id", reqID)
	testutil.Call(t, testHandler.ResumeIssueFlow, resumeReq).Want(http.StatusOK)

	rKind, rActor, rTask := latestIssueTransition(t, reqID)
	if rKind != "member" || rActor != testUserID || rTask != "" {
		t.Errorf("member resume transition = (%q, %q, %q), want (member, %s, '')", rKind, rActor, rTask, testUserID)
	}
}
