package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/quickstart"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// The catalog is the quickstart bundle's plugins, so the picker and the machine
// installer read one list.
func TestListSquadPluginCatalogMatchesQuickstartBundle(t *testing.T) {
	bundle, err := quickstart.LoadBundle()
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	req := newRequest("GET", "/api/squads/plugin-catalog", nil)
	var got []SquadPluginResponse
	testutil.Call(t, testHandler.ListSquadPluginCatalog, req).Want(http.StatusOK).JSON(&got)
	if len(got) != len(bundle.Plugins) {
		t.Fatalf("catalog has %d plugins, bundle has %d", len(got), len(bundle.Plugins))
	}
	for i, id := range bundle.Plugins {
		if got[i].Name != id {
			t.Fatalf("plugin %d = %q, want %q", i, got[i].Name, id)
		}
		if got[i].InstallHint != "claude plugin install "+id {
			t.Fatalf("install hint for %q = %q", id, got[i].InstallHint)
		}
	}
}
