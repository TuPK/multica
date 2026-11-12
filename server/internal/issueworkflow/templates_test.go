package issueworkflow

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/multica-ai/multica/server/internal/issuestatus"
)

// defaultCatalog is the workspace status catalog a fresh workspace starts
// with: the seven built-in statuses, each usable without a catalog row.
func defaultCatalog() map[string]CatalogEntry {
	catalog := map[string]CatalogEntry{}
	for _, key := range issuestatus.Canonical() {
		category, _ := issuestatus.CategoryForBehavior(key)
		catalog[key] = CatalogEntry{Key: key, Name: key, Category: category}
	}
	return catalog
}

// catalogAfterCreate is the catalog once a save has created the statuses a
// template names. Each created status gets the key DeriveKey gives its name.
func catalogAfterCreate(t *testing.T, tpl Template) map[string]CatalogEntry {
	t.Helper()
	catalog := defaultCatalog()
	for _, s := range tpl.Steps {
		if s.New == nil {
			continue
		}
		key, err := issuestatus.DeriveKey(s.New.Name, s.New.Category, nil)
		if err != nil {
			t.Fatalf("%s: derive key for %q: %v", tpl.ID, s.New.Name, err)
		}
		catalog[key] = CatalogEntry{Key: key, Name: s.New.Name, Category: s.New.Category}
	}
	return catalog
}

func templateByID(t *testing.T, id string) Template {
	t.Helper()
	for _, tpl := range Templates() {
		if tpl.ID == id {
			return tpl
		}
	}
	t.Fatalf("no template %q", id)
	return Template{}
}

// routeLines lists a template's transitions as "from branch -> to", in board
// order and branch order. A rejected branch and a blocked branch keep their ids.
func routeLines(t *testing.T, tpl Template) []string {
	t.Helper()
	steps, err := tpl.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var lines []string
	for _, s := range steps {
		for _, tr := range s.Transitions {
			lines = append(lines, s.StatusKey+" "+tr.ID+" -> "+tr.ToStatusKey)
		}
	}
	return lines
}

func TestTemplateIDsAreStable(t *testing.T) {
	var ids []string
	for _, tpl := range Templates() {
		ids = append(ids, tpl.ID)
	}
	want := []string{"delivery", "content", "community", "sdlc_auto", "bmad_planning_auto", "sales_loop", "quick_fix"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("template ids = %v, want %v", ids, want)
	}
}

// TestTemplatesPassWorkflowCreateValidation runs each template through the
// same Normalize and Validate that CreateIssueWorkflow applies to a saved
// workflow. The catalog includes the statuses the save would create first.
func TestTemplatesPassWorkflowCreateValidation(t *testing.T) {
	for _, tpl := range Templates() {
		t.Run(tpl.ID, func(t *testing.T) {
			steps, err := tpl.Resolve()
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			name, description, normalized := Normalize("Template "+tpl.ID, "", steps)
			if err := Validate(name, description, normalized, false, catalogAfterCreate(t, tpl)); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if !reflect.DeepEqual(keysOf(normalized), keysOf(steps)) {
				t.Fatalf("Normalize changed the board order: %v, want %v", keysOf(normalized), keysOf(steps))
			}
		})
	}
}

// TestTemplateCreatedStatusKeysAreDerived checks that a column the save creates
// is keyed by the server's own derivation of its name, which is the key the
// transitions name.
func TestTemplateCreatedStatusKeysAreDerived(t *testing.T) {
	for _, tpl := range Templates() {
		steps, err := tpl.Resolve()
		if err != nil {
			t.Fatalf("%s: Resolve: %v", tpl.ID, err)
		}
		for i, s := range tpl.Steps {
			if s.New == nil {
				continue
			}
			want, err := issuestatus.DeriveKey(s.New.Name, s.New.Category, nil)
			if err != nil {
				t.Fatalf("%s: DeriveKey(%q): %v", tpl.ID, s.New.Name, err)
			}
			if steps[i].StatusKey != want {
				t.Errorf("%s: %q keyed %q, want the derived key %q", tpl.ID, s.New.Name, steps[i].StatusKey, want)
			}
		}
	}
}

func TestTemplatesReferenceOnlyBuiltInStatusesByKey(t *testing.T) {
	for _, tpl := range Templates() {
		for _, step := range tpl.Steps {
			if step.New == nil && !issuestatus.IsBuiltIn(step.StatusKey) {
				t.Errorf("%s names %q by key, but only built-in statuses exist without a save", tpl.ID, step.StatusKey)
			}
			if step.New != nil && issuestatus.IsBuiltIn(step.New.Name) {
				t.Errorf("%s creates %q, which collides with a built-in status", tpl.ID, step.New.Name)
			}
		}
	}
}

func TestTemplatesListTodoDoneAndCancelled(t *testing.T) {
	for _, tpl := range Templates() {
		steps, err := tpl.Resolve()
		if err != nil {
			t.Fatalf("%s: Resolve: %v", tpl.ID, err)
		}
		def := Definition{Steps: steps}
		for _, required := range []string{issuestatus.Todo, issuestatus.Done, issuestatus.Cancelled} {
			if !def.Has(required) {
				t.Errorf("%s does not list %q", tpl.ID, required)
			}
		}
	}
}

// TestTemplateRouting pins the transitions of every template. Each line is
// "from branch -> to". A rejected branch is the rework route, and a blocked
// branch is the escape to Blocked. Both sit before the default.
func TestTemplateRouting(t *testing.T) {
	cases := map[string][]string{
		TemplateDelivery: {
			"todo default -> in_progress",
			"in_progress default -> in_review",
			"in_review rejected -> in_progress",
			"in_review default -> qa",
			"qa default -> done",
		},
		TemplateContent: {
			"todo default -> drafting",
			"drafting default -> editing",
			"editing default -> scheduled",
			"scheduled default -> done",
		},
		TemplateCommunity: {
			"todo default -> triage",
			"triage default -> replying",
			"replying default -> waiting",
			"waiting default -> done",
		},
		TemplateSDLCAuto: {
			"backlog default -> todo",
			"todo default -> in_progress",
			"analysis blocked -> blocked",
			"analysis default -> stage_done",
			"design blocked -> blocked",
			"design default -> stage_done",
			"architecture blocked -> blocked",
			"architecture default -> stage_done",
			"plan_review blocked -> blocked",
			"plan_review default -> stage_done",
			"security_review blocked -> blocked",
			"security_review default -> stage_done",
			"release_prep blocked -> blocked",
			"release_prep default -> stage_done",
			"environment blocked -> blocked",
			"environment default -> stage_done",
			"harness blocked -> blocked",
			"harness default -> stage_done",
			"in_progress rejected -> todo",
			"in_progress default -> in_review",
			"in_review rejected -> in_progress",
			"in_review default -> qa",
			"qa rejected -> in_progress",
			"qa default -> done",
			"blocked default -> in_progress",
		},
		TemplateBMADPlanningAuto: {
			"backlog default -> todo",
			"research blocked -> blocked",
			"research default -> stage_done",
			"brief blocked -> blocked",
			"brief default -> stage_done",
			"prd blocked -> blocked",
			"prd default -> stage_done",
			"design blocked -> blocked",
			"design default -> stage_done",
			"architecture blocked -> blocked",
			"architecture default -> stage_done",
			"epics blocked -> blocked",
			"epics default -> stage_done",
			"plan_review blocked -> blocked",
			"plan_review default -> stage_done",
		},
		TemplateSalesLoop: {
			"todo default -> scouting",
			"scouting default -> drafting",
			"drafting default -> checking",
			"checking rejected -> drafting",
			"checking default -> reporting",
			"reporting default -> done",
		},
		TemplateQuickFix: {
			"todo default -> in_progress",
			"in_progress default -> done",
		},
	}
	for _, tpl := range Templates() {
		want, ok := cases[tpl.ID]
		if !ok {
			t.Fatalf("no routing expectation for %s", tpl.ID)
		}
		if got := routeLines(t, tpl); !reflect.DeepEqual(got, want) {
			t.Errorf("%s routes:\n got %v\nwant %v", tpl.ID, got, want)
		}
	}
}

// TestTemplateRejectionBranches names the templates that keep a rejected
// (rework) branch. The legacy templates keep one only where they had one.
func TestTemplateRejectionBranches(t *testing.T) {
	want := map[string][]string{
		TemplateDelivery:         {"in_review"},
		TemplateSDLCAuto:         {"in_progress", "in_review", "qa"},
		TemplateSalesLoop:        {"checking"},
		TemplateContent:          nil,
		TemplateCommunity:        nil,
		TemplateBMADPlanningAuto: nil,
		TemplateQuickFix:         nil,
	}
	for _, tpl := range Templates() {
		steps, err := tpl.Resolve()
		if err != nil {
			t.Fatalf("%s: Resolve: %v", tpl.ID, err)
		}
		var got []string
		for _, s := range steps {
			for _, tr := range s.Transitions {
				if tr.ID == "rejected" {
					got = append(got, s.StatusKey)
				}
			}
		}
		if !reflect.DeepEqual(got, want[tpl.ID]) {
			t.Errorf("%s rejected branches on %v, want %v", tpl.ID, got, want[tpl.ID])
		}
	}
}

func TestDeliveryReworksRejectedReview(t *testing.T) {
	steps, err := templateByID(t, TemplateDelivery).Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	review, ok := (Definition{Steps: steps}).Step(issuestatus.InReview)
	if !ok {
		t.Fatal("delivery has no in_review step")
	}
	back, ok := review.Route(Facts{Outcome: OutcomeRejected})
	if !ok || back.ToStatusKey != issuestatus.InProgress {
		t.Fatalf("rejected review routes to %+v, want %q", back, issuestatus.InProgress)
	}
	if back.When == nil || back.When.Field != FieldOutcome || back.When.Op != OpEq || back.When.Value != OutcomeRejected {
		t.Fatalf("rejected branch condition = %+v, want outcome eq rejected", back.When)
	}
	forward, ok := review.Route(Facts{Outcome: OutcomeApproved})
	if !ok || forward.ToStatusKey != "qa" {
		t.Fatalf("approved review routes to %+v, want qa", forward)
	}
}

func TestQuickFixListsOnlyTheThreeWorkStatuses(t *testing.T) {
	steps, err := templateByID(t, TemplateQuickFix).Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := []string{issuestatus.Todo, issuestatus.InProgress, issuestatus.Done, issuestatus.Cancelled}
	if got := keysOf(steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("quick_fix statuses = %v, want %v", got, want)
	}
}

// TestTemplateStepsStayWithinLimits checks the copy and roles a template may
// carry: short instructions, non-empty transition labels, and only squad role
// ids as hints. Handlers are none everywhere, since a template names no agent.
func TestTemplateStepsStayWithinLimits(t *testing.T) {
	roles := map[string]bool{
		"lead": true, "analyst": true, "ux": true, "architect": true, "developer": true,
		"reviewer": true, "qa": true, "security": true, "devops": true,
		"plan_researcher": true, "plan_analyst": true, "plan_pm": true, "plan_ux": true,
		"plan_architect": true, "plan_reviewer": true, "plan_lead": true,
		"sales_scout": true, "sales_writer": true, "sales_checker": true, "sales_analyst": true,
	}
	for _, tpl := range Templates() {
		steps, err := tpl.Resolve()
		if err != nil {
			t.Fatalf("%s: Resolve: %v", tpl.ID, err)
		}
		for i, s := range steps {
			if s.Handler.Type != HandlerNone || s.Handler.ID != "" {
				t.Errorf("%s: %s has handler %+v, want none", tpl.ID, s.StatusKey, s.Handler)
			}
			if n := utf8.RuneCountInString(tpl.Steps[i].Instructions); n > 400 {
				t.Errorf("%s: %s instructions are %d characters, want at most 400", tpl.ID, s.StatusKey, n)
			}
			if role := tpl.Steps[i].RoleHint; role != "" && !roles[role] {
				t.Errorf("%s: %s role hint %q is not a squad role id", tpl.ID, s.StatusKey, role)
			}
			for _, tr := range s.Transitions {
				if strings.TrimSpace(tr.Label) == "" {
					t.Errorf("%s: transition %s on %s has no label", tpl.ID, tr.ID, s.StatusKey)
				}
			}
		}
	}
}
