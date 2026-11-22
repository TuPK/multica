package issueworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Room-owned workflows (spec: docs/engineering/squad-room-workflow-spec.md).
//
// An issue in a project follows the workflow of its room: the squad it is
// assigned to, or the home squad of the agent it is assigned to. A room with an
// active workflow owns its issues; a room without one refuses new work at write
// time (CheckRoomWrite) instead of falling back to a project-level workflow. The
// resolver below never refuses: it only reads, so existing issues keep working.

// Reason codes returned to clients (HTTP 422) and to agents through the CLI.
const (
	CodeRoomWorkflowRequired     = "room_workflow_required"
	CodeNonRequestWorkflowNeeded = "room_no_workflow_for_non_request"

	// Kinds of the non-request policy. The strings are the API and settings values.
	NonRequestNoAction = "no_action"
	NonRequestDefault  = "default"
	NonRequestWorkflow = "workflow"

	// NonRequestSettingKey is the workspace.settings key that holds the policy.
	NonRequestSettingKey = "non_request_workflow"
)

// RoomWorkflowError is a write refused because the issue's room has no workflow
// to follow. The handler maps it to 422 with Code.
type RoomWorkflowError struct {
	Code    string
	Room    pgtype.UUID
	Message string
}

func (e *RoomWorkflowError) Error() string { return e.Message }

// NonRequestPolicy is the workspace setting for issues in a flow room that are
// not requests of that room (spec section 4). The zero value is "no_action".
type NonRequestPolicy struct {
	Kind       string
	WorkflowID pgtype.UUID
}

// NoAction is the default policy: such issues are refused until a choice is stored.
var NoAction = NonRequestPolicy{Kind: NonRequestNoAction}

// RoomIssue is what the room rules need from an issue, whether it already exists
// or is about to be created. ProjectID must be the effective project (after a
// parent's project is inherited).
type RoomIssue struct {
	WorkspaceID   pgtype.UUID
	ProjectID     pgtype.UUID
	AssigneeType  pgtype.Text
	AssigneeID    pgtype.UUID
	ParentIssueID pgtype.UUID
	CreatorType   string
	CreatorID     pgtype.UUID
	// FlowTask is set on tasks the flow engine opened. They are owned by the flow
	// and are exempt from the write check.
	FlowTask pgtype.Text
}

// roomOf returns the room an issue belongs to, or an invalid id when it has none.
func roomOf(ctx context.Context, q Querier, in RoomIssue) (pgtype.UUID, error) {
	return routingSquad(ctx, q, in.WorkspaceID, in.AssigneeType, in.AssigneeID)
}

// squadWorkflow loads a room's active workflow, flows included. found is false
// when the room has none or its reference dangles.
func squadWorkflow(ctx context.Context, q Querier, workspaceID, squadID pgtype.UUID) (Definition, bool, error) {
	squad, err := q.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: squadID, WorkspaceID: workspaceID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Definition{}, false, nil
		}
		return Definition{}, false, err
	}
	if !squad.ActiveWorkflowID.Valid {
		return Definition{}, false, nil
	}
	row, err := q.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: squad.ActiveWorkflowID, WorkspaceID: workspaceID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Definition{}, false, nil
		}
		return Definition{}, false, err
	}
	def, err := Decode(row)
	if err != nil {
		return Definition{}, false, err
	}
	return def, true, nil
}

// IsRequest reports whether the issue is a request to its room: it has no
// parent, is assigned to the room or to the leader of a live room, and was not
// opened by that leader for itself. It mirrors handler.isSquadRequest, which
// stays the handler's view of the same rule.
func IsRequest(ctx context.Context, q Querier, in RoomIssue) (bool, error) {
	if in.ParentIssueID.Valid || !in.AssigneeID.Valid {
		return false, nil
	}
	if in.CreatorType == "agent" && in.CreatorID == in.AssigneeID {
		return false, nil
	}
	switch in.AssigneeType.String {
	case "squad":
		return true, nil
	case "agent":
		leads, err := q.IsSquadLeaderInWorkspace(ctx, db.IsSquadLeaderInWorkspaceParams{
			WorkspaceID: in.WorkspaceID,
			LeaderID:    in.AssigneeID,
		})
		return err == nil && leads, err
	}
	return false, nil
}

// NewNonRequestPolicy validates a policy chosen by a user. A workflow kind needs
// the id of a workflow in the workspace; the caller checks that the id exists.
func NewNonRequestPolicy(kind string, workflowID pgtype.UUID) (NonRequestPolicy, error) {
	switch kind {
	case NonRequestNoAction, NonRequestDefault:
		return NonRequestPolicy{Kind: kind}, nil
	case NonRequestWorkflow:
		if !workflowID.Valid {
			return NonRequestPolicy{}, errors.New("workflow_id is required for the workflow kind")
		}
		return NonRequestPolicy{Kind: kind, WorkflowID: workflowID}, nil
	}
	return NonRequestPolicy{}, fmt.Errorf("unknown non-request policy kind %q", kind)
}

// storedPolicy is the JSON shape of the policy under workspace.settings.
type storedPolicy struct {
	Kind       string `json:"kind"`
	WorkflowID string `json:"workflow_id,omitempty"`
}

// SettingValue encodes the policy for workspace.settings.non_request_workflow.
func (p NonRequestPolicy) SettingValue() ([]byte, error) {
	stored := storedPolicy{Kind: p.Kind}
	if p.Kind == NonRequestWorkflow {
		stored.WorkflowID = util.UUIDToString(p.WorkflowID)
	}
	return json.Marshal(stored)
}

// LoadNonRequestPolicy returns the workspace's setting for non-request issues in
// flow rooms. A missing or unreadable setting is NoAction, so an unknown value
// refuses the write rather than guessing a workflow.
func LoadNonRequestPolicy(ctx context.Context, q Querier, workspaceID pgtype.UUID) (NonRequestPolicy, error) {
	ws, err := q.GetWorkspace(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return NoAction, nil
		}
		return NoAction, err
	}
	var settings map[string]json.RawMessage
	if json.Unmarshal(ws.Settings, &settings) != nil {
		return NoAction, nil
	}
	raw, ok := settings[NonRequestSettingKey]
	if !ok {
		return NoAction, nil
	}
	var stored storedPolicy
	if json.Unmarshal(raw, &stored) != nil {
		return NoAction, nil
	}
	var id pgtype.UUID
	if stored.Kind == NonRequestWorkflow {
		if id.Scan(stored.WorkflowID) != nil {
			return NoAction, nil
		}
	}
	policy, err := NewNonRequestPolicy(stored.Kind, id)
	if err != nil {
		return NoAction, nil
	}
	return policy, nil
}

// RoomOf returns the room (squad) an issue would belong to, or an invalid id.
func RoomOf(ctx context.Context, q Querier, in RoomIssue) (pgtype.UUID, error) {
	return roomOf(ctx, q, in)
}

// ResolveRoomWorkflow returns the workflow an issue follows under the room rules.
// It never refuses. A nil definition means the issue is outside any project,
// as with ForIssue.
func ResolveRoomWorkflow(ctx context.Context, q Querier, in RoomIssue, policy NonRequestPolicy) (*Definition, db.Project, error) {
	if !in.ProjectID.Valid {
		return nil, db.Project{}, nil
	}
	project, err := q.GetProjectInWorkspace(ctx, db.GetProjectInWorkspaceParams{ID: in.ProjectID, WorkspaceID: in.WorkspaceID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, db.Project{}, nil
		}
		return nil, db.Project{}, err
	}
	// Engine tasks are steered by the flow; their status checks read the Default.
	if in.FlowTask.Valid {
		def, err := DefaultFor(ctx, q, in.WorkspaceID)
		return &def, project, err
	}
	room, err := roomOf(ctx, q, in)
	if err != nil {
		return nil, db.Project{}, err
	}
	if !room.Valid {
		def, err := DefaultFor(ctx, q, in.WorkspaceID)
		return &def, project, err
	}
	def, found, err := squadWorkflow(ctx, q, in.WorkspaceID, room)
	if err != nil {
		return nil, db.Project{}, err
	}
	if !found {
		def, err := DefaultFor(ctx, q, in.WorkspaceID)
		return &def, project, err
	}
	if def.Flow == nil {
		return &def, project, nil
	}
	// A flow room: its requests run on the flow (the flow engine steers them).
	request, err := IsRequest(ctx, q, in)
	if err != nil {
		return nil, db.Project{}, err
	}
	if request {
		def, err := DefaultFor(ctx, q, in.WorkspaceID)
		return &def, project, err
	}
	return nonRequestWorkflowFor(ctx, q, in.WorkspaceID, project, policy)
}

// nonRequestWorkflowFor resolves a non-request issue of a flow room by the
// workspace setting. Every choice that is not a stored workflow falls back to
// Default, so the read path never refuses.
func nonRequestWorkflowFor(ctx context.Context, q Querier, workspaceID pgtype.UUID, project db.Project, policy NonRequestPolicy) (*Definition, db.Project, error) {
	if policy.Kind == NonRequestWorkflow && policy.WorkflowID.Valid {
		row, err := q.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: policy.WorkflowID, WorkspaceID: workspaceID})
		if err == nil {
			def, err := Decode(row)
			return &def, project, err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, db.Project{}, err
		}
	}
	def, err := DefaultFor(ctx, q, workspaceID)
	return &def, project, err
}

// CheckRoomWrite refuses a write that would leave an issue in a room with no
// workflow to follow. It runs only where an issue is created, given to a room,
// or moved into a project, and it is the only place these rules refuse.
//
// Refused: a room with no active workflow, and a non-request issue of a flow room
// under the "no_action" policy. Not refused: issues outside a project (as before),
// engine tasks, and issues without a room (they follow Default).
func CheckRoomWrite(ctx context.Context, q Querier, in RoomIssue, policy NonRequestPolicy) error {
	if !in.ProjectID.Valid || in.FlowTask.Valid {
		return nil
	}
	room, err := roomOf(ctx, q, in)
	if err != nil {
		return err
	}
	if !room.Valid {
		return nil
	}
	def, found, err := squadWorkflow(ctx, q, in.WorkspaceID, room)
	if err != nil {
		return err
	}
	if !found {
		return &RoomWorkflowError{
			Code: CodeRoomWorkflowRequired,
			Room: room,
			Message: "this squad has no workflow yet: choose a workflow for it, or design one in the Workflow tab, " +
				"then assign the issue again",
		}
	}
	if def.Flow == nil {
		return nil
	}
	request, err := IsRequest(ctx, q, in)
	if err != nil {
		return err
	}
	if request {
		return nil
	}
	if policy.Kind == NonRequestNoAction || policy.Kind == "" {
		return &RoomWorkflowError{
			Code: CodeNonRequestWorkflowNeeded,
			Room: room,
			Message: "this issue is not a request of its squad, and the workspace has not chosen a workflow for " +
				"such issues: choose one in the workspace settings, then try again",
		}
	}
	return nil
}

// CheckRoomPlacement applies CheckRoomWrite with the workspace's stored policy.
// Automations, initiatives, builds, and proposals write issues without going
// through the issue handlers; they call this before the write, so they refuse
// the same way and never fall back to Default (spec section 13). The caller
// returns the error; a refusal is never dropped silently.
func CheckRoomPlacement(ctx context.Context, q Querier, in RoomIssue) error {
	policy, err := LoadNonRequestPolicy(ctx, q, in.WorkspaceID)
	if err != nil {
		return err
	}
	return CheckRoomWrite(ctx, q, in, policy)
}

// RoomIssueOf reads the room rules' inputs from an issue row.
func RoomIssueOf(issue db.Issue) RoomIssue {
	return RoomIssue{
		WorkspaceID:   issue.WorkspaceID,
		ProjectID:     issue.ProjectID,
		AssigneeType:  issue.AssigneeType,
		AssigneeID:    issue.AssigneeID,
		ParentIssueID: issue.ParentIssueID,
		CreatorType:   issue.CreatorType,
		CreatorID:     issue.CreatorID,
		FlowTask:      issue.FlowTask,
	}
}

// Resolve returns the workflow an issue follows under the room rules (spec
// section 3). It never refuses; the write checks refuse (CheckRoomWrite).
func Resolve(ctx context.Context, q Querier, in RoomIssue) (*Definition, db.Project, error) {
	policy, err := LoadNonRequestPolicy(ctx, q, in.WorkspaceID)
	if err != nil {
		return nil, db.Project{}, err
	}
	return ResolveRoomWorkflow(ctx, q, in, policy)
}
