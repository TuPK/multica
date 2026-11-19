package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// A hidden issue or project is missing for the viewer (spec 7.1 rule 5). Two
// leaks are pinned here: the duplicate original that the search-index filler
// names, and a hidden project_id accepted when a sub-issue is filed from a
// comment.

// TestSearchIndexDuplicateOfHidesRestrictedOriginal: a visible issue is marked
// a duplicate of an issue in a restricted project. The member without a
// project_member row gets the duplicate without the original's id, identifier,
// title and status; the owner still gets them.
func TestSearchIndexDuplicateOfHidesRestrictedOriginal(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	wsID := searchIndexWorkspace(t, "REF")
	memberID := dbfx.User(t, "Refs member", testutil.UniqueEmail("refs-member"))
	dbfx.Member(t, wsID, memberID, "member")
	restricted := dbfx.Project(t, "Refs restricted "+uuid.NewString()[:8], testutil.Cols{"workspace_id": wsID, "access": "restricted"})
	original := dbfx.Issue(t, "HIDDEN-ORIGINAL-TITLE", testutil.Cols{"workspace_id": wsID, "project_id": restricted})
	duplicate := dbfx.Issue(t, "visible duplicate", testutil.Cols{
		"workspace_id": wsID, "status": "cancelled", "duplicate_of_issue_id": original,
	})

	snapshotAs := func(userID string) (SearchIndexIssue, string) {
		t.Helper()
		req := requestAs(userID, http.MethodGet, "/api/search-index/snapshot?limit=50", nil)
		req.Header.Set("X-Workspace-ID", wsID)
		res := testutil.Call(t, testHandler.GetSearchIndexSnapshot, req).Want(http.StatusOK)
		var page SearchIndexSnapshotResponse
		res.JSON(&page)
		for _, is := range page.Issues {
			if is.ID == duplicate {
				return is, res.Text()
			}
		}
		t.Fatalf("the duplicate %s is missing from the snapshot of %s", duplicate, userID)
		return SearchIndexIssue{}, ""
	}

	got, body := snapshotAs(memberID)
	if got.DuplicateOf != nil || strings.Contains(body, "HIDDEN-ORIGINAL-TITLE") || strings.Contains(body, original) {
		t.Errorf("member's snapshot names the hidden original: %+v", got.DuplicateOf)
	}
	if owner, _ := snapshotAs(testUserID); owner.DuplicateOf == nil || owner.DuplicateOf.ID != original {
		t.Errorf("owner's snapshot lacks the original: %+v", owner.DuplicateOf)
	}
}

// TestRestrictedProjectIDOfCommentSubIssueAnswersMissing: filing a sub-issue
// from a comment with the id of a restricted project the member does not see
// answers like an id that names no project, in manual and agent mode, and
// files nothing.
func TestRestrictedProjectIDOfCommentSubIssueAnswersMissing(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	f := newRestrictedFixture(t)

	var agentID string
	if err := testPool.QueryRow(ctx, `
		SELECT id FROM agent
		WHERE workspace_id = $1 AND runtime_id = $2 AND archived_at IS NULL
		ORDER BY created_at ASC LIMIT 1
	`, testWorkspaceID, testRuntimeID).Scan(&agentID); err != nil {
		t.Fatalf("load test agent: %v", err)
	}
	var originalMetadata string
	if err := testPool.QueryRow(ctx, `SELECT metadata::text FROM agent_runtime WHERE id = $1`, testRuntimeID).Scan(&originalMetadata); err != nil {
		t.Fatalf("load runtime metadata: %v", err)
	}
	dbfx.Exec(t, `UPDATE agent_runtime SET metadata = '{"cli_version":"0.4.3","capabilities":["source_context_quick_create_v1"]}'::jsonb WHERE id = $1`, testRuntimeID)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE agent_runtime SET metadata = $1::jsonb WHERE id = $2`, originalMetadata, testRuntimeID)
	})

	sourceID := dbfx.Issue(t, "comment sub-issue source", nil)
	commentID := dbfx.Comment(t, sourceID, "context", testutil.Cols{"workspace_id": testWorkspaceID, "author_type": "member", "author_id": testUserID})

	var preview sourceContextPreviewResponse
	testutil.Call(t, testHandler.PreviewCommentSubIssue, withURLParam(
		requestAs(f.outsiderID, http.MethodGet, "/api/comments/"+commentID+"/sub-issue-preview", nil), "commentId", commentID,
	)).Want(http.StatusOK).JSON(&preview)

	create := func(body map[string]any) *testutil.Response {
		body["capture_token"] = preview.CaptureToken
		return testutil.Call(t, testHandler.CreateCommentSubIssue, withURLParam(
			requestAs(f.outsiderID, http.MethodPost, "/api/comments/"+commentID+"/sub-issues", body), "commentId", commentID))
	}
	manual := func(projectID string) *testutil.Response {
		return create(map[string]any{"mode": "manual", "issue": map[string]any{
			"title": "filed into a project", "status": "todo", "priority": "none", "project_id": projectID,
		}})
	}
	prompt := "file this into " + uuid.NewString()
	agent := func(projectID string) *testutil.Response {
		return create(map[string]any{"mode": "agent", "quick_create": map[string]any{
			"agent_id": agentID, "prompt": prompt, "project_id": projectID,
		}})
	}

	missingID := uuid.NewString()
	for name, call := range map[string]func(string) *testutil.Response{"manual": manual, "agent": agent} {
		missing := call(missingID)
		if missing.Code != http.StatusBadRequest {
			t.Fatalf("%s mode with a missing project answers %d %s, want 400", name, missing.Code, missing.Text())
		}
		hidden := call(f.restrictedProjectID)
		if hidden.Code != missing.Code ||
			strings.ReplaceAll(hidden.Text(), f.restrictedProjectID, "ID") != strings.ReplaceAll(missing.Text(), missingID, "ID") {
			t.Errorf("%s mode: hidden project answers %d %s, missing answers %d %s", name, hidden.Code, hidden.Text(), missing.Code, missing.Text())
		}
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE project_id = $1`, f.restrictedProjectID); n != 0 {
		t.Errorf("%d issues were filed into the hidden project", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM agent_task_queue WHERE context->>'prompt' = $1`, prompt); n != 0 {
		t.Errorf("%d agent tasks were queued for the hidden project", n)
	}
}
