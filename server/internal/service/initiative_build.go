package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// The SDLC workflow the build stage uses. The quickstart creates it from
// SDLC_AUTO_WORKFLOW in packages/core/squads/templates.ts, through the workflow
// API, with the template squad's agents as handlers. The server cannot call that
// client code, so the workflow and its instructions are repeated here. Keep the
// two in step: a change to one is a change to both.
const (
	sdlcWorkflowName        = "SDLC Auto"
	sdlcWorkflowDescription = "Build, independent review, then QA, all by agents. A rejected step goes back to the developer."
	sdlcInProgressStatusKey = "in_progress"
	sdlcQAStatusKey         = "qa"
	sdlcQAStatusName        = "QA"

	sdlcEnvironmentStatusKey = "environment"
)

// sdlcRoleAgents names the SDLC agents the workflow hands to, as the quickstart
// names them (SQUAD_ROLES in templates.ts).
//
// The first four are required to run a build. The specialists are optional: a
// squad that has one gets its step (an unblock issue or a planning stage can be
// handed to it), and a squad without it gets no such step, so the lead uses the
// substitutes in the team playbook.
var sdlcRoleAgents = []struct {
	role, name string
	required   bool
}{
	{"lead", "SDLC Lead", true},
	{"developer", "SDLC Developer", true},
	{"reviewer", "SDLC Code Reviewer", true},
	{"qa", "SDLC QA Tester", true},
	{"analyst", "SDLC Product Analyst", false},
	{"ux", "SDLC UX Designer", false},
	{"architect", "SDLC Architect", false},
	{"security", "SDLC Security Reviewer", false},
	{"devops", "SDLC DevOps", false},
}

// ErrBuildNeedsSDLCSquad: the build stage's owner is not an SDLC squad with the
// developer, reviewer and QA agents the build workflow hands to.
var ErrBuildNeedsSDLCSquad = errors.New("the build stage needs an owning squad with the SDLC developer, reviewer and QA agents")

// ErrStageOwnerRequired: the stage a launch or a build starts has no owning
// squad, so there is nobody to run its work.
var ErrStageOwnerRequired = errors.New("this stage has no owning squad")

// startBuild creates the initiative's build project on the SDLC workflow and the
// kickoff issue the developer agent starts from, inside the stage transition's
// transaction. It returns the project id, stored as build_project_id.
//
// The workflow is the workspace's "SDLC Auto" when that one already hands to this
// squad's developer, which is what the quickstart creates; otherwise a workflow
// named after the squad is created, because the handlers are the squad's agents
// and a workflow is shared by the whole workspace.
func (s *InitiativeService) startBuild(ctx context.Context, q *db.Queries, ini db.Initiative, source db.Issue, owner pgtype.UUID, actor InitiativeActor) (pgtype.UUID, error) {
	agents, err := s.sdlcBuildSetup(ctx, q, ini.WorkspaceID, owner)
	if err != nil {
		return pgtype.UUID{}, err
	}
	workflow, err := s.sdlcWorkflowFor(ctx, q, ini.WorkspaceID, owner, agents)
	if err != nil {
		return pgtype.UUID{}, err
	}

	project, err := q.CreateProject(ctx, db.CreateProjectParams{
		WorkspaceID: ini.WorkspaceID,
		Title:       fmt.Sprintf("INI-%d %s", ini.Seq, ini.Title),
		Status:      "planned",
		Priority:    "none",
		WorkflowID:  workflow.ID,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("create build project: %w", err)
	}
	policy := ResolveIssueCountPolicy(ctx, s.Issues.Entitlements, ini.WorkspaceID)
	number, err := AllocateIssueNumber(ctx, q, ini.WorkspaceID, policy)
	if err != nil {
		return pgtype.UUID{}, err
	}
	position, err := s.minTopPosition(ctx, q, ini.WorkspaceID, "todo")
	if err != nil {
		return pgtype.UUID{}, err
	}
	developer := agents["developer"]
	kickoff, err := q.CreateIssueWithOrigin(ctx, db.CreateIssueWithOriginParams{
		ID:           dbid.NewV7(),
		WorkspaceID:  ini.WorkspaceID,
		Title:        ini.Title,
		Description:  pgtype.Text{String: fmt.Sprintf("Build for initiative INI-%d. Source issue: %s.", ini.Seq, source.Title), Valid: true},
		Status:       "todo",
		Priority:     "none",
		AssigneeType: pgtype.Text{String: "agent", Valid: true},
		AssigneeID:   developer,
		CreatorType:  actor.Type,
		CreatorID:    actor.ID,
		Position:     position,
		Number:       number,
		ProjectID:    project.ID,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("create build kickoff issue: %w", err)
	}
	s.wakeAgent(ctx, q, kickoff, developer, actor,
		fmt.Sprintf("The initiative INI-%d has reached its build stage. Build from this issue's workflow, starting with the developer step.", ini.Seq))
	return project.ID, nil
}

// EnsureSDLCWorkflow returns the SDLC Auto workflow that hands to the squad's
// agents, creating it (and the workspace's QA status) when it is missing. It is
// the same setup a build stage runs, so a project assigned by hand gets the
// workflow an initiative's build would.
func (s *InitiativeService) EnsureSDLCWorkflow(ctx context.Context, q *db.Queries, ws, squad pgtype.UUID) (db.IssueWorkflow, error) {
	agents, err := s.sdlcBuildSetup(ctx, q, ws, squad)
	if err != nil {
		return db.IssueWorkflow{}, err
	}
	return s.sdlcWorkflowFor(ctx, q, ws, squad, agents)
}

// sdlcBuildSetup checks the squad can run an SDLC build, seeds the statuses the
// workflow uses, and returns the squad's SDLC agents by role.
func (s *InitiativeService) sdlcBuildSetup(ctx context.Context, q *db.Queries, ws, squad pgtype.UUID) (map[string]pgtype.UUID, error) {
	if !squad.Valid {
		return nil, ErrBuildNeedsSDLCSquad
	}
	agents, err := s.sdlcAgentsOf(ctx, q, ws, squad)
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, r := range sdlcRoleAgents {
		if _, ok := agents[r.role]; !ok && r.required {
			missing = append(missing, r.name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: missing %s", ErrBuildNeedsSDLCSquad, strings.Join(missing, ", "))
	}
	if err := issuestatus.Ensure(ctx, q, ws); err != nil {
		return nil, fmt.Errorf("seed issue statuses: %w", err)
	}
	for _, st := range sdlcStatusesFor(agents) {
		if err := ensureStatus(ctx, q, ws, st); err != nil {
			return nil, err
		}
	}
	return agents, nil
}

// sdlcAgentsOf maps the SDLC roles to the squad's agents with those names. An
// archived agent does not count.
func (s *InitiativeService) sdlcAgentsOf(ctx context.Context, q *db.Queries, ws, squad pgtype.UUID) (map[string]pgtype.UUID, error) {
	members, err := q.ListSquadMembers(ctx, squad)
	if err != nil {
		return nil, fmt.Errorf("list squad members: %w", err)
	}
	byName := map[string]pgtype.UUID{}
	for _, m := range members {
		if m.MemberType != "agent" {
			continue
		}
		agent, err := q.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: m.MemberID, WorkspaceID: ws})
		if err != nil || agent.ArchivedAt.Valid {
			continue
		}
		byName[agent.Name] = agent.ID
	}
	out := map[string]pgtype.UUID{}
	for _, r := range sdlcRoleAgents {
		if id, ok := byName[r.name]; ok {
			out[r.role] = id
		}
	}
	return out, nil
}

// sdlcStatus is a status the SDLC workflow sits on.
type sdlcStatus struct {
	key, name, description, category, color string
}

// sdlcStageStatuses are the statuses of the planning and unblock steps, by the
// role that handles them. Stage done and Plan ready have no handler.
var sdlcStageStatuses = []struct {
	role string
	sdlcStatus
}{
	{"analyst", sdlcStatus{"analysis", "Analysis", "Requirements work.", issuestatus.CategoryStarted, "#0ea5e9"}},
	{"ux", sdlcStatus{"design", "Design", "UX design work.", issuestatus.CategoryStarted, "#ec4899"}},
	{"architect", sdlcStatus{"architecture", "Architecture", "Architecture work.", issuestatus.CategoryStarted, "#6366f1"}},
	{"reviewer", sdlcStatus{"plan_review", "Plan review", "Independent review of a plan.", issuestatus.CategoryStarted, "#f59e0b"}},
	{"security", sdlcStatus{"security_review", "Security review", "Security review.", issuestatus.CategoryStarted, "#ef4444"}},
	{"devops", sdlcStatus{"release_prep", "Release prep", "Packaging and release notes.", issuestatus.CategoryStarted, "#14b8a6"}},
	{"devops", sdlcStatus{sdlcEnvironmentStatusKey, "Environment", "Builds and keeps the environment and tools running.", issuestatus.CategoryStarted, "#10b981"}},
}

var (
	sdlcStageDoneStatus = sdlcStatus{"stage_done", "Stage done", "A planning or unblock stage that finished.", issuestatus.CategoryDone, "#22c55e"}
	sdlcPlanReadyStatus = sdlcStatus{"plan_ready", "Plan ready", "The plan is ready; a person decides when it is done.", issuestatus.CategoryStarted, "#a855f7"}
	sdlcQAStatus        = sdlcStatus{sdlcQAStatusKey, sdlcQAStatusName, "Verifies the change on the running system.", issuestatus.CategoryStarted, "#8b5cf6"}
)

// sdlcStatusesFor lists the statuses the workflow needs for these agents: QA,
// the two closing statuses, and the stage status of every specialist present.
func sdlcStatusesFor(agents map[string]pgtype.UUID) []sdlcStatus {
	out := []sdlcStatus{sdlcQAStatus, sdlcStageDoneStatus, sdlcPlanReadyStatus}
	for _, st := range sdlcStageStatuses {
		if _, ok := agents[st.role]; ok {
			out = append(out, st.sdlcStatus)
		}
	}
	return out
}

// ensureStatus makes sure the workspace has a status the workflow sits on. A
// status with the same name (any case) or the same key is used; otherwise one is created.
func ensureStatus(ctx context.Context, q *db.Queries, ws pgtype.UUID, st sdlcStatus) error {
	statuses, err := q.ListIssueStatusEntries(ctx, db.ListIssueStatusEntriesParams{WorkspaceID: ws})
	if err != nil {
		return fmt.Errorf("list issue statuses: %w", err)
	}
	for _, have := range statuses {
		if strings.EqualFold(have.Name, st.name) {
			return nil
		}
	}
	if _, err := q.GetIssueStatusEntryByKey(ctx, db.GetIssueStatusEntryByKeyParams{WorkspaceID: ws, Key: st.key}); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("load %s status: %w", st.name, err)
	}
	if _, err := q.CreateIssueStatusEntry(ctx, db.CreateIssueStatusEntryParams{
		WorkspaceID: ws, Key: st.key, Name: st.name,
		Description: st.description,
		Category:    st.category, Color: st.color, Icon: "circle",
	}); err != nil {
		return fmt.Errorf("create %s status: %w", st.name, err)
	}
	return nil
}

// sdlcWorkflowFor returns the SDLC workflow that hands to this squad's agents,
// reusing one the quickstart or an earlier build made, or creating it.
func (s *InitiativeService) sdlcWorkflowFor(ctx context.Context, q *db.Queries, ws, squad pgtype.UUID, agents map[string]pgtype.UUID) (db.IssueWorkflow, error) {
	rows, err := q.ListIssueWorkflows(ctx, ws)
	if err != nil {
		return db.IssueWorkflow{}, fmt.Errorf("list workflows: %w", err)
	}
	taken := false
	for _, wf := range rows {
		isSDLC := wf.Name == sdlcWorkflowName || strings.HasPrefix(wf.Name, sdlcWorkflowName+" (")
		if !isSDLC {
			continue
		}
		taken = taken || wf.Name == sdlcWorkflowName
		def, err := issueworkflow.Decode(wf)
		if err != nil {
			return db.IssueWorkflow{}, err
		}
		if step, ok := def.Step("todo"); ok && step.Handler.Type == issueworkflow.HandlerAgent &&
			step.Handler.ID == util.UUIDToString(agents["developer"]) {
			return wf, nil
		}
	}

	name := sdlcWorkflowName
	if taken {
		squadRow, err := q.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: squad, WorkspaceID: ws})
		if err != nil {
			return db.IssueWorkflow{}, fmt.Errorf("load build squad: %w", err)
		}
		name = fmt.Sprintf("%s (%s)", sdlcWorkflowName, squadRow.Name)
		if runes := []rune(name); len(runes) > issueworkflow.MaxNameRunes {
			name = string(runes[:issueworkflow.MaxNameRunes])
		}
	}
	encoded, err := issueworkflow.EncodeSteps(issueworkflow.Arrange(sdlcSteps(agents)))
	if err != nil {
		return db.IssueWorkflow{}, fmt.Errorf("encode SDLC workflow: %w", err)
	}
	wf, err := q.CreateIssueWorkflow(ctx, db.CreateIssueWorkflowParams{
		WorkspaceID:      ws,
		Name:             name,
		Description:      sdlcWorkflowDescription,
		InitialStatusKey: issuestatus.Todo,
		Steps:            encoded,
		OwnerID:          pgtype.UUID{},
	})
	if err != nil {
		return db.IssueWorkflow{}, fmt.Errorf("create SDLC workflow: %w", err)
	}
	return wf, nil
}

// sdlcSteps is SDLC_AUTO_WORKFLOW, the same shape as the Delivery workflow: the
// developer takes the issue and builds it in progress, the reviewer checks, and
// QA verifies. Rejected work goes back to In Progress. A blocked issue is handed
// to the squad's lead, which unblocks it and sends it back to In Progress. QA's
// step is the last before done.
func sdlcSteps(agents map[string]pgtype.UUID) []issueworkflow.Step {
	agent := func(role string) issueworkflow.Handler {
		return issueworkflow.Handler{Type: issueworkflow.HandlerAgent, ID: util.UUIDToString(agents[role])}
	}
	none := issueworkflow.Handler{Type: issueworkflow.HandlerNone}
	return append(sdlcStageSteps(agents), []issueworkflow.Step{
		{StatusKey: issuestatus.Backlog, Handler: none},
		{
			StatusKey: issuestatus.Todo, Handler: agent("developer"),
			Instructions: "Pick up this issue from To Do. Read its spec, stories, and acceptance criteria, then move it to In Progress to start the build.",
		},
		{
			StatusKey: sdlcInProgressStatusKey, Handler: agent("developer"),
			BackStatusKey: issuestatus.Todo,
			Instructions:  "Build everything this issue asks for, story by story, following the story flow in your instructions. Hand it to review only when every story is implemented and verified on the running app. Record what you verified in the spec. If this issue came back from Blocked, read the lead's comment first and re-run the check it names before you hand it to review.",
		},
		{
			StatusKey: "in_review", Handler: agent("reviewer"),
			BackStatusKey: sdlcInProgressStatusKey,
			Instructions:  "Review the whole change for this issue against its acceptance criteria. Send it back to In Progress with the blocking findings, or pass it on to QA with your verdict. If you cannot finish the review, or a blocking finding is not the developer's to fix, set the issue to blocked with the blocker report from sdlc-delivery-core and give each blocking finding an Owner-role.",
		},
		{
			StatusKey: sdlcQAStatusKey, Handler: agent("qa"),
			BackStatusKey: sdlcInProgressStatusKey,
			Instructions:  "Verify the change on the running system against the acceptance criteria. NO GO sends it back to In Progress with the defects. GO or CONDITIONAL GO finishes the issue. Run the environment's env-ensure command first when docs/sdlc/environment.md has one. If you cannot run the verification (a service is down, a required check cannot run) or a defect is not the developer's to fix, set the issue to blocked with the blocker report from sdlc-delivery-core and give each defect an Owner-role.",
		},
		{
			StatusKey: issuestatus.Blocked, Handler: agent("lead"),
			BackStatusKey: sdlcInProgressStatusKey,
			Instructions:  sdlcBlockedInstructions,
		},
	}...)
}

// sdlcStageSteps are the planning and environment steps, the ones a specialist
// handles. They exist so the lead can hand an unblock issue to the member who
// owns the blocker; a squad without that member has no such step. Every one
// ends at Stage done, which counts as finished for the run queue.
func sdlcStageSteps(agents map[string]pgtype.UUID) []issueworkflow.Step {
	done := map[string]string{
		"analysis":               "When the requirements, epics, or ledger in the issue are written and posted, move the issue to Stage done.",
		"design":                 "When every screen in the issue has its states and is registered, move the issue to Stage done.",
		"architecture":           "When the design, ADRs, and component list are committed and posted, move the issue to Stage done.",
		"plan_review":            "Give one verdict. PASS: move the issue to Stage done. CONCERNS or FAIL: set the issue to blocked and list each gap with its id, so the squad lead can create the gap issues. Do not move it to Stage done on CONCERNS.",
		"security_review":        "PASS: move the issue to Stage done. FAIL or NO_GO: set the issue to blocked and list each finding with its id.",
		"release_prep":           "Move the issue to Stage done only when the report lists the version, digests, and rollback steps. Production waits for the named approval: set the issue to blocked and say what the approver must decide.",
		sdlcEnvironmentStatusKey: "Work in Environment mode. When the environment runs, `docs/sdlc/environment.md` and the one command that rebuilds it are written, and the health output is posted, move the issue to Stage done.",
	}
	skill := map[string]string{
		"analysis": "sdlc-product-analysis", "design": "sdlc-ux-design", "architecture": "sdlc-architecture",
		"plan_review": "sdlc-code-review", "security_review": "sdlc-security-review",
		"release_prep": "sdlc-devops-release", sdlcEnvironmentStatusKey: "sdlc-devops-release",
	}
	var steps []issueworkflow.Step
	for _, st := range sdlcStageStatuses {
		id, ok := agents[st.role]
		if !ok {
			continue
		}
		steps = append(steps, issueworkflow.Step{
			StatusKey:     st.key,
			Handler:       issueworkflow.Handler{Type: issueworkflow.HandlerAgent, ID: util.UUIDToString(id)},
			NextStatusKey: sdlcStageDoneStatus.key,
			BackStatusKey: issuestatus.Blocked,
			Instructions:  sdlcPlanningBrief(skill[st.key], done[st.key]),
		})
	}
	none := issueworkflow.Handler{Type: issueworkflow.HandlerNone}
	return append(steps,
		issueworkflow.Step{StatusKey: sdlcStageDoneStatus.key, Handler: none},
		issueworkflow.Step{StatusKey: sdlcPlanReadyStatus.key, Handler: none},
	)
}

// sdlcPlanningBrief is the brief a stage step gives its owner. Keep it the same
// as planningBrief in packages/core/squads/templates.ts.
func sdlcPlanningBrief(skill, done string) string {
	return "This issue is either a planning stage of a request or an unblock ruling; its description says which. Load the sdlc-delivery-core and " + skill +
		" skills. For a planning stage, do the work the issue names. " + done +
		" For an unblock ruling, follow your skill's Ruling mode and end with the Ruling comment. If you cannot proceed, set the issue to blocked with the blocker report from sdlc-delivery-core."
}

// sdlcBlockedInstructions is the lead's brief for a blocked build. Without a
// handler here, a blocked issue waits with nobody assigned to it.
const sdlcBlockedInstructions = "This issue is blocked. Load the sdlc-delivery-core and sdlc-squad-lead skills and follow the lead's Blocked work (Step 7). Read the blocker report, route its class to the specialist who owns it with an unblock issue, verify the ruling, and resume the issue at its Blocked-from status. Do not do the specialist's work yourself, do not leave the issue here, and do not wait for a person. Only an irreversible action, a person's credential, a capability the host must change, or a reached limit goes to a person, with a proposal that carries your recommendation. " +
	"Never set a build issue or its Epic done while a check it needs has not run; name that check as unverified in the report instead. Never change acceptance criteria to get past a blocker."

// wakeAgent posts a note on the issue that mentions the agent, so the mention path
// wakes it, as wakeOwner does for a squad's leader.
func (s *InitiativeService) wakeAgent(ctx context.Context, q *db.Queries, issue db.Issue, agentID pgtype.UUID, actor InitiativeActor, text string) {
	content := text
	if agent, err := q.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: issue.WorkspaceID}); err == nil {
		content = fmt.Sprintf("[@%s](mention://agent/%s) %s", agent.Name, util.UUIDToString(agent.ID), text)
	}
	if _, err := q.CreateComment(ctx, db.CreateCommentParams{
		ID:          dbid.NewV7(),
		IssueID:     issue.ID,
		WorkspaceID: issue.WorkspaceID,
		AuthorType:  actor.Type,
		AuthorID:    actor.ID,
		Content:     content,
		Type:        "comment",
	}); err != nil {
		// The stage has moved; the developer still finds the kickoff on the board.
		slog.Warn("initiative: could not post the kickoff note", "issue_id", util.UUIDToString(issue.ID), "error", err)
	}
}

// minTopPosition places a new issue at the top of its column, as the issue
// service does.
func (s *InitiativeService) minTopPosition(ctx context.Context, q *db.Queries, ws pgtype.UUID, status string) (float64, error) {
	min, err := q.MinIssuePositionInStatus(ctx, db.MinIssuePositionInStatusParams{WorkspaceID: ws, Status: status})
	if err != nil {
		return 0, fmt.Errorf("query min issue position: %w", err)
	}
	return min - 1, nil
}
