package handler

import (
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
)

// Rehearsal of the Trade pilot with the runner on (S21): the lead only plans, and the runner moves
// every step. Agents are fake, as in the manual rehearsal; the flow is the catalog's.

func TestRehearsalAuto_Trade_HappyPath(t *testing.T) {
	f, request := autoFixture(t) // planned: the lead's only call
	id := uuidToString(request.ID)

	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")
	closeTaskForEvent(t, tasksAt(t, request, "draft", 0)["crosscheck"], f.Agents.Drafter, "")
	gates := tasksAt(t, request, "gate", 0)
	closeTaskForEvent(t, gates["docs_gate"], f.Agents.DocsGate, "approved")
	if got := requestStatus(t, request); got != "gate" {
		t.Fatalf("status = %q with one gate open, want gate: the barrier waits for both", got)
	}
	closeTaskForEvent(t, gates["customs_gate"], f.Agents.CustomsGate, "approved")
	closeTaskForEvent(t, tasksAt(t, request, "handover", 0)["handover"], f.Agents.Handover, "approved")

	if got := requestStatus(t, request); got != "in_review" {
		t.Fatalf("status = %q, want in_review reached by the runner alone", got)
	}
	if n := countTasks(t, request); n != 5 {
		t.Fatalf("task issues = %d, want 5", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE parent_issue_id = $1 AND creator_type = 'agent'`, request.ID); n != 0 {
		t.Fatalf("%d sub-issues created by an agent", n)
	}
	// B6: the lead was woken for no step between the plan and the stop.
	if runs := childDoneRuns(t, id); len(runs) != 0 {
		t.Fatalf("the lead got %d child_done wake-ups: %+v", len(runs), runs)
	}
	if !containsAll(lastComment(t, request), "IN REVIEW · stop 2 · trước khi ra khỏi workspace") {
		t.Fatalf("stop 2 comment = %q", lastComment(t, request))
	}
}

func TestRehearsalAuto_Trade_NoDuplicatesAfterRestart(t *testing.T) {
	f, request := autoFixture(t)
	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")

	// The server restarts (the sweep forgets its place) and passes run again and again.
	for i := 0; i < 3; i++ {
		resetSweepCursor()
		sweep(t)
		advance(t, request)
	}

	if n := duplicateTasks(t, request); n != 0 {
		t.Fatalf("%d duplicated tasks", n)
	}
	if n := countTasks(t, request); n != 2 {
		t.Fatalf("task issues = %d, want 2 (docs and crosscheck)", n)
	}
	before := dbfx.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1`, request.ID)
	sweep(t)
	if after := dbfx.Count(t, `SELECT count(*) FROM comment WHERE issue_id = $1`, request.ID); after != before {
		t.Fatalf("a pass with nothing to do added %d comments", after-before)
	}
}

func TestRehearsalAuto_Trade_StalledTaskBlocksRequest(t *testing.T) {
	f, request := autoFixture(t)
	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")
	crosscheck := tasksAt(t, request, "draft", 0)["crosscheck"]

	// The drafter's runs end without closing the task, twice.
	for i := 0; i < 2; i++ {
		endRuns(t, crosscheck)
		backdateStep(t, request, 5*time.Hour)
		backdateRuns(t, crosscheck, 3*time.Hour)
		sweep(t)
	}

	state, _ := flowRequestState(loadIssueRow(t, uuidToString(request.ID)))
	if state.Stop != issueworkflow.StopStalled || requestStatus(t, request) != "blocked" {
		t.Fatalf("stop %v, status %q; want stalled and blocked", state.Stop, requestStatus(t, request))
	}
	if !containsAll(lastComment(t, request), "BLOCKED · stalled") {
		t.Fatalf("stalled comment = %q", lastComment(t, request))
	}
}

func TestRehearsalAuto_Trade_KillSwitchFallsBackToManual(t *testing.T) {
	f, request := autoFixture(t)
	id := uuidToString(request.ID)
	withFeatureFlag(t, testHandler, "flow_runner", false)

	closeTaskForEvent(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")

	if got := requestStatus(t, request); got != "collect" {
		t.Fatalf("status = %q with the runner off, want collect", got)
	}
	if n := sweep(t); n != 0 {
		t.Fatalf("the sweep changed %d requests with the runner off", n)
	}
	if runs := childDoneRuns(t, id); len(runs) != 1 {
		t.Fatalf("child_done runs = %d, want the lead woken to advance by hand", len(runs))
	}
	if resp := advance(t, request); resp.State.Step != "draft" {
		t.Fatalf("the lead's advance reached %q, want draft", resp.State.Step)
	}
}
