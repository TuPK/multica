package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// A request that stopped at a gate can go back to the step that drafts its work. The state says
// which step that is, and resume with to_step sends the request there in a new round.

// stopAtDocsGate stops a Trade request at stop 1 from the docs gate, the case where the draft is at fault.
func stopAtDocsGate(t *testing.T, f tradeFlowFixture, request db.Issue) {
	t.Helper()
	gates := f.driveToGate(t, request)
	blockTask(t, gates["docs_gate"])
	finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "rejected")
	if st := advance(t, request).State; st.Step != "blocked" || stopOf(st) != issueworkflow.Stop1 {
		t.Fatalf("step %q, stop %v; want blocked at stop 1", st.Step, st.Stop)
	}
}

func TestFlowAPI_StoppedAtGateNamesTheDraftingStep(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	c := newFlowCaller(t, f, request)
	stopAtDocsGate(t, f, request)

	var state IssueFlowStateResponse
	c.call("manager", testHandler.GetIssueFlow, "GET", "", nil).Want(http.StatusOK).JSON(&state)
	if state.ReworkStep != "draft" {
		t.Fatalf("rework_step = %q, want draft: the gate's rework transition goes there", state.ReworkStep)
	}
}

// Only a stopped request offers the step: a running one already has its next steps in next.
func TestFlowAPI_RunningGateHasNoReworkStepField(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	c := newFlowCaller(t, f, request)
	f.driveToGate(t, request)

	var state IssueFlowStateResponse
	c.call("manager", testHandler.GetIssueFlow, "GET", "", nil).Want(http.StatusOK).JSON(&state)
	if state.Step != "gate" || stopOf(state) != issueworkflow.StopNone {
		t.Fatalf("step %q, stop %v; want a running gate", state.Step, stopOf(state))
	}
	if state.ReworkStep != "" {
		t.Fatalf("rework_step = %q on a running request, want none", state.ReworkStep)
	}
}

// A stop at the plan step has no drafting step behind it: nothing was drafted yet.
func TestFlowAPI_StopAtPlanNamesNoDraftingStep(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	c := newFlowCaller(t, f, request)
	planBlockedByLead(t, request)

	var state IssueFlowStateResponse
	c.call("manager", testHandler.GetIssueFlow, "GET", "", nil).Want(http.StatusOK).JSON(&state)
	if stopOf(state) != issueworkflow.Stop1 || state.ReworkStep != "" {
		t.Fatalf("stop %v, rework_step %q; want stop 1 and no drafting step", stopOf(state), state.ReworkStep)
	}
}

// Resume with to_step=draft from a gate stop: the draft opens in a new round, and the rework count
// is kept because stop 1 does not use a round.
func TestFlowAPI_ResumeToDraftOpensTheDraftInANewRound(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	c := newFlowCaller(t, f, request)
	stopAtDocsGate(t, f, request)

	var state IssueFlowStateResponse
	c.call("manager", testHandler.ResumeIssueFlow, "POST", "/resume", map[string]any{"to_step": "draft"}).Want(http.StatusOK).JSON(&state)
	if state.Step != "draft" || stopOf(state) != issueworkflow.StopNone || state.Rework != 0 {
		t.Fatalf("after resume to draft: step %q, stop %v, rework %d; want draft, none, 0", state.Step, stopOf(state), state.Rework)
	}
	if n := len(tasksAt(t, request, "draft", 1)); n == 0 {
		t.Fatal("no draft task opened in the new round 1")
	}
}
