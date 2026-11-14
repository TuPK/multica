package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// flowViewMap reads GET /api/issues/{id}/flow as a generic object, so a field the view must carry is
// checked by its JSON name.
func flowViewMap(t *testing.T, issueID string) map[string]any {
	t.Helper()
	read := withURLParam(newRequest("GET", "/api/issues/"+issueID+"/flow", nil), "id", issueID)
	var out map[string]any
	testutil.Call(t, testHandler.GetIssueFlow, read).Want(http.StatusOK).JSON(&out)
	return out
}

// A request a person cancelled is closed (the engine's ClosedAt). The flow view says so, and advancing
// it does nothing.
func TestGetFlow_CancelledRequestReportsClosed(t *testing.T) {
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)
	id := uuidToString(request.ID)

	if closed, _ := flowViewMap(t, id)["closed"].(bool); closed {
		t.Fatalf("an open request reports closed")
	}
	updateIssueForTest(t, id, map[string]any{"status": "cancelled"}).Want(http.StatusOK)
	if closed, _ := flowViewMap(t, id)["closed"].(bool); !closed {
		t.Fatalf("a cancelled request's flow view says closed=%v, want true", flowViewMap(t, id)["closed"])
	}
	if kinds := actionKinds(advance(t, loadIssueRow(t, id))); len(kinds) != 1 || kinds[0] != "noop" {
		t.Fatalf("advance on a closed request returned actions %v, want [noop]", kinds)
	}
}

// A stall says why it happened in the flow view, as the stop record does (StopRecord.Reason). The
// stall comment names the step by its key when the status has no name.
func TestRehearsalGraph_StallReasonIsInTheFlowView(t *testing.T) {
	// The handover step gets a built-in status (in_progress): a built-in status has no name of its own,
	// so the stall comment has to name the step by its key.
	rig := newGraphRig(t, "trade", graphRigOpts{Mutate: func(doc map[string]any) {
		for _, step := range asMaps(doc["steps"]) {
			if step["key"] != "handover" {
				continue
			}
			step["status"] = map[string]any{"key": "in_progress"}
			for _, edge := range asMaps(step["edges"]) {
				if edge["id"] == "handover_ok" {
					edge["to"] = "done"
				}
			}
		}
	}})
	request := rig.newRequest()
	rig.complete(request, nil)
	rig.finishAll(request, "collect", 0, always(""))
	rig.advance(request)
	rig.finishAll(request, "draft", 0, always(""))
	rig.advance(request)
	rig.finishAll(request, "gate", 0, always("approved"))
	rig.advance(request)
	rig.finishAll(request, "handover", 0, always("approved"))
	st := rig.advance(request)
	if st.Stop == nil || st.Stop.Code != issueworkflow.DocStopStalled {
		t.Fatalf("stop %+v, want stalled", st.Stop)
	}

	raw, _ := json.Marshal(st.Stop)
	var stop map[string]any
	_ = json.Unmarshal(raw, &stop)
	if stop["reason"] != issueworkflow.ReasonReleaseWithoutCheckpoint {
		t.Fatalf("the flow view's stop is %v, want reason %q", stop, issueworkflow.ReasonReleaseWithoutCheckpoint)
	}

	var content string
	dbfx.QueryRow(t, `SELECT content FROM comment WHERE issue_id = $1 AND author_type = 'system' AND content LIKE 'BLOCKED · stalled%' ORDER BY created_at DESC LIMIT 1`, request.ID).Scan(&content)
	if !strings.Contains(content, "stopped moving at handover.") {
		t.Fatalf("the stall comment does not name the step: %q", content)
	}
}
