package handler

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// The files a Collect task makes are the input of the Draft task (the engine hands a step the files of
// the step before). A Collect task that closes without a file leaves the drafter and the gates with
// nothing to read, so the server refuses it.
func TestRehearsal_Trade_CollectTasksNeedAFile(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, map[string]any{"customs_lookup": true})
	collect := tasksAt(t, request, "collect", 0)

	for _, key := range []string{"docs", "customs"} {
		wantFlowRejection(t, closeIssue(t, collect[key], nil), 422, "flow_attachment_required")
	}
}

// A gate whose findings sit in the source documents themselves cannot send the draft back: the
// drafter cannot fix a total that the invoice prints wrong. It reports the grounds missing, which is
// stop 1, and no rework round is used.
func TestRehearsal_Trade_GateBlockedOnTheSourcesStopsAtOne(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	gates := f.driveToGate(t, request)

	blockTask(t, gates["docs_gate"])
	finishTask(t, gates["customs_gate"], f.Agents.CustomsGate, "rejected")
	st := advance(t, request).State

	if st.Step != "blocked" || st.Stop != issueworkflow.Stop1 || st.Rework != 0 {
		t.Fatalf("step %q, stop %v, rework %d; want blocked at stop 1 with no rework used", st.Step, st.Stop, st.Rework)
	}
	if n := len(tasksAt(t, request, "draft", 1)); n != 0 {
		t.Fatalf("%d draft tasks opened for a defect the draft cannot fix", n)
	}
}

// The gate is told when to use each outcome. The text is the manifest's; the engine only enforces the
// outcome values.
func TestRehearsal_Trade_GateInstructionsSeparateRejectedFromBlocked(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	gates := f.driveToGate(t, request)

	for _, key := range []string{"docs_gate", "customs_gate"} {
		var description string
		dbfx.QueryRow(t, `SELECT description FROM issue WHERE id = $1`, gates[key]).Scan(&description)
		if !containsAll(description, "blocked --outcome blocked", "Chỉ dùng rejected khi lỗi nằm ở bản nháp") {
			t.Fatalf("%s description does not tell when a gate blocks instead of rejecting:\n%s", key, description)
		}
	}
}

// A task that reports it cannot go on is the stop 1 of its request. The stop comment must say what
// is missing: the person who reads it has nothing else, and the task comment is on another issue.
func TestRehearsal_Trade_StopOneFromABlockedTaskSaysWhatIsMissing(t *testing.T) {
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	gates := f.driveToGate(t, request)

	dbfx.Comment(t, gates["customs_gate"], "VERDICT: CHẶN - thiếu biểu thuế bậc 1 và tỷ giá công bố\n\n## Lỗi chặn\n| # | ... |",
		testutil.Cols{"author_type": "agent", "author_id": f.Agents.CustomsGate})
	blockTask(t, gates["customs_gate"])
	advance(t, request)

	got := lastComment(t, request)
	if !containsAll(got, "BLOCKED · stop 1", "customs_gate", "thiếu biểu thuế bậc 1 và tỷ giá công bố", "cannot go on") {
		t.Fatalf("the stop 1 comment does not say what the gate found missing:\n%s", got)
	}
}
