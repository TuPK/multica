package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func queueIssue(t *testing.T, title string, number, order int) string {
	t.Helper()
	return dbfx.Issue(t, title, testutil.Cols{"status": "backlog", "number": number, "queue_order": order})
}

func queuedIssueStatus(t *testing.T, id string) string {
	t.Helper()
	var status string
	dbfx.QueryRow(t, `SELECT status FROM issue WHERE id = $1`, id).Scan(&status)
	return status
}

func promoteQueue(t *testing.T) int {
	t.Helper()
	n, err := testHandler.PromoteQueuedIssues(context.Background(), parseUUID(testWorkspaceID), pgtype.UUID{}, false)
	if err != nil {
		t.Fatalf("PromoteQueuedIssues: %v", err)
	}
	return n
}

func TestPromoteQueuedIssues_ReleasesByOrderAndRunsEqualOrdersTogether(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	a := queueIssue(t, "queue a", 93001, 1)
	b := queueIssue(t, "queue b", 93002, 1)
	c := queueIssue(t, "queue c", 93003, 2)
	d := queueIssue(t, "queue d", 93004, 4)

	if n := promoteQueue(t); n != 2 {
		t.Fatalf("first pass moved %d issues, want 2", n)
	}
	for id, want := range map[string]string{a: "todo", b: "todo", c: "backlog", d: "backlog"} {
		if got := queuedIssueStatus(t, id); got != want {
			t.Fatalf("issue %s status = %q, want %q", id, got, want)
		}
	}

	// One of the two equal orders finishing is not enough.
	dbfx.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, a)
	if n := promoteQueue(t); n != 0 {
		t.Fatalf("moved %d issues while order 1 still has an open issue", n)
	}

	// A cancelled predecessor counts as finished; the gap before order 4 is fine.
	dbfx.Exec(t, `UPDATE issue SET status = 'cancelled' WHERE id = $1`, b)
	if n := promoteQueue(t); n != 1 {
		t.Fatalf("moved %d issues after order 1 finished, want 1", n)
	}
	if got := queuedIssueStatus(t, c); got != "todo" {
		t.Fatalf("order 2 status = %q, want todo", got)
	}
	if got := queuedIssueStatus(t, d); got != "backlog" {
		t.Fatalf("order 4 status = %q, want backlog while order 2 is open", got)
	}

	// Promotion is idempotent.
	if n := promoteQueue(t); n != 0 {
		t.Fatalf("repeat pass moved %d issues", n)
	}
}

func TestSetIssueQueueOrder_ValidatesAndClears(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	id := dbfx.Issue(t, "queue order endpoint", testutil.Cols{"status": "backlog", "number": 93010})
	put := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/issues/"+id+"/queue-order", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", testUserID)
		req.Header.Set("X-Workspace-ID", testWorkspaceID)
		req = withURLParam(req, "id", id)
		testHandler.SetIssueQueueOrder(w, req)
		return w
	}
	for _, body := range []string{`{"queue_order":0}`, `{"queue_order":-3}`, `{"queue_order":1.5}`, `{"queue_order":"2"}`, `{"queue_order":9999999}`} {
		if w := put(body); w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400", body, w.Code)
		}
	}
	// Arming an order below an open issue keeps it parked.
	if w := put(`{"queue_order":3}`); w.Code != http.StatusOK {
		t.Fatalf("set: status = %d: %s", w.Code, w.Body.String())
	}
	var order *int
	dbfx.QueryRow(t, `SELECT queue_order FROM issue WHERE id = $1`, id).Scan(&order)
	if order == nil || *order != 3 {
		t.Fatalf("stored queue_order = %v, want 3", order)
	}
	if w := put(`{"queue_order":null}`); w.Code != http.StatusOK {
		t.Fatalf("clear: status = %d: %s", w.Code, w.Body.String())
	}
	dbfx.QueryRow(t, `SELECT queue_order FROM issue WHERE id = $1`, id).Scan(&order)
	if order != nil {
		t.Fatalf("queue_order after clear = %v, want NULL", *order)
	}
}

func queuePost(t *testing.T, fn http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	fn(w, req)
	return w
}

func TestQueueOrderBatchPreviewAndFrontier(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	blocker := dbfx.Issue(t, "queue blocker", testutil.Cols{"status": "in_progress", "number": 93021, "queue_order": 1})
	a := dbfx.Issue(t, "queue batch a", testutil.Cols{"status": "backlog", "number": 93022})
	b := dbfx.Issue(t, "queue batch b", testutil.Cols{"status": "backlog", "number": 93023})
	body := `{"issue_ids":["` + a + `","` + b + `"],"queue_order":2}`

	// The open order-1 issue blocks both, so nothing would start.
	w := queuePost(t, testHandler.PreviewQueueOrder, "/api/issues/queue-order/preview", body)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"release_now":0`) {
		t.Fatalf("preview while blocked: %d %s", w.Code, w.Body.String())
	}

	// Numbering them 1 as well ranks them level with the blocker: both start.
	w = queuePost(t, testHandler.PreviewQueueOrder, "/api/issues/queue-order/preview", `{"issue_ids":["`+a+`","`+b+`"],"queue_order":1}`)
	if !strings.Contains(w.Body.String(), `"release_now":2`) {
		t.Fatalf("preview level with blocker: %s", w.Body.String())
	}

	if w = queuePost(t, testHandler.BatchSetIssueQueueOrder, "/api/issues/batch-queue-order", body); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"updated":2`) {
		t.Fatalf("batch set: %d %s", w.Code, w.Body.String())
	}
	for _, id := range []string{a, b} {
		if got := queuedIssueStatus(t, id); got != "backlog" {
			t.Fatalf("issue %s status = %q, want backlog while blocked", id, got)
		}
	}

	// Unfinished numbers are listed ascending: the blocker's 1 and the batch's 2.
	fw := httptest.NewRecorder()
	freq := httptest.NewRequest(http.MethodGet, "/api/issues/queue-frontier", nil)
	freq.Header.Set("X-User-ID", testUserID)
	freq.Header.Set("X-Workspace-ID", testWorkspaceID)
	testHandler.ListQueueFrontiers(fw, freq)
	if fw.Code != http.StatusOK || !strings.Contains(fw.Body.String(), `"unfinished":[1,2]`) {
		t.Fatalf("unfinished: %d %s", fw.Code, fw.Body.String())
	}

	// Finishing the blocker releases both on the next pass.
	dbfx.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, blocker)
	if n := promoteQueue(t); n != 2 {
		t.Fatalf("moved %d issues after the blocker finished, want 2", n)
	}

	if w = queuePost(t, testHandler.BatchSetIssueQueueOrder, "/api/issues/batch-queue-order", `{"issue_ids":["`+a+`"],"queue_order":0}`); w.Code != http.StatusBadRequest {
		t.Fatalf("batch zero: %d", w.Code)
	}
}
