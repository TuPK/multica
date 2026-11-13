package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/catalog"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Rehearsal of the Trade pilot (S16 of docs/engineering/kitviet-workflow-plan.md). Unlike the engine
// tests, the workflow is not built by hand: it is the generated catalog flow `flow_kv_trade`
// (catalog/workflows/flow_kv_trade), with one fake agent per role. Agents are fake: a test closes a
// task the way the agent's CLI call would.

// catalogFlow is one flow of the generated flows.json, as the quickstart reads it.
type catalogFlow struct {
	Rev    string          `json:"rev"`
	Squad  string          `json:"squad"`
	Config json.RawMessage `json:"config"`
	Steps  []struct {
		StatusKey string `json:"statusKey"`
		Status    struct {
			Name     string `json:"name"`
			Category string `json:"category"`
		} `json:"status"`
		HumanOnly bool `json:"humanOnly"`
		Tasks     []struct {
			Key          string          `json:"key"`
			Title        string          `json:"title"`
			Kind         string          `json:"kind"`
			Role         string          `json:"role"`
			Instructions string          `json:"instructions"`
			When         json.RawMessage `json:"when"`
			Requires     []string        `json:"requires"`
		} `json:"tasks"`
		Transitions []struct {
			ID     string          `json:"id"`
			Label  string          `json:"label"`
			When   json.RawMessage `json:"when"`
			To     string          `json:"to"`
			Rework bool            `json:"rework"`
		} `json:"transitions"`
	} `json:"steps"`
}

// loadCatalogFlow reads one flow of the generated catalog.
func loadCatalogFlow(t *testing.T, id string) catalogFlow {
	t.Helper()
	raw := catalog.FlowsJSON()
	if len(raw) == 0 {
		t.Fatal(catalog.BuildHint)
	}
	var flows map[string]catalogFlow
	if err := json.Unmarshal(raw, &flows); err != nil {
		t.Fatalf("decode flows.json: %v", err)
	}
	flow, ok := flows[id]
	if !ok {
		t.Fatalf("flows.json has no %s", id)
	}
	return flow
}

// roles lists the roles the tasks of the flow name.
func (f catalogFlow) roles() []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range f.Steps {
		for _, task := range s.Tasks {
			if !seen[task.Role] {
				seen[task.Role] = true
				out = append(out, task.Role)
			}
		}
	}
	return out
}

// body turns the flow into the create request the quickstart sends, giving each role the agent
// agentOf names.
func (flow catalogFlow) body(t *testing.T, name string, agentOf func(role string) string) map[string]any {
	t.Helper()
	decode := func(raw json.RawMessage) any {
		var v any
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &v); err != nil {
				t.Fatalf("decode condition: %v", err)
			}
		}
		return v
	}
	var config any
	if err := json.Unmarshal(flow.Config, &config); err != nil {
		t.Fatal(err)
	}
	steps := make([]map[string]any, 0, len(flow.Steps))
	for _, s := range flow.Steps {
		if s.Status.Name != "" {
			ensureGoldenStatus(t, s.StatusKey, s.Status.Category)
		}
		step := map[string]any{"status_key": s.StatusKey}
		if s.HumanOnly {
			step["human_only"] = true
		}
		var tasks []map[string]any
		for _, task := range s.Tasks {
			entry := map[string]any{
				"key": task.Key, "title": task.Title, "kind": task.Kind, "instructions": task.Instructions,
				"handler": map[string]any{"type": "agent", "id": agentOf(task.Role)},
			}
			if task.When != nil {
				entry["when"] = decode(task.When)
			}
			if len(task.Requires) > 0 {
				entry["requires"] = task.Requires
			}
			tasks = append(tasks, entry)
		}
		if tasks != nil {
			step["tasks"] = tasks
		}
		var transitions []map[string]any
		for _, tr := range s.Transitions {
			entry := map[string]any{"id": tr.ID, "label": tr.Label, "when": decode(tr.When), "to_status_key": tr.To}
			if tr.Rework {
				entry["rework"] = true
			}
			transitions = append(transitions, entry)
		}
		if transitions != nil {
			step["transitions"] = transitions
		}
		steps = append(steps, step)
	}
	return map[string]any{"name": name, "flow_config": config, "template_ref": flow.Squad + "@" + flow.Rev, "steps": steps}
}

// catalogTradeFlowBody is the Trade flow with the fixture's agents.
func catalogTradeFlowBody(t *testing.T, name string, a tradeAgents) map[string]any {
	t.Helper()
	agents := map[string]string{
		"trade_docs_reader": a.Docs, "trade_customs_research": a.Customs, "trade_crosscheck_drafter": a.Drafter,
		"trade_docs_gate": a.DocsGate, "trade_customs_gate": a.CustomsGate, "trade_handover": a.Handover,
	}
	return loadCatalogFlow(t, "flow_kv_trade").body(t, name, func(role string) string {
		id, ok := agents[role]
		if !ok {
			t.Fatalf("role %s has no fixture agent", role)
		}
		return id
	})
}

func newRehearsalFixture(t *testing.T) tradeFlowFixture {
	t.Helper()
	cleanupFlowRows(t)
	seedTestCatalog(t)
	agents := newTradeAgents(t)
	wf := createFlowWorkflowForTest(t, catalogTradeFlowBody(t, "Trade rehearsal "+workflowTestSuffix(), agents))
	squadID := flowSquad(t, "Trade rehearsal squad", agents.all()...)
	dbfx.Exec(t, `UPDATE issue_workflow SET squad_id = $2 WHERE id = $1`, wf.ID, squadID)
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, squadID, wf.ID)
	return tradeFlowFixture{Agents: agents, Workflow: wf, SquadID: squadID}
}

func lastComment(t *testing.T, request db.Issue) string {
	t.Helper()
	var comment string
	dbfx.QueryRow(t, `SELECT content FROM comment WHERE issue_id = $1 ORDER BY created_at DESC LIMIT 1`, request.ID).Scan(&comment)
	return comment
}

// toHandover drives a planned request through collect and draft to the handover task.
func (f tradeFlowFixture) toHandover(t *testing.T, request db.Issue, gates map[string]string) string {
	t.Helper()
	finishTask(t, gates["docs_gate"], f.Agents.DocsGate, "approved")
	finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "approved")
	st := advance(t, request).State
	if st.Step != "handover" {
		t.Fatalf("after both gates the request is at %q, want handover", st.Step)
	}
	// The handover opens in the round the request is in: one more per rework.
	return tasksAt(t, request, "handover", st.Rework)["handover"]
}

func TestRehearsal_Trade_HappyPath(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	gates := f.driveToGate(t, request)
	handover := f.toHandover(t, request, gates)
	finishTask(t, handover, f.Agents.Handover, "approved")

	st := advance(t, request).State
	if st.Step != "in_review" || requestStatus(t, request) != "in_review" {
		t.Fatalf("step %q, status %q; want in_review", st.Step, requestStatus(t, request))
	}
	// 1 collect + 1 draft + 2 gates + 1 handover; the switches were off, so no customs and no export.
	if n := countTasks(t, request); n != 5 {
		t.Fatalf("task issues = %d, want 5", n)
	}
	// The engine made every sub-issue: none has an agent as its creator.
	if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE parent_issue_id = $1 AND creator_type = 'agent'`, request.ID); n != 0 {
		t.Fatalf("%d sub-issues were created by an agent, want 0", n)
	}
	if !containsAll(lastComment(t, request), "IN REVIEW · stop 2 · trước khi ra khỏi workspace", "mention://member/"+testUserID) {
		t.Fatalf("stop 2 comment = %q", lastComment(t, request))
	}
	// Only a member closes the request.
	updateIssueForTest(t, uuidToString(request.ID), map[string]any{"status": "done"}).Want(http.StatusOK)
	if dbfx.Count(t, `SELECT count(*) FROM issue WHERE id = $1 AND flow->>'closed_at' IS NOT NULL`, request.ID) != 1 {
		t.Fatal("flow.closed_at is empty after the member moved the request to done")
	}
}

func TestRehearsal_Trade_ReworkTwice(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	gates := f.driveToGate(t, request)
	for round := 0; round < 2; round++ {
		finishTask(t, gates["docs_gate"], f.Agents.DocsGate, "approved")
		finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "rejected")
		st := advance(t, request).State
		if st.Step != "draft" || st.Rework != round+1 {
			t.Fatalf("round %d: step %q, rework %d; want draft, %d", round, st.Step, st.Rework, round+1)
		}
		gates = f.finishDraft(t, request, round+1)
	}
	// Both gates run again in every round, the one that approved included.
	for round := 0; round < 3; round++ {
		if got := len(tasksAt(t, request, "gate", round)); got != 2 {
			t.Fatalf("round %d opened %d gates, want 2", round, got)
		}
	}
	handover := f.toHandover(t, request, gates)
	finishTask(t, handover, f.Agents.Handover, "approved")
	if st := advance(t, request).State; st.Step != "in_review" || st.Rework != 2 {
		t.Fatalf("step %q, rework %d; want in_review after two fixes", st.Step, st.Rework)
	}
}

func TestRehearsal_Trade_GateWithoutVerdictRejected(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	gates := f.driveToGate(t, request)

	attachAs(t, gates["docs_gate"], f.Agents.DocsGate)
	wantFlowRejection(t, closeIssue(t, gates["docs_gate"], nil), http.StatusUnprocessableEntity, "flow_outcome_required")
	if status, _, _ := issueStatusAndAssignee(t, gates["docs_gate"]); status == "done" {
		t.Fatal("a gate without a verdict was closed")
	}
}

func TestRehearsal_Trade_Stop1MissingInvoice(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)

	blockTask(t, tasksAt(t, request, "collect", 0)["docs"])
	st := advance(t, request).State
	if st.Step != "blocked" || st.Stop != issueworkflow.Stop1 || requestStatus(t, request) != "blocked" {
		t.Fatalf("step %q, stop %v, status %q; want blocked at stop 1", st.Step, st.Stop, requestStatus(t, request))
	}
	if !containsAll(lastComment(t, request), "BLOCKED · stop 1 · thiếu căn cứ", "mention://member/"+testUserID) {
		t.Fatalf("stop 1 comment = %q", lastComment(t, request))
	}
	if n := len(tasksAt(t, request, "draft", 0)); n != 0 {
		t.Fatalf("%d draft tasks opened on a request with missing grounds", n)
	}
}

func TestRehearsal_Trade_CustomsLookupSwitch(t *testing.T) {
	f := newRehearsalFixture(t)
	off := f.newRequest(t)
	f.plan(t, off, nil)
	if got := tasksAt(t, off, "collect", 0); len(got) != 1 || got["docs"] == "" {
		t.Fatalf("switch off: collect tasks = %v, want only docs", got)
	}
	on := f.newRequest(t)
	f.plan(t, on, map[string]any{"customs_lookup": true})
	if got := tasksAt(t, on, "collect", 0); len(got) != 2 || got["customs"] == "" {
		t.Fatalf("switch on: collect tasks = %v, want docs and customs", got)
	}
}

func TestRehearsal_Trade_AgentCannotLeaveInReview(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	moveRequest(t, request, "in_review")
	id := uuidToString(request.ID)

	wantFlowRejection(t, agentSetStatus(t, id, f.Agents.Handover, "in_review", "done"), http.StatusForbidden, "flow_human_only")
	if status, _, _ := issueStatusAndAssignee(t, id); status != "in_review" {
		t.Fatalf("status = %q, want in_review", status)
	}
}

func TestRehearsal_Trade_ExportOnlyAfterStop2(t *testing.T) {
	f := newRehearsalFixture(t)
	// The plan turns the export switch on.
	other := f.newRequest(t)
	f.plan(t, other, map[string]any{"export": true})
	finishTask(t, tasksAt(t, other, "collect", 0)["docs"], f.Agents.Docs, "")
	advance(t, other)
	finishTask(t, tasksAt(t, other, "draft", 0)["crosscheck"], f.Agents.Drafter, "")
	advance(t, other)
	otherGates := tasksAt(t, other, "gate", 0)
	if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE parent_issue_id = $1 AND flow_task = 'export'`, other.ID); n != 0 {
		t.Fatalf("export task exists before stop 2 (%d)", n)
	}
	handover := f.toHandover(t, other, otherGates)
	finishTask(t, handover, f.Agents.Handover, "approved")
	if st := advance(t, other).State; st.Step != "in_review" {
		t.Fatalf("step %q, want in_review", st.Step)
	}
	if export := tasksAt(t, other, "in_review", 0)["export"]; export == "" {
		t.Fatal("export task not opened at in_review")
	}
	if !containsAll(lastComment(t, other), "IN REVIEW · stop 2") {
		// The export task's own comments may come later; the stop comment must exist.
		if dbfx.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1 AND content LIKE 'IN REVIEW · stop 2%'`, other.ID) != 1 {
			t.Fatal("no stop 2 comment on the request")
		}
	}
}

func TestRehearsal_Trade_ThirdRejectionStop3ThenResume(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	gates := f.driveToGate(t, request)
	for round := 0; round < 3; round++ {
		finishTask(t, gates["docs_gate"], f.Agents.DocsGate, "rejected")
		finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "rejected")
		advance(t, request)
		if round < 2 {
			gates = f.finishDraft(t, request, round+1)
		}
	}
	st := advance(t, request).State
	if st.Step != "blocked" || st.Stop != issueworkflow.Stop3 {
		t.Fatalf("step %q, stop %v; want blocked at stop 3", st.Step, st.Stop)
	}
	if !containsAll(lastComment(t, request), "BLOCKED · stop 3 · hết 2 vòng sửa") {
		t.Fatalf("stop 3 comment = %q", lastComment(t, request))
	}
	state, err := testHandler.ResumeFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID)), FlowResumeInput{})
	if err != nil {
		t.Fatalf("ResumeFlow: %v", err)
	}
	if state.Step != "gate" || state.Rework != 0 {
		t.Fatalf("after resume: step %q, rework %d; want gate, 0", state.Step, state.Rework)
	}
}

func TestRehearsal_Trade_DetachRestoresPlain(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)
	docs := tasksAt(t, request, "collect", 0)["docs"]

	if err := testHandler.DetachFlow(context.Background(), loadIssueRow(t, uuidToString(request.ID)), false); err != nil {
		t.Fatalf("DetachFlow: %v", err)
	}
	if row := loadIssueRow(t, docs); row.FlowTask.Valid {
		t.Fatal("the task kept its flow columns")
	}
	updateIssueForTest(t, docs, map[string]any{"status": "done"}).Want(http.StatusOK)
	if dbfx.Count(t, `SELECT count(*) FROM issue WHERE id = $1 AND flow IS NOT NULL`, request.ID) != 0 {
		t.Fatal("the request still has a flow")
	}
}
