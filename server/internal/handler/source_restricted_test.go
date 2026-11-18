package handler

// A sub-issue keeps a frozen copy of its source comment, and its detail also
// rebuilds the live source (spec 7.1 rules 3, 5). When the source issue sits in
// a project the viewer cannot see, the rebuild must not run: the detail answers
// as it does when the source issue is deleted.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/projectaccess"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
)

// ownerScope is the scope of the workspace owner, who sees every project.
func ownerScope(t *testing.T) projectaccess.Scope {
	t.Helper()
	scope, err := projectaccess.ForViewer(context.Background(), testHandler.Queries, util.MustParseUUID(testWorkspaceID), util.MustParseUUID(testUserID))
	if err != nil {
		t.Fatalf("owner scope: %v", err)
	}
	return scope
}

// liveSourceView is the part of an issue's source context detail that comes
// from the live source and not from the frozen snapshot.
type liveSourceView struct {
	DisplayState         string                      `json:"display_state"`
	SourceIssueState     string                      `json:"source_issue_state"`
	CommentThreadState   string                      `json:"comment_thread_state"`
	AnchorCommentState   string                      `json:"anchor_comment_state"`
	CanOpenCurrentSource bool                        `json:"can_open_current_source"`
	ChangeReasons        []string                    `json:"change_reasons"`
	ChangeDetails        sourceContextChangeDetails  `json:"change_details"`
	CurrentSource        *sourceContextCurrentSource `json:"current_source"`
}

// subIssueOfSource inserts a source issue in projectID (empty: no project)
// with one comment, a sub-issue that carries a frozen copy of that comment, and
// then changes the live source: a new title and an edited comment. It returns the
// sub-issue, source issue and anchor comment ids.
func subIssueOfSource(t *testing.T, projectID, label string) (subID, sourceID, commentID string) {
	t.Helper()
	ctx := context.Background()
	sourceID = createIssueInProject(t, "source "+label, projectID)
	subID = createIssueInProject(t, "sub-issue of "+label, "")
	if err := testPool.QueryRow(ctx, `
		INSERT INTO comment (issue_id, workspace_id, author_type, author_id, content)
		VALUES ($1, $2, 'member', $3, 'frozen comment') RETURNING id
	`, sourceID, testWorkspaceID, testUserID).Scan(&commentID); err != nil {
		t.Fatalf("insert source comment: %v", err)
	}
	build, err := service.BuildSourceContext(ctx, testHandler.Queries, util.MustParseUUID(testWorkspaceID), util.MustParseUUID(commentID))
	if err != nil {
		t.Fatalf("build source context: %v", err)
	}
	snapshot, err := json.Marshal(build.Snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	dbfx.Exec(t, `
		INSERT INTO issue_source_context
			(id, workspace_id, issue_id, source_issue_id, anchor_comment_id, captured_by_user_id,
			 snapshot_version, snapshot, capture_digest, state, attached_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, 'attached', now())`,
		testWorkspaceID, subID, sourceID, commentID, testUserID, service.SourceContextVersion, snapshot, build.Digest)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue_source_context WHERE issue_id = $1`, subID)
		testPool.Exec(context.Background(), `DELETE FROM comment WHERE issue_id = $1`, sourceID)
	})
	dbfx.Exec(t, `UPDATE issue SET title = 'renamed live source '||$2, revision = revision + 1 WHERE id = $1`, sourceID, label)
	dbfx.Exec(t, `UPDATE comment SET content = 'live comment '||$2, revision = revision + 1, updated_at = now() WHERE id = $1`, commentID, label)
	return subID, sourceID, commentID
}

func sourceViewAs(t *testing.T, userID, subID string) (liveSourceView, string) {
	t.Helper()
	w := serveWorkflowRoute(testHandler.GetIssue, withURLParam(requestAs(userID, "GET", "/api/issues/"+subID, nil), "id", subID))
	if w.Code != http.StatusOK {
		t.Fatalf("GetIssue sub-issue as %s: %d %s", userID, w.Code, w.Body.String())
	}
	var out struct {
		SourceContext *liveSourceView `json:"source_context"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.SourceContext == nil {
		t.Fatalf("decode source_context: %v in %s", err, w.Body.String())
	}
	raw, _ := json.Marshal(out.SourceContext)
	return *out.SourceContext, string(raw)
}

// TestRestrictedSourceDetailAnswersMissing: the outsider sees a sub-issue whose
// source issue is in a restricted project. The detail carries nothing from the
// live source (no new title, no edited comment) and equals the detail of a
// sub-issue whose source issue was deleted. The grantee and the owner still
// see the live changes.
func TestRestrictedSourceDetailAnswersMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)

	hiddenSub, _, hiddenComment := subIssueOfSource(t, f.restrictedProjectID, "hidden")
	deletedSub, deletedSource, deletedComment := subIssueOfSource(t, "", "deleted")
	dbfx.Exec(t, `DELETE FROM comment WHERE issue_id = $1`, deletedSource)
	dbfx.Exec(t, `DELETE FROM issue WHERE id = $1`, deletedSource)

	_, hiddenRaw := sourceViewAs(t, f.outsiderID, hiddenSub)
	_, deletedRaw := sourceViewAs(t, f.outsiderID, deletedSub)
	hidden := strings.ReplaceAll(hiddenRaw, hiddenComment, "COMMENT")
	deleted := strings.ReplaceAll(deletedRaw, deletedComment, "COMMENT")
	if hidden != deleted {
		t.Errorf("a hidden source must answer like a deleted one\nhidden:  %s\ndeleted: %s", hidden, deleted)
	}

	for name, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
		view, raw := sourceViewAs(t, userID, hiddenSub)
		if view.SourceIssueState != "changed" || view.CommentThreadState != "changed" {
			t.Errorf("%s must see the live source change: %s", name, raw)
		}
	}
}
