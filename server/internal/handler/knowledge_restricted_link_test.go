package handler

// A document that names an initiative is taken into its checklist when it is
// published, which can advance the initiative and write its backing issue
// (spec 7.1 rules 3, 5). A member who cannot see the backing issue must not
// reach that write.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestRestrictedKnowledgeLinkAnswersMissing: a member's publish that names an
// initiative whose backing issue sits in a restricted project the member does
// not see answers like one naming an initiative that does not exist, and the
// initiative is unchanged. An admin's publish still files the document.
func TestRestrictedKnowledgeLinkAnswersMissing(t *testing.T) {
	fx := newRoomedIniWorkspace(t)
	restricted := dbfx.Project(t, "Restricted knowledge "+uuid.NewString()[:8], testutil.Cols{"workspace_id": fx.ID, "visibility": "private"})
	ownerJoinsProject(t, fx.ID, restricted)
	created := fx.createInitiative(t, "Hidden documented initiative")
	var issueID string
	dbfx.QueryRow(t, `SELECT issue_id FROM initiative WHERE id = $1`, created.ID).Scan(&issueID)
	dbfx.Exec(t, `UPDATE issue SET project_id = $1 WHERE id = $2`, restricted, issueID)

	previous := testHandler.Knowledge
	testHandler.Knowledge = knowledgeFor(t, fx.ID, fx.AdminID)
	t.Cleanup(func() { testHandler.Knowledge = previous })

	validation := "## Question\n\nDo bulk buyers pay?\n\n## Findings\n\nYes.\n\n## Recommendation\n\nBuild it.\n"
	publish := func(userID, initiativeID string) *httptest.ResponseRecorder {
		req := fx.as(newRequest("POST", "/api/knowledge/documents", map[string]any{
			"kind": "validation_report", "title": "Validation for hidden", "content": validation,
			"publish": true, "initiative_id": initiativeID,
		}), userID)
		return serveAs(testHandler.CreateKnowledgeDocument, req)
	}
	linkRows := func() int {
		return dbfx.Count(t, `SELECT count(*) FROM knowledge_link WHERE workspace_id = $1`, fx.ID)
	}
	before := issueRowSnapshot(t, issueID)
	eventsBefore := len(fx.detail(t, created.ID).Events)

	missingID := uuid.NewString()
	missing := publish(fx.MemberID, missingID)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("publish naming a missing initiative: want 400, got %d %s", missing.Code, missing.Body.String())
	}
	sameAnswer(t, "publish naming a hidden initiative", publish(fx.MemberID, created.ID), missing, created.ID, missingID)
	if n := linkRows(); n != 0 {
		t.Errorf("the outsider's refused publish left %d knowledge documents", n)
	}
	if after := issueRowSnapshot(t, issueID); after != before {
		t.Errorf("the outsider's publish changed the backing issue:\nbefore: %s\nafter:  %s", before, after)
	}
	if n := len(fx.detail(t, created.ID).Events); n != eventsBefore {
		t.Errorf("the outsider's publish added %d initiative events", n-eventsBefore)
	}
	if d := fx.detail(t, created.ID); d.Initiative.ArtifactsPresent != 0 {
		t.Errorf("the outsider's publish took an artifact into a hidden initiative: %+v", d.Initiative)
	}

	// An admin sees every project: the same publish files the document and takes it in.
	var view service.KnowledgeDocumentView
	testutil.Call(t, testHandler.CreateKnowledgeDocument, fx.as(newRequest("POST", "/api/knowledge/documents", map[string]any{
		"kind": "validation_report", "title": "Validation for hidden", "content": validation,
		"publish": true, "initiative_id": created.ID,
	}), fx.AdminID)).Want(http.StatusCreated).JSON(&view)
	if view.State != "published" {
		t.Fatalf("admin publish = %+v", view)
	}
	if d := fx.detail(t, created.ID); d.Initiative.ArtifactsPresent != 1 {
		t.Errorf("the admin's publish should satisfy one artifact: %+v", d.Initiative)
	}
}
