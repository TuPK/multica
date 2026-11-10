package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// A Sales Loop squad takes sources and shares; any other squad is refused, and nothing is written.
func TestSquadSources_SalesSquadWritesADocumentAndARequest(t *testing.T) {
	newKnowledgeFixture(t)
	wsfx := testutil.New(testPool, testWorkspaceID, testUserID)
	leader := wsfx.Agent(t, "sales lead "+t.Name(), "")
	sales := wsfx.Squad(t, "Sales "+t.Name(), leader, testutil.Cols{"template_key": "sales_loop"})
	plain := wsfx.Squad(t, "Plain "+t.Name(), leader)

	var out SalesSourceIntakeResponse
	testutil.Call(t, testHandler.CreateSquadSource,
		testutil.WithURLParams(newRequest(http.MethodPost, "/api/squads/"+sales+"/sources",
			map[string]any{"input": "https://example.vn/tin-tuc"}), "id", sales)).
		Want(http.StatusCreated).JSON(&out)
	if out.Document.ID == "" || out.Document.KnowledgeKind != "sales_source" || out.Document.State != "published" || out.IssueID == "" {
		t.Fatalf("intake = %+v", out)
	}

	testutil.Call(t, testHandler.CreateSquadSource,
		testutil.WithURLParams(newRequest(http.MethodPost, "/api/squads/"+plain+"/sources",
			map[string]any{"input": "https://example.vn"}), "id", plain)).
		Want(http.StatusBadRequest)

	testutil.Call(t, testHandler.CreateSquadShare,
		testutil.WithURLParams(newRequest(http.MethodPost, "/api/squads/"+sales+"/shares",
			map[string]any{"input": "Ai biết nhà cung cấp máy đóng gói?"}), "id", sales)).
		Want(http.StatusCreated)

	testutil.Call(t, testHandler.CreateSquadSource,
		testutil.WithURLParams(newRequest(http.MethodPost, "/api/squads/"+sales+"/sources",
			map[string]any{"input": "   "}), "id", sales)).
		Want(http.StatusBadRequest)
}
