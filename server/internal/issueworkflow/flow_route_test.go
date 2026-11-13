package issueworkflow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// fakeRouter is the read surface ForIssue needs, over fixed rows.
type fakeRouter struct {
	workflows map[pgtype.UUID]db.IssueWorkflow
	squad     db.Squad
	project   db.Project
	def       db.IssueWorkflow
}

func (f fakeRouter) GetIssueWorkflow(_ context.Context, arg db.GetIssueWorkflowParams) (db.IssueWorkflow, error) {
	if row, ok := f.workflows[arg.ID]; ok {
		return row, nil
	}
	return db.IssueWorkflow{}, pgx.ErrNoRows
}

func (f fakeRouter) GetDefaultIssueWorkflow(context.Context, pgtype.UUID) (db.IssueWorkflow, error) {
	return f.def, nil
}

func (f fakeRouter) GetProjectInWorkspace(context.Context, db.GetProjectInWorkspaceParams) (db.Project, error) {
	return f.project, nil
}

func (f fakeRouter) GetSquadInWorkspace(context.Context, db.GetSquadInWorkspaceParams) (db.Squad, error) {
	return f.squad, nil
}

func (f fakeRouter) GetAgentInWorkspace(context.Context, db.GetAgentInWorkspaceParams) (db.Agent, error) {
	return db.Agent{}, pgx.ErrNoRows
}

func uuidOf(b byte) pgtype.UUID {
	var id pgtype.UUID
	id.Valid = true
	id.Bytes[0] = b
	return id
}

func workflowRow(t *testing.T, id pgtype.UUID, name string, steps []Step, flow *FlowConfig) db.IssueWorkflow {
	t.Helper()
	raw, err := EncodeSteps(steps)
	if err != nil {
		t.Fatal(err)
	}
	row := db.IssueWorkflow{ID: id, Name: name, Steps: raw}
	if flow != nil {
		row.FlowConfig, err = json.Marshal(flow)
		if err != nil {
			t.Fatal(err)
		}
	}
	return row
}

// A squad whose active workflow is a flow routes its requests and tasks through
// the flow, not its other issues: those keep the project's workflow.
func TestActiveWorkflowOfSkipsFlow(t *testing.T) {
	ws, squadID, flowID, plainID, projectID := uuidOf(1), uuidOf(2), uuidOf(3), uuidOf(4), uuidOf(5)
	plainSteps := []Step{{StatusKey: "todo"}, {StatusKey: "in_progress"}, {StatusKey: "done"}}
	flow := tradeFlow()
	flowRow := workflowRow(t, flowID, "Trade", flow.Steps, flow.Flow)
	plainRow := workflowRow(t, plainID, "Delivery", plainSteps, nil)

	router := fakeRouter{
		workflows: map[pgtype.UUID]db.IssueWorkflow{flowID: flowRow, plainID: plainRow},
		squad:     db.Squad{ID: squadID, ActiveWorkflowID: flowID},
		project:   db.Project{ID: projectID, WorkspaceID: ws, WorkflowID: plainID},
	}
	assignee := pgtype.Text{String: "squad", Valid: true}

	def, ok, err := activeWorkflowOf(context.Background(), router, ws, squadID)
	if err != nil || ok {
		t.Fatalf("activeWorkflowOf(flow) = %+v, %v, %v; want no active workflow", def, ok, err)
	}
	got, _, err := ForIssue(context.Background(), router, ws, projectID, assignee, squadID)
	if err != nil || got == nil || got.Name != "Delivery" {
		t.Fatalf("ForIssue with a flow squad = %+v, %v; want the project's workflow", got, err)
	}

	// An ordinary workflow as the active one still routes.
	router.squad.ActiveWorkflowID = plainID
	router.workflows[plainID] = workflowRow(t, plainID, "Squad delivery", plainSteps, nil)
	def, ok, err = activeWorkflowOf(context.Background(), router, ws, squadID)
	if err != nil || !ok || def.Name != "Squad delivery" {
		t.Fatalf("activeWorkflowOf(plain) = %+v, %v, %v; want the squad's workflow", def, ok, err)
	}
}
