package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// AssignSquadProjectRequest names the project a squad takes.
type AssignSquadProjectRequest struct {
	ProjectID string `json:"project_id"`
}

// squadsWithSquad returns the project's squads with squadID added once. The
// project row is locked by the caller, so the list cannot change under it.
func squadsWithSquad(current []pgtype.UUID, squadID pgtype.UUID) []pgtype.UUID {
	if slices.Contains(current, squadID) {
		return current
	}
	return append(slices.Clone(current), squadID)
}

// AssignSquadProject adds a project to a squad, which shows it in its room.
// Other squads may work the same project, and assigning never refuses because
// of workflows: the project keeps its own workflow, and each squad works its
// issues through its active workflow. A squad that has no active workflow yet
// takes the SDLC Auto or planning workflow the squad's agents hand to. Its
// issues move to the statuses that workflow suggests, in the same transaction.
// A squad works one open project at a time, so one already assigned must be
// unassigned first.
func (h *Handler) AssignSquadProject(w http.ResponseWriter, r *http.Request) {
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
	var req AssignSquadProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	projectUUID, ok := parseUUIDOrBadRequest(w, req.ProjectID, "project_id")
	if !ok {
		return
	}
	wsUUID := squad.WorkspaceID
	ctx := r.Context()

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to assign project")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if err := qtx.LockIssueStatusCatalogShared(ctx, wsUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to assign project")
		return
	}
	project, err := qtx.LockProjectForWorkflowChange(ctx, db.LockProjectForWorkflowChangeParams{ID: projectUUID, WorkspaceID: wsUUID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to assign project")
		return
	}

	if _, err := qtx.GetOpenSquadProject(ctx, db.GetOpenSquadProjectParams{WorkspaceID: wsUUID, SquadID: squad.ID, ID: project.ID}); err == nil {
		writeErrorCode(w, http.StatusConflict, "squad_has_project", "this squad already has a project; unassign it first")
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to assign project")
		return
	}

	var changed []db.Issue
	var cancelled []db.AgentTaskQueue
	var plan workflowMappingPlan
	activeSquad, err := qtx.LockSquadForWorkflowChange(ctx, db.LockSquadForWorkflowChangeParams{ID: squad.ID, WorkspaceID: wsUUID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to assign project")
		return
	}
	if !activeSquad.ActiveWorkflowID.Valid {
		wf, err := h.initiativeService().EnsureSquadProjectWorkflow(ctx, qtx, wsUUID, squad.ID)
		if err != nil {
			if errors.Is(err, service.ErrBuildNeedsSDLCSquad) {
				writeErrorCode(w, http.StatusUnprocessableEntity, "build_squad_not_sdlc", err.Error())
				return
			}
			if errors.Is(err, service.ErrPlanningWorkflowMissing) {
				writeErrorCode(w, http.StatusUnprocessableEntity, "planning_workflow_missing", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to set up the squad's workflow")
			return
		}
		target, err := issueworkflow.Decode(wf)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read workflow")
			return
		}
		plan, err = h.squadWorkflowPlan(ctx, qtx, wsUUID, squad.ID, target)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to assign project")
			return
		}
		// No one picks destinations on the first activation, so every status the
		// workflow lacks moves to the status the plan suggests.
		changed, cancelled, err = remapSquadIssues(ctx, qtx, wsUUID, squad.ID, plan, suggestedMapping(plan))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to move issues to the new statuses")
			return
		}
		adopted, err := adoptSquadWorkflow(ctx, qtx, wsUUID, squad.ID, wf)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to assign project")
			return
		}
		if !adopted {
			writeErrorCode(w, http.StatusConflict, "workflow_belongs_to_other_squad", "the squad's workflow belongs to another squad")
			return
		}
	}

	unassigned, err := qtx.ClearSquadFromOtherProjects(ctx, db.ClearSquadFromOtherProjectsParams{WorkspaceID: wsUUID, SquadID: squad.ID, ID: project.ID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to assign project")
		return
	}
	assigned, err := qtx.SetProjectSquadIDs(ctx, db.SetProjectSquadIDsParams{
		ID: project.ID, WorkspaceID: wsUUID, SquadIds: squadsWithSquad(project.SquadIds, squad.ID),
	})
	if err != nil {
		slog.Warn("AssignSquadProject set squad failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to assign project")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to assign project")
		return
	}

	actorID := uuidToString(member.UserID)
	h.publishIssueSnapshots(ctx, changed, "member", actorID)
	h.broadcastCancelledWakeups(ctx, wsUUID, cancelled)
	for _, p := range unassigned {
		h.publishProjectWorkflowUpdate(ctx, workspaceID, member, p)
	}
	resp := h.publishProjectWorkflowUpdate(ctx, workspaceID, member, assigned)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":        resp,
		"updated_issues": len(changed),
		"plan":           plan,
	})
}

// UnassignSquadProject removes the squad from every project it works, so it can
// be given another one. The projects keep their workflow, issues and other squads.
func (h *Handler) UnassignSquadProject(w http.ResponseWriter, r *http.Request) {
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
	unassigned, err := h.Queries.ClearSquadProjects(r.Context(), db.ClearSquadProjectsParams{WorkspaceID: squad.WorkspaceID, SquadID: squad.ID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unassign project")
		return
	}
	for _, p := range unassigned {
		h.publishProjectWorkflowUpdate(r.Context(), workspaceID, member, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"unassigned": len(unassigned)})
}
