package issueworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// roomFake serves the rows the room rules read, keyed by id.
type roomFake struct {
	squads    map[pgtype.UUID]db.Squad
	agents    map[pgtype.UUID]db.Agent
	workflows map[pgtype.UUID]db.IssueWorkflow
	projects  map[pgtype.UUID]db.Project
	leaders   map[pgtype.UUID]bool
	def       db.IssueWorkflow
	hasDef    bool
}

func (f roomFake) GetIssueWorkflow(_ context.Context, arg db.GetIssueWorkflowParams) (db.IssueWorkflow, error) {
	if row, ok := f.workflows[arg.ID]; ok {
		return row, nil
	}
	return db.IssueWorkflow{}, pgx.ErrNoRows
}

func (f roomFake) GetDefaultIssueWorkflow(context.Context, pgtype.UUID) (db.IssueWorkflow, error) {
	if !f.hasDef {
		return db.IssueWorkflow{}, pgx.ErrNoRows
	}
	return f.def, nil
}

func (f roomFake) GetProjectInWorkspace(_ context.Context, arg db.GetProjectInWorkspaceParams) (db.Project, error) {
	if p, ok := f.projects[arg.ID]; ok {
		return p, nil
	}
	return db.Project{}, pgx.ErrNoRows
}

func (f roomFake) GetSquadInWorkspace(_ context.Context, arg db.GetSquadInWorkspaceParams) (db.Squad, error) {
	if s, ok := f.squads[arg.ID]; ok {
		return s, nil
	}
	return db.Squad{}, pgx.ErrNoRows
}

func (f roomFake) GetAgentInWorkspace(_ context.Context, arg db.GetAgentInWorkspaceParams) (db.Agent, error) {
	if a, ok := f.agents[arg.ID]; ok {
		return a, nil
	}
	return db.Agent{}, pgx.ErrNoRows
}

func (f roomFake) IsSquadLeaderInWorkspace(_ context.Context, arg db.IsSquadLeaderInWorkspaceParams) (bool, error) {
	return f.leaders[arg.LeaderID], nil
}

func (f roomFake) GetWorkspace(context.Context, pgtype.UUID) (db.Workspace, error) {
	return db.Workspace{}, pgx.ErrNoRows
}

// roomWorld is a project with a plain room, a flow room, and an empty room.
type roomWorld struct {
	ws, project                   pgtype.UUID
	plainRoom, flowRoom, bareRoom pgtype.UUID
	plainWF, flowWF, defaultWF    pgtype.UUID
	flowLeader, plainAgent        pgtype.UUID
	fake                          roomFake
}

func newRoomWorld(t *testing.T) roomWorld {
	t.Helper()
	w := roomWorld{
		ws: uuidOf(1), project: uuidOf(2),
		plainRoom: uuidOf(10), flowRoom: uuidOf(11), bareRoom: uuidOf(12),
		plainWF: uuidOf(20), flowWF: uuidOf(21), defaultWF: uuidOf(22),
		flowLeader: uuidOf(30), plainAgent: uuidOf(31),
	}
	plainSteps := []Step{{StatusKey: "todo"}, {StatusKey: "in_progress"}, {StatusKey: "done"}}
	flow := tradeFlow()
	w.fake = roomFake{
		squads: map[pgtype.UUID]db.Squad{
			w.plainRoom: {ID: w.plainRoom, WorkspaceID: w.ws, ActiveWorkflowID: w.plainWF},
			w.flowRoom:  {ID: w.flowRoom, WorkspaceID: w.ws, ActiveWorkflowID: w.flowWF},
			w.bareRoom:  {ID: w.bareRoom, WorkspaceID: w.ws},
		},
		agents: map[pgtype.UUID]db.Agent{
			w.plainAgent: {ID: w.plainAgent, WorkspaceID: w.ws, HomeSquadID: w.plainRoom},
		},
		workflows: map[pgtype.UUID]db.IssueWorkflow{
			w.plainWF:   workflowRow(t, w.plainWF, "Delivery", plainSteps, nil),
			w.flowWF:    workflowRow(t, w.flowWF, "Trade", flow.Steps, flow.Flow),
			w.defaultWF: workflowRow(t, w.defaultWF, "Default", plainSteps, nil),
		},
		projects: map[pgtype.UUID]db.Project{
			w.project: {ID: w.project, WorkspaceID: w.ws},
		},
		leaders: map[pgtype.UUID]bool{w.flowLeader: true},
		def:     workflowRow(t, w.defaultWF, "Default", plainSteps, nil),
		hasDef:  true,
	}
	return w
}

func squadAssigned(id pgtype.UUID) (pgtype.Text, pgtype.UUID) {
	return pgtype.Text{String: "squad", Valid: true}, id
}

func agentAssigned(id pgtype.UUID) (pgtype.Text, pgtype.UUID) {
	return pgtype.Text{String: "agent", Valid: true}, id
}

// A plain room's issues follow the room's workflow, requests and sub-issues alike.
func TestResolveRoomWorkflowPlainRoomOwnsItsIssues(t *testing.T) {
	w := newRoomWorld(t)
	at, aid := squadAssigned(w.plainRoom)
	in := RoomIssue{WorkspaceID: w.ws, ProjectID: w.project, AssigneeType: at, AssigneeID: aid}
	def, _, err := ResolveRoomWorkflow(context.Background(), w.fake, in, NoAction)
	if err != nil || def == nil || def.Name != "Delivery" {
		t.Fatalf("plain room = %+v, %v; want its own workflow", def, err)
	}
	// A sub-issue of the same room, given to a member agent of the room.
	at, aid = agentAssigned(w.plainAgent)
	sub := RoomIssue{WorkspaceID: w.ws, ProjectID: w.project, AssigneeType: at, AssigneeID: aid, ParentIssueID: uuidOf(40)}
	def, _, err = ResolveRoomWorkflow(context.Background(), w.fake, sub, NoAction)
	if err != nil || def == nil || def.Name != "Delivery" {
		t.Fatalf("sub-issue of a plain room = %+v, %v; want the room's workflow", def, err)
	}
}

// Issues never fall back to the project: a room without a workflow reads Default.
func TestResolveRoomWorkflowNeverFallsBackToProject(t *testing.T) {
	w := newRoomWorld(t)
	w.fake.projects[w.project] = db.Project{ID: w.project, WorkspaceID: w.ws, WorkflowID: w.plainWF}
	at, aid := squadAssigned(w.bareRoom)
	in := RoomIssue{WorkspaceID: w.ws, ProjectID: w.project, AssigneeType: at, AssigneeID: aid}
	def, _, err := ResolveRoomWorkflow(context.Background(), w.fake, in, NoAction)
	if err != nil || def == nil || def.Name != "Default" {
		t.Fatalf("bare room = %+v, %v; want Default and not the project workflow", def, err)
	}
}

// Engine tasks read Default for status checks; the flow steers them.
func TestResolveRoomWorkflowEngineTaskReadsDefault(t *testing.T) {
	w := newRoomWorld(t)
	at, aid := squadAssigned(w.flowRoom)
	in := RoomIssue{WorkspaceID: w.ws, ProjectID: w.project, AssigneeType: at, AssigneeID: aid,
		ParentIssueID: uuidOf(40), FlowTask: pgtype.Text{String: "review", Valid: true}}
	def, _, err := ResolveRoomWorkflow(context.Background(), w.fake, in, NoAction)
	if err != nil || def == nil || def.Name != "Default" {
		t.Fatalf("engine task = %+v, %v; want Default", def, err)
	}
}

// A flow room's request is steered by its flow, which the resolver does not return.
func TestResolveRoomWorkflowFlowRequestReadsDefault(t *testing.T) {
	w := newRoomWorld(t)
	at, aid := squadAssigned(w.flowRoom)
	in := RoomIssue{WorkspaceID: w.ws, ProjectID: w.project, AssigneeType: at, AssigneeID: aid}
	def, _, err := ResolveRoomWorkflow(context.Background(), w.fake, in, NoAction)
	if err != nil || def == nil || def.Name != "Default" {
		t.Fatalf("flow request = %+v, %v; want Default", def, err)
	}
}

// A non-request issue of a flow room follows the stored policy; no_action reads Default.
func TestResolveRoomWorkflowNonRequestFollowsPolicy(t *testing.T) {
	w := newRoomWorld(t)
	at, aid := agentAssigned(w.flowLeader) // leader of the flow room: a request only without a parent
	sub := RoomIssue{WorkspaceID: w.ws, ProjectID: w.project, AssigneeType: at, AssigneeID: aid, ParentIssueID: uuidOf(40)}
	// Make the agent a home-squad member of the flow room so it routes there.
	w.fake.agents[w.flowLeader] = db.Agent{ID: w.flowLeader, WorkspaceID: w.ws, HomeSquadID: w.flowRoom}
	def, _, err := ResolveRoomWorkflow(context.Background(), w.fake, sub, NoAction)
	if err != nil || def == nil || def.Name != "Default" {
		t.Fatalf("no_action non-request = %+v, %v; want Default", def, err)
	}
	chosen := NonRequestPolicy{Kind: NonRequestWorkflow, WorkflowID: w.plainWF}
	def, _, err = ResolveRoomWorkflow(context.Background(), w.fake, sub, chosen)
	if err != nil || def == nil || def.Name != "Delivery" {
		t.Fatalf("chosen non-request = %+v, %v; want the chosen workflow", def, err)
	}
}

// Issues outside a project keep their old behaviour: no workflow is returned.
func TestResolveRoomWorkflowOutsideProjectIsNil(t *testing.T) {
	w := newRoomWorld(t)
	at, aid := squadAssigned(w.plainRoom)
	in := RoomIssue{WorkspaceID: w.ws, AssigneeType: at, AssigneeID: aid}
	def, _, err := ResolveRoomWorkflow(context.Background(), w.fake, in, NoAction)
	if err != nil || def != nil {
		t.Fatalf("outside a project = %+v, %v; want nil", def, err)
	}
}

// A squad with no workflow refuses the write with room_workflow_required.
func TestCheckRoomWriteRefusesRoomWithoutWorkflow(t *testing.T) {
	w := newRoomWorld(t)
	at, aid := squadAssigned(w.bareRoom)
	in := RoomIssue{WorkspaceID: w.ws, ProjectID: w.project, AssigneeType: at, AssigneeID: aid}
	err := CheckRoomWrite(context.Background(), w.fake, in, NoAction)
	var rwe *RoomWorkflowError
	if !errors.As(err, &rwe) || rwe.Code != CodeRoomWorkflowRequired {
		t.Fatalf("CheckRoomWrite(bare room) = %v; want %s", err, CodeRoomWorkflowRequired)
	}
}

// A non-request issue in a flow room is refused while the policy is no_action.
func TestCheckRoomWriteRefusesNonRequestUnderNoAction(t *testing.T) {
	w := newRoomWorld(t)
	w.fake.agents[w.flowLeader] = db.Agent{ID: w.flowLeader, WorkspaceID: w.ws, HomeSquadID: w.flowRoom}
	at, aid := agentAssigned(w.flowLeader)
	sub := RoomIssue{WorkspaceID: w.ws, ProjectID: w.project, AssigneeType: at, AssigneeID: aid, ParentIssueID: uuidOf(40)}

	err := CheckRoomWrite(context.Background(), w.fake, sub, NoAction)
	var rwe *RoomWorkflowError
	if !errors.As(err, &rwe) || rwe.Code != CodeNonRequestWorkflowNeeded {
		t.Fatalf("non-request under no_action = %v; want %s", err, CodeNonRequestWorkflowNeeded)
	}
	if err := CheckRoomWrite(context.Background(), w.fake, sub, NonRequestPolicy{Kind: NonRequestDefault}); err != nil {
		t.Fatalf("non-request under default = %v; want allowed", err)
	}
}

// A request of a flow room is allowed; the flow runs it.
func TestCheckRoomWriteAllowsFlowRequest(t *testing.T) {
	w := newRoomWorld(t)
	at, aid := squadAssigned(w.flowRoom)
	in := RoomIssue{WorkspaceID: w.ws, ProjectID: w.project, AssigneeType: at, AssigneeID: aid}
	if err := CheckRoomWrite(context.Background(), w.fake, in, NoAction); err != nil {
		t.Fatalf("flow request = %v; want allowed", err)
	}
}

// Engine tasks and room-less issues are never refused.
func TestCheckRoomWriteExemptions(t *testing.T) {
	w := newRoomWorld(t)
	at, aid := squadAssigned(w.bareRoom)
	engine := RoomIssue{WorkspaceID: w.ws, ProjectID: w.project, AssigneeType: at, AssigneeID: aid,
		ParentIssueID: uuidOf(40), FlowTask: pgtype.Text{String: "review", Valid: true}}
	if err := CheckRoomWrite(context.Background(), w.fake, engine, NoAction); err != nil {
		t.Fatalf("engine task = %v; want exempt", err)
	}
	noRoom := RoomIssue{WorkspaceID: w.ws, ProjectID: w.project}
	if err := CheckRoomWrite(context.Background(), w.fake, noRoom, NoAction); err != nil {
		t.Fatalf("room-less issue = %v; want allowed (Default)", err)
	}
	outside := RoomIssue{WorkspaceID: w.ws, AssigneeType: at, AssigneeID: aid}
	if err := CheckRoomWrite(context.Background(), w.fake, outside, NoAction); err != nil {
		t.Fatalf("outside a project = %v; want allowed as before", err)
	}
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
