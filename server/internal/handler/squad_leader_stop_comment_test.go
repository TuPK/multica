package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCreateComment_SquadLeaderStopCommentDoesNotSelfWake is the D1 regression
// through CreateComment. The assigned squad's leader posts a stop comment on the
// request, from a same-squad worker task (the dual-role case that wakes the leader
// while the request runs). The request is blocked, so no task for the leader may
// be queued by that comment.
func TestCreateComment_SquadLeaderStopCommentDoesNotSelfWake(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	fx := newSquadCommentTriggerFixture(t)
	issueID := uuidToString(fx.Issue.ID)

	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE issue_id = $1`, issueID)
		testPool.Exec(context.Background(), `DELETE FROM comment WHERE issue_id = $1`, issueID)
	})

	if _, err := testPool.Exec(ctx, `UPDATE issue SET status = 'blocked' WHERE id = $1`, issueID); err != nil {
		t.Fatalf("set request blocked: %v", err)
	}

	// A member mention gives the leader a task on this request to act as.
	w := httptest.NewRecorder()
	r := newRequest("POST", "/api/issues/"+issueID+"/comments", map[string]any{
		"content": "[@Leader](mention://agent/" + fx.LeaderID + ") check the ledger",
	})
	r = withURLParam(r, "id", issueID)
	testHandler.CreateComment(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateComment(member): expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var trigger CommentResponse
	if err := json.NewDecoder(w.Body).Decode(&trigger); err != nil {
		t.Fatalf("decode member comment: %v", err)
	}

	var taskID string
	if err := testPool.QueryRow(ctx, `
		SELECT id FROM agent_task_queue
		WHERE issue_id = $1 AND agent_id = $2 AND status = 'queued'
		ORDER BY created_at DESC
		LIMIT 1
	`, issueID, fx.LeaderID).Scan(&taskID); err != nil {
		t.Fatalf("load leader mention task: %v", err)
	}
	// The task now runs as a same-squad worker, not as the squad leader.
	if _, err := testPool.Exec(ctx, `UPDATE agent_task_queue SET status = 'running', squad_id = $2 WHERE id = $1`, taskID, fx.SquadID); err != nil {
		t.Fatalf("mark worker task running: %v", err)
	}

	// The stop comment names no one, so only the leader's self-wake could queue a run.
	w = httptest.NewRecorder()
	r = newRequest("POST", "/api/issues/"+issueID+"/comments", map[string]any{
		"content":   "Dừng ở bước 1: thiếu sổ chi tiết.",
		"parent_id": trigger.ID,
	})
	r.Header.Set("X-Agent-ID", fx.LeaderID)
	r.Header.Set("X-Task-ID", taskID)
	r = withURLParam(r, "id", issueID)
	testHandler.CreateComment(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateComment(agent stop): expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var queued int
	if err := testPool.QueryRow(ctx, `
		SELECT count(*) FROM agent_task_queue
		WHERE issue_id = $1 AND agent_id = $2 AND status = 'queued' AND is_leader_task = TRUE
	`, issueID, fx.LeaderID).Scan(&queued); err != nil {
		t.Fatalf("count queued leader tasks: %v", err)
	}
	if queued != 0 {
		t.Fatalf("leader stop comment on a blocked request queued %d leader tasks, want 0", queued)
	}
}
