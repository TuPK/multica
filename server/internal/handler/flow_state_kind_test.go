package handler

import (
	"net/http"
	"reflect"
	"sort"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// The web client shows verdict controls only on an open gate, so the task kind
// in the flow state has to match the definition the engine runs.
func TestFlowTaskKind(t *testing.T) {
	def := issueworkflow.Definition{Steps: []issueworkflow.Step{
		{StatusKey: "collect", Tasks: []issueworkflow.Task{{Key: "docs", Kind: issueworkflow.KindWork}}},
		{StatusKey: "gate", Tasks: []issueworkflow.Task{{Key: "docs_gate", Kind: issueworkflow.KindGate}}},
	}}
	cases := []struct {
		name string
		step string
		key  string
		want string
	}{
		{name: "work task of its step", step: "collect", key: "docs", want: issueworkflow.KindWork},
		{name: "gate task of its step", step: "gate", key: "docs_gate", want: issueworkflow.KindGate},
		{name: "gate key under another step", step: "collect", key: "docs_gate", want: ""},
		{name: "unknown step", step: "missing", key: "docs", want: ""},
		{name: "empty task key", step: "collect", key: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := flowTaskKind(def, tc.step, tc.key); got != tc.want {
				t.Fatalf("flowTaskKind(%q, %q) = %q, want %q", tc.step, tc.key, got, tc.want)
			}
		})
	}
}

// The state tells a reader what they may close, and who closed what. A member who
// manages nothing sees no controls; an agent sees its own tasks only.
func TestFlowState_ViewerFieldsOnlyForManagers(t *testing.T) {
	fx := newFlowFixture(t)
	_, plain := addWorkflowTestMember(t, "flow-viewer")

	read := func(as func(*http.Request) *http.Request) IssueFlowStateResponse {
		t.Helper()
		req := withURLParam(newRequest("GET", "/api/issues/"+fx.Request+"/flow", nil), "id", fx.Request)
		if as != nil {
			req = as(req)
		}
		var state IssueFlowStateResponse
		testutil.Call(t, testHandler.GetIssueFlow, req).Want(http.StatusOK).JSON(&state)
		return state
	}
	sameIDs := func(got []string, want ...string) bool {
		sort.Strings(got)
		sort.Strings(want)
		return reflect.DeepEqual(got, want)
	}

	manager := read(nil)
	if !manager.Viewer.CanManage || !sameIDs(manager.Viewer.ClosableTaskIDs, fx.Work, fx.Gate) {
		t.Fatalf("manager viewer = %+v, want can_manage and both open tasks", manager.Viewer)
	}
	member := read(plain)
	if member.Viewer.CanManage || member.Viewer.ClosableTaskIDs == nil || len(member.Viewer.ClosableTaskIDs) != 0 {
		t.Fatalf("plain member viewer = %+v, want nothing to close", member.Viewer)
	}
	agentReq := withURLParam(newRequest("GET", "/api/issues/"+fx.Request+"/flow", nil), "id", fx.Request)
	agentReq.Header.Set("X-Actor-Source", "task_token")
	agentReq.Header.Set("X-Agent-ID", fx.GateAgent)
	var asAgent IssueFlowStateResponse
	testutil.Call(t, testHandler.GetIssueFlow, agentReq).Want(http.StatusOK).JSON(&asAgent)
	if asAgent.Viewer.CanManage || !sameIDs(asAgent.Viewer.ClosableTaskIDs, fx.Gate) {
		t.Fatalf("agent viewer = %+v, want its own gate only", asAgent.Viewer)
	}

	// A closed task is not closable again, and says who closed it.
	attachAs(t, fx.Work, fx.WorkAgent)
	agentUpdate(t, fx.Work, fx.WorkAgent, map[string]any{"status": "done"}).Want(http.StatusOK)
	closeIssue(t, fx.Gate, map[string]any{"outcome": "approved"}).Want(http.StatusOK)
	after := read(nil)
	if len(after.Viewer.ClosableTaskIDs) != 0 {
		t.Fatalf("closable after both closed = %v", after.Viewer.ClosableTaskIDs)
	}
	byTask := map[string]*IssueFlowClosedByResponse{}
	for _, task := range after.Tasks {
		byTask[task.IssueID] = task.ClosedBy
	}
	if got := byTask[fx.Work]; got == nil || got.Type != "agent" || got.ID != fx.WorkAgent || got.OnBehalf {
		t.Fatalf("work task closed_by = %+v, want its agent", got)
	}
	if got := byTask[fx.Gate]; got == nil || got.Type != "member" || got.ID != testUserID || !got.OnBehalf {
		t.Fatalf("gate closed_by = %+v, want the manager on behalf", got)
	}
	// A reopened task has no closer.
	updateIssueForTest(t, fx.Gate, map[string]any{"status": "in_progress"}).Want(http.StatusOK)
	for _, task := range read(nil).Tasks {
		if task.IssueID == fx.Gate && task.ClosedBy != nil {
			t.Fatalf("reopened gate closed_by = %+v, want none", task.ClosedBy)
		}
	}
}
