package catalog

import (
	"encoding/json"
	"fmt"
	"sync"
)

// A contract is a promise between the server and a template: the server finds or builds something by
// the template's name or id, so that template must stay in the catalog (spec 6.3.4). A template
// declares the contract in its `contracts` field. RequiredContracts lists what the server depends on;
// TestRequiredContractsProvided fails when a template that provides one is removed, renamed or
// hidden, instead of the server failing at run time in a customer's workspace.
var RequiredContracts = []string{
	// The SDLC squads and their build workflow: service/initiative_build.go hands work to these agents by name.
	"squad.sdlc",
	"workflow.sdlc_auto",
	// The planning squad and its workflow: service/squad_planning_workflow.go finds both by name.
	"squad.planning",
	"workflow.planning_auto",
	// The starter company's Executive Company card and its Librarian: service/company_template.go.
	"company.executive_office",
}

// ContractProvider is a template that declares a contract.
type ContractProvider struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Status string `json:"status"`
}

// ContractSquad is what the server reads of a squad that provides a contract.
type ContractSquad struct {
	Name  string            `json:"name"`
	Roles map[string]string `json:"roles"` // role key -> agent name
}

// ContractWorkflow is what the server reads of a workflow that provides a contract.
type ContractWorkflow struct {
	Name string `json:"name"`
}

// ContractIndex is the generated contracts.json.
type ContractIndex struct {
	Providers map[string][]ContractProvider `json:"providers"`
	Squads    map[string]ContractSquad      `json:"squads"`
	Workflows map[string]ContractWorkflow   `json:"workflows"`
}

// Stable returns the providers of a contract that are offered to users (status stable).
func (c ContractIndex) Stable(contract string) []ContractProvider {
	var out []ContractProvider
	for _, p := range c.Providers[contract] {
		if p.Status == "stable" {
			out = append(out, p)
		}
	}
	return out
}

var loadContracts = sync.OnceValues(func() (ContractIndex, error) {
	raw := read("contracts.json")
	if raw == nil {
		return ContractIndex{}, fmt.Errorf("%s", BuildHint)
	}
	var idx ContractIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return ContractIndex{}, fmt.Errorf("parse contracts.json: %w", err)
	}
	return idx, nil
})

// Contracts returns the generated index of who provides which contract.
func Contracts() (ContractIndex, error) { return loadContracts() }
