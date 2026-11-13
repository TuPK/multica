package handler

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
)

// A task goes only to an agent that is still a member of the squad that owns the
// workflow, and is not its lead (E14).

// toDraft runs a Trade request through collect, so that the next advance opens the
// draft task for the drafter.
func toDraft(t *testing.T, f tradeFlowFixture) (request string) {
	t.Helper()
	row := f.newRequest(t)
	f.plan(t, row, nil)
	finishTask(t, tasksAt(t, row, "collect", 0)["docs"], f.Agents.Docs, "")
	return uuidToString(row.ID)
}

func wantStalledForAgent(t *testing.T, resp FlowAdvanceResponse, request, agentID, reason string) {
	t.Helper()
	var stopped *FlowAction
	for i, a := range resp.Actions {
		if a.Kind == "stopped" {
			stopped = &resp.Actions[i]
		}
	}
	if stopped == nil || stopped.Code != "flow_agent_unavailable" || stopped.AgentID != agentID {
		t.Fatalf("actions = %+v, want a stop for the agent %s", resp.Actions, agentID)
	}
	state, _ := flowRequestState(loadIssueRow(t, request))
	if state.Stop != issueworkflow.StopStalled || state.Step != "blocked" {
		t.Fatalf("step %q, stop %v; want blocked, stalled", state.Step, state.Stop)
	}
	if comment := lastComment(t, loadIssueRow(t, request)); !containsAll(comment, "BLOCKED · treo", "crosscheck", reason) {
		t.Fatalf("stalled comment = %q, want it to name the task and say %q", comment, reason)
	}
}

func TestAdvance_AgentLeftSquadStalls(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := toDraft(t, f)
	dbfx.Exec(t, `DELETE FROM squad_member WHERE squad_id = $1 AND member_id = $2`, f.SquadID, f.Agents.Drafter)

	resp := advance(t, loadIssueRow(t, request))
	wantStalledForAgent(t, resp, request, f.Agents.Drafter, "no longer a member of the squad")
	if got := countTasks(t, loadIssueRow(t, request)); got != 1 {
		t.Fatalf("task issues = %d, want only the collect task: nothing opens for an agent who left", got)
	}

	// Back in the squad, the manager resumes and the task opens.
	dbfx.SquadMember(t, f.SquadID, "agent", f.Agents.Drafter)
	if _, err := testHandler.ResumeFlow(t.Context(), loadIssueRow(t, request), FlowResumeInput{ToStep: "draft"}); err != nil {
		t.Fatalf("ResumeFlow: %v", err)
	}
	if len(tasksAt(t, loadIssueRow(t, request), "draft", 1)) != 1 && len(tasksAt(t, loadIssueRow(t, request), "draft", 0)) != 1 {
		t.Fatal("the draft task did not open after the agent came back")
	}
}

func TestAdvance_AgentBecameLeadStalls(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := toDraft(t, f)
	var oldLead string
	dbfx.QueryRow(t, `SELECT leader_id::text FROM squad WHERE id = $1`, f.SquadID).Scan(&oldLead)
	dbfx.Exec(t, `UPDATE squad SET leader_id = $2 WHERE id = $1`, f.SquadID, f.Agents.Drafter)

	resp := advance(t, loadIssueRow(t, request))
	wantStalledForAgent(t, resp, request, f.Agents.Drafter, "is the lead of the squad")
	if got := countTasks(t, loadIssueRow(t, request)); got != 1 {
		t.Fatalf("task issues = %d, want only the collect task", got)
	}
}

// The agents of the flows the catalog ships are members of their squad and not its
// lead: every task of a catalog flow still opens. The 21 rehearsals run all of them
// to the end; this one checks the first step of a flow on its own.
func TestAdvance_CatalogFlowAgentsStillOpen(t *testing.T) {
	rig := newSquadRig(t, "kv_trade")
	request := rig.newRequest(t)
	if _, err := testHandler.PlanFlow(t.Context(), loadIssueRow(t, uuidToString(request.ID)), FlowPlanInput{Switches: rig.switches("", true)}); err != nil {
		t.Fatalf("PlanFlow: %v", err)
	}
	state, _ := flowRequestState(loadIssueRow(t, uuidToString(request.ID)))
	if state.Stop != issueworkflow.StopNone {
		t.Fatalf("the request stopped at %q (stop %v) with every agent in the squad", state.Step, state.Stop)
	}
	if got := tasksAt(t, request, state.Step, 0); len(got) < 2 {
		t.Fatalf("tasks of %s = %v, want the tasks of a catalog flow opened", state.Step, got)
	}
	var lead string
	dbfx.QueryRow(t, `SELECT leader_id::text FROM squad WHERE id = $1`, rig.squadID).Scan(&lead)
	for _, id := range rig.agents {
		if id == lead {
			t.Fatal("the rig gives a task to the lead of the squad")
		}
	}
}
