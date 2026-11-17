package handler

// Visibility of restricted projects on the attachment routes that find an
// attachment by id (spec 7.1 rules 3 and 5): an attachment of a hidden issue
// answers like a missing attachment, and a delete by an outsider changes
// nothing.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func insertIssueAttachment(t *testing.T, issueID, commentID, uploaderID string) string {
	t.Helper()
	cols := testutil.Cols{
		"workspace_id":  testWorkspaceID,
		"uploader_type": "member", "uploader_id": uploaderID,
		"filename": "secret.txt", "url": "https://s3.example.com/test-bucket/" + uuid.NewString() + ".txt",
		"content_type": "text/plain", "size_bytes": 12,
	}
	if issueID != "" {
		cols["issue_id"] = issueID
	}
	if commentID != "" {
		cols["comment_id"] = commentID
	}
	return dbfx.Insert(t, "attachment", cols)
}

func attachmentExists(t *testing.T, id string) bool {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM attachment WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("count attachment: %v", err)
	}
	return n == 1
}

// TestRestrictedIssueAttachmentsAnswerMissing: every by-id attachment route
// answers an outsider with the status and body of a missing attachment when the
// attachment belongs to a hidden issue, directly or through a comment, and an
// outsider's delete leaves the attachment in place. The grantee and the owner
// are not turned away.
func TestRestrictedIssueAttachmentsAnswerMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	hiddenIssueID := createIssueInProject(t, "attachment hidden "+uuid.NewString()[:8], f.restrictedProjectID)
	commentID := dbfx.Insert(t, "comment", testutil.Cols{
		"issue_id": hiddenIssueID, "workspace_id": testWorkspaceID,
		"author_type": "member", "author_id": testUserID, "content": "hidden comment",
	})

	direct := insertIssueAttachment(t, hiddenIssueID, "", testUserID)
	viaComment := insertIssueAttachment(t, "", commentID, testUserID)
	missingID := uuid.NewString()

	reads := map[string]http.HandlerFunc{
		"GetAttachmentByID":        testHandler.GetAttachmentByID,
		"GetAttachmentContent":     testHandler.GetAttachmentContent,
		"GetAttachmentForDownload": testHandler.GetAttachmentForDownload,
		"DownloadAttachment":       testHandler.DownloadAttachment,
		"DeleteAttachment":         testHandler.DeleteAttachment,
	}
	for name, handler := range reads {
		method := "GET"
		if strings.HasPrefix(name, "Delete") {
			method = "DELETE"
		}
		call := func(userID, id string) (int, string) {
			req := requestAs(userID, method, "/api/attachments/"+id+"?workspace_id="+testWorkspaceID, nil)
			w := serveAs(handler, withURLParam(req, "id", id))
			return w.Code, strings.ReplaceAll(w.Body.String(), id, "ID")
		}
		missingCode, missingBody := call(f.outsiderID, missingID)
		if missingCode != http.StatusNotFound {
			t.Fatalf("%s: a missing attachment answers %d, want 404", name, missingCode)
		}
		for label, id := range map[string]string{"issue": direct, "comment": viaComment} {
			code, body := call(f.outsiderID, id)
			if code != missingCode || body != missingBody {
				t.Errorf("%s via %s as outsider: hidden answers %d %s, missing answers %d %s",
					name, label, code, body, missingCode, missingBody)
			}
			if !strings.HasPrefix(name, "Delete") {
				if code, body := call(f.granteeID, id); code == http.StatusNotFound {
					t.Errorf("%s via %s as grantee: got %d %s", name, label, code, body)
				}
			}
		}
	}
	if !attachmentExists(t, direct) || !attachmentExists(t, viaComment) {
		t.Fatalf("an outsider's request changed the attachments")
	}

	// An outsider who uploaded the file before the project was restricted
	// still cannot delete it.
	own := insertIssueAttachment(t, hiddenIssueID, "", f.outsiderID)
	req := requestAs(f.outsiderID, "DELETE", "/api/attachments/"+own+"?workspace_id="+testWorkspaceID, nil)
	if w := serveAs(testHandler.DeleteAttachment, withURLParam(req, "id", own)); w.Code != http.StatusNotFound {
		t.Errorf("DeleteAttachment of own file in hidden issue as outsider: got %d %s, want 404", w.Code, w.Body.String())
	}
	if !attachmentExists(t, own) {
		t.Errorf("outsider deleted an attachment of a hidden issue")
	}

	// The owner still deletes.
	req = requestAs(testUserID, "DELETE", "/api/attachments/"+direct+"?workspace_id="+testWorkspaceID, nil)
	if w := serveAs(testHandler.DeleteAttachment, withURLParam(req, "id", direct)); w.Code != http.StatusNoContent {
		t.Errorf("DeleteAttachment as owner: got %d %s, want 204", w.Code, w.Body.String())
	}
}
