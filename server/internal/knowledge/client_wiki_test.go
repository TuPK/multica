package knowledge

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestWikiPagesListReadAndStats(t *testing.T) {
	m := newMock(t).
		on("GET /api/v1/knowledgebase/"+kbID+"/wiki/pages", http.StatusOK,
			[]byte(`{"pages":[{"slug":"concept/taste-mission","title":"Taste Mission","page_type":"concept","status":"published","summary":"s","content":"body","aliases":null,"category_path":["Buying"],"source_refs":["d1"]}],"total":1,"page":1,"page_size":50}`)).
		on("GET /api/v1/knowledgebase/"+kbID+"/wiki/pages/concept/taste-mission", http.StatusOK,
			[]byte(`{"slug":"concept/taste-mission","title":"Taste Mission","page_type":"concept","status":"published","content":"# Taste Mission"}`)).
		on("GET /api/v1/knowledgebase/"+kbID+"/wiki/stats", http.StatusOK,
			[]byte(`{"total_pages":3,"pages_by_type":{"summary":2,"concept":1},"total_links":4}`))
	ws := newClient(t, m).Workspace(wsKey)
	ctx := context.Background()

	list, err := ws.ListWikiPages(ctx, kbID, 1, 50, "concept")
	if err != nil || list.Total != 1 || len(list.Pages) != 1 {
		t.Fatalf("list: %+v err=%v", list, err)
	}
	if p := list.Pages[0]; p.Content != "" || p.Aliases == nil || p.Title != "Taste Mission" {
		t.Fatalf("a listed page carries no text and no nil lists: %+v", p)
	}
	reqs := m.requests("GET", "/api/v1/knowledgebase/"+kbID+"/wiki/pages")
	if len(reqs) != 1 || reqs[0].Header.Get("X-API-Key") != wsKey {
		t.Fatalf("list request: %+v", reqs)
	}

	page, err := ws.GetWikiPage(ctx, kbID, "concept/taste-mission")
	if err != nil || page.Content != "# Taste Mission" {
		t.Fatalf("get: %+v err=%v", page, err)
	}

	stats, err := ws.WikiStats(ctx, kbID)
	if err != nil || stats.TotalPages != 3 || stats.PagesByType["summary"] != 2 {
		t.Fatalf("stats: %+v err=%v", stats, err)
	}
}

func TestWikiOffIsInvalidNotAnOutage(t *testing.T) {
	m := newMock(t).on("GET /api/v1/knowledgebase/"+kbID+"/wiki/stats", http.StatusBadRequest,
		[]byte(`{"error":"error code: 1000, error message: Wiki feature is not enabled for this knowledge base"}`))
	_, err := newClient(t, m).Workspace(wsKey).WikiStats(context.Background(), kbID)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestReparseDocumentsSendsBaseAndIDs(t *testing.T) {
	m := newMock(t).on("POST /api/v1/knowledge/batch-reparse", http.StatusOK, []byte(`{"success":true,"data":{"reparse_count":2}}`))
	ws := newClient(t, m).Workspace(wsKey)
	if err := ws.ReparseDocuments(context.Background(), kbID, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	body := decode(t, m.requests("POST", "/api/v1/knowledge/batch-reparse")[0].Body)
	if body["kb_id"] != kbID {
		t.Fatalf("body = %v", body)
	}
	if ids, _ := body["ids"].([]any); len(ids) != 2 {
		t.Fatalf("ids = %v", body["ids"])
	}
	if err := ws.ReparseDocuments(context.Background(), kbID, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("no documents: %v", err)
	}
}
