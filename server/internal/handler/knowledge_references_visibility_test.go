package handler

// A knowledge document that points at something hidden (a document it
// supersedes, an issue or project it was written from) must not name it to a
// member who cannot see it (spec 7.1 rule 4). The fixture is the one of
// knowledge_project_visibility_test.go.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// knowledgeProjectFixture enables knowledge, makes a restricted project with a
// knowledge base, and files one published document in that base.
type knowledgeProjectFixture struct {
	restrictedFixture
	hiddenKey string
	hiddenDoc service.KnowledgeDocumentView
}

func newKnowledgeProjectFixture(t *testing.T) knowledgeProjectFixture {
	t.Helper()
	newKnowledgeFixture(t)
	f := newRestrictedFixture(t)
	if err := testHandler.Knowledge.EnsureProjectBase(context.Background(), parseUUID(testWorkspaceID), parseUUID(f.restrictedProjectID), "Restricted refs"); err != nil {
		t.Fatalf("ensure project base: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM knowledge_link WHERE workspace_id = $1 AND knowledge_base_id IN (SELECT id FROM knowledge_base WHERE key LIKE 'project-%')`, testWorkspaceID)
		testPool.Exec(context.Background(), `DELETE FROM knowledge_base WHERE workspace_id = $1 AND key LIKE 'project-%'`, testWorkspaceID)
	})
	p := knowledgeProjectFixture{restrictedFixture: f, hiddenKey: service.ProjectBaseKey(parseUUID(f.restrictedProjectID))}
	testutil.Call(t, testHandler.CreateKnowledgeDocument, newRequest("POST", "/api/knowledge/documents", map[string]any{
		"kb": p.hiddenKey, "title": "Hidden roadmap", "content": "Quasar is the codename.", "publish": true,
	})).Want(http.StatusCreated).JSON(&p.hiddenDoc)
	return p
}

// createMarketDoc files a published document in the "market" base as the owner.
func createMarketDoc(t *testing.T, extra map[string]any) service.KnowledgeDocumentView {
	t.Helper()
	body := map[string]any{"kb": "market", "title": "Market note " + uuid.NewString()[:6], "content": "Public text.", "publish": true}
	for k, v := range extra {
		body[k] = v
	}
	var doc service.KnowledgeDocumentView
	testutil.Call(t, testHandler.CreateKnowledgeDocument, newRequest("POST", "/api/knowledge/documents", body)).Want(http.StatusCreated).JSON(&doc)
	return doc
}

func listMarket(t *testing.T, userID string) (string, map[string]service.KnowledgeDocumentView) {
	t.Helper()
	w := serveAs(testHandler.ListKnowledgeDocuments, asUser(newRequest("GET", "/api/knowledge/documents?kb=market", nil), userID))
	if w.Code != http.StatusOK {
		t.Fatalf("ListKnowledgeDocuments as %s: %d %s", userID, w.Code, w.Body.String())
	}
	var list service.KnowledgeDocumentList
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	byID := map[string]service.KnowledgeDocumentView{}
	for _, d := range list.Documents {
		byID[d.ID] = d
	}
	return w.Body.String(), byID
}

// TestKnowledgeSupersedesDoesNotLeakHiddenDocument: a document that replaces a
// hidden one does not name it to the outsider, and the outsider cannot claim to
// replace it (the answer is the one a missing document gets).
func TestKnowledgeSupersedesDoesNotLeakHiddenDocument(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	p := newKnowledgeProjectFixture(t)
	newer := createMarketDoc(t, map[string]any{"supersedes_id": p.hiddenDoc.ID})
	if newer.SupersedesID == nil || *newer.SupersedesID != p.hiddenDoc.ID {
		t.Fatalf("the owner's document lost its supersedes link: %+v", newer.SupersedesID)
	}

	body, docs := listMarket(t, p.outsiderID)
	if got := docs[newer.ID]; got.ID == "" || got.SupersedesID != nil {
		t.Errorf("outsider sees supersedes_id %v on the replacing document, want none", got.SupersedesID)
	}
	if strings.Contains(body, p.hiddenDoc.ID) || strings.Contains(body, "Hidden roadmap") {
		t.Errorf("outsider's list names the hidden document: %s", body)
	}
	for who, userID := range map[string]string{"grantee": p.granteeID, "owner": testUserID} {
		if _, docs := listMarket(t, userID); docs[newer.ID].SupersedesID == nil || *docs[newer.ID].SupersedesID != p.hiddenDoc.ID {
			t.Errorf("%s lost the supersedes link", who)
		}
	}

	claim := func(userID, target string) *httptest.ResponseRecorder {
		return serveAs(testHandler.CreateKnowledgeDocument, asUser(newRequest("POST", "/api/knowledge/documents", map[string]any{
			"kb": "market", "title": "Replacement", "content": "Replacing.", "supersedes_id": target,
		}), userID))
	}
	missing := uuid.NewString()
	if w := claim(p.outsiderID, missing); w.Code != http.StatusBadRequest {
		t.Fatalf("superseding a missing document answers %d, want 400", w.Code)
	}
	sameAnswer(t, "supersede a hidden document as outsider", claim(p.outsiderID, p.hiddenDoc.ID), claim(p.outsiderID, missing), p.hiddenDoc.ID, missing)
	if w := claim(p.granteeID, p.hiddenDoc.ID); w.Code != http.StatusCreated {
		t.Errorf("supersede as grantee: %d %s", w.Code, w.Body.String())
	}
}

// TestKnowledgeSourceLinkDoesNotLeakHiddenSource: a document written from a
// hidden issue or project does not name that issue or project to the outsider.
func TestKnowledgeSourceLinkDoesNotLeakHiddenSource(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	p := newKnowledgeProjectFixture(t)
	hiddenIssue := createIssueInProject(t, "source hidden "+uuid.NewString()[:8], p.restrictedProjectID)
	openIssue := createIssueInProject(t, "source open "+uuid.NewString()[:8], p.workspaceProjectID)
	fromIssue := createMarketDoc(t, map[string]any{"source_type": "issue", "source_id": hiddenIssue})
	fromProject := createMarketDoc(t, map[string]any{"source_type": "project", "source_id": p.restrictedProjectID})
	fromOpen := createMarketDoc(t, map[string]any{"source_type": "issue", "source_id": openIssue})

	body, docs := listMarket(t, p.outsiderID)
	for name, d := range map[string]service.KnowledgeDocumentView{"hidden issue": fromIssue, "hidden project": fromProject} {
		if got := docs[d.ID]; got.ID == "" || got.SourceID != nil {
			t.Errorf("outsider sees source_id %v for a document from a %s, want none", got.SourceID, name)
		}
	}
	if got := docs[fromOpen.ID]; got.SourceID == nil || *got.SourceID != openIssue {
		t.Errorf("outsider lost the source of a document from a visible issue: %v", got.SourceID)
	}
	if strings.Contains(body, hiddenIssue) || strings.Contains(body, p.restrictedProjectID) {
		t.Errorf("outsider's list names the hidden source: %s", body)
	}
	for who, userID := range map[string]string{"grantee": p.granteeID, "owner": testUserID} {
		_, docs := listMarket(t, userID)
		if got := docs[fromIssue.ID]; got.SourceID == nil || *got.SourceID != hiddenIssue {
			t.Errorf("%s lost the issue source: %v", who, got.SourceID)
		}
		if got := docs[fromProject.ID]; got.SourceID == nil || *got.SourceID != p.restrictedProjectID {
			t.Errorf("%s lost the project source: %v", who, got.SourceID)
		}
	}
}
