package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// The run API of the general engine (spec 7.1): complete a lead step, set a
// variable of a request or of a task, choose an exit of a manual step, and close
// a task for its assignee.

type completeFlowStepRequest struct {
	Verdict string         `json:"verdict"`
	Vars    map[string]any `json:"vars"`
	Blocked *struct {
		Missing []string `json:"missing"`
		Reason  string   `json:"reason"`
	} `json:"blocked"`
}

// CompleteIssueFlowStep completes a lead step: its verdict and the variables it
// sets, atomically. POST …/flow/plan is the same call on the plan step.
func (h *Handler) CompleteIssueFlowStep(w http.ResponseWriter, r *http.Request) {
	var req completeFlowStepRequest
	if !decodeFlowBody(w, r, &req) {
		return
	}
	request, _, _, ok := h.loadOperatedRun(w, r, true)
	if !ok {
		return
	}
	in := FlowCompleteInput{Verdict: req.Verdict, Vars: req.Vars}
	if req.Blocked != nil {
		in.Blocked = &FlowPlanBlocked{Missing: req.Blocked.Missing, Reason: req.Blocked.Reason}
	}
	state, err := h.CompleteFlowStep(r.Context(), request, chi.URLParam(r, "step"), in)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// SetIssueFlowVar sets a variable. On a request it is set by a role the
// declaration of the variable names, or by a member who manages the squad; on a
// task it is set by the assignee while the task is open (spec 4.12).
func (h *Handler) SetIssueFlowVar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Value any `json:"value"`
	}
	if !decodeFlowBody(w, r, &req) {
		return
	}
	issue, role, ok := h.flowRequestFor(w, r, true)
	if !ok {
		return
	}
	if role == issueworkflow.RoleTask {
		task, err := h.Queries.GetIssueInWorkspace(r.Context(), db.GetIssueInWorkspaceParams{ID: parseUUID(chi.URLParam(r, "id")), WorkspaceID: issue.WorkspaceID})
		if err != nil {
			writeFlowError(w, r, errFlowNotManaged)
			return
		}
		h.setTaskFlowVar(w, r, issue, task, chi.URLParam(r, "key"), req.Value)
		return
	}
	h.setRequestFlowVar(w, r, issue, chi.URLParam(r, "key"), req.Value)
}

// setRequestFlowVar sets a variable of the request itself.
func (h *Handler) setRequestFlowVar(w http.ResponseWriter, r *http.Request, issue db.Issue, key string, value any) {
	started, err := h.EnsureFlowStarted(r.Context(), issue)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	run, err := h.loadFlowRun(r.Context(), h.Queries, started)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	actor := h.flowActorFor(r, userID, uuidToString(run.issue.WorkspaceID))
	who := flowVarSetter{By: actor.ID}
	switch {
	case actor.Type == "agent":
		if squad, have := h.flowSquadOf(r, run); have && uuidToString(squad.LeaderID) == actor.ID {
			who.Kinds = append(who.Kinds, issueworkflow.RoleKindLead)
		}
	case actor.Member != nil:
		who.Manager = h.canManageFlowRequest(r.Context(), *actor.Member, run)
		if !run.compat() && run.issue.CreatorType == "member" && uuidToString(run.issue.CreatorID) == actor.ID {
			who.Kinds = append(who.Kinds, issueworkflow.RoleKindRequester)
		}
	}
	if !who.Manager && len(who.Kinds) == 0 {
		writeFlowRejection(w, &flowRejection{
			Status: http.StatusForbidden, Code: "flow_forbidden",
			Message: "only the lead of the squad and the members who manage it can do this",
		})
		return
	}
	state, err := h.SetFlowVar(r.Context(), started, key, value, who)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// setTaskFlowVar sets a variable from a task: the assignee of the task, or a member
// who manages the squad, while the task is open and lists the variable in its sets.
func (h *Handler) setTaskFlowVar(w http.ResponseWriter, r *http.Request, request, task db.Issue, key string, value any) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	forbidden := func(message string) {
		writeFlowRejection(w, &flowRejection{Status: http.StatusForbidden, Code: "flow_var_forbidden", Message: message, Fields: map[string]any{"var": key, "task": task.FlowTask.String}})
	}
	actor := h.flowActorFor(r, userID, uuidToString(task.WorkspaceID))
	tx, qtx, run, err := h.lockFlowRun(r.Context(), request)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	manages := actor.Member != nil && h.canManageFlowRequest(r.Context(), *actor.Member, run)
	if !flowTaskAssignedTo(task, actor) && !manages {
		forbidden("only the assignee of the task, or a member who manages the squad, sets a variable from it")
		return
	}
	spec, found := flowGraphTask(run.def.Graph, task.FlowStep.String, task.FlowTask.String)
	if !found || !flowHasString(spec.Sets, key) {
		forbidden(fmt.Sprintf("the task does not set the variable %q", key))
		return
	}
	switch issuestatus.Category(r.Context(), qtx, task.WorkspaceID, task.Status) {
	case issuestatus.CategoryDone, issuestatus.CategoryClosed:
		forbidden("the task is closed: a variable is set while it is open")
		return
	}
	if task.FlowStep.String != run.state.Step || int(task.FlowRound.Int32) != run.round() || run.state.ClosedAt != "" {
		forbidden("the task is not open in the current entry of its step")
		return
	}
	chosen, err := checkFlowVars(run, map[string]any{key: value}, nil)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	if err := h.checkVarNotInUse(r.Context(), qtx, run, key); err != nil {
		writeFlowError(w, r, err)
		return
	}
	next := run.state.Clone()
	next.Vars = mergedVars(run.def.Graph, run.state.Vars, chosen)
	next.VarSets[flowVarSetKey(task.FlowStep.String, task.FlowTask.String, key)] = int(task.FlowRound.Int32) + 1
	saved, err := h.saveFlowState(r.Context(), qtx, run, next)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	if !saved {
		writeFlowError(w, r, wrongStep(run.state.Step, "the request moved while the variable was being set; read its state and try again"))
		return
	}
	details, _ := json.Marshal(map[string]any{"var": key, "value": chosen[key], "task": task.FlowTask.String, "step": task.FlowStep.String, "by": actor.ID})
	if _, err := qtx.CreateActivity(r.Context(), db.CreateActivityParams{
		ID: dbid.NewV7(), WorkspaceID: run.issue.WorkspaceID, IssueID: run.issue.ID,
		ActorType: pgtype.Text{String: "system", Valid: true}, Action: "flow_var_set", Details: details,
	}); err != nil {
		writeFlowError(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeFlowError(w, r, err)
		return
	}
	h.publishFlowUpdated(run.issue)
	state, err := h.flowStateResponse(r.Context(), run.issue, issueworkflow.RoleRequest)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// ExitIssueFlow chooses a way out of a manual step. It is the human checkpoint of
// the step (spec 4.8).
func (h *Handler) ExitIssueFlow(w http.ResponseWriter, r *http.Request) {
	request, role, ok := h.flowRequestFor(w, r, false)
	if !ok || role != issueworkflow.RoleRequest {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	actor := h.flowActorFor(r, userID, uuidToString(request.WorkspaceID))
	state, err := h.ExitFlow(r.Context(), request, chi.URLParam(r, "edgeId"), actor)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// canTakeExit says whether the actor may choose an exit of the step: a member who
// manages the squad, or a member of the exit role of the step.
func (h *Handler) canTakeExit(ctx context.Context, run *flowRun, step issueworkflow.DocStep, actor flowActor) bool {
	if actor.Member == nil {
		return false
	}
	if h.canManageFlowRequest(ctx, *actor.Member, run) {
		return true
	}
	roleKey := step.ExitRole
	if roleKey == "" {
		roleKey = issueworkflow.RoleKindManagers
	}
	switch run.def.Graph.RoleKind(roleKey) {
	case issueworkflow.RoleKindRequester:
		return run.issue.CreatorType == "member" && uuidToString(run.issue.CreatorID) == actor.ID
	case issueworkflow.RoleKindMember:
		if role, ok := run.def.Graph.Role(roleKey); ok && role.Bind != nil {
			return flowHasString(role.Bind.MemberIDs, actor.ID)
		}
	}
	return false
}

// ExitFlow takes the manual edge a person chose: it records the checkpoint, clears
// unreviewed and moves the request on as an automatic edge would.
func (h *Handler) ExitFlow(ctx context.Context, request db.Issue, edgeID string, actor flowActor) (IssueFlowStateResponse, error) {
	tx, qtx, run, err := h.lockFlowRun(ctx, request)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	defer tx.Rollback(ctx)
	g := run.def.Graph
	step, isStep := g.Step(run.state.Step)
	if run.state.ClosedAt != "" || run.state.Stop != nil || !isStep || step.Kind != issueworkflow.StepKindManual {
		return IssueFlowStateResponse{}, wrongStep(run.state.Step, fmt.Sprintf("the request is at %q: it waits for no exit", run.state.Step))
	}
	if !h.canTakeExit(ctx, run, step, actor) {
		return IssueFlowStateResponse{}, &flowRejection{
			Status: http.StatusForbidden, Code: "flow_exit_forbidden",
			Message: "only a member of the role of this step, or a member who manages the squad, chooses how it goes on",
			Fields:  map[string]any{"step": step.Key},
		}
	}
	rows, err := h.loadFlowTasks(ctx, qtx, run)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	states := taskStates(rows)
	d := issueworkflow.RouteManual(g, run.state, states, edgeID, issueworkflow.BuildGraphFacts(g, run.state, states, flowRequestFacts(run.issue)))
	if d.Kind != issueworkflow.DecisionRoute {
		return IssueFlowStateResponse{}, &flowRejection{
			Status: http.StatusConflict, Code: "flow_edge_unavailable",
			Message: fmt.Sprintf("the exit %q cannot be taken now", edgeID),
			Fields:  map[string]any{"edge": edgeID, "step": step.Key},
		}
	}
	var effects flowEffects
	routed, err := h.routeFlowRequest(ctx, qtx, run, d, rows, flowRouteNote{}, &effects, actor.ID)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	if !routed {
		return IssueFlowStateResponse{}, wrongStep(run.state.Step, "the request moved while the exit was being taken; read its state and try again")
	}
	if err := tx.Commit(ctx); err != nil {
		return IssueFlowStateResponse{}, fmt.Errorf("commit flow exit: %w", err)
	}
	h.publishFlowEffects(ctx, run.issue, &effects)
	resp, err := h.AdvanceFlow(ctx, run.issue)
	if err != nil {
		return IssueFlowStateResponse{}, err
	}
	return resp.State, nil
}

type closeFlowTaskRequest struct {
	Verdict string `json:"verdict"`
	Note    string `json:"note"`
}

// CloseIssueFlowTask closes a task for its assignee: a member who manages the
// squad gives the verdict and the files, and the rules of the assignee apply
// (spec 4.7). It is the status write of the task made by that member, and so it is
// logged as flow_manual_close.
func (h *Handler) CloseIssueFlowTask(w http.ResponseWriter, r *http.Request) {
	var req closeFlowTaskRequest
	if !decodeFlowBody(w, r, &req) {
		return
	}
	issue, role, ok := h.flowRequestFor(w, r, true)
	if !ok {
		return
	}
	if role != issueworkflow.RoleTask {
		writeFlowRejection(w, errFlowNotManaged)
		return
	}
	taskID := chi.URLParam(r, "id")
	task, err := h.Queries.GetIssueInWorkspace(r.Context(), db.GetIssueInWorkspaceParams{ID: parseUUID(taskID), WorkspaceID: issue.WorkspaceID})
	if err != nil {
		writeFlowRejection(w, errFlowNotManaged)
		return
	}
	run, err := h.loadFlowRun(r.Context(), h.Queries, issue)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	actor := h.flowActorFor(r, userID, uuidToString(issue.WorkspaceID))
	if actor.Member == nil || !h.canManageFlowRequest(r.Context(), *actor.Member, run) {
		writeFlowRejection(w, &flowRejection{
			Status: http.StatusForbidden, Code: "flow_forbidden",
			Message: "only a member who manages the squad closes a task for its assignee",
		})
		return
	}
	body := map[string]any{"status": issuestatus.Done}
	if req.Verdict != "" {
		body["outcome"] = req.Verdict
	}
	raw, _ := json.Marshal(body)
	inner := r.Clone(context.WithValue(r.Context(), flowCloseNoteKey{}, req.Note))
	inner.Method = http.MethodPut
	inner.Body = nopReadCloser{bytes.NewReader(raw)}
	inner.ContentLength = int64(len(raw))
	inner.Header = r.Header.Clone()
	inner.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.UpdateIssue(rec, inner)
	if rec.Code != http.StatusOK {
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		w.Write(rec.Body.Bytes())
		return
	}
	state, err := h.flowStateResponse(r.Context(), issue, issueworkflow.RoleRequest)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	h.fillFlowViewer(r.Context(), issue, &state, actor)
	var closed IssueFlowTaskResponse
	for _, t := range state.Tasks {
		if t.IssueID == uuidToString(task.ID) {
			closed = t
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": closed, "state": state})
}

type nopReadCloser struct{ *bytes.Reader }

func (nopReadCloser) Close() error { return nil }
