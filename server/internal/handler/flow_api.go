package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The HTTP face of the flow engine (docs/engineering/kitviet-workflow-spec.md,
// section 7.1). Reading a flow is open to workspace members. The lead of the
// squad and the members who manage it may plan, advance, set switches and
// start; only those members resume and detach.

// flowRequestFor resolves the issue in the URL to the request a flow runs and
// the role the issue plays: the request itself, or one of its tasks (whose
// request is its parent). Every failure is written.
func (h *Handler) flowRequestFor(w http.ResponseWriter, r *http.Request, allowTask bool) (db.Issue, string, bool) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return db.Issue{}, "", false
	}
	if !h.flowEngineEnabled(r.Context()) {
		writeFlowRejection(w, errFlowNotManaged)
		return db.Issue{}, "", false
	}
	if issue.FlowTask.Valid {
		if !allowTask || !issue.ParentIssueID.Valid {
			writeFlowRejection(w, errFlowNotManaged)
			return db.Issue{}, "", false
		}
		parent, err := h.Queries.GetIssueInWorkspace(r.Context(), db.GetIssueInWorkspaceParams{ID: issue.ParentIssueID, WorkspaceID: issue.WorkspaceID})
		if err != nil {
			writeFlowRejection(w, errFlowNotManaged)
			return db.Issue{}, "", false
		}
		return parent, issueworkflow.RoleTask, true
	}
	return issue, issueworkflow.RoleRequest, true
}

// writeFlowError answers an engine error: a rejection with its code, anything
// else as a 500.
func writeFlowError(w http.ResponseWriter, r *http.Request, err error) {
	if writeFlowRejection(w, err) {
		return
	}
	slog.Warn("flow request failed", append(logger.RequestAttrs(r), "error", err)...)
	writeError(w, http.StatusInternalServerError, "failed to run the workflow")
}

// flowSquadOf is the squad whose lead and managers may operate the request: the
// squad that owns its workflow, else the squad it is given to.
func (h *Handler) flowSquadOf(r *http.Request, run *flowRun) (db.Squad, bool) {
	squadID := run.row.SquadID
	if !squadID.Valid && run.issue.AssigneeType.String == "squad" {
		squadID = run.issue.AssigneeID
	}
	if !squadID.Valid {
		return db.Squad{}, false
	}
	squad, err := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{ID: squadID, WorkspaceID: run.issue.WorkspaceID})
	return squad, err == nil
}

// authorizeFlowOperator lets the lead of the squad and the members who manage
// it operate a request; leadAllowed false leaves only the members.
func (h *Handler) authorizeFlowOperator(w http.ResponseWriter, r *http.Request, run *flowRun, leadAllowed bool) (flowActor, bool) {
	workspaceID := uuidToString(run.issue.WorkspaceID)
	userID, ok := requireUserID(w, r)
	if !ok {
		return flowActor{}, false
	}
	actor := h.flowActorFor(r, userID, workspaceID)
	squad, haveSquad := h.flowSquadOf(r, run)
	switch {
	case actor.Type == "agent":
		agentID := r.Header.Get("X-Agent-ID")
		if leadAllowed && haveSquad && uuidToString(squad.LeaderID) == agentID {
			return actor, true
		}
	case actor.Member != nil:
		if haveSquad && canManageSquad(*actor.Member, squad) {
			return actor, true
		}
		if !haveSquad && isWorkspaceAdmin(*actor.Member) {
			return actor, true
		}
	}
	writeFlowRejection(w, &flowRejection{
		Status: http.StatusForbidden, Code: "flow_forbidden",
		Message: "only the lead of the squad and the members who manage it can do this",
	})
	return flowActor{}, false
}

// loadOperatedRun resolves the request in the URL, checks the caller and
// returns the fresh request. Every failure is written.
func (h *Handler) loadOperatedRun(w http.ResponseWriter, r *http.Request, leadAllowed bool) (db.Issue, *flowRun, flowActor, bool) {
	request, role, ok := h.flowRequestFor(w, r, false)
	if !ok || role != issueworkflow.RoleRequest {
		return db.Issue{}, nil, flowActor{}, false
	}
	started, err := h.EnsureFlowStarted(r.Context(), request)
	if err != nil {
		writeFlowError(w, r, err)
		return db.Issue{}, nil, flowActor{}, false
	}
	run, err := h.loadFlowRun(r.Context(), h.Queries, started)
	if err != nil {
		writeFlowError(w, r, err)
		return db.Issue{}, nil, flowActor{}, false
	}
	actor, ok := h.authorizeFlowOperator(w, r, run, leadAllowed)
	if !ok {
		return db.Issue{}, nil, flowActor{}, false
	}
	return started, run, actor, true
}

// GetIssueFlow returns the flow state of a request, or of the request a task
// belongs to. A request that qualifies and has not started yet is started.
func (h *Handler) GetIssueFlow(w http.ResponseWriter, r *http.Request) {
	request, role, ok := h.flowRequestFor(w, r, true)
	if !ok {
		return
	}
	started, err := h.EnsureFlowStarted(r.Context(), request)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	state, err := h.flowStateResponse(r.Context(), started, role)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

type planFlowRequest struct {
	Switches map[string]any `json:"switches"`
	Blocked  *struct {
		Missing []string `json:"missing"`
		Reason  string   `json:"reason"`
	} `json:"blocked"`
}

// PlanIssueFlow records the lead's plan.
func (h *Handler) PlanIssueFlow(w http.ResponseWriter, r *http.Request) {
	var req planFlowRequest
	if !decodeFlowBody(w, r, &req) {
		return
	}
	request, _, _, ok := h.loadOperatedRun(w, r, true)
	if !ok {
		return
	}
	in := FlowPlanInput{Switches: req.Switches}
	if req.Blocked != nil {
		in.Blocked = &FlowPlanBlocked{Missing: req.Blocked.Missing, Reason: req.Blocked.Reason}
	}
	state, err := h.PlanFlow(r.Context(), request, in)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// AdvanceIssueFlow runs the engine on a request.
func (h *Handler) AdvanceIssueFlow(w http.ResponseWriter, r *http.Request) {
	request, _, _, ok := h.loadOperatedRun(w, r, true)
	if !ok {
		return
	}
	resp, err := h.AdvanceFlow(r.Context(), request)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// SetIssueFlowSwitch sets one switch of a request.
func (h *Handler) SetIssueFlowSwitch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Value any `json:"value"`
	}
	if !decodeFlowBody(w, r, &req) {
		return
	}
	request, _, actor, ok := h.loadOperatedRun(w, r, true)
	if !ok {
		return
	}
	state, err := h.SetFlowSwitch(r.Context(), request, chi.URLParam(r, "key"), req.Value, actor.Type == "member")
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// ResumeIssueFlow continues a stopped request.
func (h *Handler) ResumeIssueFlow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ToStep      string `json:"to_step"`
		ResetRework bool   `json:"reset_rework"`
	}
	if !decodeFlowBody(w, r, &req) {
		return
	}
	request, _, _, ok := h.loadOperatedRun(w, r, false)
	if !ok {
		return
	}
	state, err := h.ResumeFlow(r.Context(), request, FlowResumeInput{ToStep: req.ToStep, ResetRework: req.ResetRework})
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// DetachIssueFlow takes a request off its flow.
func (h *Handler) DetachIssueFlow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CancelTasks bool `json:"cancel_tasks"`
	}
	if !decodeFlowBody(w, r, &req) {
		return
	}
	request, _, _, ok := h.loadOperatedRun(w, r, false)
	if !ok {
		return
	}
	if err := h.DetachFlow(r.Context(), request, req.CancelTasks); err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"detached": true})
}

// StartIssueFlow puts a request on its squad's flow when it has not started.
// It refuses an issue that has a parent or children, and a squad that has no
// workflow that runs on a flow.
func (h *Handler) StartIssueFlow(w http.ResponseWriter, r *http.Request) {
	issue, role, ok := h.flowRequestFor(w, r, false)
	if !ok || role != issueworkflow.RoleRequest {
		return
	}
	reject := func(message string) {
		writeFlowRejection(w, &flowRejection{Status: http.StatusConflict, Code: "flow_cannot_start", Message: message})
	}
	if len(issue.Flow) > 0 {
		state, err := h.flowStateResponse(r.Context(), issue, issueworkflow.RoleRequest)
		if err != nil {
			writeFlowError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
		return
	}
	if issue.ParentIssueID.Valid {
		reject("a sub-issue cannot start a flow")
		return
	}
	if issue.AssigneeType.String != "squad" || !issue.AssigneeID.Valid {
		reject("only an issue given to a squad can start a flow")
		return
	}
	children, err := h.Queries.ListChildIssues(r.Context(), issue.ID)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	if len(children) > 0 {
		reject("an issue that has sub-issues cannot start a flow")
		return
	}
	squad, err := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{ID: issue.AssigneeID, WorkspaceID: issue.WorkspaceID})
	if err != nil || !squad.ActiveWorkflowID.Valid {
		reject("the squad has no workflow that runs on a flow")
		return
	}
	row, err := h.Queries.GetIssueWorkflow(r.Context(), db.GetIssueWorkflowParams{ID: squad.ActiveWorkflowID, WorkspaceID: issue.WorkspaceID})
	if err != nil || len(row.FlowConfig) == 0 {
		reject("the squad has no workflow that runs on a flow")
		return
	}
	probe := &flowRun{issue: issue, row: row}
	if _, ok := h.authorizeFlowOperator(w, r, probe, true); !ok {
		return
	}
	started, err := service.StartFlow(r.Context(), h.Queries, issue, row)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	state, err := h.flowStateResponse(r.Context(), started, issueworkflow.RoleRequest)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	h.publishFlowUpdated(started)
	writeJSON(w, http.StatusOK, state)
}

// SetIssueWorkflowFlowAuto turns the background runner on or off for a
// workflow's requests. Workspace admins and owners only.
func (h *Handler) SetIssueWorkflowFlowAuto(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return
	}
	if !isWorkspaceAdmin(member) {
		writeError(w, http.StatusForbidden, "only workspace admins can turn the runner on")
		return
	}
	idUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workflow id")
	if !ok {
		return
	}
	var req struct {
		Auto bool `json:"auto"`
	}
	if !decodeFlowBody(w, r, &req) {
		return
	}
	row, err := h.Queries.GetIssueWorkflow(r.Context(), db.GetIssueWorkflowParams{ID: idUUID, WorkspaceID: wsUUID})
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if len(row.FlowConfig) == 0 {
		writeErrorCode(w, http.StatusConflict, "flow_cannot_start", "only a workflow that runs on a flow has a runner")
		return
	}
	updated, err := h.Queries.SetIssueWorkflowFlowAuto(r.Context(), db.SetIssueWorkflowFlowAutoParams{FlowAuto: req.Auto, ID: idUUID, WorkspaceID: wsUUID})
	if err != nil {
		slog.Warn("set flow auto failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	h.publishIssueWorkflowChanged(workspaceID, member, "updated")
	h.writeIssueWorkflow(w, r, http.StatusOK, updated)
}

// decodeFlowBody reads an optional JSON body; an empty body is the zero value.
func decodeFlowBody(w http.ResponseWriter, r *http.Request, dest any) bool {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return false
	}
	if len(raw) == 0 {
		return true
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
