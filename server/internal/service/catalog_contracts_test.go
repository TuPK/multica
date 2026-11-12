package service

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/catalog"
)

// The server finds these catalog templates by name. If a template is renamed in catalog/, the server
// stops finding it at run time; these tests fail first.

func TestSDLCSquadsProvideTheAgentsTheBuildHandsTo(t *testing.T) {
	idx, err := catalog.Contracts()
	if err != nil {
		t.Fatal(err)
	}
	providers := idx.Stable("squad.sdlc")
	if len(providers) == 0 {
		t.Fatal("no stable squad provides squad.sdlc")
	}
	for _, p := range providers {
		squad := idx.Squads[p.ID]
		for _, r := range sdlcRoleAgents {
			got, ok := squad.Roles[r.role]
			switch {
			case ok && got != r.name:
				t.Errorf("squad %s names its %s agent %q, but the build looks for %q", p.ID, r.role, got, r.name)
			case !ok && r.required:
				t.Errorf("squad %s has no %s role, which the build requires", p.ID, r.role)
			}
		}
	}
}

func TestSDLCWorkflowNameMatchesTheCatalog(t *testing.T) {
	idx, err := catalog.Contracts()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range idx.Stable("workflow.sdlc_auto") {
		if got := idx.Workflows[p.ID].Name; got != sdlcWorkflowName {
			t.Errorf("catalog workflow %s is named %q, the server looks for %q", p.ID, got, sdlcWorkflowName)
		}
	}
}

func TestPlanningSquadAndWorkflowMatchTheNamesTheServerFinds(t *testing.T) {
	idx, err := catalog.Contracts()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range idx.Stable("squad.planning") {
		found := false
		for _, name := range idx.Squads[p.ID].Roles {
			found = found || name == planningLeadAgentName
		}
		if !found {
			t.Errorf("squad %s has no agent named %q, which EnsureSquadProjectWorkflow uses to recognise a planning squad", p.ID, planningLeadAgentName)
		}
	}
	for _, p := range idx.Stable("workflow.planning_auto") {
		if got := idx.Workflows[p.ID].Name; got != planningWorkflowName {
			t.Errorf("catalog workflow %s is named %q, the server looks for %q", p.ID, got, planningWorkflowName)
		}
	}
}

func TestStarterCompanyHasTheExecutiveOfficeAndItsLibrarian(t *testing.T) {
	data, err := loadCompanyData()
	if err != nil {
		t.Fatal(err)
	}
	card, ok := data.squad("executive_office")
	if !ok {
		t.Fatal("the starter company has no executive_office card")
	}
	for _, role := range card.Roles {
		if role.Key == "librarian" && role.Name != "" {
			return
		}
	}
	t.Fatal("the executive_office card has no librarian role")
}
