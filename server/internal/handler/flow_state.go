package handler

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// IssueFlowTaskResponse is one task the engine opened for a request.
type IssueFlowTaskResponse struct {
	IssueID    string `json:"issue_id"`
	Identifier string `json:"identifier"`
	Step       string `json:"step"`
	Task       string `json:"task"`
	Round      int    `json:"round"`
	// Kind is "gate" or "work" as the workflow defines the task; "" when the
	// definition has no such task.
	Kind       string  `json:"kind"`
	Status     string  `json:"status"`
	Outcome    *string `json:"outcome"`
	AssigneeID *string `json:"assignee_id"`
	// ClosedAt is when the task issue reached a finished status; null while open.
	ClosedAt *string `json:"closed_at"`
	// ClosedBy is who closed the task, null while it is open or when nobody is known.
	ClosedBy *IssueFlowClosedByResponse `json:"closed_by"`
}

// IssueFlowClosedByResponse says who closed a task. OnBehalf is true when a member
// who manages the squad closed it for its assignee.
type IssueFlowClosedByResponse struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	OnBehalf bool   `json:"on_behalf"`
}

// IssueFlowViewerResponse is what the reader of the state may do. It is empty for
// a reader who manages nothing and has no task of their own.
type IssueFlowViewerResponse struct {
	// CanManage: the reader manages the squad that runs the request.
	CanManage bool `json:"can_manage"`
	// ClosableTaskIDs are the open tasks the reader may close: their own, or any
	// when they manage the squad.
	ClosableTaskIDs []string `json:"closable_task_ids"`
}

// IssueFlowNextResponse is a way out of the current step.
type IssueFlowNextResponse struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	To     string `json:"to"`
	Rework bool   `json:"rework"`
}

// IssueFlowStateResponse is the state of a request on a flow
// (GET /api/issues/{id}/flow).
type IssueFlowStateResponse struct {
	Managed bool `json:"managed"`
	// Role is "task" when the issue asked about is a task: the state is its
	// request's.
	Role         string                  `json:"role"`
	WorkflowID   string                  `json:"workflow_id"`
	WorkflowName string                  `json:"workflow_name"`
	Step         string                  `json:"step"`
	Rework       int                     `json:"rework"`
	Stop         issueworkflow.Stop      `json:"stop"`
	Switches     map[string]any          `json:"switches"`
	Auto         bool                    `json:"auto"`
	EnteredAt    string                  `json:"entered_at"`
	Tasks        []IssueFlowTaskResponse `json:"tasks"`
	Next         []IssueFlowNextResponse `json:"next"`
	// Viewer is filled for the reader of GET /api/issues/{id}/flow; the other answers carry the empty value.
	Viewer IssueFlowViewerResponse `json:"viewer"`
	// ReworkStep is the step a rework sends the request back to from the step it stopped at:
	// the drafting step, where a person sends the request to fix the draft. It is empty unless
	// the request is stopped and that step has a rework transition.
	ReworkStep string `json:"rework_step"`
}

// FlowAction is one thing an advance did.
type FlowAction struct {
	// Kind is opened_tasks, routed, stopped or noop.
	Kind  string   `json:"kind"`
	Step  string   `json:"step"`
	Tasks []string `json:"tasks"`
	// Code and AgentID are set on a stop for an agent that cannot take its
	// task (flow_agent_unavailable): the advance reports it instead of failing.
	Code    string `json:"code,omitempty"`
	AgentID string `json:"agent_id,omitempty"`
}

// FlowPending is a task the request still waits for.
type FlowPending struct {
	Task    string `json:"task"`
	IssueID string `json:"issue_id"`
	Status  string `json:"status"`
}

// FlowAdvanceResponse is the answer of an advance
// (POST /api/issues/{id}/flow/advance).
type FlowAdvanceResponse struct {
	State   IssueFlowStateResponse `json:"state"`
	Actions []FlowAction           `json:"actions"`
	Pending []FlowPending          `json:"pending"`
}

// flowStateResponse builds the state of the request. role is "request" or
// "task" (the issue the caller asked about).
func (h *Handler) flowStateResponse(ctx context.Context, request db.Issue, role string) (IssueFlowStateResponse, error) {
	run, err := h.loadFlowRun(ctx, h.Queries, request)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	tasks, err := h.Queries.ListFlowTasks(ctx, db.ListFlowTasksParams{ParentIssueID: request.ID, WorkspaceID: request.WorkspaceID})
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	prefix := h.getIssuePrefix(ctx, request.WorkspaceID)
	resolver := issuestatus.NewResolver(request.WorkspaceID)
	out := IssueFlowStateResponse{
		Managed:      true,
		Role:         role,
		WorkflowID:   run.state.WorkflowID,
		WorkflowName: run.def.Name,
		Step:         run.state.Step,
		Rework:       run.state.Rework,
		Stop:         run.state.Stop,
		Switches:     run.state.Switches,
		Auto:         run.row.FlowAuto,
		EnteredAt:    run.state.EnteredAt,
		Tasks:        make([]IssueFlowTaskResponse, 0, len(tasks)),
		Next:         []IssueFlowNextResponse{},
		Viewer:       IssueFlowViewerResponse{ClosableTaskIDs: []string{}},
	}
	if out.Switches == nil {
		out.Switches = map[string]any{}
	}
	closedByAction, err := h.flowManualActions(ctx, tasks)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	for _, t := range tasks {
		item := IssueFlowTaskResponse{
			IssueID:    uuidToString(t.ID),
			Identifier: prefix + "-" + strconv.Itoa(int(t.Number)),
			Step:       t.FlowStep.String,
			Task:       t.FlowTask.String,
			Round:      int(t.FlowRound.Int32),
			Kind:       flowTaskKind(run.def, t.FlowStep.String, t.FlowTask.String),
			Status:     t.Status,
			Outcome:    textToPtr(t.Outcome),
			AssigneeID: uuidToPtr(t.AssigneeID),
		}
		switch resolver.Category(ctx, h.Queries, t.Status) {
		case issuestatus.CategoryDone, issuestatus.CategoryClosed:
			item.ClosedAt = timestampToPtr(t.UpdatedAt)
			item.ClosedBy = flowClosedBy(t, closedByAction[t.ID])
		}
		out.Tasks = append(out.Tasks, item)
	}
	if step, ok := run.def.Step(run.state.Step); ok {
		for _, tr := range step.Transitions {
			out.Next = append(out.Next, IssueFlowNextResponse{ID: tr.ID, Label: tr.Label, To: tr.ToStatusKey, Rework: tr.Rework})
		}
	}
	if run.state.Stop != issueworkflow.StopNone {
		out.ReworkStep = reworkStepOf(run.def, run.state.BlockedFrom)
	}
	return out, nil
}

// reworkStepOf returns the step the first rework transition of from leads to, or "" when from has none.
func reworkStepOf(def issueworkflow.Definition, from string) string {
	step, ok := def.Step(from)
	if !ok {
		return ""
	}
	for _, tr := range step.Transitions {
		if tr.Rework {
			return tr.ToStatusKey
		}
	}
	return ""
}

// flowTaskKind returns the kind ("gate" or "work") of the task a step defines,
// or "" when the definition has no such task.
func flowTaskKind(def issueworkflow.Definition, step, key string) string {
	st, ok := def.Step(step)
	if !ok {
		return ""
	}
	for _, task := range st.Tasks {
		if task.Key == key {
			return task.Kind
		}
	}
	return ""
}

// flowManualActions reads the latest close or reopen a member made on each task.
func (h *Handler) flowManualActions(ctx context.Context, tasks []db.Issue) (map[pgtype.UUID]db.ListFlowManualActionsRow, error) {
	out := map[pgtype.UUID]db.ListFlowManualActionsRow{}
	if len(tasks) == 0 {
		return out, nil
	}
	ids := make([]pgtype.UUID, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	rows, err := h.Queries.ListFlowManualActions(ctx, db.ListFlowManualActionsParams{WorkspaceID: tasks[0].WorkspaceID, IssueIds: ids})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.IssueID] = row
	}
	return out, nil
}

// flowClosedBy says who closed a closed task: the member whose close for the
// assignee is the latest entry of the log, else the assignee.
func flowClosedBy(task db.Issue, last db.ListFlowManualActionsRow) *IssueFlowClosedByResponse {
	if last.Action == "flow_manual_close" && last.ActorID.Valid {
		return &IssueFlowClosedByResponse{Type: last.ActorType.String, ID: uuidToString(last.ActorID), OnBehalf: true}
	}
	if task.AssigneeID.Valid {
		return &IssueFlowClosedByResponse{Type: task.AssigneeType.String, ID: uuidToString(task.AssigneeID)}
	}
	return nil
}

// fillFlowViewer says what the reader of the state may do: whether they manage the
// squad that runs the request, and which open tasks they may close.
func (h *Handler) fillFlowViewer(ctx context.Context, request db.Issue, state *IssueFlowStateResponse, actor flowActor) {
	run, err := h.loadFlowRun(ctx, h.Queries, request)
	if err != nil {
		return
	}
	manages := actor.Member != nil && h.canManageFlowRequest(ctx, *actor.Member, run)
	state.Viewer.CanManage = manages
	for _, t := range state.Tasks {
		if t.ClosedAt != nil {
			continue
		}
		if manages || (t.AssigneeID != nil && *t.AssigneeID == actor.ID) {
			state.Viewer.ClosableTaskIDs = append(state.Viewer.ClosableTaskIDs, t.IssueID)
		}
	}
}
