package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// A squad's workflow library (MUL-7420). A squad owns several workflows, and
// one of them is its active workflow. The squad's issues in a project take the
// active workflow, so switching it moves only that squad's issues onto the new
// statuses. Other squads and the project's own issues are untouched.

// CreateSquadWorkflowRequest adds a workflow to a squad's library. The new
// workflow starts as a copy of SourceWorkflowID's statuses and handoffs.
type CreateSquadWorkflowRequest struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	SourceWorkflowID string `json:"source_workflow_id"`
}

// SetSquadActiveWorkflowRequest switches a squad's active workflow. StatusMapping
// and DryRun work as they do for a project's workflow switch.
type SetSquadActiveWorkflowRequest struct {
	WorkflowID    string            `json:"workflow_id"`
	StatusMapping map[string]string `json:"status_mapping,omitempty"`
	DryRun        bool              `json:"dry_run,omitempty"`
}

// ListSquadWorkflows returns a squad's library and which workflow is active.
func (h *Handler) ListSquadWorkflows(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return
	}
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	rows, err := h.Queries.ListSquadIssueWorkflows(r.Context(), db.ListSquadIssueWorkflowsParams{WorkspaceID: squad.WorkspaceID, SquadID: squad.ID})
	if err != nil {
		slog.Warn("ListSquadWorkflows failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to list workflows")
		return
	}
	openRequests := h.openFlowRequestCounts(r.Context(), squad.WorkspaceID)
	resp := make([]IssueWorkflowResponse, 0, len(rows))
	for _, row := range rows {
		def, err := issueworkflow.Decode(row)
		if err != nil {
			slog.Warn("ListSquadWorkflows decode failed", append(logger.RequestAttrs(r), "workflow_id", uuidToString(row.ID), "error", err)...)
			continue
		}
		item := issueWorkflowToResponse(row, def, nil)
		item.OpenFlowRequests = openRequests[item.ID]
		resp = append(resp, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active_workflow_id": uuidToPtr(squad.ActiveWorkflowID),
		"workflows":          resp,
		"total":              len(resp),
	})
}

// CreateSquadWorkflow adds a copy of a workflow to a squad's library.
func (h *Handler) CreateSquadWorkflow(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return
	}
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	if !canManageSquad(member, squad) {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
	if squad.ArchivedAt.Valid {
		writeError(w, http.StatusConflict, "squad is archived")
		return
	}
	var req CreateSquadWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sourceID, ok := parseUUIDOrBadRequest(w, req.SourceWorkflowID, "source_workflow_id")
	if !ok {
		return
	}
	wsUUID := squad.WorkspaceID
	ctx := r.Context()

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if err := qtx.LockIssueStatusCatalogShared(ctx, wsUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	source, err := qtx.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: sourceID, WorkspaceID: wsUUID})
	if err != nil {
		writeError(w, http.StatusNotFound, "source workflow not found")
		return
	}
	// A copy would keep the steps without the flow_config that gives them meaning.
	if len(source.FlowConfig) > 0 {
		writeManagedByTemplate(w)
		return
	}
	sourceSteps, err := issueworkflow.DecodeSteps(source.Steps)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read workflow")
		return
	}
	name, description, steps := issueworkflow.Normalize(req.Name, req.Description, sourceSteps)
	catalog, err := h.workflowStatusCatalog(ctx, qtx, wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load issue statuses")
		return
	}
	if err := issueworkflow.Validate(name, description, steps, false, catalog); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_workflow", err.Error())
		return
	}
	raw, err := issueworkflow.EncodeSteps(steps)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	row, err := qtx.CreateIssueWorkflow(ctx, db.CreateIssueWorkflowParams{
		WorkspaceID:      wsUUID,
		Name:             name,
		Description:      description,
		InitialStatusKey: issueworkflow.StartStatus,
		Steps:            raw,
		OwnerID:          member.UserID,
		SquadID:          squad.ID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeErrorCode(w, http.StatusConflict, "workflow_name_taken", "a workflow with this name already exists")
			return
		}
		slog.Warn("CreateSquadWorkflow failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	h.publishIssueWorkflowChanged(workspaceID, member, "created")
	h.writeIssueWorkflow(w, r, http.StatusCreated, row)
}

// SetSquadActiveWorkflow makes a workflow of the squad's library, or a
// workspace-level one, the squad's active workflow. The squad's issues on
// statuses the new workflow does not list move to the statuses the caller maps
// them to, in the same transaction. Other squads and the project's own issues
// are not touched. The move is administrative: it neither reassigns issues nor
// starts runs.
func (h *Handler) SetSquadActiveWorkflow(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return
	}
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	if !canManageSquad(member, squad) {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
	if squad.ArchivedAt.Valid {
		writeError(w, http.StatusConflict, "squad is archived")
		return
	}
	var req SetSquadActiveWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	workflowID, ok := parseUUIDOrBadRequest(w, req.WorkflowID, "workflow_id")
	if !ok {
		return
	}
	wsUUID := squad.WorkspaceID
	ctx := r.Context()

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change workflow")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	// Shared catalog lock: a workflow edit (exclusive) cannot change the
	// target's steps between the census and the remap below.
	if err := qtx.LockIssueStatusCatalogShared(ctx, wsUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change workflow")
		return
	}
	if _, err := qtx.LockSquadForWorkflowChange(ctx, db.LockSquadForWorkflowChangeParams{ID: squad.ID, WorkspaceID: wsUUID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "squad not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to change workflow")
		return
	}
	row, err := qtx.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: workflowID, WorkspaceID: wsUUID})
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if row.IsDefault {
		writeErrorCode(w, http.StatusBadRequest, "workflow_is_default", "the Default workflow cannot be a squad's active workflow")
		return
	}
	if row.SquadID.Valid && row.SquadID != squad.ID {
		writeErrorCode(w, http.StatusConflict, "workflow_belongs_to_other_squad", "this workflow belongs to another squad's library")
		return
	}
	target, err := issueworkflow.Decode(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read workflow")
		return
	}
	plan, err := h.squadWorkflowPlan(ctx, qtx, wsUUID, squad.ID, target)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change workflow")
		return
	}
	if req.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "plan": plan})
		return
	}
	if msg := checkMapping(plan, req.StatusMapping, target.Has); msg != "" {
		writeMappingRequired(w, plan, msg)
		return
	}
	adopted, err := adoptSquadWorkflow(ctx, qtx, wsUUID, squad.ID, row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change workflow")
		return
	}
	if !adopted {
		writeErrorCode(w, http.StatusConflict, "workflow_belongs_to_other_squad", "this workflow belongs to another squad's library")
		return
	}
	changed, cancelledWakeups, err := remapSquadIssues(ctx, qtx, wsUUID, squad.ID, plan, req.StatusMapping)
	if err != nil {
		slog.Warn("SetSquadActiveWorkflow remap failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to move issues to the new statuses")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change workflow")
		return
	}
	h.publishIssueSnapshots(ctx, changed, "member", uuidToString(member.UserID))
	h.broadcastCancelledWakeups(ctx, wsUUID, cancelledWakeups)
	h.publishIssueWorkflowChanged(workspaceID, member, "squad_active")
	writeJSON(w, http.StatusOK, map[string]any{
		"squad_id":           uuidToString(squad.ID),
		"active_workflow_id": uuidToString(row.ID),
		"updated_issues":     len(changed),
		"plan":               plan,
	})
}

// adoptSquadWorkflow makes wf the squad's active workflow. A workspace-level
// workflow joins the squad's library first. It reports false, and writes
// nothing, when another squad owns wf.
func adoptSquadWorkflow(ctx context.Context, q *db.Queries, wsUUID, squadID pgtype.UUID, wf db.IssueWorkflow) (bool, error) {
	if wf.SquadID.Valid && wf.SquadID != squadID {
		return false, nil
	}
	if !wf.SquadID.Valid {
		if _, err := q.SetIssueWorkflowSquad(ctx, db.SetIssueWorkflowSquadParams{SquadID: squadID, ID: wf.ID, WorkspaceID: wsUUID}); err != nil {
			return false, err
		}
	}
	if _, err := q.SetSquadActiveWorkflow(ctx, db.SetSquadActiveWorkflowParams{WorkflowID: wf.ID, ID: squadID, WorkspaceID: wsUUID}); err != nil {
		return false, err
	}
	return true, nil
}

// squadWorkflowPlan censors a squad's routed issues against a target workflow,
// as a project's workflow switch censors the project's issues.
func (h *Handler) squadWorkflowPlan(ctx context.Context, q *db.Queries, wsUUID, squadID pgtype.UUID, target issueworkflow.Definition) (workflowMappingPlan, error) {
	// A flow runs the squad's requests and their tasks, not its other issues:
	// those keep the project's workflow, so no issue changes status.
	if target.Flow != nil {
		return workflowMappingPlan{Required: []StatusMappingRequirement{}, Unchanged: []StatusIssueCount{}}, nil
	}
	catalog, err := h.workflowStatusCatalog(ctx, q, wsUUID)
	if err != nil {
		return workflowMappingPlan{}, err
	}
	counts, err := q.CountSquadIssuesByStatus(ctx, db.CountSquadIssuesByStatusParams{WorkspaceID: wsUUID, SquadID: squadID})
	if err != nil {
		return workflowMappingPlan{}, err
	}
	categoryOf := func(key string) string { return catalog[key].Category }
	return mappingPlan(squadCountsAsProject(counts), target.Has, func(key string) string {
		return issueworkflow.MoveTarget(target, key, categoryOf)
	}), nil
}

// suggestedMapping maps each status a plan requires to the status the plan
// suggests for it.
func suggestedMapping(plan workflowMappingPlan) map[string]string {
	out := make(map[string]string, len(plan.Required))
	for _, req := range plan.Required {
		out[req.StatusKey] = req.SuggestedStatusKey
	}
	return out
}

// squadCountsAsProject gives squad census rows the shape mappingPlan reads.
func squadCountsAsProject(rows []db.CountSquadIssuesByStatusRow) []db.CountProjectIssuesByStatusRow {
	out := make([]db.CountProjectIssuesByStatusRow, len(rows))
	for i, row := range rows {
		out[i] = db.CountProjectIssuesByStatusRow{Status: row.Status, IssueCount: row.IssueCount}
	}
	return out
}

// mergeStatusCounts adds two censuses of issues that do not overlap, one per
// status, sorted by status.
func mergeStatusCounts(a, b []db.CountProjectIssuesByStatusRow) []db.CountProjectIssuesByStatusRow {
	sums := map[string]int64{}
	for _, c := range slices.Concat(a, b) {
		sums[c.Status] += c.IssueCount
	}
	out := make([]db.CountProjectIssuesByStatusRow, 0, len(sums))
	for status, n := range sums {
		out = append(out, db.CountProjectIssuesByStatusRow{Status: status, IssueCount: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Status < out[j].Status })
	return out
}

// remapSquadIssues applies a validated mapping to a squad's routed issues
// inside the caller's transaction, with the same effects as remapIssues.
func remapSquadIssues(ctx context.Context, qtx *db.Queries, wsUUID, squadID pgtype.UUID, plan workflowMappingPlan, mapping map[string]string) ([]db.Issue, []db.AgentTaskQueue, error) {
	var changed []db.Issue
	var cancelled []db.AgentTaskQueue
	for _, req := range plan.Required {
		to := mapping[req.StatusKey]
		if to == "" {
			return nil, nil, errors.New("remap: no destination status for " + req.StatusKey)
		}
		rows, err := qtx.RemapSquadIssueStatus(ctx, db.RemapSquadIssueStatusParams{
			WorkspaceID: wsUUID,
			SquadID:     squadID,
			FromStatus:  req.StatusKey,
			ToStatus:    to,
		})
		if err != nil {
			return nil, nil, err
		}
		stopped, err := stopWakeupsOfMoved(ctx, qtx, rows)
		if err != nil {
			return nil, nil, err
		}
		cancelled = append(cancelled, stopped...)
		changed = append(changed, rows...)
	}
	return changed, cancelled, nil
}
