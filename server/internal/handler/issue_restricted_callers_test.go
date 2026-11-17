package handler

// Routes that read an issue by the id in the request and then write or answer
// with it (spec 7.1 rules 3, 5). The outsider is a plain member not granted the
// restricted project; a hidden issue must answer like one that does not exist
// and change nothing.

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestRestrictedIssueCallersAnswerMissing: pin, file upload, the comment
// sub-issue routes, the PR auto-complete routes and the brief inquiry answer the
// outsider like the same call on an issue that does not exist.
func TestRestrictedIssueCallersAnswerMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	t.Run("pin an issue", func(t *testing.T) {
		f := newRestrictedFixture(t)
		hiddenID := createIssueInProject(t, "pin hidden "+uuid.NewString()[:8], f.restrictedProjectID)
		missingID := uuid.NewString()
		call := func(id string) *httptest.ResponseRecorder {
			return serveAs(testHandler.CreatePin, requestAs(f.outsiderID, "POST", "/api/pins",
				map[string]any{"item_type": "issue", "item_id": id}))
		}
		missing := call(missingID)
		if missing.Code != http.StatusNotFound {
			t.Fatalf("pin of a missing issue: want 404, got %d %s", missing.Code, missing.Body.String())
		}
		sameAnswer(t, "CreatePin as outsider", call(hiddenID), missing, hiddenID, missingID)
		if n := dbfx.Count(t, `SELECT count(*) FROM pinned_item WHERE user_id = $1`, f.outsiderID); n != 0 {
			t.Errorf("the outsider's pin of a hidden issue was stored (%d rows)", n)
		}
	})

	t.Run("upload a file to an issue", func(t *testing.T) {
		f := newRestrictedFixture(t)
		hiddenID := createIssueInProject(t, "upload hidden "+uuid.NewString()[:8], f.restrictedProjectID)
		missingID := uuid.NewString()
		original := testHandler.Storage
		testHandler.Storage = &mockStorage{}
		t.Cleanup(func() { testHandler.Storage = original })
		call := func(id string) *httptest.ResponseRecorder {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, err := writer.CreateFormFile("file", "hidden-issue.txt")
			if err != nil {
				t.Fatal(err)
			}
			part.Write([]byte("hello"))
			writer.WriteField("issue_id", id)
			writer.Close()
			req := httptest.NewRequest("POST", "/api/upload-file", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("X-User-ID", f.outsiderID)
			req.Header.Set("X-Workspace-ID", testWorkspaceID)
			return serveAs(testHandler.UploadFile, req)
		}
		missing := call(missingID)
		if missing.Code != http.StatusForbidden {
			t.Fatalf("upload to a missing issue: want 403, got %d %s", missing.Code, missing.Body.String())
		}
		sameAnswer(t, "UploadFile as outsider", call(hiddenID), missing, hiddenID, missingID)
		if n := dbfx.Count(t, `SELECT count(*) FROM attachment WHERE issue_id = $1`, hiddenID); n != 0 {
			t.Errorf("the outsider attached a file to a hidden issue (%d rows)", n)
		}
	})

	t.Run("comment sub-issue of a hidden issue", func(t *testing.T) {
		f := newRestrictedFixture(t)
		hiddenID := createIssueInProject(t, "sub-issue source hidden "+uuid.NewString()[:8], f.restrictedProjectID)
		commentID := dbfx.Comment(t, hiddenID, "a comment on a hidden issue")
		missingID := uuid.NewString()

		// The owner's preview gives the token the create route wants.
		var preview sourceContextPreviewResponse
		testutil.Call(t, testHandler.PreviewCommentSubIssue,
			withURLParam(newRequest("GET", "/api/comments/"+commentID+"/sub-issue-preview", nil), "commentId", commentID)).
			Want(http.StatusOK).JSON(&preview)
		if preview.CaptureToken == "" {
			t.Fatal("the owner's preview carries no capture token")
		}

		preview1 := func(id string) *httptest.ResponseRecorder {
			return serveAs(testHandler.PreviewCommentSubIssue,
				withURLParam(requestAs(f.outsiderID, "GET", "/api/comments/"+id+"/sub-issue-preview", nil), "commentId", id))
		}
		missing := preview1(missingID)
		if missing.Code == http.StatusOK {
			t.Fatalf("preview of a missing comment answered 200: %s", missing.Body.String())
		}
		sameAnswer(t, "PreviewCommentSubIssue as outsider", preview1(commentID), missing, commentID, missingID)

		create := func(id string) *httptest.ResponseRecorder {
			return serveAs(testHandler.CreateCommentSubIssue, withURLParam(requestAs(f.outsiderID, "POST",
				"/api/comments/"+id+"/sub-issues", map[string]any{
					"mode": "manual", "capture_token": preview.CaptureToken, "issue": map[string]any{"title": "sub of hidden"},
				}), "commentId", id))
		}
		missing = create(missingID)
		if missing.Code == http.StatusCreated {
			t.Fatalf("create from a missing comment answered 201: %s", missing.Body.String())
		}
		sameAnswer(t, "CreateCommentSubIssue as outsider", create(commentID), missing, commentID, missingID)
		if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE parent_issue_id = $1`, hiddenID); n != 0 {
			t.Errorf("the outsider opened %d sub-issues under a hidden issue", n)
		}
	})

	t.Run("pr auto-complete routes", func(t *testing.T) {
		f := newRestrictedFixture(t)
		hiddenID := createIssueInProject(t, "pr hidden "+uuid.NewString()[:8], f.restrictedProjectID)
		missingID := uuid.NewString()
		routes := []struct {
			name    string
			handler http.HandlerFunc
			method  string
			path    string
			body    any
		}{
			{"SetIssuePRAutoComplete", testHandler.SetIssuePRAutoComplete, "PUT", "/pr-auto-complete", map[string]any{"disabled": true}},
			{"LinkIssuePullRequest", testHandler.LinkIssuePullRequest, "POST", "/pull-requests", map[string]any{"url": "https://github.com/acme/app/pull/1"}},
			{"UnlinkIssuePullRequest", testHandler.UnlinkIssuePullRequest, "DELETE", "/pull-requests/" + uuid.NewString(), nil},
		}
		for _, route := range routes {
			call := func(id string) *httptest.ResponseRecorder {
				req := requestAs(f.outsiderID, route.method, "/api/issues/"+id+route.path, route.body)
				return serveAs(route.handler, withURLParams(req, "id", id, "prId", uuid.NewString()))
			}
			missing := call(missingID)
			if missing.Code != http.StatusNotFound {
				t.Fatalf("%s on a missing issue: want 404, got %d %s", route.name, missing.Code, missing.Body.String())
			}
			sameAnswer(t, route.name+" as outsider", call(hiddenID), missing, hiddenID, missingID)
		}
		if n := dbfx.Count(t, `SELECT count(*) FROM issue_pr_automation WHERE issue_id = $1`, hiddenID); n != 0 {
			t.Errorf("the outsider changed the PR auto-complete setting of a hidden issue")
		}
	})

	t.Run("brief inquiry on a hidden issue", func(t *testing.T) {
		e := newInquiryEnv(t)
		outsider := dbfx.User(t, "Inquiry outsider", "inquiry-outsider-"+uuid.NewString()[:8]+"@multica.test")
		dbfx.Member(t, e.WorkspaceID, outsider, "member")
		restricted := dbfx.Project(t, "Restricted inquiry "+uuid.NewString()[:8], testutil.Cols{"workspace_id": e.WorkspaceID, "access": "restricted"})
		dbfx.Exec(t, `UPDATE issue SET project_id = $1 WHERE id = $2`, restricted, e.IssueID)

		ask := func(issueID string) *httptest.ResponseRecorder {
			taskID := dbfx.Task(t, e.Secretary, testutil.Cols{
				"status": "running", "runtime_id": e.RuntimeID,
				"originator_source": "direct_human", "originator_user_id": testUserID, "accountable_user_id": testUserID,
			})
			req := newRequest("POST", "/api/briefs/inquiries", map[string]any{"squad_id": e.SquadID, "issue_id": issueID, "question": "Is it on track?"})
			req.Header.Set("X-Workspace-ID", e.WorkspaceID)
			req = asAgent(req, e.Secretary, taskID)
			req.Header.Set("X-User-ID", outsider)
			return serveAs(testHandler.AskBriefInquiry, req)
		}
		missingID := uuid.NewString()
		missing := ask(missingID)
		if missing.Code != http.StatusBadRequest {
			t.Fatalf("inquiry on a missing issue: want 400, got %d %s", missing.Code, missing.Body.String())
		}
		sameAnswer(t, "AskBriefInquiry as outsider", ask(e.IssueID), missing, e.IssueID, missingID)
		if n := e.count(t); n != 0 {
			t.Errorf("the outsider's inquiry on a hidden issue was stored (%d rows)", n)
		}
	})
}
