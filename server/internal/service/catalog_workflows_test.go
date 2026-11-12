package service

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
)

// The designer's SDLC Auto template (issueworkflow.Templates) is a deliberate variant of the build
// workflow in the catalog: handlers are always none, instructions are shortened, and it adds the done
// and cancelled columns. What must not drift is which statuses it has and which role each hands to.
func TestDesignerSDLCAutoMatchesTheCatalogWorkflow(t *testing.T) {
	workflows, err := loadCatalogWorkflows()
	if err != nil {
		t.Fatal(err)
	}
	build, ok := workflows["sdlc_auto"]
	if !ok {
		t.Fatal("the catalog has no sdlc_auto workflow")
	}
	roleOf := map[string]string{}
	wantKeys := map[string]bool{issuestatus.Done: true, issuestatus.Cancelled: true}
	for _, st := range build.Steps {
		wantKeys[st.StatusKey] = true
		if st.Role != nil {
			roleOf[st.StatusKey] = *st.Role
		}
	}

	var designer *issueworkflow.Template
	for _, tpl := range issueworkflow.Templates() {
		if tpl.ID == issueworkflow.TemplateSDLCAuto {
			tpl := tpl
			designer = &tpl
		}
	}
	if designer == nil {
		t.Fatal("no designer template sdlc_auto")
	}
	got := map[string]bool{}
	for _, step := range designer.Steps {
		key := step.StatusKey
		if step.New != nil {
			derived, err := issuestatus.DeriveKey(step.New.Name, step.New.Category, nil)
			if err != nil {
				t.Fatal(err)
			}
			key = derived
		}
		got[key] = true
		if want, has := roleOf[key]; has && step.RoleHint != want {
			t.Errorf("designer step %s hints role %q, the build workflow hands it to %q", key, step.RoleHint, want)
		}
	}
	for key := range wantKeys {
		if !got[key] {
			t.Errorf("designer sdlc_auto has no %s step; the build workflow has", key)
		}
	}
	for key := range got {
		if !wantKeys[key] {
			t.Errorf("designer sdlc_auto has a %s step the build workflow lacks", key)
		}
	}
}
