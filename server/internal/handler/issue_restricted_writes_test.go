package handler

// Visibility of restricted projects on the issue write paths (spec 7.1 rules 5,
// 6 and 7). The fixture is the one of project_restricted_visibility_test.go:
// the outsider is a plain member who is not granted the restricted project, the
// grantee is granted, the owner is testUserID.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/agent"
)

// issueRowSnapshot is the whole issue row as JSON, or "<deleted>" when the row
// is gone. Two equal snapshots mean a write changed nothing.
func issueRowSnapshot(t *testing.T, issueID string) string {
	t.Helper()
	var snapshot string
	if err := testPool.QueryRow(context.Background(),
		`SELECT coalesce((SELECT to_jsonb(i)::text FROM issue i WHERE i.id = $1), '<deleted>')`, issueID).Scan(&snapshot); err != nil {
		t.Fatalf("snapshot issue %s: %v", issueID, err)
	}
	return snapshot
}

// sameAnswer fails unless hidden and missing have the status and body of each
// other, apart from the id the caller asked about.
func sameAnswer(t *testing.T, name string, hidden, missing *httptest.ResponseRecorder, hiddenID, missingID string) {
	t.Helper()
	if hidden.Code != missing.Code ||
		strings.ReplaceAll(hidden.Body.String(), hiddenID, "ID") != strings.ReplaceAll(missing.Body.String(), missingID, "ID") {
		t.Errorf("%s: hidden answers %d %s, missing answers %d %s",
			name, hidden.Code, hidden.Body.String(), missing.Code, missing.Body.String())
	}
}

func issueIdentifierOf(t *testing.T, issueID string) string {
	t.Helper()
	prefix := testHandler.getIssuePrefix(context.Background(), parseUUID(testWorkspaceID))
	return prefix + "-" + strconv.Itoa(issueNumber(t, issueID))
}

// TestRestrictedIssueWritesAnswerMissing: for the outsider every write on an
// issue of a restricted project answers like the same write on an issue that
// does not exist, and changes nothing.
func TestRestrictedIssueWritesAnswerMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	seedTestCatalog(t)
	f := newRestrictedFixture(t)
	hiddenID := createIssueInProject(t, "hidden write target "+uuid.NewString()[:8], f.restrictedProjectID)
	// Backlog, so a queue preview that leaks would count it.
	dbfx.Exec(t, `UPDATE issue SET status = 'backlog' WHERE id = $1`, hiddenID)
	openID := createIssueInProject(t, "open write target "+uuid.NewString()[:8], f.workspaceProjectID)
	missingID := uuid.NewString()

	// Routes whose missing-issue answer is 404.
	single := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		path    string // {id} is replaced by the issue id
		body    any
	}{
		{"UpdateIssue title", testHandler.UpdateIssue, "PUT", "/api/issues/{id}", map[string]any{"title": "hacked"}},
		{"UpdateIssue status", testHandler.UpdateIssue, "PUT", "/api/issues/{id}", map[string]any{"status": "done"}},
		{"UpdateIssue description", testHandler.UpdateIssue, "PUT", "/api/issues/{id}", map[string]any{"description": "hacked"}},
		{"UpdateIssue project", testHandler.UpdateIssue, "PUT", "/api/issues/{id}", map[string]any{"project_id": f.workspaceProjectID}},
		{"DeleteIssue", testHandler.DeleteIssue, "DELETE", "/api/issues/{id}", nil},
		{"MoveIssue", testHandler.MoveIssue, "POST", "/api/issues/{id}/move", map[string]any{"before_id": nil, "after_id": nil, "status": "done"}},
		{"SetIssueMetadataKey", testHandler.SetIssueMetadataKey, "PUT", "/api/issues/{id}/metadata/k", map[string]any{"value": "v"}},
		{"DeleteIssueMetadataKey", testHandler.DeleteIssueMetadataKey, "DELETE", "/api/issues/{id}/metadata/k", nil},
		{"SetIssueRunQueue", testHandler.SetIssueRunQueue, "PUT", "/api/issues/{id}/run-queue", map[string]any{"queued": true}},
		{"PreviewWorkflowHandoff", testHandler.PreviewWorkflowHandoff, "GET", "/api/issues/{id}/workflow-handoff?status=in_progress", nil},
	}
	for _, route := range single {
		call := func(userID, id string) *httptest.ResponseRecorder {
			req := requestAs(userID, route.method, strings.ReplaceAll(route.path, "{id}", id), route.body)
			return serveAs(route.handler, withURLParams(req, "id", id, "key", "k"))
		}
		hidden, missing := call(f.outsiderID, hiddenID), call(f.outsiderID, missingID)
		if missing.Code != http.StatusNotFound {
			t.Fatalf("%s: a missing issue answers %d, want 404", route.name, missing.Code)
		}
		sameAnswer(t, route.name+" as outsider", hidden, missing, hiddenID, missingID)
	}
	before := issueRowSnapshot(t, hiddenID)

	// Batch routes skip an id they cannot find, so a hidden id is skipped the same way.
	batch := []struct {
		name    string
		handler http.HandlerFunc
		path    string
		body    func(id string) any
	}{
		{"BatchUpdateIssues", testHandler.BatchUpdateIssues, "/api/issues/batch-update", func(id string) any {
			return map[string]any{"issue_ids": []string{id}, "updates": map[string]any{"title": "hacked", "status": "done"}}
		}},
		{"BatchDeleteIssues", testHandler.BatchDeleteIssues, "/api/issues/batch-delete", func(id string) any {
			return map[string]any{"issue_ids": []string{id}}
		}},
		{"BatchSetRunQueue", testHandler.BatchSetRunQueue, "/api/issues/batch-run-queue", func(id string) any {
			return map[string]any{"issue_ids": []string{id}, "queued": true}
		}},
		{"PreviewRunQueue", testHandler.PreviewRunQueue, "/api/issues/run-queue/preview", func(id string) any {
			return map[string]any{"issue_ids": []string{id}, "queued": true}
		}},
	}
	for _, route := range batch {
		call := func(userID, id string) *httptest.ResponseRecorder {
			return serveAs(route.handler, requestAs(userID, "POST", route.path+"?workspace_id="+testWorkspaceID, route.body(id)))
		}
		hidden, missing := call(f.outsiderID, hiddenID), call(f.outsiderID, missingID)
		if missing.Code != http.StatusOK {
			t.Fatalf("%s: a missing id answers %d %s, want 200", route.name, missing.Code, missing.Body.String())
		}
		sameAnswer(t, route.name+" as outsider", hidden, missing, hiddenID, missingID)
	}
	if after := issueRowSnapshot(t, hiddenID); after != before {
		t.Errorf("a write of the outsider changed the hidden issue:\nbefore: %s\nafter:  %s", before, after)
	}

	// A write that names a hidden issue or project in a field of a visible issue.
	t.Run("parent", func(t *testing.T) {
		snapshot := issueRowSnapshot(t, openID)
		call := func(id string) *httptest.ResponseRecorder {
			return serveAs(testHandler.UpdateIssue, withURLParam(requestAs(f.outsiderID, "PUT", "/api/issues/"+openID,
				map[string]any{"parent_issue_id": id}), "id", openID))
		}
		sameAnswer(t, "UpdateIssue parent", call(hiddenID), call(missingID), hiddenID, missingID)
		batchCall := func(id string) *httptest.ResponseRecorder {
			return serveAs(testHandler.BatchUpdateIssues, requestAs(f.outsiderID, "POST", "/api/issues/batch-update?workspace_id="+testWorkspaceID,
				map[string]any{"issue_ids": []string{openID}, "updates": map[string]any{"parent_issue_id": id}}))
		}
		sameAnswer(t, "BatchUpdateIssues parent", batchCall(hiddenID), batchCall(missingID), hiddenID, missingID)
		if got := issueRowSnapshot(t, openID); got != snapshot {
			t.Errorf("the open issue changed: %s", got)
		}
	})
	t.Run("duplicate mark target", func(t *testing.T) {
		snapshot := issueRowSnapshot(t, openID)
		call := func(id string) *httptest.ResponseRecorder {
			return serveAs(testHandler.UpdateIssue, withURLParam(requestAs(f.outsiderID, "PUT", "/api/issues/"+openID,
				map[string]any{"duplicate_of_issue_id": id}), "id", openID))
		}
		missing := call(missingID)
		if missing.Code != http.StatusBadRequest {
			t.Fatalf("a missing duplicate target answers %d %s, want 400", missing.Code, missing.Body.String())
		}
		sameAnswer(t, "UpdateIssue duplicate mark", call(hiddenID), missing, hiddenID, missingID)
		if got := issueRowSnapshot(t, openID); got != snapshot {
			t.Errorf("the open issue changed: %s", got)
		}
	})
	t.Run("run queue wait", func(t *testing.T) {
		snapshot := issueRowSnapshot(t, openID)
		call := func(id string) *httptest.ResponseRecorder {
			return serveAs(testHandler.SetIssueRunQueue, withURLParam(requestAs(f.outsiderID, "PUT", "/api/issues/"+openID+"/run-queue",
				map[string]any{"queued": true, "after_issue_ids": []string{id}}), "id", openID))
		}
		missing := call(missingID)
		if missing.Code != http.StatusBadRequest {
			t.Fatalf("a missing wait answers %d %s, want 400", missing.Code, missing.Body.String())
		}
		sameAnswer(t, "SetIssueRunQueue wait", call(hiddenID), missing, hiddenID, missingID)
		batchCall := func(id string) *httptest.ResponseRecorder {
			return serveAs(testHandler.PreviewRunQueue, requestAs(f.outsiderID, "POST", "/api/issues/run-queue/preview?workspace_id="+testWorkspaceID,
				map[string]any{"issue_ids": []string{openID}, "queued": true, "after_issue_ids": []string{id}}))
		}
		sameAnswer(t, "PreviewRunQueue wait", batchCall(hiddenID), batchCall(missingID), hiddenID, missingID)
		if got := issueRowSnapshot(t, openID); got != snapshot {
			t.Errorf("the open issue changed: %s", got)
		}
	})
	t.Run("move anchor", func(t *testing.T) {
		call := func(id string) *httptest.ResponseRecorder {
			return serveAs(testHandler.MoveIssue, withURLParam(requestAs(f.outsiderID, "POST", "/api/issues/"+openID+"/move",
				map[string]any{"before_id": id, "after_id": nil}), "id", openID))
		}
		missing := call(missingID)
		if missing.Code == http.StatusOK {
			t.Fatalf("a missing anchor answers 200")
		}
		sameAnswer(t, "MoveIssue anchor", call(hiddenID), missing, hiddenID, missingID)
	})
	t.Run("destination project", func(t *testing.T) {
		snapshot := issueRowSnapshot(t, openID)
		update := serveAs(testHandler.UpdateIssue, withURLParam(requestAs(f.outsiderID, "PUT", "/api/issues/"+openID,
			map[string]any{"project_id": f.restrictedProjectID}), "id", openID))
		if update.Code != http.StatusNotFound || !strings.Contains(update.Body.String(), "project not found") {
			t.Errorf("UpdateIssue into a hidden project: want 404 project not found, got %d %s", update.Code, update.Body.String())
		}
		batchMove := serveAs(testHandler.BatchUpdateIssues, requestAs(f.outsiderID, "POST", "/api/issues/batch-update?workspace_id="+testWorkspaceID,
			map[string]any{"issue_ids": []string{openID}, "updates": map[string]any{"project_id": f.restrictedProjectID}}))
		if batchMove.Code != http.StatusNotFound || !strings.Contains(batchMove.Body.String(), "project not found") {
			t.Errorf("BatchUpdateIssues into a hidden project: want 404 project not found, got %d %s", batchMove.Code, batchMove.Body.String())
		}
		if got := issueRowSnapshot(t, openID); got != snapshot {
			t.Errorf("the open issue moved: %s", got)
		}
	})

	// The same writes work for the people who see the project.
	grantee := serveAs(testHandler.UpdateIssue, withURLParam(requestAs(f.granteeID, "PUT", "/api/issues/"+hiddenID,
		map[string]any{"title": "renamed by grantee"}), "id", hiddenID))
	if grantee.Code != http.StatusOK {
		t.Errorf("UpdateIssue as grantee: want 200, got %d %s", grantee.Code, grantee.Body.String())
	}
}

// TestRestrictedIssueWritesDoNotNameHiddenProject: the draft brief of a
// workflow names the project it is for, so a hidden project reads as a missing
// one, and the preview of a close names no sub-issue the caller cannot see.
func TestRestrictedIssueWritesDoNotNameHiddenProject(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	seedTestCatalog(t)
	f := newRestrictedFixture(t)
	var restrictedTitle string
	dbfx.QueryRow(t, `SELECT title FROM project WHERE id = $1`, f.restrictedProjectID).Scan(&restrictedTitle)
	workflowID := projectWorkflowID(t, f.workspaceProjectID)

	t.Run("PreviewIssueWorkflowBrief", func(t *testing.T) {
		preview := func(userID, projectID string) *httptest.ResponseRecorder {
			req := withURLParam(requestAs(userID, "POST", "/api/issue-workflows/"+workflowID+"/brief?workspace_id="+testWorkspaceID, map[string]any{
				"status_key": "in_review", "handler": map[string]any{"type": "member", "id": userID}, "project_id": projectID,
			}), "id", workflowID)
			return serveAs(testHandler.PreviewIssueWorkflowBrief, req)
		}
		hidden, missing := preview(f.outsiderID, f.restrictedProjectID), preview(f.outsiderID, uuid.NewString())
		if missing.Code != http.StatusOK {
			t.Fatalf("a missing project answers %d %s, want 200", missing.Code, missing.Body.String())
		}
		if strings.Contains(hidden.Body.String(), restrictedTitle) {
			t.Errorf("brief for the outsider names the hidden project: %s", hidden.Body.String())
		}
		if owner := preview(testUserID, f.restrictedProjectID); !strings.Contains(owner.Body.String(), restrictedTitle) {
			t.Errorf("brief for the owner lacks the project name, so the fixture does not reach this read: %s", owner.Body.String())
		}
	})

	t.Run("PreviewWorkflowHandoff project", func(t *testing.T) {
		openID := createIssueInProject(t, "handoff open "+uuid.NewString()[:8], f.workspaceProjectID)
		preview := func(userID, projectID string) *httptest.ResponseRecorder {
			req := withURLParam(requestAs(userID, "GET", "/api/issues/"+openID+"/workflow-handoff?project_id="+projectID, nil), "id", openID)
			return serveAs(testHandler.PreviewWorkflowHandoff, req)
		}
		missingProjectID := uuid.NewString()
		missing := preview(f.outsiderID, missingProjectID)
		if missing.Code != http.StatusNotFound {
			t.Fatalf("a missing project answers %d, want 404", missing.Code)
		}
		sameAnswer(t, "PreviewWorkflowHandoff project", preview(f.outsiderID, f.restrictedProjectID), missing, f.restrictedProjectID, missingProjectID)
		if owner := preview(testUserID, f.restrictedProjectID); owner.Code != http.StatusOK {
			t.Errorf("owner preview into the restricted project: want 200, got %d %s", owner.Code, owner.Body.String())
		}
	})

	// An agent that acts for the outsider closes a visible parent whose open
	// sub-issue is hidden: the refusal, if any, names nothing of the sub-issue.
	t.Run("openSubIssuesBlockingClose", func(t *testing.T) {
		agentID := seededReadyAgentID(t)
		parentID := dbfx.Issue(t, "close parent "+uuid.NewString()[:8], testutil.Cols{
			"project_id": f.workspaceProjectID, "status": "in_progress", "assignee_type": "agent", "assignee_id": agentID,
		})
		childTitle := "secret sub-issue " + uuid.NewString()[:8]
		childID := dbfx.Issue(t, childTitle, testutil.Cols{
			"project_id": f.restrictedProjectID, "parent_issue_id": parentID, "status": "backlog",
		})
		var runtimeID string
		dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID)
		taskID := dbfx.Task(t, agentID, testutil.Cols{
			"issue_id": parentID, "runtime_id": runtimeID, "status": "running", "started_at": time.Now(),
		})
		req := withURLParam(requestAs(f.outsiderID, "PUT", "/api/issues/"+parentID, map[string]any{"status": "done"}), "id", parentID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		w := serveAs(testHandler.UpdateIssue, req)
		// A sub-issue the caller cannot see does not keep the parent open.
		if w.Code == http.StatusConflict {
			t.Errorf("a hidden open sub-issue blocked the close: %d %s", w.Code, w.Body.String())
		}
		for _, secret := range []string{issueIdentifierOf(t, childID), childTitle, childID} {
			if strings.Contains(w.Body.String(), secret) {
				t.Errorf("close answer %d names the hidden sub-issue (%q): %s", w.Code, secret, w.Body.String())
			}
		}
	})
}

// TestCreateIssueChecksCreatorAndProject: an outsider cannot file an issue in a
// restricted project, or under a parent in one; the grantee can.
func TestCreateIssueChecksCreatorAndProject(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	hiddenParentID := createIssueInProject(t, "hidden parent "+uuid.NewString()[:8], f.restrictedProjectID)
	create := func(userID string, body map[string]any) *httptest.ResponseRecorder {
		body["status"], body["priority"] = "todo", "none"
		w := serveAs(testHandler.CreateIssue, requestAs(userID, "POST", "/api/issues?workspace_id="+testWorkspaceID, body))
		if w.Code == http.StatusCreated {
			var issue IssueResponse
			json.Unmarshal(w.Body.Bytes(), &issue)
			t.Cleanup(func() { deleteTestIssue(t, issue.ID) })
		}
		return w
	}
	countTitled := func(title string) int {
		var n int
		dbfx.QueryRow(t, `SELECT count(*) FROM issue WHERE workspace_id = $1 AND title = $2`, testWorkspaceID, title).Scan(&n)
		return n
	}

	title := "outsider create " + uuid.NewString()[:8]
	w := create(f.outsiderID, map[string]any{"title": title, "project_id": f.restrictedProjectID})
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "project not found") {
		t.Errorf("create in a hidden project as outsider: want 404 project not found, got %d %s", w.Code, w.Body.String())
	}
	if n := countTitled(title); n != 0 {
		t.Errorf("the refused create left %d issue(s)", n)
	}

	missingParentID := uuid.NewString()
	parentCall := func(parentID string) *httptest.ResponseRecorder {
		return create(f.outsiderID, map[string]any{"title": "outsider child " + uuid.NewString()[:8], "parent_issue_id": parentID})
	}
	missing := parentCall(missingParentID)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("a missing parent answers %d %s, want 400", missing.Code, missing.Body.String())
	}
	sameAnswer(t, "CreateIssue under a hidden parent", parentCall(hiddenParentID), missing, hiddenParentID, missingParentID)
	// The same, when the child picks an agent assignee: that path loads the parent first.
	agentParentCall := func(parentID string) *httptest.ResponseRecorder {
		return create(f.outsiderID, map[string]any{
			"title": "outsider agent child " + uuid.NewString()[:8], "parent_issue_id": parentID,
			"assignee_type": "agent", "assignee_id": seededReadyAgentID(t),
		})
	}
	sameAnswer(t, "CreateIssue under a hidden parent with an agent assignee", agentParentCall(hiddenParentID), agentParentCall(missingParentID), hiddenParentID, missingParentID)

	for who, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
		if w := create(userID, map[string]any{"title": who + " create " + uuid.NewString()[:8], "project_id": f.restrictedProjectID}); w.Code != http.StatusCreated {
			t.Errorf("create in the restricted project as %s: want 201, got %d %s", who, w.Code, w.Body.String())
		}
		if w := create(userID, map[string]any{"title": who + " child " + uuid.NewString()[:8], "parent_issue_id": hiddenParentID}); w.Code != http.StatusCreated {
			t.Errorf("create under the restricted parent as %s: want 201, got %d %s", who, w.Code, w.Body.String())
		}
	}
	if w := create(f.outsiderID, map[string]any{"title": "outsider open " + uuid.NewString()[:8], "project_id": f.workspaceProjectID}); w.Code != http.StatusCreated {
		t.Errorf("create in a workspace project as outsider: want 201, got %d %s", w.Code, w.Body.String())
	}
}

// TestQuickCreateIssueRespectsProject: a quick-create task pins the project and
// parent the requester names, so it needs the same visibility as a create.
func TestQuickCreateIssueRespectsProject(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	seedTestCatalog(t)
	f := newRestrictedFixture(t)
	hiddenParentID := createIssueInProject(t, "quick hidden parent "+uuid.NewString()[:8], f.restrictedProjectID)
	agentID := seededReadyAgentID(t)
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID)
	dbfx.Exec(t, `UPDATE agent_runtime SET metadata = jsonb_build_object('cli_version', $1::text) WHERE id = $2`,
		agent.MinQuickCreateFieldsCLIVersion, runtimeID)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE agent_runtime SET metadata = '{}'::jsonb WHERE id = $1`, runtimeID)
	})
	countTasks := func() int {
		var n int
		dbfx.QueryRow(t, `SELECT count(*) FROM agent_task_queue WHERE agent_id = $1 AND context->>'type' = 'quick_create'`, agentID).Scan(&n)
		return n
	}
	quick := func(userID string, extra map[string]any) *httptest.ResponseRecorder {
		body := map[string]any{"agent_id": agentID, "prompt": "file a follow-up"}
		for k, v := range extra {
			body[k] = v
		}
		w := serveAs(testHandler.QuickCreateIssue, requestAs(userID, "POST", "/api/issues/quick-create?workspace_id="+testWorkspaceID, body))
		if w.Code == http.StatusAccepted {
			var resp QuickCreateIssueResponse
			json.Unmarshal(w.Body.Bytes(), &resp)
			t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE id = $1`, resp.TaskID) })
		}
		return w
	}

	// Proof that the fixture reaches the project check: the grantee and the owner are accepted.
	for who, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
		if w := quick(userID, map[string]any{"project_id": f.restrictedProjectID}); w.Code != http.StatusAccepted {
			t.Fatalf("quick-create into the restricted project as %s: want 202, got %d %s", who, w.Code, w.Body.String())
		}
	}

	before := countTasks()
	w := quick(f.outsiderID, map[string]any{"project_id": f.restrictedProjectID})
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "project not found") {
		t.Errorf("quick-create into a hidden project as outsider: want 404 project not found, got %d %s", w.Code, w.Body.String())
	}
	missingParentID := uuid.NewString()
	missing := quick(f.outsiderID, map[string]any{"parent_issue_id": missingParentID})
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("a missing parent answers %d %s, want 400", missing.Code, missing.Body.String())
	}
	sameAnswer(t, "QuickCreateIssue under a hidden parent", quick(f.outsiderID, map[string]any{"parent_issue_id": hiddenParentID}), missing, hiddenParentID, missingParentID)
	if after := countTasks(); after != before {
		t.Errorf("a refused quick-create enqueued %d task(s)", after-before)
	}
}

// TestSubscriberMustSeeProject: a member who does not see a restricted project
// cannot be added as a watcher of its issues.
func TestSubscriberMustSeeProject(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	issueID := createIssueInProject(t, "watch target "+uuid.NewString()[:8], f.restrictedProjectID)
	subscribe := func(userID string) *httptest.ResponseRecorder {
		req := withURLParam(requestAs(testUserID, "POST", "/api/issues/"+issueID+"/subscribe",
			map[string]any{"user_id": userID, "user_type": "member"}), "id", issueID)
		return serveAs(testHandler.SubscribeToIssue, req)
	}
	watching := func(userID string) bool {
		var n int
		dbfx.QueryRow(t, `SELECT count(*) FROM issue_subscriber WHERE issue_id = $1 AND user_type = 'member' AND user_id = $2 AND unsubscribed_at IS NULL`, issueID, userID).Scan(&n)
		return n > 0
	}

	if w := subscribe(f.outsiderID); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "project_member_invalid") {
		t.Errorf("subscribe an outsider: want 400 project_member_invalid, got %d %s", w.Code, w.Body.String())
	}
	if watching(f.outsiderID) {
		t.Errorf("the refused subscribe left a watcher row")
	}
	if w := subscribe(f.granteeID); w.Code != http.StatusOK || !watching(f.granteeID) {
		t.Errorf("subscribe the grantee: want 200 and a watcher row, got %d %s", w.Code, w.Body.String())
	}
	// A workspace-project issue takes any member.
	openID := createIssueInProject(t, "watch open "+uuid.NewString()[:8], f.workspaceProjectID)
	req := withURLParam(requestAs(testUserID, "POST", "/api/issues/"+openID+"/subscribe",
		map[string]any{"user_id": f.outsiderID, "user_type": "member"}), "id", openID)
	if w := serveAs(testHandler.SubscribeToIssue, req); w.Code != http.StatusOK {
		t.Errorf("subscribe an outsider to a workspace-project issue: want 200, got %d %s", w.Code, w.Body.String())
	}
}

// TestUpdateIssueMovesDropWatchers: moving an issue into a restricted project
// drops the assignee and the watchers who cannot see it, in the same request;
// moving it out of one keeps everyone.
func TestUpdateIssueMovesDropWatchers(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	watch := func(issueID string, userIDs ...string) {
		for _, userID := range userIDs {
			dbfx.Exec(t, `INSERT INTO issue_subscriber (issue_id, user_type, user_id, reason) VALUES ($1, 'member', $2, 'manual')`, issueID, userID)
		}
	}
	watchers := func(issueID string) map[string]bool {
		rows, err := testPool.Query(context.Background(), `SELECT user_id::text FROM issue_subscriber WHERE issue_id = $1 AND user_type = 'member' AND unsubscribed_at IS NULL`, issueID)
		if err != nil {
			t.Fatalf("list watchers: %v", err)
		}
		defer rows.Close()
		out := map[string]bool{}
		for rows.Next() {
			var id string
			rows.Scan(&id)
			out[id] = true
		}
		return out
	}
	assignee := func(issueID string) string {
		var id *string
		dbfx.QueryRow(t, `SELECT assignee_id::text FROM issue WHERE id = $1`, issueID).Scan(&id)
		if id == nil {
			return ""
		}
		return *id
	}
	newOpenIssue := func(title string) string {
		issueID := createIssueInProject(t, title+" "+uuid.NewString()[:8], f.workspaceProjectID)
		dbfx.Exec(t, `UPDATE issue SET assignee_type = 'member', assignee_id = $2 WHERE id = $1`, issueID, f.outsiderID)
		watch(issueID, f.outsiderID, f.granteeID)
		return issueID
	}
	checkMoved := func(t *testing.T, issueID string) {
		t.Helper()
		got := watchers(issueID)
		if got[f.outsiderID] {
			t.Errorf("the outsider still watches the moved issue")
		}
		if !got[f.granteeID] {
			t.Errorf("the grantee lost the watch on the moved issue")
		}
		if id := assignee(issueID); id != "" {
			t.Errorf("the outsider still holds the moved issue: assignee %s", id)
		}
	}

	t.Run("update into a restricted project", func(t *testing.T) {
		issueID := newOpenIssue("move single")
		w := serveAs(testHandler.UpdateIssue, withURLParam(requestAs(testUserID, "PUT", "/api/issues/"+issueID,
			map[string]any{"project_id": f.restrictedProjectID}), "id", issueID))
		if w.Code != http.StatusOK {
			t.Fatalf("move: %d %s", w.Code, w.Body.String())
		}
		checkMoved(t, issueID)
	})
	t.Run("batch update into a restricted project", func(t *testing.T) {
		issueID := newOpenIssue("move batch")
		w := serveAs(testHandler.BatchUpdateIssues, requestAs(testUserID, "POST", "/api/issues/batch-update?workspace_id="+testWorkspaceID,
			map[string]any{"issue_ids": []string{issueID}, "updates": map[string]any{"project_id": f.restrictedProjectID}}))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"updated":1`) {
			t.Fatalf("batch move: %d %s", w.Code, w.Body.String())
		}
		checkMoved(t, issueID)
	})
	t.Run("update out of a restricted project", func(t *testing.T) {
		issueID := createIssueInProject(t, "move out "+uuid.NewString()[:8], f.restrictedProjectID)
		watch(issueID, f.granteeID)
		w := serveAs(testHandler.UpdateIssue, withURLParam(requestAs(testUserID, "PUT", "/api/issues/"+issueID,
			map[string]any{"project_id": f.workspaceProjectID}), "id", issueID))
		if w.Code != http.StatusOK {
			t.Fatalf("move out: %d %s", w.Code, w.Body.String())
		}
		if !watchers(issueID)[f.granteeID] {
			t.Errorf("moving into a workspace project dropped a watcher who sees it")
		}
	})
}

// TestBatchTriageLocksRespectVisibility: a hidden issue in Triage does not
// refuse a batch that sets a parent: the batch skips it like a missing id.
func TestBatchTriageLocksRespectVisibility(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	hiddenID := createIssueInProject(t, "hidden triage "+uuid.NewString()[:8], f.restrictedProjectID)
	dbfx.Exec(t, `UPDATE issue SET triage_state = 'pending' WHERE id = $1`, hiddenID)
	openTriageID := createIssueInProject(t, "open triage "+uuid.NewString()[:8], f.workspaceProjectID)
	dbfx.Exec(t, `UPDATE issue SET triage_state = 'pending' WHERE id = $1`, openTriageID)
	parentID := createIssueInProject(t, "triage parent "+uuid.NewString()[:8], f.workspaceProjectID)
	missingID := uuid.NewString()

	// The guard on its own: it lets the batch go on when the only triage entry
	// is one the caller cannot see, and refuses it for an entry the caller sees.
	rawUpdates := map[string]json.RawMessage{"parent_issue_id": json.RawMessage(strconv.Quote(parentID))}
	guard := func(userID, id string) (bool, *httptest.ResponseRecorder) {
		w := httptest.NewRecorder()
		req := requestAs(userID, "POST", "/api/issues/batch-update?workspace_id="+testWorkspaceID, nil)
		return testHandler.validateBatchTriageLocks(w, req, parseUUID(testWorkspaceID), []string{id}, rawUpdates), w
	}
	missingOK, missing := guard(f.outsiderID, missingID)
	hiddenOK, hidden := guard(f.outsiderID, hiddenID)
	if !missingOK || !hiddenOK || hidden.Body.String() != missing.Body.String() {
		t.Errorf("hidden triage entry: ok=%v body=%q; missing id: ok=%v body=%q", hiddenOK, hidden.Body.String(), missingOK, missing.Body.String())
	}
	if ok, w := guard(f.outsiderID, openTriageID); ok || w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "issue_in_triage") {
		t.Errorf("a visible triage entry: want 400 issue_in_triage, got ok=%v %d %s", ok, w.Code, w.Body.String())
	}
	if ok, w := guard(testUserID, hiddenID); ok || w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "issue_in_triage") {
		t.Errorf("a hidden triage entry as owner: want 400 issue_in_triage, got ok=%v %d %s", ok, w.Code, w.Body.String())
	}
}

// TestSystemWakeupsOmitHiddenIssues: the child-done rule of a visible parent
// counts and names only the sub-issues the caller sees, and the workspace count
// of customized rules leaves out hidden issues.
func TestSystemWakeupsOmitHiddenIssues(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	createChild := func(parentID, title, projectID string) IssueResponse {
		t.Helper()
		w := httptest.NewRecorder()
		testHandler.CreateIssue(w, newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
			"title": title, "status": "in_progress", "parent_issue_id": parentID, "project_id": projectID,
		}))
		if w.Code != http.StatusCreated {
			t.Fatalf("create sub-issue: %d %s", w.Code, w.Body.String())
		}
		var child IssueResponse
		json.Unmarshal(w.Body.Bytes(), &child)
		t.Cleanup(func() { cleanupChildDoneIssue(child.ID) })
		return child
	}
	parentID := createIssueInProject(t, "wakeup parent "+uuid.NewString()[:8], f.workspaceProjectID)
	t.Cleanup(func() { cleanupChildDoneIssue(parentID) })
	visible := createChild(parentID, "wakeup visible child "+uuid.NewString()[:8], f.workspaceProjectID)
	hidden := createChild(parentID, "wakeup hidden child "+uuid.NewString()[:8], f.restrictedProjectID)

	rulesAs := func(userID string) []systemWakeupResponse {
		w := serveAs(testHandler.ListIssueSystemWakeups, withURLParam(requestAs(userID, "GET", "/api/issues/"+parentID+"/system-wakeups", nil), "id", parentID))
		if w.Code != http.StatusOK {
			t.Fatalf("ListIssueSystemWakeups as %s: %d %s", userID, w.Code, w.Body.String())
		}
		var rules []systemWakeupResponse
		json.Unmarshal(w.Body.Bytes(), &rules)
		return rules
	}
	owner, outsider := rulesAs(testUserID), rulesAs(f.outsiderID)
	if len(owner) != 1 || owner[0].Total != 2 || owner[0].Remaining != 2 {
		t.Fatalf("the owner's rule does not count both sub-issues: %+v", owner)
	}
	if len(outsider) != 1 || outsider[0].Total != 1 || outsider[0].Remaining != 1 ||
		len(outsider[0].Waiting) != 1 || outsider[0].Waiting[0] != visible.Identifier {
		t.Errorf("the outsider's rule must count and name only the visible sub-issue %s (hidden %s): %+v", visible.Identifier, hidden.Identifier, outsider)
	}

	// Workspace default: customized counts open issues whose rule a person changed.
	hiddenParentID := createIssueInProject(t, "wakeup hidden parent "+uuid.NewString()[:8], f.restrictedProjectID)
	t.Cleanup(func() { cleanupChildDoneIssue(hiddenParentID) })
	createChild(hiddenParentID, "wakeup child of hidden "+uuid.NewString()[:8], f.restrictedProjectID)
	if w := putChildDoneRule(t, hiddenParentID, map[string]any{"enabled": true, "instruction": "custom"}); w.Code != http.StatusOK {
		t.Fatalf("customize the hidden parent's rule: %d %s", w.Code, w.Body.String())
	}
	customized := func(userID string) int64 {
		w := serveAs(testHandler.ListWorkspaceSystemWakeups, requestAs(userID, "GET", "/api/system-wakeups?workspace_id="+testWorkspaceID, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("ListWorkspaceSystemWakeups as %s: %d %s", userID, w.Code, w.Body.String())
		}
		var rules []workspaceSystemWakeupResponse
		json.Unmarshal(w.Body.Bytes(), &rules)
		if len(rules) != 1 {
			t.Fatalf("workspace rules: %s", w.Body.String())
		}
		return rules[0].Customized
	}
	if o, p := customized(testUserID), customized(f.outsiderID); o-p != 1 {
		t.Errorf("customized rules: owner %d, outsider %d; want the outsider to miss exactly the hidden parent", o, p)
	}
}

// TestRunQueueWaitsIgnoreHiddenIssues: the cycle check follows no path through
// an issue the caller cannot see, so it cannot tell the caller that a hidden
// issue waits on a visible one.
func TestRunQueueWaitsIgnoreHiddenIssues(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	a := createIssueInProject(t, "queue a "+uuid.NewString()[:8], f.workspaceProjectID)
	b := createIssueInProject(t, "queue b "+uuid.NewString()[:8], f.workspaceProjectID)
	hidden := createIssueInProject(t, "queue hidden "+uuid.NewString()[:8], f.restrictedProjectID)
	// b waits on hidden, and hidden waits on a: queueing a behind b closes a loop.
	dbfx.Exec(t, `UPDATE issue SET run_queued = true, run_after_issue_ids = ARRAY[$2::uuid] WHERE id = $1`, b, hidden)
	dbfx.Exec(t, `UPDATE issue SET run_queued = true, run_after_issue_ids = ARRAY[$2::uuid] WHERE id = $1`, hidden, a)

	queueAfterB := func(userID string) *httptest.ResponseRecorder {
		return serveAs(testHandler.SetIssueRunQueue, withURLParam(requestAs(userID, "PUT", "/api/issues/"+a+"/run-queue",
			map[string]any{"queued": true, "after_issue_ids": []string{b}}), "id", a))
	}
	if w := queueAfterB(testUserID); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "waits on it") {
		t.Fatalf("owner: a loop through the hidden issue must be refused, got %d %s", w.Code, w.Body.String())
	}
	if w := queueAfterB(f.outsiderID); w.Code != http.StatusOK {
		t.Errorf("outsider: the loop runs through a hidden issue and must not be reported, got %d %s", w.Code, w.Body.String())
	}
}

// TestBatchUpdateAssigneeMustSeeProject: a batch that assigns a restricted
// project's issue to a member who cannot see the project skips that issue, like
// the single update refuses it.
func TestBatchUpdateAssigneeMustSeeProject(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	issueID := createIssueInProject(t, "batch assign "+uuid.NewString()[:8], f.restrictedProjectID)
	assign := func(userID string) string {
		w := serveAs(testHandler.BatchUpdateIssues, requestAs(testUserID, "POST", "/api/issues/batch-update?workspace_id="+testWorkspaceID,
			map[string]any{"issue_ids": []string{issueID}, "updates": map[string]any{"assignee_type": "member", "assignee_id": userID}}))
		return w.Body.String()
	}
	if got := assign(f.outsiderID); got != `{"updated":0}`+"\n" && got != `{"updated":0}` {
		t.Errorf("assign the outsider: want updated 0, got %s", got)
	}
	var assignee *string
	dbfx.QueryRow(t, `SELECT assignee_id::text FROM issue WHERE id = $1`, issueID).Scan(&assignee)
	if assignee != nil {
		t.Errorf("the outsider holds the issue: %s", *assignee)
	}
	if got := assign(f.granteeID); !strings.Contains(got, `"updated":1`) {
		t.Errorf("assign the grantee: want updated 1, got %s", got)
	}
}
