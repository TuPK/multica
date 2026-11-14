package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// FlowChoiceError says why the workflow chosen for a request cannot be used
// (spec 16.5). The handlers answer it with 422 flow_workflow_invalid.
type FlowChoiceError struct{ Message string }

func (e *FlowChoiceError) Error() string { return e.Message }

// ResolveFlowChoice returns the workflow a request given to a squad may run on
// when its creator or operator names one. It has to be a workflow of that squad
// that runs on a flow: any published version of any of its lineages, or the
// version that is active now. A rehearsal (trial) needs a workflow of schema 2,
// because the state of schema 1 has nowhere to say it is one.
func ResolveFlowChoice(ctx context.Context, q *db.Queries, workspaceID pgtype.UUID, assigneeType pgtype.Text, assigneeID, workflowID pgtype.UUID, trial bool) (db.IssueWorkflow, error) {
	if assigneeType.String != "squad" || !assigneeID.Valid {
		return db.IssueWorkflow{}, &FlowChoiceError{Message: "only an issue given to a squad runs on a workflow of that squad"}
	}
	squad, err := q.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: assigneeID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.IssueWorkflow{}, &FlowChoiceError{Message: "the squad was not found"}
	}
	if err != nil {
		return db.IssueWorkflow{}, err
	}
	row, err := q.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: workflowID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.IssueWorkflow{}, &FlowChoiceError{Message: "the workflow was not found"}
	}
	if err != nil {
		return db.IssueWorkflow{}, err
	}
	if row.SquadID != squad.ID && row.ID != squad.ActiveWorkflowID {
		return db.IssueWorkflow{}, &FlowChoiceError{Message: "the workflow does not belong to the squad of this request"}
	}
	def, err := issueworkflow.Decode(row)
	if err != nil || !def.HasFlow() {
		return db.IssueWorkflow{}, &FlowChoiceError{Message: "the workflow does not run on a flow"}
	}
	if trial && def.Graph.Compat == issueworkflow.CompatV1 {
		return db.IssueWorkflow{}, &FlowChoiceError{Message: "a workflow of schema 1 cannot be rehearsed"}
	}
	return row, nil
}

// StartFlowIfEligible puts a request on its squad's flow. A request is an issue
// with no parent that is assigned to a squad whose active workflow is a flow,
// and that is not finished yet. It is idempotent: an issue that already has a
// flow, and every other issue, comes back unchanged. Many paths create requests
// (spec section 4.4), so each of them calls this after the create.
func StartFlowIfEligible(ctx context.Context, q *db.Queries, issue db.Issue) (db.Issue, error) {
	return StartFlowIfEligibleOn(ctx, q, issue, nil, false)
}

// StartFlowIfEligibleOn is StartFlowIfEligible for a request whose creator chose
// the workflow (already checked with ResolveFlowChoice). A nil choice means the
// squad's active workflow. trial starts the run as a rehearsal (spec 16.5).
func StartFlowIfEligibleOn(ctx context.Context, q *db.Queries, issue db.Issue, chosen *db.IssueWorkflow, trial bool) (db.Issue, error) {
	if len(issue.Flow) > 0 || issue.FlowTask.Valid || issue.ParentIssueID.Valid {
		return issue, nil
	}
	if issue.AssigneeType.String != "squad" || !issue.AssigneeID.Valid {
		return issue, nil
	}
	switch issuestatus.Category(ctx, q, issue.WorkspaceID, issue.Status) {
	case issuestatus.CategoryDone, issuestatus.CategoryClosed:
		return issue, nil
	}
	row := db.IssueWorkflow{}
	if chosen != nil {
		row = *chosen
	} else {
		squad, err := q.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: issue.AssigneeID, WorkspaceID: issue.WorkspaceID})
		if err != nil || !squad.ActiveWorkflowID.Valid {
			return issue, nil
		}
		row, err = q.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: squad.ActiveWorkflowID, WorkspaceID: issue.WorkspaceID})
		if err != nil {
			return issue, nil
		}
	}
	def, err := issueworkflow.Decode(row)
	if err != nil || !def.HasFlow() {
		return issue, nil
	}
	return startFlow(ctx, q, issue, row, def, trial)
}

// StartFlow puts an issue on the flow of the given workflow, whatever its
// assignee. It returns the issue as stored; one that already has a flow is
// returned unchanged.
func StartFlow(ctx context.Context, q *db.Queries, issue db.Issue, row db.IssueWorkflow) (db.Issue, error) {
	return StartFlowAs(ctx, q, issue, row, false)
}

// StartFlowAs is StartFlow that can mark the run as a rehearsal of the workflow
// (spec 16.5): the state says trial, and the engine never lets a trial release
// work that nobody saw.
func StartFlowAs(ctx context.Context, q *db.Queries, issue db.Issue, row db.IssueWorkflow, trial bool) (db.Issue, error) {
	def, err := issueworkflow.Decode(row)
	if err != nil {
		return issue, err
	}
	if !def.HasFlow() {
		return issue, fmt.Errorf("workflow %s is not a flow", util.UUIDToString(row.ID))
	}
	return startFlow(ctx, q, issue, row, def, trial)
}

func startFlow(ctx context.Context, q *db.Queries, issue db.Issue, row db.IssueWorkflow, def issueworkflow.Definition, trial bool) (db.Issue, error) {
	if trial && def.Graph.Compat == issueworkflow.CompatV1 {
		return issue, fmt.Errorf("workflow %s is of schema 1 and cannot be rehearsed", util.UUIDToString(row.ID))
	}
	initial := issueworkflow.InitialState(def.Graph, nil, time.Now().UTC().Format(time.RFC3339))
	initial.WorkflowID = util.UUIDToString(row.ID)
	initial.Trial = trial
	var raw []byte
	var err error
	if def.Graph.Compat == issueworkflow.CompatV1 {
		// A request on a schema 1 workflow keeps the schema 1 form of the state (spec 2.4).
		raw, err = json.Marshal(issueworkflow.Unadapt(initial))
	} else {
		raw, err = json.Marshal(initial)
	}
	if err != nil {
		return issue, err
	}
	started, err := q.InitIssueFlow(ctx, db.InitIssueFlowParams{Flow: raw, ID: issue.ID, WorkspaceID: issue.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		// Another caller started it first; return what is stored.
		return q.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{ID: issue.ID, WorkspaceID: issue.WorkspaceID})
	}
	if err != nil {
		return issue, fmt.Errorf("start flow: %w", err)
	}
	return started, nil
}
