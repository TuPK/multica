package handler

// A project's knowledge base follows the project (spec 7.1 rule 4). A member who
// cannot see a restricted project meets its documents like those of a base that
// does not exist, in the base list, the document list, search, the Wiki and a
// write. The fixture is the one of project_restricted_visibility_test.go.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestKnowledgeDocumentFollowsProjectVisibility: the owner files a document in a
// restricted project's base. The outsider gets the answers the same request gets
// for a missing base; the grantee and the owner read and search it.
func TestKnowledgeDocumentFollowsProjectVisibility(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	newKnowledgeFixture(t)
	f := newRestrictedFixture(t)
	if err := testHandler.Knowledge.EnsureProjectBase(context.Background(), parseUUID(testWorkspaceID), parseUUID(f.restrictedProjectID), "Restricted docs"); err != nil {
		t.Fatalf("ensure project base: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM knowledge_link WHERE workspace_id = $1 AND knowledge_base_id IN (SELECT id FROM knowledge_base WHERE key LIKE 'project-%')`, testWorkspaceID)
		testPool.Exec(context.Background(), `DELETE FROM knowledge_base WHERE workspace_id = $1 AND key LIKE 'project-%'`, testWorkspaceID)
	})
	hiddenKey := service.ProjectBaseKey(parseUUID(f.restrictedProjectID))
	missingKey := "project-ffffffffffff"

	var doc service.KnowledgeDocumentView
	testutil.Call(t, testHandler.CreateKnowledgeDocument, newRequest("POST", "/api/knowledge/documents", map[string]any{
		"kb": hiddenKey, "title": "Hidden plan", "content": "Zorblax is the codename.", "publish": true,
	})).Want(http.StatusCreated).JSON(&doc)

	as := func(userID string, req *http.Request) *http.Request { return asUser(req, userID) }
	list := func(userID, kb string) *httptest.ResponseRecorder {
		return serveAs(testHandler.ListKnowledgeDocuments, as(userID, newRequest("GET", "/api/knowledge/documents?kb="+kb, nil)))
	}
	search := func(userID, kb string) *httptest.ResponseRecorder {
		path := "/api/knowledge/search?q=Zorblax"
		if kb != "" {
			path += "&kb=" + kb
		}
		return serveAs(testHandler.SearchKnowledge, as(userID, newRequest("GET", path, nil)))
	}
	wiki := func(userID, key string) *httptest.ResponseRecorder {
		return serveAs(testHandler.GetKnowledgeWiki, withURLParam(as(userID, newRequest("GET", "/api/knowledge/bases/"+key+"/wiki", nil)), "key", key))
	}
	write := func(userID, kb string) *httptest.ResponseRecorder {
		return serveAs(testHandler.CreateKnowledgeDocument, as(userID, newRequest("POST", "/api/knowledge/documents", map[string]any{
			"kb": kb, "title": "Outsider note", "content": "Not allowed.",
		})))
	}

	for name, call := range map[string]func(userID, key string) *httptest.ResponseRecorder{
		"ListKnowledgeDocuments": list, "SearchKnowledge": search, "GetKnowledgeWiki": wiki, "CreateKnowledgeDocument": write,
	} {
		missing := call(f.outsiderID, missingKey)
		sameAnswer(t, name+" as outsider", call(f.outsiderID, hiddenKey), missing, hiddenKey, missingKey)
		if name == "CreateKnowledgeDocument" && missing.Code != http.StatusNotFound {
			t.Fatalf("%s: a missing base answers %d, want 404", name, missing.Code)
		}
		if name != "GetKnowledgeWiki" {
			for who, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
				if name == "CreateKnowledgeDocument" {
					continue
				}
				if w := call(userID, hiddenKey); w.Code != http.StatusOK || (name != "SearchKnowledge" && !strings.Contains(w.Body.String(), "Hidden plan")) {
					t.Errorf("%s as %s: %d %s", name, who, w.Code, w.Body.String())
				}
			}
		}
	}

	// Without naming the base, the outsider's search and the base list leave it out.
	if body := search(f.outsiderID, "").Body.String(); strings.Contains(body, "Hidden plan") {
		t.Errorf("search as outsider returns the hidden document: %s", body)
	}
	if body := search(f.granteeID, "").Body.String(); !strings.Contains(body, "Hidden plan") {
		t.Errorf("search as grantee lacks the document: %s", body)
	}
	bases := func(userID string) string {
		w := serveAs(testHandler.ListKnowledgeBases, as(userID, newRequest("GET", "/api/knowledge/bases", nil)))
		if w.Code != http.StatusOK {
			t.Fatalf("ListKnowledgeBases as %s: %d %s", userID, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	if body := bases(f.outsiderID); strings.Contains(body, hiddenKey) {
		t.Errorf("base list as outsider names the hidden project's base: %s", body)
	}
	if body := bases(f.granteeID); !strings.Contains(body, hiddenKey) {
		t.Errorf("base list as grantee lacks the project's base: %s", body)
	}
}
