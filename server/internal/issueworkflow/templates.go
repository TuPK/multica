package issueworkflow

import (
	"fmt"

	"github.com/multica-ai/multica/server/internal/issuestatus"
)

// System workflow templates. A template is a starting point the designer copies
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

// Templates returns the system templates. Each call builds new values, so a
// caller may change the result without affecting later calls.
func Templates() []Template {
	return []Template{
		{ID: TemplateDelivery, Steps: []TemplateStep{
			keyed(issuestatus.Todo, defaultTo(issuestatus.InProgress)),
			keyed(issuestatus.InProgress, defaultTo(issuestatus.InReview)),
			// A rejected review goes back to In Progress; any other outcome moves on to QA.
			keyed(issuestatus.InReview, rejectedTo(issuestatus.InProgress), defaultTo("qa")),
			created("QA", issuestatus.CategoryStarted, defaultTo(issuestatus.Done)),
			keyed(issuestatus.Done),
			keyed(issuestatus.Cancelled),
		}},
		{ID: TemplateContent, Steps: []TemplateStep{
			keyed(issuestatus.Todo, defaultTo("drafting")),
			created("Drafting", issuestatus.CategoryStarted, defaultTo("editing")),
			created("Editing", issuestatus.CategoryStarted, defaultTo("scheduled")),
			created("Scheduled", issuestatus.CategoryStarted, defaultTo(issuestatus.Done)),
			keyed(issuestatus.Done),
			keyed(issuestatus.Cancelled),
		}},
		{ID: TemplateCommunity, Steps: []TemplateStep{
			keyed(issuestatus.Todo, defaultTo("triage")),
			created("Triage", issuestatus.CategoryUnstarted, defaultTo("replying")),
			created("Replying", issuestatus.CategoryStarted, defaultTo("waiting")),
			created("Waiting", issuestatus.CategoryStarted, defaultTo(issuestatus.Done)),
			keyed(issuestatus.Done),
			keyed(issuestatus.Cancelled),
		}},
		sdlcAutoTemplate(),
		bmadPlanningAutoTemplate(),
		salesLoopTemplate(),
		quickFixTemplate(),
	}
}

func sdlcAutoTemplate() Template {
	return Template{ID: TemplateSDLCAuto, Steps: []TemplateStep{
		keyed(issuestatus.Backlog, defaultTo(issuestatus.Todo)),
		keyed(issuestatus.Todo, defaultTo(issuestatus.InProgress)).
			withRole("developer", "Read the issue's spec, stories and acceptance criteria, then start the build."),
		planningStage("Analysis", "analyst", "Write the requirements, epics and stories for the request, and post them on the issue.", "stage_done"),
		planningStage("Design", "ux", "Design every screen the requirements need, with its states, and register each screen.", "stage_done"),
		planningStage("Architecture", "architect", "Write the design, decisions and component list for the change, and commit them.", "stage_done"),
		planningStage("Plan review", "reviewer", "Give one verdict on the planning documents. Report blocked with each gap listed when they are not ready.", "stage_done"),
		planningStage("Security review", "security", "Review the planned change for security risks. Report blocked with each finding listed when it fails.", "stage_done"),
		planningStage("Release prep", "devops", "Prepare the version, digests and rollback steps. Production needs a named approval, so report blocked and state the decision needed.", "stage_done"),
		planningStage("Environment", "devops", "Keep the environment running and its rebuild command working, and post the health check.", "stage_done"),
		planningStage("Harness", "architect", "Check that the repository is ready for agents to build in, and post the result of every blocking check.", "stage_done"),
		created("Stage done", issuestatus.CategoryDone),
		created("Plan ready", issuestatus.CategoryStarted),
		keyed(issuestatus.InProgress, rejectedTo(issuestatus.Todo), defaultTo(issuestatus.InReview)).
			withRole("developer", "Build every story the issue asks for and verify it on the running app before handing it to review."),
		keyed(issuestatus.InReview, rejectedTo(issuestatus.InProgress), defaultTo("qa")).
			withRole("reviewer", "Review the whole change against its acceptance criteria. Send it back with the blocking findings, or pass it on to QA."),
		created("QA", issuestatus.CategoryStarted, rejectedTo(issuestatus.InProgress), defaultTo(issuestatus.Done)).
			withRole("qa", "Verify the change on the running system against the acceptance criteria. Send defects back to In Progress, or finish the issue."),
		keyed(issuestatus.Blocked, defaultTo(issuestatus.InProgress)).
			withRole("lead", "Route each blocker to the owner who can unblock it, verify the ruling, and resume the issue at the step it left."),
		keyed(issuestatus.Done),
		keyed(issuestatus.Cancelled),
	}}
}

func bmadPlanningAutoTemplate() Template {
	return Template{ID: TemplateBMADPlanningAuto, Steps: []TemplateStep{
		keyed(issuestatus.Backlog, defaultTo(issuestatus.Todo)),
		keyed(issuestatus.Todo),
		planningStage("Research", "plan_researcher", "Research the market, domain, competitors and users, with a dated source for each claim.", "stage_done"),
		planningStage("Brief", "plan_analyst", "Turn the request and its research into a product brief, and test the weak ideas.", "stage_done"),
		planningStage("PRD", "plan_pm", "Write or extend the PRD, keep existing ids stable, and update its change log.", "stage_done"),
		planningStage("Design", "plan_ux", "Design or extend the screens, flows and states the requirements need.", "stage_done"),
		planningStage("Architecture", "plan_architect", "Record the architecture, decisions and impact of the change.", "stage_done"),
		planningStage("Epics", "plan_pm", "Write the epics, stories and traceability table, and check they are ready to build.", "stage_done"),
		planningStage("Plan review", "plan_reviewer", "Check the planning documents agree and can be built. Report blocked with each gap listed when they do not.", "stage_done"),
		created("Stage done", issuestatus.CategoryDone),
		created("Plan ready", issuestatus.CategoryStarted),
		keyed(issuestatus.Blocked).
			withRole("plan_lead", "Route each blocker to the member who owns the document, verify the answer, and resume the issue."),
		keyed(issuestatus.Done),
		keyed(issuestatus.Cancelled),
	}}
}

func salesLoopTemplate() Template {
	return Template{ID: TemplateSalesLoop, Steps: []TemplateStep{
		keyed(issuestatus.Todo, defaultTo("scouting")),
		created("Scouting", issuestatus.CategoryStarted, defaultTo("drafting")).
			withRole("sales_scout", "Run the lane's active sources, score the organisations, and write their accounts with a source and date for each claim."),
		created("Drafting", issuestatus.CategoryStarted, defaultTo("checking")).
			withRole("sales_writer", "Draft outreach for qualified accounts in the channel the lane allows. Nothing is sent."),
		created("Checking", issuestatus.CategoryStarted, rejectedTo("drafting"), defaultTo("reporting")).
			withRole("sales_checker", "Check each draft against the channel and compliance rules. Send a failing draft back to Drafting with the reason."),
		created("Reporting", issuestatus.CategoryStarted, defaultTo(issuestatus.Done)).
			withRole("sales_analyst", "Write the sales report for the scan, and list each lane that was skipped."),
		keyed(issuestatus.Done),
		keyed(issuestatus.Cancelled),
	}}
}

func quickFixTemplate() Template {
	return Template{ID: TemplateQuickFix, Steps: []TemplateStep{
		keyed(issuestatus.Todo, defaultTo(issuestatus.InProgress)).
			withRole("lead", "Take the bug or small fix and start it."),
		keyed(issuestatus.InProgress, defaultTo(issuestatus.Done)).
			withRole("developer", "Reproduce the defect, make the smallest fix, and verify it on the running app before finishing."),
		keyed(issuestatus.Done),
		keyed(issuestatus.Cancelled),
	}}
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

// keyed is a column for a status the workspace has by key.
func keyed(key string, branches ...Transition) TemplateStep {
	return TemplateStep{StatusKey: key, Transitions: branches}
}

// created is a column for a status the save creates.
func created(name, category string, branches ...Transition) TemplateStep {
	return TemplateStep{New: &TemplateStatus{Name: name, Category: category}, Transitions: branches}
}

// planningStage is a column an owner works and then moves on from. The owner
// reports blocked when it cannot pass the step; any other outcome moves the
// issue to next.
func planningStage(name, role, instructions, next string) TemplateStep {
	return created(name, issuestatus.CategoryStarted, blockedTo(issuestatus.Blocked), defaultTo(next)).
		withRole(role, instructions)
}

// withRole sets the squad role the column suggests and a short summary of its work.
func (s TemplateStep) withRole(role, instructions string) TemplateStep {
	s.RoleHint = role
	s.Instructions = instructions
	return s
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
