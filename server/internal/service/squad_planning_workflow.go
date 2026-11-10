package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The planning squad is built by the squad quickstart (BMAD_PLANNING in
// packages/core/squads/templates.ts), which also creates its workflow. The server
// does not repeat that workflow: it finds the one the quickstart made and checks it
// hands to this squad's lead. Only the two names below are shared with the template.
const (
	planningWorkflowName  = "BMAD Planning Auto"
	planningLeadAgentName = "Blueprint Lead"
)

// ErrPlanningWorkflowMissing: the squad is a planning squad but the workspace has
// no "BMAD Planning Auto" workflow that hands to its lead. The quickstart creates it.
var ErrPlanningWorkflowMissing = errors.New("the planning squad's workflow does not exist yet; create it with the squad quickstart")

// squadAgentIDsByName maps the names of a squad's live agents to their ids.
func squadAgentIDsByName(ctx context.Context, q *db.Queries, ws, squad pgtype.UUID) (map[string]pgtype.UUID, error) {
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
	return byName, nil
}

// EnsureSquadProjectWorkflow returns the workflow a project takes when it is given
// to the squad: the planning workflow for a squad that has the planning lead, and
// the SDLC Auto workflow for any other squad.
func (s *InitiativeService) EnsureSquadProjectWorkflow(ctx context.Context, q *db.Queries, ws, squad pgtype.UUID) (db.IssueWorkflow, error) {
	byName, err := squadAgentIDsByName(ctx, q, ws, squad)
	if err != nil {
		return db.IssueWorkflow{}, err
	}
	if lead, ok := byName[planningLeadAgentName]; ok {
		return planningWorkflowFor(ctx, q, ws, lead)
	}
	return s.EnsureSDLCWorkflow(ctx, q, ws, squad)
}

// planningWorkflowFor finds the planning workflow whose blocked step is handed to
// this lead. The quickstart names it "BMAD Planning Auto"; a second copy keeps the
// name with a suffix, so the name's prefix is matched and the lead decides.
func planningWorkflowFor(ctx context.Context, q *db.Queries, ws, lead pgtype.UUID) (db.IssueWorkflow, error) {
	rows, err := q.ListIssueWorkflows(ctx, ws)
	if err != nil {
		return db.IssueWorkflow{}, fmt.Errorf("list workflows: %w", err)
	}
	for _, wf := range rows {
		if wf.Name != planningWorkflowName && !strings.HasPrefix(wf.Name, planningWorkflowName+" (") {
			continue
		}
		def, err := issueworkflow.Decode(wf)
		if err != nil {
			return db.IssueWorkflow{}, err
		}
		if step, ok := def.Step(issuestatus.Blocked); ok && step.Handler.Type == issueworkflow.HandlerAgent &&
			step.Handler.ID == util.UUIDToString(lead) {
			return wf, nil
		}
	}
	return db.IssueWorkflow{}, ErrPlanningWorkflowMissing
}
