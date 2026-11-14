package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// The operating numbers of one version of a flow (plan P26, spec 16.6, criterion N10).

// insightDoc is the publishable flow with a loop: a failed check sends the request back to the write
// step twice before it stops.
func (rig *draftRig) insightDoc() map[string]any {
	doc := rig.publishableDoc(rig.agents[0])
	doc["loops"] = []any{map[string]any{"key": "redo", "header": "write", "max": 2}}
	check := asMaps(doc["steps"])[2]
	edges := asMaps(check["edges"])
	back := docEdge("c_redo", "write", map[string]any{"all": []any{joinIs("fail"), map[string]any{"field": "loop.redo.count", "op": "lt", "value": 2}}})
	back["loop"] = "redo"
	check["edges"] = append([]any{edges[0], back}, toAnys(edges[1:])...)
	return doc
}

func toAnys(maps []map[string]any) []any {
	out := make([]any, len(maps))
	for i, m := range maps {
		out[i] = m
	}
	return out
}

// request makes a request pinned to a version, with the state given, and returns its id.
func (rig *draftRig) request(workflowID string, state issueworkflow.State, age time.Duration) string {
	rig.t.Helper()
	state.WorkflowID = workflowID
	state.Version = issueworkflow.FlowDocVersion
	raw, err := json.Marshal(state)
	if err != nil {
		rig.t.Fatal(err)
	}
	return dbfx.Issue(rig.t, "insight request "+workflowTestSuffix(), testutil.Cols{
		"status": "todo", "flow": testutil.Raw(fmt.Sprintf(`'%s'::jsonb`, string(raw))), "created_at": time.Now().Add(-age),
	})
}

func (rig *draftRig) insights(as func(*http.Request) *http.Request, workflowID, query string) *testutil.Response {
	rig.t.Helper()
	req := newRequest("GET", "/api/issue-workflows/"+workflowID+"/flow-insights"+query, nil)
	if as != nil {
		req = as(req)
	}
	return testutil.Call(rig.t, testHandler.GetFlowInsights, withURLParam(req, "id", workflowID))
}

func (rig *draftRig) report(workflowID, query string) issueworkflow.Report {
	rig.t.Helper()
	var report issueworkflow.Report
	rig.insights(nil, workflowID, query).Want(http.StatusOK).JSON(&report)
	return report
}

func closedState(trail ...issueworkflow.TrailEntry) issueworkflow.State {
	return issueworkflow.State{Step: "done", ClosedAt: insightTime(0), Trail: trail}
}

func insightTime(minutes int) string {
	return time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC).Add(time.Duration(minutes) * time.Minute).Format(time.RFC3339)
}

func leftAt(step, edge string, minutes int) issueworkflow.TrailEntry {
	return issueworkflow.TrailEntry{Step: step, Activation: 1, Edge: edge, At: insightTime(minutes)}
}

func TestFlowInsights_OnlyThisVersion_TrialExcludedByDefault(t *testing.T) {
	rig := newDraftRig(t, 1)
	v1 := rig.publishOK(rig.draftWith("Insight one", rig.insightDoc()), nil)
	v2 := rig.publishOK(rig.draftWith("Insight two", rig.publishableDoc(rig.agents[0])), nil)

	done := rig.request(v1.ID, closedState(leftAt("write", "w_ok", 0), leftAt("check", "c_pass", 30)), time.Hour)
	rig.request(v1.ID, issueworkflow.State{Step: "check", Loops: map[string]int{"redo": 1}, Trail: []issueworkflow.TrailEntry{leftAt("write", "w_ok", 0), leftAt("check", "c_redo", 20), leftAt("write", "w_ok", 25)}}, 2*time.Hour)
	rig.request(v1.ID, issueworkflow.State{Step: "check", Trial: true, Stop: &issueworkflow.StopRecord{Code: "stalled", From: "check", Reason: "flow_agent_unavailable"}, Trail: []issueworkflow.TrailEntry{leftAt("write", "w_ok", 0)}}, 3*time.Hour)
	rig.request(v2.ID, closedState(leftAt("write", "w_ok", 0)), time.Hour)

	// A person closed a task for its assignee: the log of the task counts it for the version.
	task := dbfx.Issue(t, "insight task "+workflowTestSuffix(), testutil.Cols{"parent_issue_id": done, "flow_step": "write", "flow_task": "draft", "flow_round": 1})
	dbfx.Exec(t, `INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
		VALUES ($1, $2, 'member', $3, 'flow_manual_close', '{}')`, testWorkspaceID, task, testUserID)

	// By default a trial request is not counted.
	report := rig.report(v1.ID, "")
	if report.Requests.Total != 2 || report.Requests.Open != 1 || report.Requests.ByEnd["terminal:done"] != 1 || len(report.Requests.ByEnd) != 1 {
		t.Fatalf("requests = %+v, want 2 (1 open, 1 done) without the trial and without the other version", report.Requests)
	}
	if report.Truncated || report.ManualCloses != 1 || len(report.Stalls) != 0 {
		t.Fatalf("truncated %v, manual closes %d, stalls %+v", report.Truncated, report.ManualCloses, report.Stalls)
	}
	steps := map[string]issueworkflow.StepInsight{}
	for _, s := range report.Steps {
		steps[s.Key] = s
	}
	if steps["check"].Entered != 3 || steps["write"].Entered != 3 {
		t.Fatalf("steps = %+v", report.Steps)
	}
	// The loop of the definition is in the report, with its bound.
	if len(report.Loops) != 1 || report.Loops[0].Key != "redo" || report.Loops[0].Max != 2 || len(report.Loops[0].Histogram) != 3 || report.Loops[0].Histogram[1] != 1 || report.Loops[0].Histogram[0] != 1 {
		t.Fatalf("loops = %+v", report.Loops)
	}

	// Asked for, the trial request is counted and its stall with it.
	with := rig.report(v1.ID, "?include_trial=true")
	if with.Requests.Total != 3 || with.Requests.ByEnd["stalled"] != 1 || len(with.Stalls) != 1 || with.Stalls[0].Reason != "flow_agent_unavailable" {
		t.Fatalf("with trials = %+v stalls %+v", with.Requests, with.Stalls)
	}

	// The other version has its own numbers: one request, and no loop it does not declare.
	other := rig.report(v2.ID, "")
	if other.Requests.Total != 1 || other.ManualCloses != 0 || len(other.Loops) != 0 {
		t.Fatalf("version two = %+v", other)
	}
	// A workflow that is not a flow of this workspace is not found.
	wantCode(t, rig.insights(nil, "00000000-0000-4000-8000-000000000001", ""), http.StatusNotFound, "workflow_not_found")
}

func TestFlowInsights_NotManager403(t *testing.T) {
	rig := newDraftRig(t, 1)
	v1 := rig.publishOK(rig.draftWith("Insight manager", rig.publishableDoc(rig.agents[0])), nil)
	rig.request(v1.ID, closedState(leftAt("write", "w_ok", 0)), time.Hour)

	_, asOther := addWorkflowTestMember(t, "insightother")
	wantCode(t, rig.insights(asOther, v1.ID, ""), http.StatusForbidden, "flow_forbidden")

	// The member who made the squad manages it, and reads.
	asManager := rig.managerMember("insightmgr")
	var report issueworkflow.Report
	rig.insights(asManager, v1.ID, "").Want(http.StatusOK).JSON(&report)
	if report.Requests.Total != 1 {
		t.Fatalf("report = %+v", report.Requests)
	}
}

func TestFlowInsights_AgentActor403(t *testing.T) {
	rig := newDraftRig(t, 1)
	v1 := rig.publishOK(rig.draftWith("Insight agent", rig.publishableDoc(rig.agents[0])), nil)
	agent := rig.agents[0]
	taskID := createHandlerTestTaskForAgent(t, agent)
	asAgent := func(r *http.Request) *http.Request {
		r.Header.Set("X-Actor-Source", "task_token")
		r.Header.Set("X-Agent-ID", agent)
		r.Header.Set("X-Task-ID", taskID)
		return r
	}
	// The agent carries the credentials of a manager and is still refused.
	wantCode(t, rig.insights(asAgent, v1.ID, ""), http.StatusForbidden, "flow_forbidden")
}

func TestFlowInsights_Truncated(t *testing.T) {
	rig := newDraftRig(t, 1)
	v1 := rig.publishOK(rig.draftWith("Insight cut", rig.publishableDoc(rig.agents[0])), nil)
	// The oldest request is the one that is left out.
	rig.request(v1.ID, closedState(leftAt("write", "w_ok", 0)), 48*time.Hour)
	for i := 0; i < 500; i++ {
		rig.request(v1.ID, issueworkflow.State{Step: "write"}, time.Duration(i+1)*time.Minute)
	}
	report := rig.report(v1.ID, "")
	if !report.Truncated || report.Requests.Total != 500 || report.Requests.Open != 500 || len(report.Requests.ByEnd) != 0 {
		t.Fatalf("truncated %v, requests %+v: want the newest 500 and the oldest left out", report.Truncated, report.Requests)
	}
	// Exactly 500 is not a cut.
	dbfx.Exec(t, `DELETE FROM issue WHERE workspace_id = $1 AND flow->>'workflow_id' = $2 AND flow ? 'closed_at'`, testWorkspaceID, v1.ID)
	if again := rig.report(v1.ID, ""); again.Truncated || again.Requests.Total != 500 {
		t.Fatalf("truncated %v, total %d with exactly 500 matching", again.Truncated, again.Requests.Total)
	}
}

func TestFlowInsights_FlagOff404(t *testing.T) {
	rig := newDraftRig(t, 1)
	v1 := rig.publishOK(rig.draftWith("Insight flag", rig.publishableDoc(rig.agents[0])), nil)
	var caps FlowCapabilitiesResponse
	testutil.Call(t, testHandler.GetFlowCapabilities, newRequest("GET", "/api/flow/capabilities", nil)).Want(http.StatusOK).JSON(&caps)
	if !caps.Features.Insights {
		t.Fatalf("features = %+v, want insights with authoring on", caps.Features)
	}

	withFlowFlags(t, false, true)
	wantCode(t, rig.insights(nil, v1.ID, ""), http.StatusNotFound, "flow_authoring_disabled")
	caps = FlowCapabilitiesResponse{}
	testutil.Call(t, testHandler.GetFlowCapabilities, newRequest("GET", "/api/flow/capabilities", nil)).Want(http.StatusOK).JSON(&caps)
	if caps.Features.Insights {
		t.Fatalf("features = %+v, want insights off with authoring off", caps.Features)
	}
}
