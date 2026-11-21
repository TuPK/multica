package handler

// Derived rows follow the issue or project they belong to (spec 7.1 rule 4).
// A member who cannot see an issue must not read, change or count its comments,
// attachments, activity, reactions, subscribers, inbox items, unread marks or
// agent tasks. The fixture is the one of project_restricted_visibility_test.go:
// a restricted project, a workspace project, an outsider and a grantee.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// memberCtx puts the workspace on the request the way the workspace middleware
// does, for handlers that read it from the context.
func memberCtx(req *http.Request) *http.Request {
	return req.WithContext(middleware.SetMemberContext(req.Context(), testWorkspaceID, db.Member{}))
}

func serveWorkspaceAs(handler http.HandlerFunc, userID, method, path string, body any) *httptest.ResponseRecorder {
	return serveAs(handler, memberCtx(requestAs(userID, method, path, body)))
}

func inboxItemFor(t *testing.T, userID, issueID, itemType string, archived bool) string {
	t.Helper()
	return dbfx.Insert(t, "inbox_item", testutil.Cols{
		"workspace_id": testWorkspaceID, "recipient_type": "member", "recipient_id": userID,
		"type": itemType, "severity": "info", "issue_id": issueID,
		"title": "derived " + uuid.NewString()[:8], "archived": archived,
	})
}

func subscribeRow(t *testing.T, issueID, userID string) {
	t.Helper()
	dbfx.Exec(t, `INSERT INTO issue_subscriber (issue_id, user_type, user_id, reason) VALUES ($1, 'member', $2, 'manual')`, issueID, userID)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue_subscriber WHERE issue_id = $1 AND user_id = $2`, issueID, userID)
	})
}

func idsIn(t *testing.T, body string, key string) map[string]bool {
	t.Helper()
	var wrapper map[string][]string
	if err := json.Unmarshal([]byte(body), &wrapper); err != nil {
		t.Fatalf("decode %s: %v: %s", key, err, body)
	}
	out := map[string]bool{}
	for _, id := range wrapper[key] {
		out[id] = true
	}
	return out
}

// TestDerivedRowsFollowIssueVisibility has one subtest per derived table of the
// read-path inventory. In each, the outsider meets the hidden issue's rows like
// rows of a missing issue, while the grantee and the owner read them.
func TestDerivedRowsFollowIssueVisibility(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	hiddenID := createIssueInProject(t, "derived hidden "+uuid.NewString()[:8], f.restrictedProjectID)
	openID := createIssueInProject(t, "derived open "+uuid.NewString()[:8], f.workspaceProjectID)
	missingID := uuid.NewString()
	issueRead := func(handler http.HandlerFunc, userID, id, path string) *httptest.ResponseRecorder {
		req := requestAs(userID, "GET", strings.Replace(path, "%s", id, 1), nil)
		return serveAs(handler, withURLParam(req, "id", id))
	}

	t.Run("comment", func(t *testing.T) {
		commentID := dbfx.Comment(t, hiddenID, "hidden comment text")
		missingComment := uuid.NewString()
		// Every route that finds a comment by its own id.
		byID := map[string]func(userID, id string) *httptest.ResponseRecorder{
			"UpdateComment": func(userID, id string) *httptest.ResponseRecorder {
				req := requestAs(userID, "PUT", "/api/comments/"+id, map[string]any{"content": "changed"})
				return serveAs(testHandler.UpdateComment, withURLParam(req, "commentId", id))
			},
			"DeleteComment": func(userID, id string) *httptest.ResponseRecorder {
				req := requestAs(userID, "DELETE", "/api/comments/"+id, nil)
				return serveAs(testHandler.DeleteComment, withURLParam(req, "commentId", id))
			},
			"ResolveComment": func(userID, id string) *httptest.ResponseRecorder {
				req := requestAs(userID, "POST", "/api/comments/"+id+"/resolve", nil)
				return serveAs(testHandler.ResolveComment, withURLParam(req, "commentId", id))
			},
			"UnresolveComment": func(userID, id string) *httptest.ResponseRecorder {
				req := requestAs(userID, "DELETE", "/api/comments/"+id+"/resolve", nil)
				return serveAs(testHandler.UnresolveComment, withURLParam(req, "commentId", id))
			},
			"AddReaction": func(userID, id string) *httptest.ResponseRecorder {
				req := requestAs(userID, "POST", "/api/comments/"+id+"/reactions", map[string]any{"emoji": "👍"})
				return serveAs(testHandler.AddReaction, withURLParam(req, "commentId", id))
			},
			"RemoveReaction": func(userID, id string) *httptest.ResponseRecorder {
				req := requestAs(userID, "DELETE", "/api/comments/"+id+"/reactions", map[string]any{"emoji": "👍"})
				return serveAs(testHandler.RemoveReaction, withURLParam(req, "commentId", id))
			},
		}
		for name, call := range byID {
			missing := call(f.outsiderID, missingComment)
			if missing.Code != http.StatusNotFound {
				t.Fatalf("%s: a missing comment answers %d, want 404", name, missing.Code)
			}
			sameAnswer(t, name+" as outsider", call(f.outsiderID, commentID), missing, commentID, missingComment)
		}
		var content string
		dbfx.QueryRow(t, `SELECT content FROM comment WHERE id = $1`, commentID).Scan(&content)
		if content != "hidden comment text" {
			t.Errorf("an outsider's request changed the comment: %q", content)
		}
		if n := dbfx.Count(t, `SELECT count(*) FROM comment_reaction WHERE comment_id = $1`, commentID); n != 0 {
			t.Errorf("an outsider's reaction was stored: %d rows", n)
		}

		list := func(userID, id string) *httptest.ResponseRecorder {
			return issueRead(testHandler.ListComments, userID, id, "/api/issues/%s/comments")
		}
		sameAnswer(t, "ListComments as outsider", list(f.outsiderID, hiddenID), list(f.outsiderID, missingID), hiddenID, missingID)
		for who, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
			w := list(userID, hiddenID)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "hidden comment text") {
				t.Errorf("ListComments as %s: %d %s", who, w.Code, w.Body.String())
			}
		}
		react := byID["AddReaction"](f.granteeID, commentID)
		if react.Code != http.StatusCreated && react.Code != http.StatusOK {
			t.Errorf("AddReaction as grantee: %d %s", react.Code, react.Body.String())
		}
	})

	t.Run("attachment", func(t *testing.T) {
		att := insertIssueAttachment(t, hiddenID, "", testUserID)
		list := func(userID, id string) *httptest.ResponseRecorder {
			return issueRead(testHandler.ListAttachments, userID, id, "/api/issues/%s/attachments?workspace_id="+testWorkspaceID)
		}
		sameAnswer(t, "ListAttachments as outsider", list(f.outsiderID, hiddenID), list(f.outsiderID, missingID), hiddenID, missingID)
		if w := list(f.granteeID, hiddenID); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), att) {
			t.Errorf("ListAttachments as grantee: %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("activity_log", func(t *testing.T) {
		dbfx.Insert(t, "activity_log", testutil.Cols{
			"workspace_id": testWorkspaceID, "issue_id": hiddenID, "actor_type": "member", "actor_id": testUserID,
			"action": "description_updated", "details": testutil.Raw(`'{"marker":"hidden-activity"}'::jsonb`),
		})
		timeline := func(userID, id string) *httptest.ResponseRecorder {
			return issueRead(testHandler.ListTimeline, userID, id, "/api/issues/%s/timeline")
		}
		sameAnswer(t, "ListTimeline as outsider", timeline(f.outsiderID, hiddenID), timeline(f.outsiderID, missingID), hiddenID, missingID)
		if w := timeline(f.granteeID, hiddenID); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "hidden-activity") {
			t.Errorf("ListTimeline as grantee: %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("issue_reaction", func(t *testing.T) {
		add := func(userID, id string) *httptest.ResponseRecorder {
			req := requestAs(userID, "POST", "/api/issues/"+id+"/reactions", map[string]any{"emoji": "🎉"})
			return serveAs(testHandler.AddIssueReaction, withURLParam(req, "id", id))
		}
		sameAnswer(t, "AddIssueReaction as outsider", add(f.outsiderID, hiddenID), add(f.outsiderID, missingID), hiddenID, missingID)
		if n := dbfx.Count(t, `SELECT count(*) FROM issue_reaction WHERE issue_id = $1`, hiddenID); n != 0 {
			t.Errorf("an outsider's issue reaction was stored: %d rows", n)
		}
		if w := add(f.granteeID, hiddenID); w.Code >= 300 {
			t.Errorf("AddIssueReaction as grantee: %d %s", w.Code, w.Body.String())
		}
		remove := func(userID, id string) *httptest.ResponseRecorder {
			req := requestAs(userID, "DELETE", "/api/issues/"+id+"/reactions", map[string]any{"emoji": "🎉"})
			return serveAs(testHandler.RemoveIssueReaction, withURLParam(req, "id", id))
		}
		sameAnswer(t, "RemoveIssueReaction as outsider", remove(f.outsiderID, hiddenID), remove(f.outsiderID, missingID), hiddenID, missingID)
		if n := dbfx.Count(t, `SELECT count(*) FROM issue_reaction WHERE issue_id = $1`, hiddenID); n != 1 {
			t.Errorf("the grantee's reaction count = %d, want 1", n)
		}
	})

	t.Run("subscriber", func(t *testing.T) {
		list := func(userID, id string) *httptest.ResponseRecorder {
			return issueRead(testHandler.ListIssueSubscribers, userID, id, "/api/issues/%s/subscribers")
		}
		sameAnswer(t, "ListIssueSubscribers as outsider", list(f.outsiderID, hiddenID), list(f.outsiderID, missingID), hiddenID, missingID)
		if w := list(f.granteeID, hiddenID); w.Code != http.StatusOK {
			t.Errorf("ListIssueSubscribers as grantee: %d %s", w.Code, w.Body.String())
		}

		// The outsider subscribed while the project was open and lost access
		// since: the subscription list must not name the hidden issue.
		subscribeRow(t, hiddenID, f.outsiderID)
		subscribeRow(t, openID, f.outsiderID)
		subscribeRow(t, hiddenID, f.granteeID)
		subscribed := func(userID string) map[string]bool {
			w := serveWorkspaceAs(testHandler.ListSubscribedIssueIDs, userID, "GET", "/api/issues/subscribed", nil)
			if w.Code != http.StatusOK {
				t.Fatalf("ListSubscribedIssueIDs as %s: %d %s", userID, w.Code, w.Body.String())
			}
			return idsIn(t, w.Body.String(), "issue_ids")
		}
		if got := subscribed(f.outsiderID); got[hiddenID] || !got[openID] {
			t.Errorf("outsider's subscriptions = %v, want only the open issue %s", got, openID)
		}
		if got := subscribed(f.granteeID); !got[hiddenID] {
			t.Errorf("grantee's subscriptions = %v, want the hidden issue %s", got, hiddenID)
		}
	})

	t.Run("inbox", func(t *testing.T) {
		hiddenItem := inboxItemFor(t, f.outsiderID, hiddenID, "status_changed", false)
		openItem := inboxItemFor(t, f.outsiderID, openID, "status_changed", false)
		granteeItem := inboxItemFor(t, f.granteeID, hiddenID, "status_changed", false)

		ids := func(userID string) map[string]bool {
			w := serveAs(inboxWorkspaceHandler(testHandler.ListInbox), requestAs(userID, "GET", "/api/inbox", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("ListInbox as %s: %d %s", userID, w.Code, w.Body.String())
			}
			var items []InboxItemResponse
			if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
				t.Fatalf("decode inbox: %v", err)
			}
			out := map[string]bool{}
			for _, it := range items {
				out[it.ID] = true
			}
			return out
		}
		if got := ids(f.outsiderID); got[hiddenItem] || !got[openItem] {
			t.Errorf("outsider's inbox = %v, want only %s", got, openItem)
		}
		if got := ids(f.granteeID); !got[granteeItem] {
			t.Errorf("grantee's inbox = %v, want %s", got, granteeItem)
		}

		// An item the outsider can no longer see is not an item to open.
		mark := func(id string) *httptest.ResponseRecorder {
			req := requestAs(f.outsiderID, "POST", "/api/inbox/"+id+"/read", nil)
			return serveAs(inboxWorkspaceHandler(testHandler.MarkInboxRead), withURLParam(req, "id", id))
		}
		missingItem := uuid.NewString()
		sameAnswer(t, "MarkInboxRead as outsider", mark(hiddenItem), mark(missingItem), hiddenItem, missingItem)
		if n := dbfx.Count(t, `SELECT count(*) FROM inbox_item WHERE id = $1 AND read`, hiddenItem); n != 0 {
			t.Errorf("an outsider's request changed the hidden inbox item")
		}
	})

	t.Run("task", func(t *testing.T) {
		agentID := createHandlerTestAgent(t, "DerivedTaskAgent", []byte("[]"))
		runtimeID := handlerTestRuntimeID(t)
		task := func(issueID, status string) string {
			return dbfx.Task(t, agentID, testutil.Cols{"issue_id": issueID, "status": status, "runtime_id": runtimeID})
		}
		hiddenTask := task(hiddenID, "running")
		openTask := task(openID, "queued")

		snapshot := func(userID string) string {
			w := serveWorkspaceAs(testHandler.ListWorkspaceAgentTaskSnapshot, userID, "GET", "/api/agent-task-snapshot", nil)
			if w.Code != http.StatusOK {
				t.Fatalf("ListWorkspaceAgentTaskSnapshot as %s: %d %s", userID, w.Code, w.Body.String())
			}
			return w.Body.String()
		}
		if body := snapshot(f.outsiderID); strings.Contains(body, hiddenTask) || !strings.Contains(body, openTask) {
			t.Errorf("snapshot as outsider = %s, want the open task only", body)
		}
		if body := snapshot(f.granteeID); !strings.Contains(body, hiddenTask) {
			t.Errorf("snapshot as grantee lacks the hidden issue's task: %s", body)
		}

		history := func(userID string) string {
			req := requestAs(userID, "GET", "/api/agents/"+agentID+"/tasks", nil)
			w := serveAs(testHandler.ListAgentTasks, withURLParam(req, "id", agentID))
			if w.Code != http.StatusOK {
				t.Fatalf("ListAgentTasks as %s: %d %s", userID, w.Code, w.Body.String())
			}
			return w.Body.String()
		}
		// The run stays in the history, redacted: its issue is not named.
		if body := history(f.outsiderID); strings.Contains(body, hiddenID) || !strings.Contains(body, hiddenTask) || !strings.Contains(body, openTask) {
			t.Errorf("agent history as outsider = %s, want the hidden run without its issue", body)
		}
		if body := history(testUserID); !strings.Contains(body, hiddenTask) {
			t.Errorf("agent history as owner lacks the hidden issue's task: %s", body)
		}

		working := func(userID string) string {
			w := serveWorkspaceAs(testHandler.ListWorkspaceWorkingAgents, userID, "GET", "/api/agents/working", nil)
			if w.Code != http.StatusOK {
				t.Fatalf("ListWorkspaceWorkingAgents as %s: %d %s", userID, w.Code, w.Body.String())
			}
			return w.Body.String()
		}
		if body := working(f.outsiderID); strings.Contains(body, hiddenID) {
			t.Errorf("working agents as outsider name the hidden issue: %s", body)
		}
		if body := working(f.granteeID); !strings.Contains(body, hiddenID) {
			t.Errorf("working agents as grantee lack the hidden issue: %s", body)
		}

		// A visible parent's family read must not list a hidden child's run.
		parentID := createIssueInProject(t, "family parent "+uuid.NewString()[:8], f.workspaceProjectID)
		dbfx.Exec(t, `UPDATE issue SET parent_issue_id = $1 WHERE id = $2`, parentID, hiddenID)
		family := func(userID string) string {
			req := requestAs(userID, "GET", "/api/issues/"+parentID+"/task-runs?scope=family", nil)
			w := serveAs(testHandler.ListTasksByIssue, withURLParam(req, "id", parentID))
			if w.Code != http.StatusOK {
				t.Fatalf("family read as %s: %d %s", userID, w.Code, w.Body.String())
			}
			return w.Body.String()
		}
		if body := family(f.outsiderID); strings.Contains(body, hiddenTask) {
			t.Errorf("family read as outsider lists the hidden child's run: %s", body)
		}
		if body := family(f.granteeID); !strings.Contains(body, hiddenTask) {
			t.Errorf("family read as grantee lacks the hidden child's run: %s", body)
		}
	})

	t.Run("project_resource", func(t *testing.T) {
		dbfx.Insert(t, "project_resource", testutil.Cols{
			"project_id": f.restrictedProjectID, "workspace_id": testWorkspaceID,
			"resource_type": "github_repo", "resource_ref": `{"url":"https://github.com/example/hidden-repo"}`, "position": 0,
		})
		list := func(userID, id string) *httptest.ResponseRecorder {
			req := requestAs(userID, "GET", "/api/projects/"+id+"/resources", nil)
			return serveAs(testHandler.ListProjectResources, withURLParam(req, "id", id))
		}
		missingProject := uuid.NewString()
		sameAnswer(t, "ListProjectResources as outsider", list(f.outsiderID, f.restrictedProjectID), list(f.outsiderID, missingProject), f.restrictedProjectID, missingProject)
		if w := list(f.granteeID, f.restrictedProjectID); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "hidden-repo") {
			t.Errorf("ListProjectResources as grantee: %d %s", w.Code, w.Body.String())
		}
	})
}

// TestInboxHidesIssueAfterAccessLost: an inbox item written while the member
// could see the project disappears from every inbox read once the member is
// removed from the restricted project, and returns when the grant does.
func TestInboxHidesIssueAfterAccessLost(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	issueID := createIssueInProject(t, "inbox access lost "+uuid.NewString()[:8], f.restrictedProjectID)
	active := inboxItemFor(t, f.granteeID, issueID, "status_changed", false)
	// An issue with an active item is left out of the archive by design, so the
	// archived item sits on a second issue of the same project.
	archivedIssueID := createIssueInProject(t, "inbox archived lost "+uuid.NewString()[:8], f.restrictedProjectID)
	archived := inboxItemFor(t, f.granteeID, archivedIssueID, "mentioned", true)

	list := func(handler http.HandlerFunc, path string) string {
		w := serveAs(inboxWorkspaceHandler(handler), requestAs(f.granteeID, "GET", path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	reads := map[string]func() string{
		"ListInbox":              func() string { return list(testHandler.ListInbox, "/api/inbox") },
		"ListArchivedInbox":      func() string { return list(testHandler.ListArchivedInbox, "/api/inbox/archived") },
		"ListArchivedInboxPage":  func() string { return list(testHandler.ListArchivedInboxPage, "/api/inbox/archived/page") },
		"GetArchivedInboxFacets": func() string { return list(testHandler.GetArchivedInboxFacets, "/api/inbox/archived/facets") },
		"CountUnreadInbox":       func() string { return list(testHandler.CountUnreadInbox, "/api/inbox/unread-count") },
	}
	want := func(label, name, body string, visible bool) {
		t.Helper()
		switch name {
		case "ListInbox":
			if strings.Contains(body, active) != visible {
				t.Errorf("%s: %s contains active item = %v, want %v: %s", label, name, !visible, visible, body)
			}
		case "ListArchivedInbox", "ListArchivedInboxPage":
			if strings.Contains(body, archived) != visible {
				t.Errorf("%s: %s contains archived item = %v, want %v: %s", label, name, !visible, visible, body)
			}
		case "CountUnreadInbox":
			wantBody := `"count":0`
			if visible {
				wantBody = `"count":1`
			}
			if !strings.Contains(strings.ReplaceAll(body, " ", ""), wantBody) {
				t.Errorf("%s: %s = %s, want %s", label, name, body, wantBody)
			}
		case "GetArchivedInboxFacets":
			if strings.Contains(body, `"high"`) || strings.Contains(body, `"todo"`) {
				if !visible {
					t.Errorf("%s: %s still counts the hidden issue: %s", label, name, body)
				}
			}
		}
	}
	for name, read := range reads {
		want("with access", name, read(), true)
	}

	dbfx.Exec(t, `DELETE FROM project_member WHERE project_id = $1 AND user_id = $2`, f.restrictedProjectID, f.granteeID)
	for name, read := range reads {
		want("access lost", name, read(), false)
	}
	summary := serveAs(testHandler.UnreadInboxSummary, requestAs(f.granteeID, "GET", "/api/inbox/unread-summary", nil))
	if summary.Code != http.StatusOK || strings.Contains(summary.Body.String(), testWorkspaceID) {
		t.Errorf("UnreadInboxSummary after access lost: %d %s, want no entry for the workspace", summary.Code, summary.Body.String())
	}

	dbfx.Exec(t, `INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3)`, f.restrictedProjectID, testWorkspaceID, f.granteeID)
	for name, read := range reads {
		want("access regained", name, read(), true)
	}
}

// TestIssueUnreadFollowsIssueVisibility: the sidebar's unread list drops an
// issue the member can no longer see.
func TestIssueUnreadFollowsIssueVisibility(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	hiddenID := createIssueInProject(t, "unread hidden "+uuid.NewString()[:8], f.restrictedProjectID)
	openID := createIssueInProject(t, "unread open "+uuid.NewString()[:8], f.workspaceProjectID)
	for _, id := range []string{hiddenID, openID} {
		dbfx.Exec(t, `INSERT INTO issue_unread (workspace_id, user_id, issue_id, outcome) VALUES ($1, $2, $3, 'manual')`, testWorkspaceID, f.outsiderID, id)
		dbfx.Exec(t, `INSERT INTO issue_unread (workspace_id, user_id, issue_id, outcome) VALUES ($1, $2, $3, 'manual')`, testWorkspaceID, f.granteeID, id)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue_unread WHERE issue_id = ANY($1::uuid[])`, []string{hiddenID, openID})
	})
	unread := func(userID string) string {
		w := serveWorkspaceAs(testHandler.ListIssueUnread, userID, "GET", "/api/issues/unread?cursor="+url.QueryEscape(""), nil)
		if w.Code != http.StatusOK {
			t.Fatalf("ListIssueUnread as %s: %d %s", userID, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	if body := unread(f.outsiderID); strings.Contains(body, hiddenID) || !strings.Contains(body, openID) {
		t.Errorf("unread list as outsider = %s, want only the open issue", body)
	}
	if body := unread(f.granteeID); !strings.Contains(body, hiddenID) || !strings.Contains(body, openID) {
		t.Errorf("unread list as grantee = %s, want both issues", body)
	}
}

// TestAggregateCountsExcludeHidden: the dashboard counts of tasks and usage
// leave out the work done on issues of a project the viewer cannot see, for
// the workspace totals and for the project filter alike. Owners see all.
func TestAggregateCountsExcludeHidden(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	hiddenID := createIssueInProject(t, "aggregate hidden "+uuid.NewString()[:8], f.restrictedProjectID)
	openID := createIssueInProject(t, "aggregate open "+uuid.NewString()[:8], f.workspaceProjectID)
	agentID := createHandlerTestAgent(t, "AggregateAgent", []byte("[]"))
	runtimeID := handlerTestRuntimeID(t)

	started := time.Now().UTC().Add(-30 * time.Minute)
	completed := started.Add(10 * time.Minute)
	run := func(issueID, status string) {
		dbfx.Task(t, agentID, testutil.Cols{
			"issue_id": issueID, "status": status, "runtime_id": runtimeID,
			"started_at": started, "completed_at": completed,
		})
	}
	// Two completed runs and one failed run on the hidden issue, one completed
	// run on the open one.
	run(hiddenID, "completed")
	run(hiddenID, "completed")
	run(hiddenID, "failed")
	run(openID, "completed")
	const model = "aggregate-hidden-model"
	for projectID, tokens := range map[string]int{f.restrictedProjectID: 700, f.workspaceProjectID: 30} {
		dbfx.Exec(t, `
			INSERT INTO task_usage_hourly (bucket_hour, workspace_id, runtime_id, agent_id, project_id, provider, model, input_tokens, task_count)
			VALUES (date_trunc('hour', now()), $1, $2, $3, $4, 'claude', $5, $6, 1)`,
			testWorkspaceID, runtimeID, agentID, projectID, model, tokens)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM task_usage_hourly WHERE model = $1`, model)
	})

	get := func(handler http.HandlerFunc, userID, path string) string {
		w := serveAs(handler, requestAs(userID, "GET", path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s as %s: %d %s", path, userID, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	// Totals over the agent's own tasks, so other tests' rows do not count.
	taskTotal := func(userID string) int {
		var rows []DashboardAgentRunTimeResponse
		body := get(testHandler.GetDashboardAgentRunTime, userID, "/api/dashboard/agent-runtime?days=7")
		if err := json.Unmarshal([]byte(body), &rows); err != nil {
			t.Fatalf("decode agent-runtime: %v", err)
		}
		for _, r := range rows {
			if r.AgentID == agentID {
				return int(r.TaskCount)
			}
		}
		return 0
	}
	if got := taskTotal(testUserID); got != 4 {
		t.Fatalf("owner sees %d runs, want 4", got)
	}
	// Workspace rollups count a private project's runs for everyone.
	if got := taskTotal(f.outsiderID); got != 4 {
		t.Errorf("outsider sees %d runs, want 4", got)
	}
	if got := taskTotal(f.granteeID); got != 4 {
		t.Errorf("grantee sees %d runs, want 4", got)
	}

	sum := func(body string, field string) int {
		var rows []map[string]any
		if err := json.Unmarshal([]byte(body), &rows); err != nil {
			t.Fatalf("decode: %v: %s", err, body)
		}
		total := 0
		for _, r := range rows {
			if v, ok := r[field].(float64); ok {
				total += int(v)
			}
		}
		return total
	}
	// Runs per day, failures per day: the date-bucketed series.
	daily := func(userID string) (tasks, failedTasks int) {
		tasks = sum(get(testHandler.GetDashboardRunTimeDaily, userID, "/api/dashboard/runtime/daily?days=7"), "task_count")
		failedTasks = sum(get(testHandler.GetDashboardRunTimeDaily, userID, "/api/dashboard/runtime/daily?days=7"), "failed_count")
		return
	}
	ownerTasks, ownerFailed := daily(testUserID)
	outTasks, outFailed := daily(f.outsiderID)
	if ownerTasks != outTasks || ownerFailed != outFailed {
		t.Errorf("runtime/daily: owner (%d tasks, %d failed), outsider (%d, %d); workspace totals must match",
			ownerTasks, ownerFailed, outTasks, outFailed)
	}
	failures := func(userID string) int {
		return sum(get(testHandler.GetDashboardFailuresDaily, userID, "/api/dashboard/failures/daily?days=7"), "task_count")
	}
	if d := failures(testUserID) - failures(f.outsiderID); d != 0 {
		t.Errorf("failures/daily: the outsider misses %d terminal runs, want 0", d)
	}

	// Usage rows keyed by project: the outsider's totals leave out the hidden
	// project, and asking for that project by id returns nothing.
	modelTokens := func(userID, path string) int {
		var rows []DashboardUsageDailyResponse
		if err := json.Unmarshal([]byte(get(testHandler.GetDashboardUsageDaily, userID, path)), &rows); err != nil {
			t.Fatalf("decode usage: %v", err)
		}
		total := 0
		for _, r := range rows {
			if r.Model == model {
				total += int(r.InputTokens)
			}
		}
		return total
	}
	if got := modelTokens(testUserID, "/api/dashboard/usage/daily?days=7"); got != 730 {
		t.Fatalf("owner usage = %d tokens, want 730", got)
	}
	if got := modelTokens(f.outsiderID, "/api/dashboard/usage/daily?days=7"); got != 730 {
		t.Errorf("outsider usage = %d tokens, want the workspace total 730", got)
	}
	if got := modelTokens(f.outsiderID, "/api/dashboard/usage/daily?days=7&project_id="+f.restrictedProjectID); got != 0 {
		t.Errorf("outsider usage filtered to the hidden project = %d tokens, want 0", got)
	}
	if got := modelTokens(f.granteeID, "/api/dashboard/usage/daily?days=7&project_id="+f.restrictedProjectID); got != 700 {
		t.Errorf("grantee usage filtered to the hidden project = %d tokens, want 700", got)
	}
}
