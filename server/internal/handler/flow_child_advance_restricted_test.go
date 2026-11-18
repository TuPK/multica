package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// closeChildAs closes the collect task of a request as userID, the way a member
// who can see the child does: the child's status write is the event the runner
// reacts to.
func closeChildAs(t *testing.T, f tradeFlowFixture, request string, userID string) {
	t.Helper()
	docs := tasksAt(t, loadIssueRow(t, request), "collect", 0)["docs"]
	attachAs(t, docs, f.Agents.Docs)
	endRuns(t, docs)
	// A squad manager may change the task; the member manages the squad, not the flow.
	dbfx.Exec(t, `UPDATE squad SET creator_id = $2 WHERE id = $1`, f.SquadID, userID)
	req := withURLParam(requestAs(userID, "PUT", "/api/issues/"+docs, map[string]any{"status": "done"}), "id", docs)
	testutil.Call(t, testHandler.UpdateIssue, req).Want(http.StatusOK)
}

// A visible child's write advances its request, so a member who sees the child
// but not the request (in a restricted project) must not move that request.
func TestChildWriteDoesNotAdvanceHiddenRequest(t *testing.T) {
	f, request := autoFixture(t)
	hidden := newRestrictedFixture(t)
	dbfx.Exec(t, `UPDATE issue SET project_id = $2 WHERE id = $1`, uuidToString(request.ID), hidden.restrictedProjectID)

	closeChildAs(t, f, uuidToString(request.ID), hidden.outsiderID)

	if got := requestStatus(t, request); got != "collect" {
		t.Errorf("request status = %q after a write by a member who cannot see it, want collect", got)
	}
}

// Control: the same write on a request the member sees does advance it.
func TestChildWriteAdvancesRequestTheWriterSees(t *testing.T) {
	f, request := autoFixture(t)
	hidden := newRestrictedFixture(t)

	closeChildAs(t, f, uuidToString(request.ID), hidden.outsiderID)

	if got := requestStatus(t, request); got != "draft" {
		t.Errorf("request status = %q after a visible child's write, want draft", got)
	}
}
