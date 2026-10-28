package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Automatic run queue. A backlog issue carrying a queue_order moves to todo on
// its own once every lower-numbered issue in its scope (project, or no project)
// is finished; issues sharing a number start together. The release rule is the
// ListReleasableQueuedIssues query: it holds no state of its own, so the event
// listener and the periodic sweep can both call PromoteQueuedIssues freely.

const (
	maxQueueOrder         = 1_000_000
	queuePromoteBatchSize = 200
)

type setIssueQueueOrderRequest struct {
	// QueueOrder is the issue's number in the queue; null takes it out.
	QueueOrder *int32 `json:"queue_order"`
}

// SetIssueQueueOrder arms or clears an issue's place in the run queue, then
// promotes whatever the change makes ready.
func (h *Handler) SetIssueQueueOrder(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var req setIssueQueueOrderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.QueueOrder != nil && (*req.QueueOrder < 1 || *req.QueueOrder > maxQueueOrder) {
		writeError(w, http.StatusBadRequest, "queue_order must be a whole number from 1 to 1000000")
		return
	}
	order := pgtype.Int4{}
	if req.QueueOrder != nil {
		order = pgtype.Int4{Int32: *req.QueueOrder, Valid: true}
	}
	updated, err := h.Queries.SetIssueQueueOrder(r.Context(), db.SetIssueQueueOrderParams{
		ID:          issue.ID,
		WorkspaceID: issue.WorkspaceID,
		QueueOrder:  order,
	})
	if err != nil {
		slog.Warn("set issue queue order failed", "error", err, "issue_id", uuidToString(issue.ID))
		writeError(w, http.StatusInternalServerError, "failed to set queue order")
		return
	}

	workspaceID := uuidToString(updated.WorkspaceID)
	userID := requestUserID(r)
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	resp := issueToResponse(updated, h.getIssuePrefix(r.Context(), updated.WorkspaceID))
	h.fillStatusCategory(r.Context(), updated.WorkspaceID, &resp)
	h.publish(protocol.EventIssueUpdated, workspaceID, actorType, actorID, map[string]any{"issue": resp})

	if _, err := h.PromoteQueuedIssues(r.Context(), updated.WorkspaceID, updated.ProjectID, false); err != nil {
		slog.Warn("promote queued issues failed", "error", err, "workspace_id", workspaceID)
	}
	// Promotion may have moved this very issue; answer with what is stored now.
	if current, err := h.Queries.GetIssueInWorkspace(r.Context(), db.GetIssueInWorkspaceParams{ID: updated.ID, WorkspaceID: updated.WorkspaceID}); err == nil {
		resp = issueToResponse(current, h.getIssuePrefix(r.Context(), current.WorkspaceID))
		h.fillStatusCategory(r.Context(), current.WorkspaceID, &resp)
	}
	writeJSON(w, http.StatusOK, resp)
}

// PromoteQueuedIssues moves every backlog issue whose turn has come to todo,
// through the same workflow handoff and run start as a person dragging it
// there. anyScope reads all workspaces; otherwise it reads one workspace and
// project (an invalid projectID is the no-project scope). It returns how many
// issues moved.
func (h *Handler) PromoteQueuedIssues(ctx context.Context, workspaceID, projectID pgtype.UUID, anyScope bool) (int, error) {
	ready, err := h.Queries.ListReleasableQueuedIssues(ctx, db.ListReleasableQueuedIssuesParams{
		AnyScope:    anyScope,
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		PageLimit:   queuePromoteBatchSize,
	})
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, issue := range ready {
		if h.promoteQueuedIssue(ctx, issue) {
			moved++
		}
	}
	return moved, nil
}

// promoteQueuedIssue moves one backlog issue to todo. It reports false when the
// issue changed under it or its project workflow refuses todo; the next pass
// looks at it again.
func (h *Handler) promoteQueuedIssue(ctx context.Context, prev db.Issue) bool {
	const target = "todo"
	params := db.UpdateIssueParams{
		ID:               prev.ID,
		AssigneeType:     prev.AssigneeType,
		AssigneeID:       prev.AssigneeID,
		StartDate:        prev.StartDate,
		DueDate:          prev.DueDate,
		ParentIssueID:    prev.ParentIssueID,
		ProjectID:        prev.ProjectID,
		Stage:            prev.Stage,
		Status:           pgtype.Text{String: target, Valid: true},
		ExpectedRevision: pgtype.Int8{Int64: prev.Revision, Valid: true},
	}
	handoff, err := h.applyIssueWorkflow(ctx, prev, &params, true, false, false, nil)
	if err != nil {
		slog.Warn("queue promote: workflow refused", "error", err, "issue_id", uuidToString(prev.ID))
		return false
	}

	writeCtx := withWakeupHandedTo(ctx, h.handedToAgent(ctx, prev, params, handoff != nil))
	issue, err := h.updateIssueWithStatusGuard(writeCtx, prev.WorkspaceID, target, params)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("queue promote: write failed", "error", err, "issue_id", uuidToString(prev.ID))
		}
		return false
	}

	workspaceID := uuidToString(issue.WorkspaceID)
	resp := issueToResponse(issue, h.getIssuePrefix(ctx, issue.WorkspaceID))
	h.fillStatusCategory(ctx, issue.WorkspaceID, &resp)
	assigneeChanged := handoff != nil &&
		(prev.AssigneeType.String != issue.AssigneeType.String || uuidToString(prev.AssigneeID) != uuidToString(issue.AssigneeID))
	trigger, willRun := h.IssueService.WillEnqueueRun(ctx, service.IssueTriggerInput{
		Issue:              issue,
		PrevStatus:         prev.Status,
		AssigneeChanged:    assigneeChanged || handoff != nil,
		StatusChanged:      true,
		EnteredHandoffStep: handoff != nil,
	}, service.IssueTriggerProbe{})

	payload := map[string]any{
		"issue":              resp,
		"assignee_changed":   assigneeChanged,
		"status_changed":     true,
		"prev_assignee_type": textToPtr(prev.AssigneeType),
		"prev_assignee_id":   uuidToPtr(prev.AssigneeID),
		"prev_status":        prev.Status,
		"prev_priority":      prev.Priority,
		"creator_type":       prev.CreatorType,
		"creator_id":         uuidToString(prev.CreatorID),
		"source":             "issue_queue",
	}
	if handoff != nil {
		payload["workflow_handoff"] = workflowHandoffPayload(issue, handoff, h.workflowHandoffBrief(ctx, issue, handoff), willRun)
	}
	h.publish(protocol.EventIssueUpdated, workspaceID, "system", "", payload)
	if willRun {
		h.dispatchIssueRun(ctx, issue, trigger, "system", "", "")
	}
	if issue.ParentIssueID.Valid {
		h.processChildEvents(ctx, issue.ParentIssueID)
	}
	return true
}

const maxQueueBatch = 200

type batchQueueOrderRequest struct {
	IssueIDs   []string `json:"issue_ids"`
	QueueOrder *int32   `json:"queue_order"`
}

func queueOrderValue(w http.ResponseWriter, order *int32) (pgtype.Int4, bool) {
	if order == nil {
		return pgtype.Int4{}, true
	}
	if *order < 1 || *order > maxQueueOrder {
		writeError(w, http.StatusBadRequest, "queue_order must be a whole number from 1 to 1000000")
		return pgtype.Int4{}, false
	}
	return pgtype.Int4{Int32: *order, Valid: true}, true
}

func (h *Handler) decodeBatchQueueOrder(w http.ResponseWriter, r *http.Request) (batchQueueOrderRequest, []pgtype.UUID, pgtype.UUID, bool) {
	var req batchQueueOrderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, nil, pgtype.UUID{}, false
	}
	if len(req.IssueIDs) == 0 || len(req.IssueIDs) > maxQueueBatch {
		writeError(w, http.StatusBadRequest, "issue_ids must hold 1 to 200 ids")
		return req, nil, pgtype.UUID{}, false
	}
	ids, ok := parseUUIDSliceOrBadRequest(w, req.IssueIDs, "issue_ids")
	if !ok {
		return req, nil, pgtype.UUID{}, false
	}
	workspaceID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return req, nil, pgtype.UUID{}, false
	}
	return req, ids, workspaceID, true
}

// BatchSetIssueQueueOrder numbers several issues at once, then promotes
// whatever the change makes ready.
func (h *Handler) BatchSetIssueQueueOrder(w http.ResponseWriter, r *http.Request) {
	req, ids, workspaceID, ok := h.decodeBatchQueueOrder(w, r)
	if !ok {
		return
	}
	order, ok := queueOrderValue(w, req.QueueOrder)
	if !ok {
		return
	}
	updated, err := h.Queries.SetIssuesQueueOrder(r.Context(), db.SetIssuesQueueOrderParams{
		WorkspaceID: workspaceID,
		Ids:         ids,
		QueueOrder:  order,
	})
	if err != nil {
		slog.Warn("batch set queue order failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to set queue order")
		return
	}
	wsString := uuidToString(workspaceID)
	actorType, actorID := h.resolveActor(r, requestUserID(r), wsString)
	prefix := h.getIssuePrefix(r.Context(), workspaceID)
	scopes := map[pgtype.UUID]bool{}
	for _, issue := range updated {
		resp := issueToResponse(issue, prefix)
		h.fillStatusCategory(r.Context(), workspaceID, &resp)
		h.publish(protocol.EventIssueUpdated, wsString, actorType, actorID, map[string]any{"issue": resp})
		scopes[issue.ProjectID] = true
	}
	for project := range scopes {
		if _, err := h.PromoteQueuedIssues(r.Context(), workspaceID, project, false); err != nil {
			slog.Warn("promote queued issues failed", "error", err, "workspace_id", wsString)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": len(updated)})
}

type queueOrderPreviewResponse struct {
	// ReleaseNow counts the backlog issues among the request that would move to
	// todo at once: no unfinished issue below the number in their project.
	ReleaseNow int `json:"release_now"`
	// RunCount counts those that would also start an agent run.
	RunCount int `json:"run_count"`
}

// PreviewQueueOrder answers "if I number these issues, how many start now".
func (h *Handler) PreviewQueueOrder(w http.ResponseWriter, r *http.Request) {
	req, ids, workspaceID, ok := h.decodeBatchQueueOrder(w, r)
	if !ok {
		return
	}
	order, ok := queueOrderValue(w, req.QueueOrder)
	if !ok {
		return
	}
	out := queueOrderPreviewResponse{}
	if !order.Valid {
		writeJSON(w, http.StatusOK, out)
		return
	}
	issues, err := h.Queries.ListIssuesByIDsInWorkspace(r.Context(), db.ListIssuesByIDsInWorkspaceParams{WorkspaceID: workspaceID, Ids: ids})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to preview queue order")
		return
	}
	blocked := map[pgtype.UUID]bool{}
	for _, issue := range issues {
		if issue.Status != "backlog" || issue.TriageState.Valid {
			continue
		}
		isBlocked, seen := blocked[issue.ProjectID]
		if !seen {
			n, err := h.Queries.CountQueueBlockers(r.Context(), db.CountQueueBlockersParams{
				WorkspaceID: workspaceID,
				ProjectID:   issue.ProjectID,
				QueueOrder:  order.Int32,
				ExcludeIds:  ids,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to preview queue order")
				return
			}
			isBlocked = n > 0
			blocked[issue.ProjectID] = isBlocked
		}
		if isBlocked {
			continue
		}
		out.ReleaseNow++
		next := issue
		next.Status = "todo"
		if _, willRun := h.IssueService.WillEnqueueRun(r.Context(), service.IssueTriggerInput{
			Issue: next, PrevStatus: issue.Status, StatusChanged: true,
		}, service.IssueTriggerProbe{}); willRun {
			out.RunCount++
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type queueFrontierResponse struct {
	ProjectID *string `json:"project_id"`
	Frontier  int32   `json:"frontier"`
}

// ListQueueFrontiers gives, per project, the lowest queue number still
// unfinished. A backlog issue numbered above it is waiting on that number.
func (h *Handler) ListQueueFrontiers(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return
	}
	rows, err := h.Queries.ListQueueFrontiers(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list queue frontiers")
		return
	}
	out := make([]queueFrontierResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, queueFrontierResponse{ProjectID: uuidToPtr(row.ProjectID), Frontier: row.Frontier})
	}
	writeJSON(w, http.StatusOK, out)
}
