package handler

import (
	"context"
	"net/http"
	"testing"
)

// A person who cancels a work task whose file is part of the step does not make
// the step pass without it (E7); a gate that is cancelled is still a rejection.

// atDraft runs a Trade request to the draft step, where the task crosscheck
// requires a file, and returns it with that task.
func atDraft(t *testing.T) (tradeFlowFixture, string, string) {
	t.Helper()
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)
	finishTask(t, tasksAt(t, request, "collect", 0)["docs"], f.Agents.Docs, "")
	advance(t, request)
	crosscheck := tasksAt(t, request, "draft", 0)["crosscheck"]
	if crosscheck == "" {
		t.Fatal("the draft step did not open its task")
	}
	return f, uuidToString(request.ID), crosscheck
}

func TestAdvance_ManagerCancelledWorkTaskReopens(t *testing.T) {
	_, request, crosscheck := atDraft(t)
	row := loadIssueRow(t, request)

	updateIssueForTest(t, crosscheck, map[string]any{"status": "cancelled"}).Want(http.StatusOK)
	endRuns(t, crosscheck)
	resp := advance(t, row)

	if status, _, _ := issueStatusAndAssignee(t, crosscheck); status != "todo" {
		t.Fatalf("the cancelled task is %q, want reopened to todo", status)
	}
	if resp.State.Step != "draft" || len(tasksAt(t, row, "gate", 0)) != 0 {
		t.Fatalf("step %q, gate tasks %v: the step passed without the file", resp.State.Step, tasksAt(t, row, "gate", 0))
	}
	if kinds := actionKinds(resp); len(kinds) != 1 || kinds[0] != "reopened_tasks" {
		t.Fatalf("actions = %v, want [reopened_tasks]", kinds)
	}
	if n := queuedRuns(t, crosscheck); n != 1 {
		t.Fatalf("queued runs of the reopened task = %d, want 1: its agent has to do the work again", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'flow_task_reopened'`, crosscheck); n != 1 {
		t.Fatalf("flow_task_reopened entries = %d, want 1", n)
	}
	// The same activation: no second task, no new round.
	if got := tasksAt(t, row, "draft", 0); len(got) != 1 {
		t.Fatalf("draft tasks = %v", got)
	}
	if again := advance(t, row); actionKinds(again)[0] != "noop" {
		t.Fatalf("a second advance did %v, want nothing", actionKinds(again))
	}
}

func TestAdvance_CancelledGateStillRejects(t *testing.T) {
	f, request, crosscheck := atDraft(t)
	row := loadIssueRow(t, request)
	finishTask(t, crosscheck, f.Agents.Drafter, "")
	advance(t, row)
	gates := tasksAt(t, row, "gate", 0)

	updateIssueForTest(t, gates["docs_gate"], map[string]any{"status": "cancelled"}).Want(http.StatusOK)
	endRuns(t, gates["docs_gate"])
	finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "approved")
	state := advance(t, row).State

	if state.Step != "draft" || state.Rework != 1 {
		t.Fatalf("step %q, rework %d; want the draft back for a fix", state.Step, state.Rework)
	}
	if status, _, _ := issueStatusAndAssignee(t, gates["docs_gate"]); status != "cancelled" {
		t.Fatalf("the cancelled gate is %q: a gate is not reopened, it is a rejection", status)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'flow_task_reopened'`, gates["docs_gate"]); n != 0 {
		t.Fatalf("a cancelled gate was logged as reopened")
	}
}

// stopAtDraft parks a request at the draft step the way a stall does.
func stopAtDraft(t *testing.T, request string) {
	t.Helper()
	dbfx.Exec(t, `UPDATE issue SET status = 'blocked',
		flow = flow || '{"stop":"stalled","blocked_from":"draft","step":"blocked"}'::jsonb WHERE id = $1`, request)
}

func TestResume_SkipCancelledWorkTaskContinues(t *testing.T) {
	t.Run("without skip_cancelled the task is reopened", func(t *testing.T) {
		_, request, crosscheck := atDraft(t)
		row := loadIssueRow(t, request)
		updateIssueForTest(t, crosscheck, map[string]any{"status": "cancelled"}).Want(http.StatusOK)
		endRuns(t, crosscheck)
		stopAtDraft(t, request)

		state, err := testHandler.ResumeFlow(context.Background(), loadIssueRow(t, request), FlowResumeInput{})
		if err != nil {
			t.Fatalf("ResumeFlow: %v", err)
		}
		if state.Step != "draft" || len(tasksAt(t, row, "gate", 0)) != 0 {
			t.Fatalf("step %q, gate tasks %v", state.Step, tasksAt(t, row, "gate", 0))
		}
		if status, _, _ := issueStatusAndAssignee(t, crosscheck); status != "todo" {
			t.Fatalf("the cancelled task is %q after resume, want reopened", status)
		}
	})

	t.Run("with skip_cancelled the step goes on without it", func(t *testing.T) {
		_, request, crosscheck := atDraft(t)
		row := loadIssueRow(t, request)
		updateIssueForTest(t, crosscheck, map[string]any{"status": "cancelled"}).Want(http.StatusOK)
		endRuns(t, crosscheck)
		stopAtDraft(t, request)

		state, err := testHandler.ResumeFlow(context.Background(), loadIssueRow(t, request), FlowResumeInput{SkipCancelled: true})
		if err != nil {
			t.Fatalf("ResumeFlow: %v", err)
		}
		if state.Step != "gate" {
			t.Fatalf("step %q, want the request on at the gate", state.Step)
		}
		if got := tasksAt(t, row, "gate", 0); len(got) != 2 {
			t.Fatalf("gate tasks = %v, want both gates opened", got)
		}
		if status, _, _ := issueStatusAndAssignee(t, crosscheck); status != "cancelled" {
			t.Fatalf("the skipped task is %q, want it left cancelled", status)
		}
		if n := dbfx.Count(t, `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'flow_task_skipped'`, crosscheck); n != 1 {
			t.Fatalf("flow_task_skipped entries = %d, want 1", n)
		}
		// Later advances leave it alone.
		advance(t, row)
		if status, _, _ := issueStatusAndAssignee(t, crosscheck); status != "cancelled" {
			t.Fatalf("a later advance reopened the skipped task (%q)", status)
		}
	})
}
