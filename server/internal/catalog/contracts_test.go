package catalog

import "testing"

func TestRequiredContractsProvided(t *testing.T) {
	idx, err := Contracts()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range RequiredContracts {
		if len(idx.Stable(id)) == 0 {
			t.Errorf("contract %s has no stable provider: a template that the server finds by name was removed, renamed or hidden", id)
		}
	}
}

func TestEveryDeclaredContractIsOneTheServerKnows(t *testing.T) {
	idx, err := Contracts()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, id := range RequiredContracts {
		known[id] = true
	}
	for id := range idx.Providers {
		if !known[id] {
			t.Errorf("a template declares contract %s, which the server does not list in RequiredContracts; add it there or drop it from the template", id)
		}
	}
}
