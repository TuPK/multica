package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Handoff between squads: the rule a squad hands its finished requests by, and the
// handoff of one request. The service decides; these handlers resolve the caller.

func (h *Handler) squadHandoffService() *service.SquadHandoffService {
	return &service.SquadHandoffService{Queries: h.Queries, TxStarter: h.TxStarter, Issues: h.IssueService}
}

// SquadHandoffRuleResponse is a squad's rule. A squad with no rule reports mode off and no target.
type SquadHandoffRuleResponse struct {
	SquadID         string  `json:"squad_id"`
	TargetSquadID   *string `json:"target_squad_id"`
	TargetSquadName *string `json:"target_squad_name"`
	Mode            string  `json:"mode"`
}

// SetSquadHandoffRuleRequest sets the rule. A missing target_squad_id removes the rule.
type SetSquadHandoffRuleRequest struct {
	TargetSquadID *string `json:"target_squad_id"`
	Mode          *string `json:"mode"`
}

// IssueHandoffResponse is what a request's handoff looks like: the rule that applies to
// its squad and the handoff row, when there is one.
type IssueHandoffResponse struct {
	Rule    *SquadHandoffRuleResponse `json:"rule"`
	Handoff *HandoffResponse          `json:"handoff"`
	// Status is none, off, pending, handed_off or declined: what a caller does next.
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// HandoffResponse is one handoff row with the squad and issue it reached.
type HandoffResponse struct {
	ID              string  `json:"id"`
	SourceIssueID   string  `json:"source_issue_id"`
	TargetIssueID   *string `json:"target_issue_id"`
	TargetSquadID   string  `json:"target_squad_id"`
	TargetSquadName *string `json:"target_squad_name"`
	Mode            string  `json:"mode"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"created_at"`
	DecidedAt       *string `json:"decided_at"`
}

func (h *Handler) handoffRuleResponse(r *http.Request, ws pgtype.UUID, squad pgtype.UUID, rule *db.SquadHandoffRule) SquadHandoffRuleResponse {
	out := SquadHandoffRuleResponse{SquadID: uuidToString(squad), Mode: service.HandoffModeOff}
	if rule == nil {
		return out
	}
	out.Mode = rule.Mode
	target := uuidToString(rule.TargetSquadID)
	out.TargetSquadID = &target
	if sq, err := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{ID: rule.TargetSquadID, WorkspaceID: ws}); err == nil {
		name := sq.Name
		out.TargetSquadName = &name
	}
	return out
}

func (h *Handler) handoffResponse(r *http.Request, ws pgtype.UUID, row *db.SquadHandoff) *HandoffResponse {
	if row == nil {
		return nil
	}
	out := &HandoffResponse{
		ID:            uuidToString(row.ID),
		SourceIssueID: uuidToString(row.SourceIssueID),
		TargetSquadID: uuidToString(row.TargetSquadID),
		Mode:          row.Mode,
		Status:        row.Status,
		CreatedAt:     row.CreatedAt.Time.UTC().Format(time.RFC3339),
	}
	if row.TargetIssueID.Valid {
		id := uuidToString(row.TargetIssueID)
		out.TargetIssueID = &id
	}
	if row.DecidedAt.Valid {
		at := row.DecidedAt.Time.UTC().Format(time.RFC3339)
		out.DecidedAt = &at
	}
	if sq, err := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{ID: row.TargetSquadID, WorkspaceID: ws}); err == nil {
		name := sq.Name
		out.TargetSquadName = &name
	}
	return out
}

// GetSquadHandoffRule returns a squad's handoff rule. Any member may read it.
func (h *Handler) GetSquadHandoffRule(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	squadID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "squad id")
	if !ok {
		return
	}
	if _, err := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{ID: squadID, WorkspaceID: wsUUID}); err != nil {
		writeError(w, http.StatusNotFound, "squad not found")
		return
	}
	rule, err := h.squadHandoffService().GetRule(r.Context(), wsUUID, squadID)
	if err != nil {
		slog.Warn("read squad handoff rule failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to read the handoff rule")
		return
	}
	writeJSON(w, http.StatusOK, h.handoffRuleResponse(r, wsUUID, squadID, rule))
}

// SetSquadHandoffRule saves a squad's handoff rule, or removes it when no target is sent.
// Owners and admins only: it decides who receives the squad's finished work.
func (h *Handler) SetSquadHandoffRule(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	actor, ok := h.governanceBossFor(w, r, workspaceID)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	squadID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "squad id")
	if !ok {
		return
	}
	var req SetSquadHandoffRuleRequest
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	svc := h.squadHandoffService()
	if req.TargetSquadID == nil || *req.TargetSquadID == "" {
		if err := svc.ClearRule(r.Context(), wsUUID, squadID, actor); err != nil {
			slog.Warn("clear squad handoff rule failed", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to remove the handoff rule")
			return
		}
		writeJSON(w, http.StatusOK, h.handoffRuleResponse(r, wsUUID, squadID, nil))
		return
	}
	if req.Mode == nil {
		writeError(w, http.StatusBadRequest, "mode is required: off, ask or auto")
		return
	}
	targetID, ok := parseUUIDOrBadRequest(w, *req.TargetSquadID, "target_squad_id")
	if !ok {
		return
	}
	rule, err := svc.SetRule(r.Context(), wsUUID, squadID, targetID, *req.Mode, actor)
	switch {
	case errors.Is(err, service.ErrHandoffModeInvalid), errors.Is(err, service.ErrHandoffTargetInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, service.ErrHandoffSquadNotFound):
		writeError(w, http.StatusNotFound, "squad not found")
		return
	case errors.Is(err, service.ErrGovernanceActorNotBoss):
		writeError(w, http.StatusForbidden, err.Error())
		return
	case err != nil:
		slog.Warn("set squad handoff rule failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to save the handoff rule")
		return
	}
	writeJSON(w, http.StatusOK, h.handoffRuleResponse(r, wsUUID, squadID, &rule))
}

// GetIssueHandoff returns the handoff state of a request: the rule that applies to the
// squad holding it, and its handoff when one was filed. Any member may read it.
func (h *Handler) GetIssueHandoff(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	svc := h.squadHandoffService()
	out := IssueHandoffResponse{Status: "none"}
	if issue.AssigneeType.Valid && issue.AssigneeType.String == "squad" && issue.AssigneeID.Valid {
		rule, err := svc.GetRule(r.Context(), issue.WorkspaceID, issue.AssigneeID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read the handoff rule")
			return
		}
		if rule != nil {
			resp := h.handoffRuleResponse(r, issue.WorkspaceID, issue.AssigneeID, rule)
			out.Rule = &resp
			out.Status = service.HandoffStatusOff
			if rule.Mode != service.HandoffModeOff {
				out.Status = "none"
			}
		}
	}
	row, err := svc.Get(r.Context(), issue.WorkspaceID, issue.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read the handoff")
		return
	}
	if row != nil {
		out.Handoff = h.handoffResponse(r, issue.WorkspaceID, row)
		out.Status = row.Status
	}
	writeJSON(w, http.StatusOK, out)
}

// RequestIssueHandoff hands a request to the squad its holder's rule names. The squad's
// lead asks for it when its work is done, and the rule decides what happens (off,
// ask, auto). An owner or admin hands it off now, as the boss's decision.
func (h *Handler) RequestIssueHandoff(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	workspaceID := uuidToString(issue.WorkspaceID)
	c, ok := h.initiativeCallerFor(w, r, workspaceID)
	if !ok {
		return
	}
	actor := service.HandoffActor{Type: c.actor.Type, ID: c.actor.ID}
	svc := h.squadHandoffService()
	var (
		res service.HandoffResult
		err error
	)
	switch {
	case c.actor.Type == service.GovernanceActorAgent:
		if !h.isLeaderOfIssueSquad(r, issue, c.actor.ID) {
			writeError(w, http.StatusForbidden, "only the leader of the squad holding this request can ask for its handoff")
			return
		}
		res, err = svc.Request(r.Context(), issue.WorkspaceID, issue, actor)
	case c.admin:
		res, err = svc.HandOff(r.Context(), issue.WorkspaceID, issue, actor)
	default:
		writeError(w, http.StatusForbidden, "only the owner or an admin can hand a request off")
		return
	}
	h.writeHandoffResult(w, r, issue, res, err)
}

// DeclineIssueHandoff closes a pending handoff without running it. Owners and admins only.
func (h *Handler) DeclineIssueHandoff(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	actor, ok := h.governanceBossFor(w, r, uuidToString(issue.WorkspaceID))
	if !ok {
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	res, err := h.squadHandoffService().Decline(r.Context(), issue.WorkspaceID, issue, service.HandoffActor{Type: actor.Type, ID: actor.ID}, req.Note)
	h.writeHandoffResult(w, r, issue, res, err)
}

// isLeaderOfIssueSquad reports whether the agent leads the squad holding the issue.
func (h *Handler) isLeaderOfIssueSquad(r *http.Request, issue db.Issue, agentID pgtype.UUID) bool {
	if !issue.AssigneeType.Valid || issue.AssigneeType.String != "squad" || !issue.AssigneeID.Valid {
		return false
	}
	squad, err := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{ID: issue.AssigneeID, WorkspaceID: issue.WorkspaceID})
	return err == nil && util.UUIDToString(squad.LeaderID) == util.UUIDToString(agentID)
}

func (h *Handler) writeHandoffResult(w http.ResponseWriter, r *http.Request, issue db.Issue, res service.HandoffResult, err error) {
	switch {
	case errors.Is(err, service.ErrHandoffNotAssigned), errors.Is(err, service.ErrHandoffNoRule),
		errors.Is(err, service.ErrHandoffChained), errors.Is(err, service.ErrHandoffNothingPending):
		writeErrorCode(w, http.StatusConflict, "handoff_not_applicable", err.Error())
		return
	case errors.Is(err, service.ErrHandoffPacketMissing):
		writeErrorCode(w, http.StatusUnprocessableEntity, "handoff_packet_missing", err.Error())
		return
	case errors.Is(err, service.ErrHandoffNoProject), errors.Is(err, service.ErrHandoffTargetInvalid):
		writeErrorCode(w, http.StatusUnprocessableEntity, "handoff_target_unavailable", err.Error())
		return
	case errors.Is(err, service.ErrGovernanceActorNotBoss):
		writeError(w, http.StatusForbidden, err.Error())
		return
	case err != nil:
		slog.Warn("squad handoff failed", append(logger.RequestAttrs(r), "error", err, "issue_id", uuidToString(issue.ID))...)
		writeError(w, http.StatusInternalServerError, "failed to hand the request off")
		return
	}
	out := IssueHandoffResponse{Status: res.Status, Reason: res.Reason, Handoff: h.handoffResponse(r, issue.WorkspaceID, res.Handoff)}
	if rule, rerr := h.squadHandoffService().GetRule(r.Context(), issue.WorkspaceID, issue.AssigneeID); rerr == nil && rule != nil {
		resp := h.handoffRuleResponse(r, issue.WorkspaceID, issue.AssigneeID, rule)
		out.Rule = &resp
	}
	writeJSON(w, http.StatusOK, out)
}
