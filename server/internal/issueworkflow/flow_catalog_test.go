package issueworkflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/multica-ai/multica/server/internal/catalog"
)

// catalogFlow is one entry of the generated flows.json (scripts/catalog build-workflows.mjs).
type catalogFlow struct {
	ID     string            `json:"id"`
	Rev    string            `json:"rev"`
	Squad  string            `json:"squad"`
	Config json.RawMessage   `json:"config"`
	Steps  []catalogFlowStep `json:"steps"`
}

type catalogFlowStep struct {
	StatusKey string `json:"statusKey"`
	Status    struct {
		Key      string `json:"key"`
		Name     string `json:"name"`
		Category string `json:"category"`
	} `json:"status"`
	HumanOnly bool `json:"humanOnly"`
	Tasks     []struct {
		Key          string     `json:"key"`
		Title        string     `json:"title"`
		Kind         string     `json:"kind"`
		Role         string     `json:"role"`
		Instructions string     `json:"instructions"`
		When         *Condition `json:"when"`
		Requires     []string   `json:"requires"`
	} `json:"tasks"`
	Transitions []struct {
		ID     string     `json:"id"`
		Label  string     `json:"label"`
		When   *Condition `json:"when"`
		To     string     `json:"to"`
		Rework bool       `json:"rework"`
	} `json:"transitions"`
}

// definition turns a catalog flow into the Definition the server stores, giving each role its own
// agent id, as the quickstart does with the squad's real agents.
func (f catalogFlow) definition() (Definition, map[string]CatalogEntry, error) {
	cfg, err := ParseFlowConfig(f.Config)
	if err != nil {
		return Definition{}, nil, err
	}
	agents := map[string]string{}
	agentFor := func(role string) string {
		if id, ok := agents[role]; ok {
			return id
		}
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", len(agents)+1)
		agents[role] = id
		return id
	}
	def := Definition{Name: f.ID, Flow: cfg}
	catalogEntries := map[string]CatalogEntry{}
	for _, s := range f.Steps {
		step := Step{StatusKey: s.StatusKey, Handler: Handler{Type: HandlerNone}, HumanOnly: s.HumanOnly}
		if s.Status.Name != "" {
			catalogEntries[s.StatusKey] = CatalogEntry{Key: s.StatusKey, Name: s.Status.Name, Category: s.Status.Category}
		}
		for _, t := range s.Tasks {
			step.Tasks = append(step.Tasks, Task{
				Key: t.Key, Title: t.Title, Kind: t.Kind, Handler: Handler{Type: HandlerAgent, ID: agentFor(t.Role)},
				Instructions: t.Instructions, When: t.When, Requires: t.Requires,
			})
		}
		for _, t := range s.Transitions {
			step.Transitions = append(step.Transitions, Transition{ID: t.ID, Label: t.Label, When: t.When, ToStatusKey: t.To, Rework: t.Rework})
		}
		def.Steps = append(def.Steps, step)
	}
	return def, catalogEntries, nil
}

func loadCatalogFlows(t *testing.T) map[string]catalogFlow {
	t.Helper()
	raw := catalog.FlowsJSON()
	if len(raw) == 0 {
		return nil
	}
	var flows map[string]catalogFlow
	if err := json.Unmarshal(raw, &flows); err != nil {
		t.Fatalf("decode flows.json: %v", err)
	}
	return flows
}

// A19: every flow the catalog ships passes the server's own validation. With no flows (or a catalog
// that has not been built) there is nothing to check and the test passes.
func TestCatalogFlowsPassValidateFlow(t *testing.T) {
	flows := loadCatalogFlows(t)
	ids := make([]string, 0, len(flows))
	for id := range flows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			f := flows[id]
			def, entries, err := f.definition()
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateWithFlow(f.ID, "", def.Steps, false, entries, def.Flow); err != nil {
				t.Fatalf("%s: %v", id, err)
			}
			if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(f.Rev) {
				t.Fatalf("%s: rev %q must be 8 hex digits", id, f.Rev)
			}
			if f.ID != "flow_"+f.Squad {
				t.Fatalf("%s: id must be flow_%s", id, f.Squad)
			}
			// The roles of the squad the manifest names: the same rules as the
			// catalog tool, with the roster of that squad.
			roster := squadRoster(t, f)
			if issues := ValidateSchema1ForSquad(def, roster); HasErrors(issues) {
				t.Fatalf("%s: ValidateSchema1ForSquad:%s", id, describeIssues(issues))
			}
		})
	}
}

// squadManifest is the part of catalog/squads/<id>/template.json that the roster needs.
type squadManifest struct {
	Squad struct {
		Leader string `json:"leader"`
		Roles  []struct {
			Key string `json:"key"`
		} `json:"roles"`
	} `json:"squad"`
}

// squadRoster builds the roster of the squad a catalog flow names (flow.squad in
// its manifest). Every role of the squad other than the leader is an agent. The
// agents that the flow uses have the ids definition() gave them; the others get
// ids of their own, so that a role no task uses is found.
func squadRoster(t *testing.T, f catalogFlow) Roster {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "catalog", "squads", f.Squad, "template.json"))
	if err != nil {
		t.Fatalf("%s: squad manifest: %v", f.ID, err)
	}
	var m squadManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: squad manifest: %v", f.ID, err)
	}
	roleIDs, _ := manifestRoleKeys(f)
	idOfRole := map[string]string{}
	for id, role := range roleIDs {
		idOfRole[role] = id
	}
	roster := Roster{Lead: "00000000-0000-4000-8000-00000000ffff", RoleKeys: map[string]string{}}
	spare := 0
	for _, r := range m.Squad.Roles {
		if r.Key == m.Squad.Leader {
			roster.RoleKeys[roster.Lead] = r.Key
			continue
		}
		id, used := idOfRole[r.Key]
		if !used {
			spare++
			id = fmt.Sprintf("00000000-0000-4000-8000-0000000fe%03d", spare)
		}
		roster.Agents = append(roster.Agents, id)
		roster.RoleKeys[id] = r.Key
	}
	return roster
}

// forkInputs gives what the validator needs to check the fork of a catalog flow:
// the definition as a stored workflow row holds it (the fixed statuses blocked,
// done and cancelled are steps; the catalog manifest leaves them out), the
// validation context with the roster of the flow's squad, and the status catalog
// that has the steps of the flow.
func forkInputs(t *testing.T, f catalogFlow) (Definition, ValidateContext) {
	t.Helper()
	def, _, err := f.definition()
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	statuses := DefaultStatusCatalog()
	for _, s := range def.Steps {
		have[s.StatusKey] = true
		if _, ok := statuses[s.StatusKey]; !ok {
			statuses[s.StatusKey] = StatusInfo{Category: "started"}
		}
	}
	for _, key := range []string{"blocked", "done", "cancelled"} {
		if !have[key] {
			def.Steps = append(def.Steps, Step{StatusKey: key, Handler: Handler{Type: HandlerNone}})
		}
	}
	return def, ValidateContext{Roster: squadRoster(t, f), Statuses: statuses}
}
