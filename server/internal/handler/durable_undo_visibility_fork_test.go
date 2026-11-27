package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// An effect taken by a task on a private project's issue is hidden from a workspace admin
// outside that project: the undo answers as it does for a missing effect.
func TestUndoHidesEffectOfPrivateProjectFromOutsider(t *testing.T) {
	withGmailDraftTools(t)
	f := effectsOnFixture(t)
	pf := newPrivateProjectFixture(t)
	if _, err := testPool.Exec(context.Background(), `UPDATE agent_task_queue SET issue_id = $2 WHERE id = $1`, f.taskID, pf.secretIssue); err != nil {
		t.Fatalf("move the task onto the private issue: %v", err)
	}
	id := seedGmailDraftEffect(t, f, time.Now().Add(time.Hour), true)

	answer := func(userID, effectID string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := effectAPIRequest(t, http.MethodPost, "/api/effects/"+effectID+"/undo", userID, "", nil)
		f.h.UndoEffect(w, testutil.WithURLParams(req, "id", effectID))
		return w
	}

	missing := answer(pf.outsideAdmin, uuid.NewString())
	hidden := answer(pf.outsideAdmin, id)
	if hidden.Code != missing.Code || hidden.Body.String() != missing.Body.String() {
		t.Fatalf("outsider admin: hidden effect answered %d %s; a missing effect answers %d %s",
			hidden.Code, hidden.Body.String(), missing.Code, missing.Body.String())
	}
	if hidden.Code != http.StatusNotFound {
		t.Fatalf("outsider admin: got %d, want 404", hidden.Code)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM effect_event WHERE id=$1 AND status=$2`, id, "executed"); n != 1 {
		t.Fatalf("a hidden effect must stay executed")
	}

	// Control: a project member gets past the visibility check (the member who acted is the owner,
	// so the answer is the role refusal, not the missing-effect answer).
	if member := answer(pf.insider, id); member.Code == http.StatusNotFound {
		t.Fatalf("project member got the missing-effect answer: %d %s", member.Code, member.Body.String())
	}
}
