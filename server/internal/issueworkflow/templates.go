package issueworkflow

import (
	"encoding/json"
	"fmt"

	"github.com/multica-ai/multica/server/internal/catalog"
	"github.com/multica-ai/multica/server/internal/issuestatus"
)

// System workflow templates. Their data lives in catalog/workflows/designer_*; this file reads it.
// A template is a starting point the designer copies
// into an unsaved draft. Saving the draft goes through the same create path as
// any workflow, so every template must pass Validate.
//
// A template column names either an existing status by key, or a status the
// save creates by name. A created status takes the key issuestatus.DeriveKey
// gives its name, and the template's transitions name that key. The frontend
// maps each id to its translated name and description, so a new id needs a
// matching frontend entry.
const (
	TemplateDelivery         = "delivery"
	TemplateContent          = "content"
	TemplateCommunity        = "community"
	TemplateSDLCAuto         = "sdlc_auto"
	TemplateBMADPlanningAuto = "bmad_planning_auto"
	TemplateSalesLoop        = "sales_loop"
	TemplateQuickFix         = "quick_fix"
)

// TemplateStatus is a status a template creates when it is saved.
type TemplateStatus struct {
	Name     string
	Category string
}

// TemplateStep is one column of a template. Exactly one of StatusKey and New
// is set. RoleHint is a squad role id the designer shows as a suggestion; it is
// never stored, and a template never names an agent, so every handler is none.
type TemplateStep struct {
	StatusKey    string
	New          *TemplateStatus
	RoleHint     string
	Instructions string
	Transitions  []Transition
}

// Template is a system starting point for a workflow, with its columns in board
// order. Every template lists todo, done and cancelled.
type Template struct {
	ID    string
	Steps []TemplateStep
}

// Templates returns the system templates, in the order the picker lists them. They are read from the
// catalog (catalog/workflows/designer_*, built into the JSON the server embeds by `pnpm catalog:build`).
// Each call builds new values, so a caller may change the result without affecting later calls.
//
// It panics when the catalog has not been built or a template does not parse: the server cannot serve
// the designer without its templates, and a build step that skipped the catalog should fail loudly.
func Templates() []Template {
	raw := catalog.DesignerWorkflowsJSON()
	if len(raw) == 0 {
		panic("issueworkflow templates: " + catalog.BuildHint)
	}
	var data []catalogTemplate
	if err := json.Unmarshal(raw, &data); err != nil {
		panic(fmt.Sprintf("issueworkflow templates: parse catalog data: %v", err))
	}
	out := make([]Template, len(data))
	for i, t := range data {
		steps := make([]TemplateStep, len(t.Steps))
		for j, s := range t.Steps {
			steps[j] = s.toStep()
		}
		out[i] = Template{ID: t.ID, Steps: steps}
	}
	return out
}

// catalogTemplate is a designer template as the catalog writes it.
type catalogTemplate struct {
	ID    string                `json:"id"`
	Steps []catalogTemplateStep `json:"steps"`
}

type catalogTemplateStep struct {
	StatusKey string `json:"statusKey"`
	New       *struct {
		Name     string `json:"name"`
		Category string `json:"category"`
	} `json:"new"`
	RoleHint     string `json:"roleHint"`
	Instructions string `json:"instructions"`
	Transitions  []struct {
		On string `json:"on"`
		To string `json:"to"`
	} `json:"transitions"`
}

func (s catalogTemplateStep) toStep() TemplateStep {
	step := TemplateStep{StatusKey: s.StatusKey, RoleHint: s.RoleHint, Instructions: s.Instructions}
	if s.New != nil {
		step.New = &TemplateStatus{Name: s.New.Name, Category: s.New.Category}
	}
	for _, t := range s.Transitions {
		switch t.On {
		case "":
			step.Transitions = append(step.Transitions, defaultTo(t.To))
		case OutcomeRejected:
			step.Transitions = append(step.Transitions, rejectedTo(t.To))
		case OutcomeBlocked:
			step.Transitions = append(step.Transitions, blockedTo(t.To))
		default:
			panic(fmt.Sprintf("issueworkflow templates: transition on %q is not a known outcome", t.On))
		}
	}
	return step
}

// Resolve returns the steps a save of the template writes. A created status is
// keyed by issuestatus.DeriveKey of its name, the key the transitions name.
// Handlers are none, and the instructions are kept as the template gives them.
func (t Template) Resolve() ([]Step, error) {
	steps := make([]Step, len(t.Steps))
	for i, ts := range t.Steps {
		key := ts.StatusKey
		if ts.New != nil {
			derived, err := issuestatus.DeriveKey(ts.New.Name, ts.New.Category, nil)
			if err != nil {
				return nil, fmt.Errorf("template %s: status %q: %w", t.ID, ts.New.Name, err)
			}
			key = derived
		}
		steps[i] = Step{
			StatusKey:    key,
			Handler:      Handler{Type: HandlerNone},
			Instructions: ts.Instructions,
			Transitions:  ts.Transitions,
		}
	}
	return steps, nil
}

// defaultTo is the branch taken when no other condition matches.
func defaultTo(to string) Transition {
	return Transition{ID: "default", Label: "Default", ToStatusKey: to}
}

// rejectedTo is taken when the handler reports rejected: a review or QA sends
// the work back.
func rejectedTo(to string) Transition {
	return Transition{ID: "rejected", Label: "Rejected", When: outcomeCondition(OutcomeRejected), ToStatusKey: to}
}

// blockedTo is taken when the handler reports blocked: it cannot pass the step.
func blockedTo(to string) Transition {
	return Transition{ID: "blocked", Label: "Blocked", When: outcomeCondition(OutcomeBlocked), ToStatusKey: to}
}
