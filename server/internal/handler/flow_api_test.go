package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The flow HTTP API (S8 of docs/engineering/kitviet-workflow-plan.md).

// flowCaller sends a flow request as one of the people the permission table
// names: the member who manages the squad ("manager"), a member who does not
// ("plain"), the squad's lead agent ("lead"), and another agent ("other").
type flowCaller struct {
	t         *testing.T
	f         tradeFlowFixture
	plain     func(*http.Request) *http.Request
	otherID   string
	requestID string
}

func newFlowCaller(t *testing.T, f tradeFlowFixture, request db.Issue) flowCaller {
	t.Helper()
	_, asPlain := addWorkflowTestMember(t, "flow-api-plain")
	return flowCaller{t: t, f: f, plain: asPlain, otherID: f.Agents.Drafter, requestID: uuidToString(request.ID)}
}

func (c flowCaller) call(who string, h http.HandlerFunc, method, suffix string, body any, params ...string) *testutil.Response {
	c.t.Helper()
	req := newRequest(method, "/api/issues/"+c.requestID+"/flow"+suffix, body)
	req = withURLParams(req, append([]string{"id", c.requestID}, params...)...)
	switch who {
	case "plain":
		req = c.plain(req)
	case "lead", "other":
		var agentID string
		dbfx.QueryRow(c.t, `SELECT leader_id::text FROM squad WHERE id = $1`, c.f.SquadID).Scan(&agentID)
		if who == "other" {
			agentID = c.otherID
		}
		runID := goldenRun(c.t, c.requestID, agentID, "todo")
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", runID)
	}
	return testutil.Call(c.t, h, req)
}

func TestFlowAPI_Permissions(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	c := newFlowCaller(t, f, request)

	cases := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		suffix  string
		body    any
		params  []string
		allowed map[string]int // who -> expected status; anyone else is refused with flow_forbidden
	}{
		{name: "read", handler: testHandler.GetIssueFlow, method: "GET", suffix: "",
			allowed: map[string]int{"manager": 200, "plain": 200, "lead": 200, "other": 200}},
		{name: "plan", handler: testHandler.PlanIssueFlow, method: "POST", suffix: "/plan", body: map[string]any{"switches": map[string]any{}},
			allowed: map[string]int{"manager": 200, "lead": 200}},
		{name: "advance", handler: testHandler.AdvanceIssueFlow, method: "POST", suffix: "/advance", body: map[string]any{},
			allowed: map[string]int{"manager": 200, "lead": 200}},
		{name: "switch", handler: testHandler.SetIssueFlowSwitch, method: "PUT", suffix: "/switches/export", body: map[string]any{"value": true}, params: []string{"key", "export"},
			allowed: map[string]int{"manager": 200, "lead": 200}},
		{name: "resume", handler: testHandler.ResumeIssueFlow, method: "POST", suffix: "/resume", body: map[string]any{},
			allowed: map[string]int{"manager": 409}}, // allowed, but the request is not stopped
		{name: "detach", handler: testHandler.DetachIssueFlow, method: "POST", suffix: "/detach", body: map[string]any{"cancel_tasks": false},
			allowed: map[string]int{"manager": 200}},
	}
	// Every call gets a request still at the plan step: a plan is admitted once.
	for _, tc := range cases {
		for _, who := range []string{"plain", "other", "lead", "manager"} {
			c.requestID = uuidToString(f.newRequest(t).ID)
			status, allowedHere := tc.allowed[who]
			if !allowedHere {
				body := c.call(who, tc.handler, tc.method, tc.suffix, tc.body, tc.params...).Want(http.StatusForbidden).Map()
				if body["code"] != "flow_forbidden" {
					t.Errorf("%s as %s: code = %v, want flow_forbidden", tc.name, who, body["code"])
				}
				continue
			}
			c.call(who, tc.handler, tc.method, tc.suffix, tc.body, tc.params...).Want(status)
		}
	}
}

func TestFlowAPI_StateShape(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	c := newFlowCaller(t, f, request)
	c.call("lead", testHandler.PlanIssueFlow, "POST", "/plan", map[string]any{"switches": map[string]any{"customs_lookup": true}}).Want(http.StatusOK)

	var state IssueFlowStateResponse
	c.call("manager", testHandler.GetIssueFlow, "GET", "", nil).Want(http.StatusOK).JSON(&state)
	if !state.Managed || state.Role != issueworkflow.RoleRequest || state.WorkflowID != f.Workflow.ID || state.WorkflowName != f.Workflow.Name {
		t.Fatalf("state = %+v", state)
	}
	if state.Step != "collect" || state.Rework != 0 || stopOf(state) != issueworkflow.StopNone || state.EnteredAt == "" || state.Auto {
		t.Fatalf("state = %+v", state)
	}
	if state.Switches["customs_lookup"] != true || state.Switches["route"] != "shipment_dossier" || state.Switches["export"] != false {
		t.Fatalf("switches = %v", state.Switches)
	}
	if len(state.Tasks) != 2 {
		t.Fatalf("tasks = %+v, want docs and customs", state.Tasks)
	}
	for _, task := range state.Tasks {
		if task.IssueID == "" || task.Identifier == "" || task.Step != "collect" || task.Round != 0 || task.Status != "todo" || task.Outcome != nil || task.ClosedAt != nil {
			t.Fatalf("task = %+v", task)
		}
	}
	if len(state.Next) != 2 || state.Next[0].To != "blocked" || state.Next[1].To != "draft" || state.Next[1].Rework {
		t.Fatalf("next = %+v", state.Next)
	}

	// Asked about a task, it answers with the request's state and the role task.
	task := tasksAt(t, request, "collect", 0)["docs"]
	var asTask IssueFlowStateResponse
	req := withURLParam(newRequest("GET", "/api/issues/"+task+"/flow", nil), "id", task)
	testutil.Call(t, testHandler.GetIssueFlow, req).Want(http.StatusOK).JSON(&asTask)
	if asTask.Role != issueworkflow.RoleTask || asTask.Step != "collect" || asTask.WorkflowID != f.Workflow.ID {
		t.Fatalf("state as asked of a task = %+v", asTask)
	}

	var advanced FlowAdvanceResponse
	c.call("lead", testHandler.AdvanceIssueFlow, "POST", "/advance", map[string]any{}).Want(http.StatusOK).JSON(&advanced)
	if len(advanced.Actions) != 1 || advanced.Actions[0].Kind != "noop" || len(advanced.Pending) != 2 || advanced.State.Step != "collect" {
		t.Fatalf("advance = %+v", advanced)
	}
}

func TestFlowAPI_NotManaged404(t *testing.T) {
	seedTestCatalog(t)
	plain := dbfx.Issue(t, "not a flow "+workflowTestSuffix())
	body := testutil.Call(t, testHandler.GetIssueFlow, withURLParam(newRequest("GET", "/api/issues/"+plain+"/flow", nil), "id", plain)).
		Want(http.StatusNotFound).Map()
	if body["code"] != "flow_not_managed" {
		t.Fatalf("code = %v, want flow_not_managed", body["code"])
	}
	for name, h := range map[string]http.HandlerFunc{
		"plan": testHandler.PlanIssueFlow, "advance": testHandler.AdvanceIssueFlow,
		"resume": testHandler.ResumeIssueFlow, "detach": testHandler.DetachIssueFlow,
	} {
		got := testutil.Call(t, h, withURLParam(newRequest("POST", "/api/issues/"+plain+"/flow/"+name, map[string]any{}), "id", plain)).
			Want(http.StatusNotFound).Map()
		if got["code"] != "flow_not_managed" {
			t.Errorf("%s: code = %v, want flow_not_managed", name, got["code"])
		}
	}
	// With the engine off a flow request reads as not managed.
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	withFeatureFlag(t, testHandler, "flow_engine", false)
	got := testutil.Call(t, testHandler.GetIssueFlow, withURLParam(newRequest("GET", "/api/issues/"+uuidToString(request.ID)+"/flow", nil), "id", uuidToString(request.ID))).
		Want(http.StatusNotFound).Map()
	if got["code"] != "flow_not_managed" {
		t.Fatalf("engine off: code = %v, want flow_not_managed", got["code"])
	}
}

func TestFlowAPI_SwitchLocked409(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	c := newFlowCaller(t, f, request)
	c.call("lead", testHandler.PlanIssueFlow, "POST", "/plan", map[string]any{"switches": map[string]any{}}).Want(http.StatusOK)

	body := c.call("lead", testHandler.SetIssueFlowSwitch, "PUT", "/switches/export", map[string]any{"value": true}, "key", "export").
		Want(http.StatusConflict).Map()
	if body["code"] != "flow_switch_locked" || body["step"] != "collect" {
		t.Fatalf("lead after the plan: %v, want flow_switch_locked at collect", body)
	}
	// A member who manages the squad may, and it is logged.
	c.call("manager", testHandler.SetIssueFlowSwitch, "PUT", "/switches/export", map[string]any{"value": true}, "key", "export").Want(http.StatusOK)
	if n := dbfx.Count(t, `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'flow_switch_set'`, request.ID); n != 1 {
		t.Fatalf("flow_switch_set entries = %d, want 1", n)
	}
	// A value the flow does not declare is refused with what is allowed.
	bad := c.call("manager", testHandler.SetIssueFlowSwitch, "PUT", "/switches/route", map[string]any{"value": "nowhere"}, "key", "route").
		Want(http.StatusUnprocessableEntity).Map()
	if bad["code"] != "flow_switch_invalid" || bad["switch"] != "route" {
		t.Fatalf("invalid value: %v", bad)
	}
	unknown := c.call("manager", testHandler.SetIssueFlowSwitch, "PUT", "/switches/missing", map[string]any{"value": true}, "key", "missing").
		Want(http.StatusUnprocessableEntity).Map()
	if unknown["code"] != "flow_switch_invalid" {
		t.Fatalf("unknown switch: %v", unknown)
	}
}

func TestFlowAPI_PlanOnlyAtPlanStep409(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	c := newFlowCaller(t, f, request)
	c.call("lead", testHandler.PlanIssueFlow, "POST", "/plan", map[string]any{}).Want(http.StatusOK)
	body := c.call("lead", testHandler.PlanIssueFlow, "POST", "/plan", map[string]any{}).Want(http.StatusConflict).Map()
	if body["code"] != "flow_wrong_step" || body["step"] != "collect" {
		t.Fatalf("second plan: %v, want flow_wrong_step at collect", body)
	}
	bad := f.newRequest(t)
	c.requestID = uuidToString(bad.ID)
	invalid := c.call("lead", testHandler.PlanIssueFlow, "POST", "/plan", map[string]any{"switches": map[string]any{"customs_lookup": "yes"}}).
		Want(http.StatusUnprocessableEntity).Map()
	if invalid["code"] != "flow_switch_invalid" {
		t.Fatalf("plan with a bad switch value: %v", invalid)
	}
}

func TestFlowAPI_PlanBlockedStopsAtOne(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	c := newFlowCaller(t, f, request)
	var state IssueFlowStateResponse
	c.call("lead", testHandler.PlanIssueFlow, "POST", "/plan", map[string]any{
		"blocked": map[string]any{"missing": []string{"invoice", "packing list"}, "reason": "no invoice"},
	}).Want(http.StatusOK).JSON(&state)
	if state.Step != "blocked" || stopOf(state) != issueworkflow.Stop1 {
		t.Fatalf("state = %+v, want blocked at stop 1", state)
	}
	var comment string
	dbfx.QueryRow(t, `SELECT content FROM comment WHERE issue_id = $1 ORDER BY created_at DESC LIMIT 1`, request.ID).Scan(&comment)
	if !containsAll(comment, "BLOCKED · stop 1", "invoice", "packing list", "mention://member/"+testUserID) {
		t.Fatalf("stop 1 comment = %q", comment)
	}
	if got := countTasks(t, request); got != 0 {
		t.Fatalf("tasks opened for a blocked plan: %d", got)
	}
}

func TestFlowAPI_StartRules(t *testing.T) {
	f := newTradeFlowFixture(t)
	c := newFlowCaller(t, f, db.Issue{})

	// An issue given to the squad that no path started yet starts on request.
	id := dbfx.Issue(t, "unstarted request "+workflowTestSuffix(), testutil.Cols{"assignee_type": "squad", "assignee_id": f.SquadID})
	c.requestID = id
	var state IssueFlowStateResponse
	c.call("manager", testHandler.StartIssueFlow, "POST", "/start", map[string]any{}).Want(http.StatusOK).JSON(&state)
	if state.Step != "todo" || state.WorkflowID != f.Workflow.ID {
		t.Fatalf("started state = %+v", state)
	}

	for name, cols := range map[string]testutil.Cols{
		"a sub-issue":      {"assignee_type": "squad", "assignee_id": f.SquadID, "parent_issue_id": id},
		"an agent's issue": {"assignee_type": "agent", "assignee_id": f.Agents.Docs},
	} {
		other := dbfx.Issue(t, "cannot start "+name+" "+workflowTestSuffix(), cols)
		c.requestID = other
		body := c.call("manager", testHandler.StartIssueFlow, "POST", "/start", map[string]any{}).Want(http.StatusConflict).Map()
		if body["code"] != "flow_cannot_start" {
			t.Errorf("%s: code = %v, want flow_cannot_start", name, body["code"])
		}
	}
	withChild := dbfx.Issue(t, "request with a child "+workflowTestSuffix(), testutil.Cols{"assignee_type": "squad", "assignee_id": f.SquadID})
	dbfx.Issue(t, "its child "+workflowTestSuffix(), testutil.Cols{"parent_issue_id": withChild})
	c.requestID = withChild
	c.call("manager", testHandler.StartIssueFlow, "POST", "/start", map[string]any{}).Want(http.StatusConflict)

	plainSquad := dbfx.Squad(t, "No flow squad "+workflowTestSuffix(), f.Agents.Docs)
	noFlow := dbfx.Issue(t, "request of a squad with no flow "+workflowTestSuffix(), testutil.Cols{"assignee_type": "squad", "assignee_id": plainSquad})
	c.requestID = noFlow
	c.call("manager", testHandler.StartIssueFlow, "POST", "/start", map[string]any{}).Want(http.StatusConflict)
}

func TestFlowAPI_FlowAutoAdminOnly(t *testing.T) {
	f := newTradeFlowFixture(t)
	_, asMember := addWorkflowTestMember(t, "flow-auto")
	put := func(as func(*http.Request) *http.Request, auto bool) *testutil.Response {
		req := withURLParam(newRequest("PUT", "/api/issue-workflows/"+f.Workflow.ID+"/flow-auto", map[string]any{"auto": auto}), "id", f.Workflow.ID)
		if as != nil {
			req = as(req)
		}
		return testutil.Call(t, testHandler.SetIssueWorkflowFlowAuto, req)
	}
	put(asMember, true).Want(http.StatusForbidden)
	var wf IssueWorkflowResponse
	put(nil, true).Want(http.StatusOK).JSON(&wf)
	if !wf.FlowAuto {
		t.Fatal("flow_auto did not turn on")
	}
	put(nil, false).Want(http.StatusOK).JSON(&wf)
	if wf.FlowAuto {
		t.Fatal("flow_auto did not turn off")
	}
	// A workflow with no flow has no runner.
	plain := createTestWorkflow(t, "Plain auto "+workflowTestSuffix(), "in_progress")
	req := withURLParam(newRequest("PUT", "/api/issue-workflows/"+plain.ID+"/flow-auto", map[string]any{"auto": true}), "id", plain.ID)
	testutil.Call(t, testHandler.SetIssueWorkflowFlowAuto, req).Want(http.StatusConflict)
}

// stopOf reads the stop of a state the way schema 1 numbered it: none, 1, 3 or stalled.
// The answer carries the stop as an object (spec 7.1); the tests of schema 1 flows speak the old numbers.
func stopOf(state IssueFlowStateResponse) issueworkflow.Stop {
	switch {
	case state.Stop == nil:
		return issueworkflow.StopNone
	case state.Stop.LegacyNo == 1:
		return issueworkflow.Stop1
	case state.Stop.LegacyNo == 3:
		return issueworkflow.Stop3
	}
	return issueworkflow.StopStalled
}

// Spec 16.4: an agent does not turn the runner of a workflow on or off, whoever it acts for.
func TestFlowAuto_AgentActorRefused(t *testing.T) {
	f := newTradeFlowFixture(t)
	agent := f.Agents.Docs
	taskID := createHandlerTestTaskForAgent(t, agent)
	req := withURLParam(newRequest("PUT", "/api/issue-workflows/"+f.Workflow.ID+"/flow-auto", map[string]any{"auto": true}), "id", f.Workflow.ID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agent)
	req.Header.Set("X-Task-ID", taskID)
	body := testutil.Call(t, testHandler.SetIssueWorkflowFlowAuto, req).Want(http.StatusForbidden).Map()
	if body["code"] != "flow_forbidden" || body["error"] != "only workspace admins can turn the runner on, not an agent" {
		t.Fatalf("answer = %v", body)
	}
	var auto bool
	dbfx.QueryRow(t, `SELECT flow_auto FROM issue_workflow WHERE id = $1`, f.Workflow.ID).Scan(&auto)
	if auto {
		t.Fatal("an agent turned flow_auto on")
	}
}
