package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Choosing the workflow of a request and rehearsing a version (spec 16.5, 7.1; criteria N8, N9).

// startAs calls POST /flow/start on an issue. A nil body sends none.
func startAs(t *testing.T, issueID string, body any, as func(*http.Request) *http.Request) *testutil.Response {
	t.Helper()
	req := newRequest("POST", "/api/issues/"+issueID+"/flow/start", body)
	if as != nil {
		req = as(req)
	}
	return testutil.Call(t, testHandler.StartIssueFlow, withURLParam(req, "id", issueID))
}

// squadIssue is an issue given to the squad that no path started on a flow.
func squadIssue(t *testing.T, squadID string) string {
	t.Helper()
	return dbfx.Issue(t, "chosen request "+workflowTestSuffix(), testutil.Cols{"status": "todo", "assignee_type": "squad", "assignee_id": squadID})
}

// twoVersions publishes two versions for the rig's squad and makes the first the active one.
func (rig *draftRig) twoVersions() (active, other IssueWorkflowResponse) {
	rig.t.Helper()
	active = rig.publishOK(rig.draftWith("Active version", rig.publishableDoc(rig.agents[0])), nil)
	other = rig.publishOK(rig.draftWith("Other version", rig.publishableDoc(rig.agents[0])), nil)
	rig.setActive(active.ID)
	return active, other
}

func TestStart_ExplicitVersionOfSameSquad(t *testing.T) {
	rig := newDraftRig(t, 1)
	active, other := rig.twoVersions()

	// No body: the squad's active version, as before.
	var plain IssueFlowStateResponse
	startAs(t, squadIssue(t, rig.squadID), nil, nil).Want(http.StatusOK).JSON(&plain)
	if plain.WorkflowID != active.ID || plain.Trial {
		t.Fatalf("without a body: workflow %s trial %v, want the active %s and no trial", plain.WorkflowID, plain.Trial, active.ID)
	}

	// An explicit version of the same squad that is not the active one.
	var chosen IssueFlowStateResponse
	startAs(t, squadIssue(t, rig.squadID), map[string]any{"workflow_id": other.ID}, nil).Want(http.StatusOK).JSON(&chosen)
	if chosen.WorkflowID != other.ID || chosen.Trial {
		t.Fatalf("with workflow_id: workflow %s trial %v, want %s and no trial", chosen.WorkflowID, chosen.Trial, other.ID)
	}
	if got := squadActiveWorkflowID(t, rig.squadID); got != active.ID {
		t.Fatalf("choosing a version moved the squad's active version to %s", got)
	}
}

func TestStart_OtherSquadOrNonFlowRejected(t *testing.T) {
	rig := newDraftRig(t, 1)
	active, _ := rig.twoVersions()
	otherRig := newDraftRig(t, 1)
	foreign := otherRig.publishOK(otherRig.draftWith("Foreign version", otherRig.publishableDoc(otherRig.agents[0])), nil)

	// A workflow of the squad that does not run on a flow.
	plainWorkflow := createTestWorkflow(t, "Not a flow "+workflowTestSuffix())
	dbfx.Exec(t, `UPDATE issue_workflow SET squad_id = $2 WHERE id = $1`, plainWorkflow.ID, rig.squadID)

	missing := "00000000-0000-4000-8000-000000000001"
	for name, id := range map[string]string{
		"a version of another squad":   foreign.ID,
		"a workflow that is not flow":  plainWorkflow.ID,
		"a workflow that is not there": missing,
	} {
		issue := squadIssue(t, rig.squadID)
		wantCode(t, startAs(t, issue, map[string]any{"workflow_id": id}, nil), http.StatusUnprocessableEntity, "flow_workflow_invalid")
		if flow := issueFlowRaw(t, issue); flow != "" {
			t.Errorf("%s: the refused start left a flow on the issue: %s", name, flow)
		}
	}
	startAs(t, squadIssue(t, rig.squadID), map[string]any{"workflow_id": "not-a-uuid"}, nil).Want(http.StatusBadRequest)

	// The request still starts on the active version afterwards.
	var state IssueFlowStateResponse
	startAs(t, squadIssue(t, rig.squadID), map[string]any{}, nil).Want(http.StatusOK).JSON(&state)
	if state.WorkflowID != active.ID {
		t.Fatalf("workflow %s, want the active %s", state.WorkflowID, active.ID)
	}
}

// issueFlowRaw is the stored flow of an issue, or "" when it has none.
func issueFlowRaw(t *testing.T, issueID string) string {
	t.Helper()
	var flow *string
	dbfx.QueryRow(t, `SELECT flow::text FROM issue WHERE id = $1`, issueID).Scan(&flow)
	if flow == nil {
		return ""
	}
	return *flow
}

// releaseDoc is the trade flow with a policy that lets work leave unseen and a handover that goes straight out.
func releaseDoc(doc map[string]any) {
	doc["policy"] = map[string]any{"allow_unattended_release": true}
	for _, step := range asMaps(doc["steps"]) {
		if step["key"] != "handover" {
			continue
		}
		for _, edge := range asMaps(step["edges"]) {
			if edge["id"] == "handover_ok" {
				edge["to"] = "done"
			}
		}
	}
}

// driveToHandover runs a started request of the trade flow until its handover is done.
func (r *graphRig) driveToHandover(request db.Issue) IssueFlowStateResponse {
	r.t.Helper()
	r.complete(request, nil)
	r.finishAll(request, "collect", 0, always(""))
	r.advance(request)
	r.finishAll(request, "draft", 0, always(""))
	r.advance(request)
	r.finishAll(request, "gate", 0, always("approved"))
	r.advance(request)
	r.finishAll(request, "handover", 0, always("approved"))
	return r.advance(request)
}

// cloneWorkflowParams writes the rig's workflow again under another name, as a second version of the squad.
func cloneWorkflowParams(t *testing.T, rig *graphRig) db.CreateIssueWorkflowParams {
	t.Helper()
	return db.CreateIssueWorkflowParams{
		WorkspaceID:      rig.row.WorkspaceID,
		Name:             "Second version " + workflowTestSuffix(),
		InitialStatusKey: rig.row.InitialStatusKey,
		Steps:            rig.row.Steps,
		OwnerID:          rig.row.OwnerID,
		SquadID:          rig.row.SquadID,
		FlowConfig:       rig.row.FlowConfig,
	}
}

func TestTrial_PinsVersionAndForbidsUnattendedRelease(t *testing.T) {
	withFlowFlags(t, true, true)
	rig := newGraphRig(t, "trade", graphRigOpts{Mutate: releaseDoc})
	// Another version becomes the squad's active one, so the rehearsal has to be pinned to the chosen version.
	other, err := testHandler.Queries.CreateIssueWorkflow(context.Background(), cloneWorkflowParams(t, rig))
	if err != nil {
		t.Fatalf("write the second version: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE squad SET active_workflow_id = NULL WHERE id = $1`, rig.squadID)
		testPool.Exec(context.Background(), `DELETE FROM issue_workflow WHERE id = $1`, other.ID)
	})
	otherID := uuidToString(other.ID)
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, rig.squadID, otherID)
	chosen := uuidToString(rig.row.ID)

	// N8: the rehearsal runs on the chosen version and says so.
	var trial IssueFlowStateResponse
	trialIssue := squadIssue(t, rig.squadID)
	startAs(t, trialIssue, map[string]any{"workflow_id": chosen, "trial": true}, nil).Want(http.StatusOK).JSON(&trial)
	if trial.WorkflowID != chosen || !trial.Trial {
		t.Fatalf("trial state: workflow %s trial %v, want %s and trial", trial.WorkflowID, trial.Trial, chosen)
	}
	// A trial without workflow_id uses the active version and is still a trial.
	var activeTrial IssueFlowStateResponse
	startAs(t, squadIssue(t, rig.squadID), map[string]any{"trial": true}, nil).Want(http.StatusOK).JSON(&activeTrial)
	if activeTrial.WorkflowID != otherID || !activeTrial.Trial {
		t.Fatalf("trial without a version: workflow %s trial %v, want the active %s and trial", activeTrial.WorkflowID, activeTrial.Trial, otherID)
	}

	// Work that nobody saw does not leave the workspace in a trial, whatever the policy says.
	request := loadIssueRow(t, trialIssue)
	st := rig.driveToHandover(request)
	if st.Stop == nil || st.Stop.Code != issueworkflow.DocStopStalled || !st.Unreviewed || st.Step != "handover" {
		t.Fatalf("trial: stop %+v unreviewed %v step %q, want a stall at the handover with work nobody saw", st.Stop, st.Unreviewed, st.Step)
	}
	if requestStatus(t, request) == "done" {
		t.Fatal("the trial left the workspace without a person")
	}

	// The same version, not a trial: the policy applies and the request leaves (N8 is about trials only).
	var normal IssueFlowStateResponse
	normalIssue := squadIssue(t, rig.squadID)
	startAs(t, normalIssue, map[string]any{"workflow_id": chosen}, nil).Want(http.StatusOK).JSON(&normal)
	if normal.Trial {
		t.Fatal("a start without trial is marked as a trial")
	}
	rig.driveToHandover(loadIssueRow(t, normalIssue))
	rig.wantClosedAt(loadIssueRow(t, normalIssue), "done")

	// Only a person who manages the squad rehearses: its lead does not.
	agent := squadLeadID(t, rig.squadID)
	req := newRequest("POST", "/api/issues/"+normalIssue+"/flow/start", map[string]any{"trial": true})
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agent)
	fresh := squadIssue(t, rig.squadID)
	req = withURLParam(req, "id", fresh)
	wantFlowRejection(t, testutil.Call(t, testHandler.StartIssueFlow, req), http.StatusForbidden, "flow_forbidden")
}

func TestTrial_SchemaOneVersionRefused(t *testing.T) {
	withFlowFlags(t, true, true)
	f := newTradeFlowFixture(t)
	issue := squadIssue(t, f.SquadID)
	wantCode(t, startAs(t, issue, map[string]any{"trial": true}, nil), http.StatusUnprocessableEntity, "flow_workflow_invalid")
	if flow := issueFlowRaw(t, issue); flow != "" {
		t.Fatalf("a refused trial left a flow: %s", flow)
	}
}

// createSquadIssue posts a new issue given to the squad, with a chosen version when flowWorkflowID is set.
func createSquadIssue(t *testing.T, squadID, flowWorkflowID string, as func(*http.Request) *http.Request, extra ...map[string]any) *testutil.Response {
	t.Helper()
	body := map[string]any{"title": "Created request " + workflowTestSuffix(), "assignee_type": "squad", "assignee_id": squadID}
	if flowWorkflowID != "" {
		body["flow_workflow_id"] = flowWorkflowID
	}
	for _, more := range extra {
		for k, v := range more {
			body[k] = v
		}
	}
	req := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, body)
	if as != nil {
		req = as(req)
	}
	return testutil.Call(t, testHandler.CreateIssue, req)
}

func pinnedWorkflow(t *testing.T, issueID string) string {
	t.Helper()
	var id *string
	dbfx.QueryRow(t, `SELECT flow->>'workflow_id' FROM issue WHERE id = $1`, issueID).Scan(&id)
	if id == nil {
		return ""
	}
	return *id
}

func TestCreateIssue_FlowWorkflowIDChoosesTheVersion(t *testing.T) {
	rig := newDraftRig(t, 1)
	active, other := rig.twoVersions()
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id = $1 AND title LIKE 'Created request %'`, testWorkspaceID)
	})

	var chosen IssueResponse
	createSquadIssue(t, rig.squadID, other.ID, nil).Want(http.StatusCreated).JSON(&chosen)
	if got := pinnedWorkflow(t, chosen.ID); got != other.ID {
		t.Fatalf("the request is on %q, want the chosen %s", got, other.ID)
	}
	var plain IssueResponse
	createSquadIssue(t, rig.squadID, "", nil).Want(http.StatusCreated).JSON(&plain)
	if got := pinnedWorkflow(t, plain.ID); got != active.ID {
		t.Fatalf("without a choice the request is on %q, want the active %s", got, active.ID)
	}

	// A version of another squad is refused and nothing is created.
	otherRig := newDraftRig(t, 1)
	foreign := otherRig.publishOK(otherRig.draftWith("Foreign choice", otherRig.publishableDoc(otherRig.agents[0])), nil)
	before := dbfx.Count(t, `SELECT count(*) FROM issue WHERE workspace_id = $1 AND title LIKE 'Created request %'`, testWorkspaceID)
	wantCode(t, createSquadIssue(t, rig.squadID, foreign.ID, nil), http.StatusUnprocessableEntity, "flow_workflow_invalid")
	if after := dbfx.Count(t, `SELECT count(*) FROM issue WHERE workspace_id = $1 AND title LIKE 'Created request %'`, testWorkspaceID); after != before {
		t.Fatalf("a refused choice created an issue (%d -> %d)", before, after)
	}
}

func TestCreateIssue_FlowWorkflowIDIgnoredForAgentActor(t *testing.T) {
	rig := newDraftRig(t, 1)
	active, other := rig.twoVersions()
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id = $1 AND title LIKE 'Created request %'`, testWorkspaceID)
	})
	agent := squadLeadID(t, rig.squadID)
	asAgent := func(req *http.Request) *http.Request {
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agent)
		return req
	}
	// The agent's choice has no effect, and it is not an error either (N9).
	var created IssueResponse
	createSquadIssue(t, rig.squadID, other.ID, asAgent).Want(http.StatusCreated).JSON(&created)
	if got := pinnedWorkflow(t, created.ID); got != active.ID {
		t.Fatalf("the agent's request is on %q, want the active %s", got, active.ID)
	}
}

func TestCreateIssue_FlowTrialStartsARehearsalOnTheChosenVersion(t *testing.T) {
	rig := newDraftRig(t, 1)
	active, other := rig.twoVersions()
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id = $1 AND title LIKE 'Created request %'`, testWorkspaceID)
	})
	trial := map[string]any{"flow_trial": true}

	var created IssueResponse
	createSquadIssue(t, rig.squadID, other.ID, nil, trial).Want(http.StatusCreated).JSON(&created)
	state, err := testHandler.flowStateResponse(context.Background(), loadIssueRow(t, created.ID), issueworkflow.RoleRequest)
	if err != nil || state.WorkflowID != other.ID || !state.Trial {
		t.Fatalf("state %+v, err %v; want a trial on %s", state, err, other.ID)
	}
	if got := squadActiveWorkflowID(t, rig.squadID); got != active.ID {
		t.Fatalf("the rehearsal moved the active version to %s", got)
	}

	// A trial without a named version rehearses the active one.
	var onActive IssueResponse
	createSquadIssue(t, rig.squadID, "", nil, trial).Want(http.StatusCreated).JSON(&onActive)
	if got := pinnedWorkflow(t, onActive.ID); got != active.ID {
		t.Fatalf("a trial with no version is on %q, want the active %s", got, active.ID)
	}

	// Only a person who manages the squad rehearses; an agent's request is not a trial.
	_, asPlain := addWorkflowTestMember(t, "trial-plain")
	wantCode(t, createSquadIssue(t, rig.squadID, other.ID, asPlain, trial), http.StatusForbidden, "flow_forbidden")
	agent := squadLeadID(t, rig.squadID)
	var byAgent IssueResponse
	createSquadIssue(t, rig.squadID, other.ID, func(req *http.Request) *http.Request {
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agent)
		return req
	}, trial).Want(http.StatusCreated).JSON(&byAgent)
	if st, err := testHandler.flowStateResponse(context.Background(), loadIssueRow(t, byAgent.ID), issueworkflow.RoleRequest); err != nil || st.Trial || st.WorkflowID != active.ID {
		t.Fatalf("an agent's request: %+v, err %v; want the active version and no trial", st, err)
	}
}

// The fields of this file are part of flow authoring: with its flag off they are refused, and a request
// that does not use them runs as before (spec 7.1; FF_FLOW_AUTHORING).

func TestStart_FieldsAre404WhenAuthoringOff(t *testing.T) {
	rig := newDraftRig(t, 1)
	active, other := rig.twoVersions()
	withFlowFlags(t, false, true)

	for name, body := range map[string]map[string]any{
		"a chosen version":          {"workflow_id": other.ID},
		"a trial":                   {"trial": true},
		"a chosen version in trial": {"workflow_id": other.ID, "trial": true},
	} {
		issue := squadIssue(t, rig.squadID)
		wantCode(t, startAs(t, issue, body, nil), http.StatusNotFound, "flow_authoring_disabled")
		if flow := issueFlowRaw(t, issue); flow != "" {
			t.Errorf("%s: the refused start left a flow on the issue: %s", name, flow)
		}
	}

	// A start that asks for nothing new works as it always did, with or without a body.
	for name, body := range map[string]any{"no body": nil, "empty body": map[string]any{}, "trial false": map[string]any{"trial": false}} {
		var state IssueFlowStateResponse
		startAs(t, squadIssue(t, rig.squadID), body, nil).Want(http.StatusOK).JSON(&state)
		if state.WorkflowID != active.ID || state.Trial {
			t.Errorf("%s: workflow %s trial %v, want the active %s and no trial", name, state.WorkflowID, state.Trial, active.ID)
		}
	}
}

func TestCreateIssue_FlowFieldsAre404WhenAuthoringOff(t *testing.T) {
	rig := newDraftRig(t, 1)
	active, other := rig.twoVersions()
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE workspace_id = $1 AND title LIKE 'Created request %'`, testWorkspaceID)
	})
	withFlowFlags(t, false, true)

	count := func() int {
		return dbfx.Count(t, `SELECT count(*) FROM issue WHERE workspace_id = $1 AND title LIKE 'Created request %'`, testWorkspaceID)
	}
	before := count()
	wantCode(t, createSquadIssue(t, rig.squadID, other.ID, nil), http.StatusNotFound, "flow_authoring_disabled")
	wantCode(t, createSquadIssue(t, rig.squadID, "", nil, map[string]any{"flow_trial": true}), http.StatusNotFound, "flow_authoring_disabled")
	if after := count(); after != before {
		t.Fatalf("a refused create wrote an issue (%d -> %d)", before, after)
	}

	// Without the fields, a squad's request still starts on its active version.
	var created IssueResponse
	createSquadIssue(t, rig.squadID, "", nil).Want(http.StatusCreated).JSON(&created)
	if got := pinnedWorkflow(t, created.ID); got != active.ID {
		t.Fatalf("the request is on %q, want the active %s", got, active.ID)
	}
}
