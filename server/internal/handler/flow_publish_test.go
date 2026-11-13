package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Publishing a draft as a locked version (plan P15, spec 8.1, criteria C1 to C8).

// publishableDoc is a flow that publishes without a warning: the lead plans, an agent
// of the squad writes, and a person of the workspace (the owner) checks. Its two board
// statuses are new names, which only an admin may create.
func (rig *draftRig) publishableDoc(agent string) map[string]any {
	suffix := workflowTestSuffix()
	writeName, checkName := "Write "+suffix, "Check "+suffix
	t := rig.t
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue_status WHERE workspace_id = $1 AND name IN ($2, $3)`, testWorkspaceID, writeName, checkName)
	})
	doc := baseDoc(
		[]any{
			map[string]any{"key": "writer", "kind": "agent", "bind": map[string]any{"agent_ids": []string{agent}}},
			map[string]any{"key": "checker", "kind": "member", "bind": map[string]any{"member_ids": []string{testUserID}}},
		},
		docLead("write"),
		docStep("write", []any{docTask("draft", "work", "writer", nil)}, docEdge("w_ok", "check", nil)),
		docStep("check", []any{docTask("sign", "gate", "checker", nil)},
			docEdge("c_pass", "done", joinIs("pass")),
			docStopEdge("c_fail", "loop_exhausted", joinIs("fail")),
			docStopEdge("c_block", "grounds_missing", joinIs("block")),
			docStopEdge("c_none", "undecided", nil)),
	)
	doc["policy"] = map[string]any{}
	steps := asMaps(doc["steps"])
	steps[1]["status"] = map[string]any{"name": writeName, "category": "started"}
	steps[2]["status"] = map[string]any{"name": checkName, "category": "started"}
	return doc
}

// draftWith makes a draft that holds the document.
func (rig *draftRig) draftWith(name string, doc map[string]any) FlowDraftResponse {
	rig.t.Helper()
	d := rig.newDraft(name)
	var saved FlowDraftResponse
	rig.patchDraft(d.ID, map[string]any{"revision": d.Revision, "doc": doc}).Want(http.StatusOK).JSON(&saved)
	return saved
}

func (rig *draftRig) publish(as func(*http.Request) *http.Request, d FlowDraftResponse, extra map[string]any) *testutil.Response {
	rig.t.Helper()
	body := map[string]any{"revision": d.Revision}
	for k, v := range extra {
		body[k] = v
	}
	return rig.call(testHandler.PublishFlowDraft, as, "POST", "/flow-drafts/"+d.ID+"/publish", body, "draftId", d.ID)
}

func (rig *draftRig) publishOK(d FlowDraftResponse, extra map[string]any) IssueWorkflowResponse {
	rig.t.Helper()
	var wf IssueWorkflowResponse
	rig.publish(nil, d, extra).Want(http.StatusCreated).JSON(&wf)
	return wf
}

// planeMember is a plain member of the workspace who made the squad, so manages it
// without being an admin.
func (rig *draftRig) managerMember(label string) func(*http.Request) *http.Request {
	rig.t.Helper()
	id, as := addWorkflowTestMember(rig.t, label)
	dbfx.Exec(rig.t, `UPDATE squad SET creator_id = $2 WHERE id = $1`, rig.squadID, id)
	return as
}

func workflowFlowConfig(t *testing.T, id string) string {
	t.Helper()
	var raw string
	dbfx.QueryRow(t, `SELECT flow_config::text FROM issue_workflow WHERE id = $1`, id).Scan(&raw)
	return raw
}

func TestPublish_CreatesLockedVersion(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.draftWith("Intake", rig.publishableDoc(rig.agents[0]))

	wf := rig.publishOK(d, nil)
	if wf.Version == nil || *wf.Version != 1 || wf.LineageID == nil || wf.PublishedAt == nil || wf.PublishedBy == nil || *wf.PublishedBy != testUserID {
		t.Fatalf("published row = %+v", wf)
	}
	if wf.Name != "Intake · v1" || wf.SquadID == nil || *wf.SquadID != rig.squadID || wf.TemplateRef != nil || wf.FlowAuto {
		t.Fatalf("published row = %+v", wf)
	}
	if wf.FlowConfig == nil || wf.FlowConfig.Version != 2 || len(wf.FlowDoc) == 0 {
		t.Fatalf("the row has no document: %+v", wf)
	}
	// The row keeps the document with its statuses resolved to keys, and the
	// statuses the document uses as its steps.
	var stored issueworkflow.FlowDoc
	if err := json.Unmarshal(wf.FlowDoc, &stored); err != nil {
		t.Fatal(err)
	}
	for _, s := range stored.Steps {
		if s.Status.Key == "" {
			t.Fatalf("step %s kept a status name: %+v", s.Key, s.Status)
		}
	}
	keys := make([]string, 0, len(wf.Steps))
	for _, s := range wf.Steps {
		keys = append(keys, s.StatusKey)
	}
	if !hasString(keys, "todo") || !hasString(keys, "done") || !hasString(keys, stored.Steps[1].Status.Key) {
		t.Fatalf("steps = %v", keys)
	}
	// The server does not make the version active.
	if got := squadActiveWorkflowID(t, rig.squadID); got != "" {
		t.Fatalf("publishing made %s the squad's workflow", got)
	}
	// The draft stays, based on the new version.
	after := rig.getDraft(d.ID)
	if after.BaseWorkflowID == nil || *after.BaseWorkflowID != wf.ID || after.LineageID == nil || *after.LineageID != *wf.LineageID {
		t.Fatalf("draft after publish = %+v", after)
	}
}

func TestPublish_SecondVersionLeavesFirstUntouched(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.draftWith("Twice", rig.publishableDoc(rig.agents[0]))
	first := rig.publishOK(d, nil)
	before := workflowFlowConfig(t, first.ID)

	d = rig.getDraft(d.ID)
	doc := rig.publishableDoc(rig.agents[0])
	asMaps(doc["steps"])[1]["title"] = "Write it again"
	var saved FlowDraftResponse
	rig.patchDraft(d.ID, map[string]any{"revision": d.Revision, "doc": doc}).Want(http.StatusOK).JSON(&saved)
	second := rig.publishOK(saved, nil)

	if second.Version == nil || *second.Version != 2 || *second.LineageID != *first.LineageID || second.Name != "Twice · v2" || second.ID == first.ID {
		t.Fatalf("second = %+v, first = %+v", second, first)
	}
	if workflowFlowConfig(t, first.ID) != before {
		t.Fatal("the first version changed when the second was published")
	}
	// The same draft published again at its old revision is a conflict.
	wantCode(t, rig.publish(nil, saved, nil), http.StatusConflict, "flow_draft_conflict")
}

func TestPublish_RunningRequestKeepsPinnedVersion(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.draftWith("Pinned", rig.publishableDoc(rig.agents[0]))
	v1 := rig.publishOK(d, nil)
	rig.setActive(v1.ID)

	issueID := dbfx.Issue(t, "Pinned request "+workflowTestSuffix(), testutil.Cols{
		"status": "todo", "assignee_type": "squad", "assignee_id": rig.squadID,
	})
	started, err := testHandler.EnsureFlowStarted(context.Background(), loadIssueRow(t, issueID))
	if err != nil || len(started.Flow) == 0 {
		t.Fatalf("EnsureFlowStarted: flow %q, err %v", started.Flow, err)
	}
	pinned := func() string {
		var id string
		dbfx.QueryRow(t, `SELECT COALESCE(flow->>'workflow_id', '') FROM issue WHERE id = $1`, issueID).Scan(&id)
		return id
	}
	if pinned() != v1.ID {
		t.Fatalf("the request is on %s, want v1 %s", pinned(), v1.ID)
	}

	d = rig.getDraft(d.ID)
	doc := rig.publishableDoc(rig.agents[0])
	asMaps(doc["steps"])[1]["title"] = "Write v2"
	var saved FlowDraftResponse
	rig.patchDraft(d.ID, map[string]any{"revision": d.Revision, "doc": doc}).Want(http.StatusOK).JSON(&saved)
	v2 := rig.publishOK(saved, nil)
	rig.setActive(v2.ID)

	if got := squadActiveWorkflowID(t, rig.squadID); got != v2.ID {
		t.Fatalf("active = %s, want v2", got)
	}
	if pinned() != v1.ID {
		t.Fatalf("the request moved to %s when v2 became active", pinned())
	}
	// It still runs: the state is read from v1.
	state, err := testHandler.flowStateResponse(context.Background(), loadIssueRow(t, issueID), issueworkflow.RoleRequest)
	if err != nil || state.WorkflowID != v1.ID {
		t.Fatalf("state = %+v, err %v", state, err)
	}
}

// setActive makes a workflow the squad's, through the route that already does that.
func (rig *draftRig) setActive(workflowID string) {
	rig.t.Helper()
	rig.call(testHandler.SetSquadActiveWorkflow, nil, "POST", "/workflow", map[string]any{"workflow_id": workflowID}).Want(http.StatusOK)
}

func TestPublish_MakeActiveThenSetSquadActive(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.draftWith("Active", rig.publishableDoc(rig.agents[0]))
	wf := rig.publishOK(d, map[string]any{"make_active": true})
	// make_active is the client's to carry out: the server leaves the squad alone.
	if got := squadActiveWorkflowID(t, rig.squadID); got != "" {
		t.Fatalf("the server made %s active", got)
	}
	rig.setActive(wf.ID)
	if got := squadActiveWorkflowID(t, rig.squadID); got != wf.ID {
		t.Fatalf("active = %s, want %s", got, wf.ID)
	}
}

func TestPublish_ReturnsAllIssues(t *testing.T) {
	rig := newDraftRig(t, 1)
	doc := rig.publishableDoc(rig.agents[0])
	roles := asMaps(doc["roles"])
	// R02: a role with nobody bound. R03: another bound to a person who is not in the workspace.
	delete(roles[0], "bind")
	roles[1]["bind"] = map[string]any{"member_ids": []string{"00000000-0000-4000-8000-000000000001"}}
	// R04: a role no task uses. P02: a message that mentions a person.
	doc["roles"] = []any{roles[0], roles[1], map[string]any{"key": "idle", "kind": "agent"}}
	// R01: a task of a role the document does not have. R07: the gate is the role that did the work.
	write := asMaps(doc["steps"])[1]
	write["tasks"] = []any{write["tasks"].([]any)[0], docTask("extra", "work", "ghost", nil)}
	asMaps(asMaps(doc["steps"])[2]["tasks"])[0]["role"] = "writer"
	// F01: a condition that reads a task the flow does not have.
	asMaps(write["edges"])[0]["when"] = map[string]any{"field": "task.ghost.outcome", "op": "eq", "value": "x"}
	doc["messages"].(map[string]any)["stop1"] = "see mention://member/abc"
	d := rig.draftWith("Broken", doc)

	body := wantCode(t, rig.publish(nil, d, nil), http.StatusUnprocessableEntity, "flow_draft_invalid")
	issues, _ := body["issues"].([]any)
	codes := map[string]bool{}
	rules := map[string]bool{}
	for _, it := range issues {
		issue := it.(map[string]any)
		codes[issue["code"].(string)] = true
		if params, ok := issue["params"].(map[string]any); ok {
			if rule, ok := params["rule"].(string); ok {
				rules[rule] = true
			}
		}
	}
	// C4: the rules R01 to R04, R07, F01 and P02 are all in one answer.
	for _, want := range []string{"R01", "R02", "R03", "R04", "R07", "F01", "P02"} {
		if !rules[want] {
			t.Fatalf("issues lack rule %s: rules %v, codes %v", want, rules, codes)
		}
	}
	var n int
	dbfx.QueryRow(t, `SELECT count(*) FROM issue_workflow WHERE squad_id = $1`, rig.squadID).Scan(&n)
	if n != 0 {
		t.Fatalf("a refused publish wrote %d rows", n)
	}
}

func TestPublish_Forbidden(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.draftWith("Forbidden", rig.publishableDoc(rig.agents[0]))

	// Not the squad's manager.
	_, asOther := addWorkflowTestMember(t, "publishother")
	wantCode(t, rig.publish(asOther, d, nil), http.StatusForbidden, "flow_publish_forbidden")

	// A manager who may not wire the agent: it is another member's private agent.
	ownerID, _ := addWorkflowTestMember(t, "privateowner")
	private := dbfx.Agent(t, "Private "+workflowTestSuffix(), handlerTestRuntimeID(t), testutil.Cols{
		"workspace_id": testWorkspaceID, "visibility": "private", "owner_id": ownerID,
	})
	dbfx.SquadMember(t, rig.squadID, "agent", private)
	asManager := rig.managerMember("publishmanager")
	privateDoc := rig.publishableDoc(private)
	// Existing statuses, so that the status rule does not answer first.
	steps := asMaps(privateDoc["steps"])
	steps[1]["status"] = map[string]any{"key": "in_progress"}
	steps[2]["status"] = map[string]any{"key": "in_review"}
	var saved FlowDraftResponse
	rig.patchDraft(d.ID, map[string]any{"revision": d.Revision, "doc": privateDoc}).Want(http.StatusOK).JSON(&saved)
	body := wantCode(t, rig.publish(asManager, saved, map[string]any{"ack": []string{"no_gate"}}), http.StatusForbidden, "flow_agent_forbidden")
	if body["agent_id"] != private {
		t.Fatalf("agent_id = %v, want %s", body["agent_id"], private)
	}
}

func TestPublish_NewStatusNeedsAdmin(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.draftWith("Statuses", rig.publishableDoc(rig.agents[0]))
	asManager := rig.managerMember("statusmanager")
	wantCode(t, rig.publish(asManager, d, nil), http.StatusForbidden, "status_create_forbidden")
	// The refused publish made nothing.
	var n int
	dbfx.QueryRow(t, `SELECT count(*) FROM issue_workflow WHERE squad_id = $1`, rig.squadID).Scan(&n)
	if n != 0 {
		t.Fatalf("%d rows written", n)
	}
	// An admin creates the statuses.
	rig.publishOK(d, nil)
}

// gateByAgents is a document whose work is checked only by another agent: nobody looks
// at it before it is released.
func (rig *draftRig) gateByAgents(first, second string) map[string]any {
	doc := rig.publishableDoc(first)
	roles := asMaps(doc["roles"])
	roles[1]["kind"] = "agent"
	roles[1]["bind"] = map[string]any{"agent_ids": []string{second}}
	// What leaves the workspace is what the warning is about.
	asMaps(doc["terminals"])[0]["leaves_workspace"] = true
	return doc
}

func TestPublish_ReleaseWarningNeedsAdminAck(t *testing.T) {
	rig := newDraftRig(t, 2)
	d := rig.draftWith("Unattended", rig.gateByAgents(rig.agents[0], rig.agents[1]))

	body := wantCode(t, rig.publish(nil, d, nil), http.StatusUnprocessableEntity, "flow_ack_required")
	if required, _ := body["required"].([]any); len(required) != 1 || required[0] != issueworkflow.AckUnattendedRelease {
		t.Fatalf("required = %v", body["required"])
	}
	// A manager who is not an admin cannot give it.
	asManager := rig.managerMember("ackmanager")
	wantCode(t, rig.publish(asManager, d, map[string]any{"ack": []string{"allow_unattended_release"}}), http.StatusForbidden, "flow_ack_forbidden")
	wantCode(t, rig.publish(nil, d, map[string]any{"ack": []string{"made_up"}}), http.StatusBadRequest, "flow_ack_unknown")

	wf := rig.publishOK(d, map[string]any{"ack": []string{"allow_unattended_release"}})
	var doc issueworkflow.FlowDoc
	if err := json.Unmarshal(wf.FlowDoc, &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.Policy.AllowUnattendedRelease || len(doc.Policy.Acks) != 1 || doc.Policy.Acks[0].By != testUserID {
		t.Fatalf("policy = %+v", doc.Policy)
	}
}

func TestPublish_NoGateWarningNeedsAck(t *testing.T) {
	rig := newDraftRig(t, 1)
	doc := rig.publishableDoc(rig.agents[0])
	// The person's task is work, not a gate: nothing is checked independently.
	asMaps(asMaps(doc["steps"])[2]["tasks"])[0]["kind"] = "work"
	d := rig.draftWith("Ungated", doc)

	body := wantCode(t, rig.publish(nil, d, nil), http.StatusUnprocessableEntity, "flow_ack_required")
	if required, _ := body["required"].([]any); len(required) != 1 || required[0] != issueworkflow.AckNoGate {
		t.Fatalf("required = %v", body["required"])
	}
	rig.publishOK(d, map[string]any{"ack": []string{"no_gate"}})
}

func TestPublish_StaleBase409(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.draftWith("Stale", rig.publishableDoc(rig.agents[0]))
	v1 := rig.publishOK(d, nil)

	// Two drafts are made from v1; the first publishes v2.
	fork := func(name string) FlowDraftResponse {
		var f FlowDraftResponse
		rig.createDraft(map[string]any{"name": name, "from": map[string]any{"kind": "workflow", "workflow_id": v1.ID, "keep_bind": true}}).
			Want(http.StatusCreated).JSON(&f)
		return f
	}
	a, b := fork("Stale A"), fork("Stale B")
	if a.BaseWorkflowID == nil || *a.BaseWorkflowID != v1.ID || a.LineageID == nil || *a.LineageID != *v1.LineageID || a.Origin != "fork" {
		t.Fatalf("fork = %+v", a)
	}
	v2 := rig.publishOK(a, nil)
	if v2.Version == nil || *v2.Version != 2 {
		t.Fatalf("v2 = %+v", v2)
	}
	body := wantCode(t, rig.publish(nil, b, nil), http.StatusConflict, "flow_draft_stale")
	if body["latest_workflow_id"] != v2.ID {
		t.Fatalf("latest_workflow_id = %v, want %s", body["latest_workflow_id"], v2.ID)
	}
}

func TestPublish_NameTaken409(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.draftWith("Taken", rig.publishableDoc(rig.agents[0]))
	other := dbfx.Insert(t, "issue_workflow", testutil.Cols{
		"workspace_id": testWorkspaceID, "name": "Taken · v1", "initial_status_key": "todo", "steps": testutil.Raw(`'[]'::jsonb`),
	})
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue_workflow WHERE id = $1`, other) })
	wantCode(t, rig.publish(nil, d, nil), http.StatusConflict, "workflow_name_taken")
}

func TestPublish_DoesNotInheritFlowAuto(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.draftWith("Auto", rig.publishableDoc(rig.agents[0]))
	v1 := rig.publishOK(d, nil)
	dbfx.Exec(t, `UPDATE issue_workflow SET flow_auto = TRUE WHERE id = $1`, v1.ID)

	d = rig.getDraft(d.ID)
	v2 := rig.publishOK(d, nil)
	if v2.FlowAuto {
		t.Fatal("the new version came out with flow_auto on")
	}
	var auto bool
	dbfx.QueryRow(t, `SELECT flow_auto FROM issue_workflow WHERE id = $1`, v1.ID).Scan(&auto)
	if !auto {
		t.Fatal("publishing a version switched the first one's flow_auto off")
	}
}

func TestPublish_LongNameIsCut(t *testing.T) {
	rig := newDraftRig(t, 1)
	name := strings.Repeat("é", issueworkflow.MaxDraftNameRunes)
	d := rig.draftWith(name, rig.publishableDoc(rig.agents[0]))
	wf := rig.publishOK(d, nil)
	if got := len([]rune(wf.Name)); got > issueworkflow.MaxNameRunes || !strings.HasSuffix(wf.Name, " · v1") {
		t.Fatalf("name %q has %d runes", wf.Name, got)
	}
}

func TestPublish_FlagOffLeavesVersionsAndRequestsAlone(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.draftWith("Flagged", rig.publishableDoc(rig.agents[0]))
	v1 := rig.publishOK(d, nil)
	rig.setActive(v1.ID)
	issueID := dbfx.Issue(t, "Flagged request "+workflowTestSuffix(), testutil.Cols{
		"status": "todo", "assignee_type": "squad", "assignee_id": rig.squadID,
	})
	if _, err := testHandler.EnsureFlowStarted(context.Background(), loadIssueRow(t, issueID)); err != nil {
		t.Fatal(err)
	}

	withFlowFlags(t, false, false)
	wantCode(t, rig.publish(nil, d, nil), http.StatusNotFound, "flow_authoring_disabled")
	// The version is still there and the request still reads its state from it.
	var got IssueWorkflowResponse
	req := withURLParam(workflowRequest("GET", "/api/issue-workflows/"+v1.ID, nil), "id", v1.ID)
	testutil.Call(t, testHandler.GetIssueWorkflow, req).Want(http.StatusOK).JSON(&got)
	if got.Version == nil || *got.Version != 1 {
		t.Fatalf("version = %+v", got)
	}
	state, err := testHandler.flowStateResponse(context.Background(), loadIssueRow(t, issueID), issueworkflow.RoleRequest)
	if err != nil || state.WorkflowID != v1.ID {
		t.Fatalf("state = %+v, err %v", state, err)
	}
}

// ---------------------------------------------------------------------- locks

func TestUpdateIssueWorkflow_PublishedLocked409(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.draftWith("Locked", rig.publishableDoc(rig.agents[0]))
	v1 := rig.publishOK(d, nil)
	before := workflowFlowConfig(t, v1.ID)

	req := withURLParam(workflowRequest("PATCH", "/api/issue-workflows/"+v1.ID, map[string]any{"name": "Renamed"}), "id", v1.ID)
	wantCode(t, testutil.Call(t, testHandler.UpdateIssueWorkflow, req), http.StatusConflict, "workflow_locked")
	var name string
	dbfx.QueryRow(t, `SELECT name FROM issue_workflow WHERE id = $1`, v1.ID).Scan(&name)
	if name != v1.Name || workflowFlowConfig(t, v1.ID) != before {
		t.Fatalf("the locked row changed: %q", name)
	}
}

func TestSetHandoff_PublishedLocked409(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.draftWith("Locked handoff", rig.publishableDoc(rig.agents[0]))
	v1 := rig.publishOK(d, nil)
	req := withURLParams(workflowRequest("PUT", "/api/issue-workflows/"+v1.ID+"/handoffs/todo", map[string]any{
		"handler": map[string]any{"type": "agent", "id": rig.agents[0]},
	}), "id", v1.ID, "status", "todo")
	wantCode(t, testutil.Call(t, testHandler.SetIssueWorkflowHandoff, req), http.StatusConflict, "workflow_locked")
}

func TestCatalogRow_KeepsManagedByTemplate409(t *testing.T) {
	rig := newDraftRig(t, 2)
	body := flowGuardWorkflowBody("Catalog "+workflowTestSuffix(), rig.agents[0], rig.agents[1])
	body["template_ref"] = "flow_kv_demo@aaaa1111"
	wf := createFlowWorkflowForTest(t, body)
	req := withURLParam(workflowRequest("PATCH", "/api/issue-workflows/"+wf.ID, map[string]any{"name": "Renamed " + workflowTestSuffix()}), "id", wf.ID)
	wantCode(t, testutil.Call(t, testHandler.UpdateIssueWorkflow, req), http.StatusConflict, "workflow_managed_by_template")
}

// ------------------------------------------------------------ CreateIssueWorkflow

func TestCreateIssueWorkflow_SchemaOneWithLeaderTask422(t *testing.T) {
	rig := newDraftRig(t, 2)
	lead := squadLeadID(t, rig.squadID)
	body := flowGuardWorkflowBody("Leader task "+workflowTestSuffix(), lead, rig.agents[1])
	body["squad_id"] = rig.squadID
	resp := testutil.Call(t, testHandler.CreateIssueWorkflow, workflowRequest("POST", "/api/issue-workflows", body))
	out := wantCode(t, resp, http.StatusUnprocessableEntity, "flow_workflow_invalid")
	issues, _ := out["issues"].([]any)
	found := false
	for _, it := range issues {
		if it.(map[string]any)["code"] == "role_is_lead" {
			found = true
		}
	}
	if !found {
		t.Fatalf("issues = %v, want role_is_lead", issues)
	}

	// A version 2 document does not go through this route.
	v2 := map[string]any{"name": "Doc " + workflowTestSuffix(), "flow_config": map[string]any{"version": 2}, "squad_id": rig.squadID}
	wantCode(t, testutil.Call(t, testHandler.CreateIssueWorkflow, workflowRequest("POST", "/api/issue-workflows", v2)), http.StatusBadRequest, "flow_schema_unsupported")

	// Without the lead, with every worker used, the same flow is stored as the squad's.
	ok := flowGuardWorkflowBody("Fits the squad "+workflowTestSuffix(), rig.agents[0], rig.agents[1])
	ok["squad_id"] = rig.squadID
	var wf IssueWorkflowResponse
	testutil.Call(t, testHandler.CreateIssueWorkflow, workflowRequest("POST", "/api/issue-workflows", ok)).Want(http.StatusCreated).JSON(&wf)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue_workflow WHERE id = $1`, wf.ID) })
	if wf.SquadID == nil || *wf.SquadID != rig.squadID || wf.Version != nil || wf.PublishedAt != nil {
		t.Fatalf("created row = %+v", wf)
	}
}

func squadLeadID(t *testing.T, squadID string) string {
	t.Helper()
	var id string
	dbfx.QueryRow(t, `SELECT leader_id::text FROM squad WHERE id = $1`, squadID).Scan(&id)
	return id
}

// ----------------------------------------------------------------------- forks

func TestFork_FromSchemaOneRowGivesCleanV2Draft(t *testing.T) {
	rig := newDraftRig(t, 2)
	body := flowGuardWorkflowBody("Old flow "+workflowTestSuffix(), rig.agents[0], rig.agents[1])
	body["squad_id"] = rig.squadID
	row := createFlowWorkflowForTest(t, body)

	var d FlowDraftResponse
	rig.createDraft(map[string]any{"from": map[string]any{"kind": "workflow", "workflow_id": row.ID}}).Want(http.StatusCreated).JSON(&d)
	if d.Origin != "fork" || d.BaseWorkflowID == nil || *d.BaseWorkflowID != row.ID {
		t.Fatalf("draft = %+v", d)
	}
	doc, issues := issueworkflow.ParseFlowDoc(d.Doc)
	if len(issues) > 0 || doc.Version != issueworkflow.FlowDocVersion {
		t.Fatalf("the fork is not a document of schema 2: %v", issues)
	}
	bound := map[string]bool{}
	for _, role := range doc.Roles {
		if role.Kind != issueworkflow.RoleKindAgent || role.Bind == nil || len(role.Bind.AgentIDs) != 1 {
			t.Fatalf("role %+v is not an agent bound to one agent", role)
		}
		id := role.Bind.AgentIDs[0]
		bound[id] = true
		if want := "a_" + strings.ReplaceAll(id, "-", "")[:8]; role.Key != want {
			t.Fatalf("role key %q, want %q", role.Key, want)
		}
		var name string
		dbfx.QueryRow(t, `SELECT name FROM agent WHERE id = $1`, id).Scan(&name)
		if role.Label != name {
			t.Fatalf("role label %q, want the agent's name %q", role.Label, name)
		}
	}
	if !bound[rig.agents[0]] || !bound[rig.agents[1]] {
		t.Fatalf("the roles bind %v, want both agents", bound)
	}
	// The doc has nothing a schema 1 row would carry that schema 2 does not know.
	if strings.Contains(string(d.Doc), "template_ref") {
		t.Fatalf("the fork refers to a template: %s", d.Doc)
	}
}

func TestFork_ForkNeverReceivesKitVietUpgrade(t *testing.T) {
	rig := newDraftRig(t, 2)
	// A row a catalog flow made. The upgrade of a Kit Viet squad upgrades only a workflow whose
	// template_ref starts with the flow's id and "@"; a fork must not match that.
	body := flowGuardWorkflowBody("Kit Viet flow "+workflowTestSuffix(), rig.agents[0], rig.agents[1])
	body["template_ref"] = "flow_kv_demo@aaaa1111"
	row := createFlowWorkflowForTest(t, body)
	if row.TemplateRef == nil {
		t.Fatalf("the catalog row has no template_ref: %+v", row)
	}

	var d FlowDraftResponse
	rig.createDraft(map[string]any{"name": "My copy", "from": map[string]any{"kind": "workflow", "workflow_id": row.ID}}).Want(http.StatusCreated).JSON(&d)
	// The bound agents are in the squad's roster for the fork to publish.
	wf := rig.publishOK(d, map[string]any{"ack": []string{"allow_unattended_release"}})
	if wf.TemplateRef != nil {
		t.Fatalf("the fork has template_ref %q", *wf.TemplateRef)
	}
	var ref *string
	dbfx.QueryRow(t, `SELECT template_ref FROM issue_workflow WHERE id = $1`, wf.ID).Scan(&ref)
	if ref != nil {
		t.Fatalf("stored template_ref = %q", *ref)
	}
	if strings.Contains(string(wf.FlowDoc), "flow_kv_demo") {
		t.Fatalf("the document of the fork names the template: %s", wf.FlowDoc)
	}
}

// ---------------------------------------------------------------- compatibility

// K11: an old client reads a row of schema 2 through its old schema and shows the projection.
func TestLegacyProjection_OldGraphSheetShapeUnchanged(t *testing.T) {
	rig := newDraftRig(t, 2)
	d := rig.draftWith("Projected", rig.publishableDoc(rig.agents[0]))
	v1 := rig.publishOK(d, nil)

	req := withURLParam(workflowRequest("GET", "/api/issue-workflows/"+v1.ID, nil), "id", v1.ID)
	body := testutil.Call(t, testHandler.GetIssueWorkflow, req).Want(http.StatusOK).Map()
	steps, _ := body["steps"].([]any)
	if len(steps) == 0 {
		t.Fatalf("no steps: %v", body)
	}
	// Every step has the keys the old zod schema reads, with the types it reads.
	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		if _, ok := step["status_key"].(string); !ok {
			t.Fatalf("step has no status_key: %v", step)
		}
		if _, ok := step["handler"].(map[string]any); !ok {
			t.Fatalf("step has no handler: %v", step)
		}
		if _, ok := step["transitions"].([]any); !ok {
			t.Fatalf("step has no transitions array: %v", step)
		}
	}
	config, _ := body["flow_config"].(map[string]any)
	if config == nil || config["version"].(float64) != 2 {
		t.Fatalf("flow_config = %v", body["flow_config"])
	}
	for _, key := range []string{"template_ref", "flow_auto", "open_flow_requests", "lineage_id", "version", "published_at", "published_by", "flow_doc"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("the response lacks %s", key)
		}
	}

	// A row of schema 1 keeps the shape it had, and carries no document.
	wf := createFlowWorkflowForTest(t, flowGuardWorkflowBody("Shape "+workflowTestSuffix(), rig.agents[0], rig.agents[1]))
	oldBody := testutil.Call(t, testHandler.GetIssueWorkflow, withURLParam(workflowRequest("GET", "/api/issue-workflows/"+wf.ID, nil), "id", wf.ID)).Want(http.StatusOK).Map()
	if _, ok := oldBody["flow_doc"]; ok {
		t.Fatalf("a row of schema 1 carries a document: %v", oldBody["flow_doc"])
	}
	if oldBody["version"] != nil || oldBody["published_at"] != nil || oldBody["lineage_id"] != nil {
		t.Fatalf("a row of schema 1 reports a version: %v", oldBody)
	}
}

// C18: publishing and locking leave a workflow that is not a flow as it was. The goldens
// themselves are TestWorkflowGolden_SDLCAutoWrites and TestWorkflowGolden_PlanningAutoWrites.
func TestWorkflowGolden_StillGreen(t *testing.T) {
	seedTestCatalog(t)
	wf := createTestWorkflow(t, "Plain "+workflowTestSuffix(), "in_progress")
	if wf.Version != nil || wf.LineageID != nil || wf.PublishedAt != nil || wf.PublishedBy != nil || len(wf.FlowDoc) != 0 {
		t.Fatalf("a plain workflow carries version fields: %+v", wf)
	}
	renamed := "Plain renamed " + workflowTestSuffix()
	var updated IssueWorkflowResponse
	req := withURLParam(workflowRequest("PATCH", "/api/issue-workflows/"+wf.ID, map[string]any{"name": renamed}), "id", wf.ID)
	testutil.Call(t, testHandler.UpdateIssueWorkflow, req).Want(http.StatusOK).JSON(&updated)
	if updated.Name != renamed {
		t.Fatalf("name = %q, want %q", updated.Name, renamed)
	}
}
