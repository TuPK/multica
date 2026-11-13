package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// StartFlowIfEligible puts a request on its squad's flow. A request is an issue
// with no parent that is assigned to a squad whose active workflow is a flow,
// and that is not finished yet. It is idempotent: an issue that already has a
// flow, and every other issue, comes back unchanged. Many paths create requests
// (spec section 4.4), so each of them calls this after the create.
func StartFlowIfEligible(ctx context.Context, q *db.Queries, issue db.Issue) (db.Issue, error) {
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
	squad, err := q.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: issue.AssigneeID, WorkspaceID: issue.WorkspaceID})
	if err != nil || !squad.ActiveWorkflowID.Valid {
		return issue, nil
	}
	row, err := q.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: squad.ActiveWorkflowID, WorkspaceID: issue.WorkspaceID})
	if err != nil {
		return issue, nil
	}
	def, err := issueworkflow.Decode(row)
	if err != nil || !def.HasFlow() {
		return issue, nil
	}
	return startFlow(ctx, q, issue, row, def)
}

// StartFlow puts an issue on the flow of the given workflow, whatever its
// assignee. It returns the issue as stored; one that already has a flow is
// returned unchanged.
func StartFlow(ctx context.Context, q *db.Queries, issue db.Issue, row db.IssueWorkflow) (db.Issue, error) {
	def, err := issueworkflow.Decode(row)
	if err != nil {
		return issue, err
	}
	if !def.HasFlow() {
		return issue, fmt.Errorf("workflow %s is not a flow", util.UUIDToString(row.ID))
	}
	return startFlow(ctx, q, issue, row, def)
}

func startFlow(ctx context.Context, q *db.Queries, issue db.Issue, row db.IssueWorkflow, def issueworkflow.Definition) (db.Issue, error) {
	initial := issueworkflow.InitialState(def.Graph, nil, time.Now().UTC().Format(time.RFC3339))
	initial.WorkflowID = util.UUIDToString(row.ID)
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
