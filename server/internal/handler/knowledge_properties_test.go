package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// accountRecord is the Record of a sales account a squad writes, with its required headings.
const accountRecord = "# Acme Co\n\n## Record\n\nstage: watch\nfit_score: 40\n\n## Why now\n\nA new branch.\n\n## Evidence\n\nA dated article.\n\n## Contact path\n\nNone yet.\n\n## Fit\n\nScored.\n"

func TestKnowledgeProperties_CreateReadAndFilterOverHTTP(t *testing.T) {
	fx := newKnowledgeFixture(t)

	// A person publishes an account. Its Record fills the properties the kind declares.
	var doc service.KnowledgeDocumentView
	testutil.Call(t, testHandler.CreateKnowledgeDocument, docRequest(map[string]any{
		"kb": "sales", "kind": "sales_account", "title": "Acme Co", "content": accountRecord,
		"publish": true, "idempotency_key": "sales:account:http:acme",
		"properties": map[string]any{"owner": "maria"},
	})).Want(http.StatusCreated).JSON(&doc)
	if doc.Properties["stage"] != "watch" || doc.Properties["fit_score"] != 40.0 || doc.Properties["owner"] != "maria" {
		t.Fatalf("properties = %+v (the Record and the request should both be there)", doc.Properties)
	}

	// A property the kind does not declare is refused, and the document keeps its values.
	testutil.Call(t, testHandler.UpdateKnowledgeDocumentProperties,
		testutil.WithURLParams(newRequest(http.MethodPatch, "/api/knowledge/documents/"+doc.ID+"/properties",
			map[string]any{"properties": map[string]any{"colour": "red"}}), "id", doc.ID)).Want(http.StatusBadRequest)

	// A person moves the stage; the change is written, and the document is found by its new value.
	var moved service.KnowledgeDocumentView
	testutil.Call(t, testHandler.UpdateKnowledgeDocumentProperties,
		testutil.WithURLParams(newRequest(http.MethodPatch, "/api/knowledge/documents/"+doc.ID+"/properties",
			map[string]any{"properties": map[string]any{"stage": "qualified", "fit_score": 72}}), "id", doc.ID)).
		Want(http.StatusOK).JSON(&moved)
	if moved.Properties["stage"] != "qualified" || moved.Properties["fit_score"] != 72.0 {
		t.Fatalf("after the change = %+v", moved.Properties)
	}

	var collection service.KnowledgeCollectionView
	testutil.Call(t, testHandler.ListKnowledgeCollection,
		newRequest(http.MethodGet, "/api/knowledge/collections?kind=sales_account&kb=sales&field.stage=qualified", nil)).
		Want(http.StatusOK).JSON(&collection)
	if len(collection.Documents) != 1 || collection.Documents[0].ID != doc.ID {
		t.Fatalf("collection = %+v", collection.Documents)
	}
	if len(collection.Fields) == 0 {
		t.Fatal("the collection carries no fields for the view to render its columns")
	}

	// A collection needs a kind, and a filter needs a declared field.
	testutil.Call(t, testHandler.ListKnowledgeCollection, newRequest(http.MethodGet, "/api/knowledge/collections", nil)).Want(http.StatusBadRequest)
	testutil.Call(t, testHandler.ListKnowledgeCollection,
		newRequest(http.MethodGet, "/api/knowledge/collections?kind=sales_account&field.colour=red", nil)).Want(http.StatusBadRequest)

	// A kind without fields takes no properties.
	testutil.Call(t, testHandler.CreateKnowledgeDocument, docRequest(map[string]any{
		"kb": "market", "kind": "market_report", "title": "Plain", "content": "# Plain\n\n## Summary\n\nx\n\n## Market size\n\nx\n\n## Trends\n\nx\n\n## Sources\n\nx\n",
		"properties": map[string]any{"stage": "x"},
	})).Want(http.StatusBadRequest)
	_ = fx
}

func TestKnowledgeProperties_AnAgentCannotMoveAPersonsStageBack(t *testing.T) {
	fx := newKnowledgeFixture(t)
	var doc service.KnowledgeDocumentView
	testutil.Call(t, testHandler.CreateKnowledgeDocument, docRequest(map[string]any{
		"kb": "sales", "kind": "sales_account", "title": "Beta Co", "content": strings.Replace(accountRecord, "Acme Co", "Beta Co", 1),
		"publish": true, "idempotency_key": "sales:account:http:beta",
	})).Want(http.StatusCreated).JSON(&doc)
	patch := func(req *http.Request) *testutil.Response {
		return testutil.Call(t, testHandler.UpdateKnowledgeDocumentProperties, testutil.WithURLParams(req, "id", doc.ID))
	}
	patch(newRequest(http.MethodPatch, "/api/knowledge/documents/"+doc.ID+"/properties",
		map[string]any{"properties": map[string]any{"stage": "meeting"}})).Want(http.StatusOK)
	// The person's value stands: the agent may not move it back.
	patch(fx.asAgent(newRequest(http.MethodPatch, "/api/knowledge/documents/"+doc.ID+"/properties",
		map[string]any{"properties": map[string]any{"stage": "watch"}}))).Want(http.StatusBadRequest)
}
