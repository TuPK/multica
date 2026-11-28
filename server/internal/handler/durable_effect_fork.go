package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// EffectResponse is one effect event as the API shows it (durable spec 4.4). The
// parameters of the call are never included.
type EffectResponse struct {
	ID            string     `json:"id"`
	CreatedAt     time.Time  `json:"created_at"`
	Source        string     `json:"source"`
	Target        string     `json:"target"`
	EffectClass   string     `json:"effect_class"`
	Reversibility string     `json:"reversibility"`
	Decision      string     `json:"decision"`
	Status        string     `json:"status"`
	Summary       string     `json:"summary"`
	TaskID        *string    `json:"task_id"`
	AgentID       *string    `json:"agent_id"`
	MemberID      *string    `json:"member_id"`
	UndoAvailable bool       `json:"undo_available"`
	UndoUntil     *time.Time `json:"undo_until"`
}

// DeclareEffectResponse answers a declared action: the event, the policy's decision,
// and the intent when the decision is to rehearse (durable spec 4.5).
type DeclareEffectResponse struct {
	EventID  string  `json:"event_id"`
	Decision string  `json:"decision"`
	Status   string  `json:"status"`
	IntentID *string `json:"intent_id"`
}

// effectUndoOpen reports whether an event can be undone now: it is executed, it has
// an undo, and its window has not closed (durable spec 4.6).
func effectUndoOpen(ev db.EffectEvent, now time.Time) bool {
	return ev.Status == "executed" && ev.Undo != nil && ev.UndoUntil.Valid && now.Before(ev.UndoUntil.Time)
}

func effectResponseOf(ev db.EffectEvent, now time.Time) EffectResponse {
	out := EffectResponse{
		ID:            util.UUIDToString(ev.ID),
		CreatedAt:     ev.CreatedAt.Time,
		Source:        ev.Source,
		Target:        ev.Target,
		EffectClass:   ev.EffectClass,
		Reversibility: ev.Reversibility,
		Decision:      ev.Decision,
		Status:        ev.Status,
		Summary:       ev.Summary,
		UndoAvailable: effectUndoOpen(ev, now),
	}
	if ev.TaskID.Valid {
		s := util.UUIDToString(ev.TaskID)
		out.TaskID = &s
	}
	if ev.AgentID.Valid {
		s := util.UUIDToString(ev.AgentID)
		out.AgentID = &s
	}
	if ev.MemberID.Valid {
		s := util.UUIDToString(ev.MemberID)
		out.MemberID = &s
	}
	if ev.UndoUntil.Valid {
		t := ev.UndoUntil.Time
		out.UndoUntil = &t
	}
	return out
}

// DeclareEffect records an action an agent is about to take outside the platform
// (POST /api/effects/declare). The platform cannot stop it; it answers with the
// policy's decision, which the CLI turns into an exit code (durable spec 4.5).
func (h *Handler) DeclareEffect(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	if h.durable.Effects == nil {
		writeError(w, http.StatusServiceUnavailable, "the effect log is not configured")
		return
	}
	var req struct {
		Class         string `json:"class"`
		Reversibility string `json:"reversibility"`
		Target        string `json:"target"`
		Summary       string `json:"summary"`
		Rationale     string `json:"rationale"`
	}
	if !readCappedJSON(w, r, durableRequestBodyLimit, &req, "invalid request body") {
		return
	}
	if !validDeclaredClass(req.Class) {
		writeError(w, http.StatusUnprocessableEntity, "class must be one of read, write_internal, write, send, share, delete, spend")
		return
	}
	rev := req.Reversibility
	if rev == "" {
		rev = connectedapps.DefaultReversibility(req.Class)
	}
	if req.Class != connectedapps.EffectRead && !validDeclaredReversibility(rev) {
		writeError(w, http.StatusUnprocessableEntity, "reversibility must be reversible, compensable or irreversible")
		return
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		writeError(w, http.StatusUnprocessableEntity, "target is required")
		return
	}

	in := connectedapps.EffectRequest{
		WS:            wsUUID,
		MemberID:      member.UserID,
		Source:        connectedapps.SourceDeclared,
		Target:        target,
		Class:         req.Class,
		Reversibility: rev,
		Summary:       strings.TrimSpace(req.Summary),
		Prior:         connectedapps.PolicyAllow,
	}
	if task, ok := h.taskFromRequestHeader(r); ok {
		in.TaskID, in.AgentID = task.ID, task.AgentID
	}
	ticket, err := h.durable.Effects.Gate(r.Context(), in)
	if err != nil {
		slog.Error("declare effect: gate", "error", err)
		writeError(w, http.StatusInternalServerError, "could not record the declared action")
		return
	}
	// A rehearsed declaration becomes an intent the person decides on. The platform
	// does not run it: the decision goes back to the agent (durable spec 5.2).
	var intentID *string
	if ticket.Decision == connectedapps.PolicyRehearse && h.durable.Intents != nil {
		id, err := h.durable.Intents.Rehearse(r.Context(), ticket.EventID, in, connectedapps.CanonicalCall{}, req.Rationale)
		if err != nil {
			slog.Error("declare effect: rehearse", "error", err)
			writeError(w, http.StatusInternalServerError, "could not hold the declared action for approval")
			return
		}
		s := util.UUIDToString(id)
		intentID = &s
	}
	writeJSON(w, http.StatusOK, DeclareEffectResponse{
		EventID:  util.UUIDToString(ticket.EventID),
		Decision: ticket.Decision,
		Status:   "declared",
		IntentID: intentID,
	})
}

// ListEffects returns the workspace's effect events, newest first (GET /api/effects).
// Filters: effect_class, source, decision, agent_id, task_id, before (RFC 3339, the
// created_at of the last row of the previous page), limit (1 to 200, default 50).
func (h *Handler) ListEffects(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	q := r.URL.Query()
	params := db.ListEffectEventsParams{WorkspaceID: wsUUID, RowLimit: 50}
	if v := q.Get("effect_class"); v != "" {
		params.EffectClass = pgtype.Text{String: v, Valid: true}
	}
	if v := q.Get("source"); v != "" {
		params.Source = pgtype.Text{String: v, Valid: true}
	}
	if v := q.Get("decision"); v != "" {
		params.Decision = pgtype.Text{String: v, Valid: true}
	}
	if v := q.Get("agent_id"); v != "" {
		id, ok := parseUUIDOrBadRequest(w, v, "agent_id")
		if !ok {
			return
		}
		params.AgentID = id
	}
	if v := q.Get("task_id"); v != "" {
		id, ok := parseUUIDOrBadRequest(w, v, "task_id")
		if !ok {
			return
		}
		params.TaskID = id
	}
	if v := q.Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "before must be an RFC 3339 time")
			return
		}
		params.Before = pgtype.Timestamptz{Time: t, Valid: true}
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		params.RowLimit = int32(n)
	}
	rows, err := h.Queries.ListEffectEvents(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list effects")
		return
	}
	now := time.Now()
	out := make([]EffectResponse, 0, len(rows))
	for _, ev := range rows {
		out = append(out, effectResponseOf(ev, now))
	}
	writeJSON(w, http.StatusOK, map[string]any{"effects": out})
}

// UndoEffect takes back an executed effect (POST /api/effects/{id}/undo, durable spec
// 4.6). The owner, an admin, or the member who acted may undo, and only inside the
// undo window. The undo runs through the gateway as that member, and records its own
// event.
func (h *Handler) UndoEffect(w http.ResponseWriter, r *http.Request) {
	if isMachineCredentialActor(r) {
		writeError(w, http.StatusForbidden, "this endpoint is only available to human actors")
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return
	}
	if h.durable.Effects == nil {
		writeError(w, http.StatusServiceUnavailable, "undo is not configured")
		return
	}
	if !h.durableEffectsEnabled(r.Context()) {
		writeError(w, http.StatusServiceUnavailable, "durable effects is off")
		return
	}
	ev, err := h.Queries.GetEffectEvent(r.Context(), db.GetEffectEventParams{ID: id, WorkspaceID: wsUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "effect not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load effect")
		return
	}
	if h.effectHiddenFrom(r, workspaceID, ev) {
		writeError(w, http.StatusNotFound, "effect not found")
		return
	}
	if member.Role != "owner" && member.Role != "admin" && member.UserID != ev.MemberID {
		writeError(w, http.StatusForbidden, "only the owner, an admin, or the member who acted can undo this")
		return
	}
	if ev.Status != "executed" {
		writeErrorCode(w, http.StatusConflict, "effect_not_executed", "only an executed effect can be undone")
		return
	}
	if ev.Undo == nil {
		writeErrorCode(w, http.StatusUnprocessableEntity, "not_undoable", "không hoàn tác được")
		return
	}
	if !effectUndoOpen(ev, time.Now()) {
		writeErrorCode(w, http.StatusConflict, "undo_window_closed", "the undo window for this effect has closed")
		return
	}
	if ev.EffectClass == connectedapps.EffectWriteInternal {
		h.undoInternalEffect(w, r, wsUUID, id, member, ev)
		return
	}
	if h.ConnectedApps == nil {
		writeError(w, http.StatusServiceUnavailable, "undo is not configured")
		return
	}
	var spec connectedapps.UndoSpec
	if err := json.Unmarshal(ev.Undo, &spec); err != nil || spec.Tool == "" {
		writeErrorCode(w, http.StatusUnprocessableEntity, "not_undoable", "không hoàn tác được")
		return
	}
	claimed, err := h.Queries.ClaimEffectEventUndo(r.Context(), db.ClaimEffectEventUndoParams{ID: id, WorkspaceID: wsUUID})
	if err != nil {
		slog.Error("undo effect: claim", "error", err, "effect_id", uuidToString(id))
		writeError(w, http.StatusInternalServerError, "could not claim effect for undo")
		return
	}
	if claimed == 0 {
		writeErrorCode(w, http.StatusConflict, "effect_not_executed", "only an executed effect can be undone")
		return
	}
	if _, err := h.ConnectedApps.ExecuteUndo(r.Context(), wsUUID, ev.MemberID, ev.Target, spec); err != nil {
		if _, revertErr := h.Queries.RevertEffectEventUndo(r.Context(), db.RevertEffectEventUndoParams{ID: id, WorkspaceID: wsUUID}); revertErr != nil {
			slog.Error("undo effect: revert claim", "error", revertErr, "effect_id", uuidToString(id))
		}
		slog.Error("undo effect", "error", err, "effect_id", uuidToString(id))
		writeError(w, http.StatusBadGateway, "the undo call failed; the effect is unchanged")
		return
	}
	updated, err := h.Queries.MarkEffectEventUndone(r.Context(), db.MarkEffectEventUndoneParams{ID: id, WorkspaceID: wsUUID})
	if err != nil || updated == 0 {
		slog.Error("undo effect: mark undone", "error", err, "updated", updated, "effect_id", uuidToString(id))
		writeError(w, http.StatusInternalServerError, "the undo ran but the effect could not be marked undone")
		return
	}
	undoEventID := ""
	if undoID, err := h.Queries.InsertEffectEvent(r.Context(), db.InsertEffectEventParams{
		WorkspaceID:   wsUUID,
		MemberID:      member.UserID,
		Source:        connectedapps.SourceConnectedApp,
		Target:        ev.Target,
		EffectClass:   ev.EffectClass,
		Reversibility: ev.Reversibility,
		Decision:      connectedapps.PolicyAllow,
		Status:        "executed",
		Summary:       truncateEffectSummary("Hoàn tác " + uuidToString(id)),
	}); err != nil {
		slog.Error("undo effect: record undo event", "error", err, "effect_id", uuidToString(id))
	} else {
		undoEventID = uuidToString(undoID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"effect_id":     uuidToString(id),
		"status":        "undone",
		"undo_event_id": undoEventID,
	})
}

// Supported v1 internal revision undo kinds (durable spec 4.6). Any other kind answers 422
// "không hoàn tác được".
const (
	InternalUndoIssueDescription = "issue_description"
	InternalUndoAgentInstruction = "agent_instruction"
	InternalUndoKnowledgeDraft   = "knowledge_draft"
)

// internalUndoSpec is stored in effect_event.undo for reversible write_internal actions.
type internalUndoSpec struct {
	Kind     string `json:"kind"`
	TargetID string `json:"target_id"`
	Revision int64  `json:"revision,omitempty"`
}

var errInternalNotUndoable = errors.New("không hoàn tác được")

func (h *Handler) undoInternalEffect(w http.ResponseWriter, r *http.Request, wsUUID, id pgtype.UUID, member db.Member, ev db.EffectEvent) {
	var spec internalUndoSpec
	if err := json.Unmarshal(ev.Undo, &spec); err != nil {
		writeErrorCode(w, http.StatusUnprocessableEntity, "not_undoable", "không hoàn tác được")
		return
	}
	switch spec.Kind {
	case InternalUndoIssueDescription, InternalUndoAgentInstruction, InternalUndoKnowledgeDraft:
	default:
		writeErrorCode(w, http.StatusUnprocessableEntity, "not_undoable", "không hoàn tác được")
		return
	}
	targetID, err := util.ParseUUID(spec.TargetID)
	if err != nil || !targetID.Valid {
		writeErrorCode(w, http.StatusUnprocessableEntity, "not_undoable", "không hoàn tác được")
		return
	}
	// Pre-check that a knowledge_draft undo targets a draft before claiming the event.
	if spec.Kind == InternalUndoKnowledgeDraft {
		link, err := h.Queries.GetKnowledgeLink(r.Context(), db.GetKnowledgeLinkParams{ID: targetID, WorkspaceID: wsUUID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && link.State != service.KnowledgeLinkDraft) {
			writeErrorCode(w, http.StatusUnprocessableEntity, "not_undoable", "không hoàn tác được")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load knowledge draft")
			return
		}
	}

	claimed, err := h.Queries.ClaimEffectEventUndo(r.Context(), db.ClaimEffectEventUndoParams{ID: id, WorkspaceID: wsUUID})
	if err != nil {
		slog.Error("undo internal effect: claim", "error", err, "effect_id", uuidToString(id))
		writeError(w, http.StatusInternalServerError, "could not claim effect for undo")
		return
	}
	if claimed == 0 {
		writeErrorCode(w, http.StatusConflict, "effect_not_executed", "only an executed effect can be undone")
		return
	}

	undoErr := h.applyInternalUndo(r.Context(), wsUUID, member, spec, targetID)
	if undoErr != nil {
		if _, revertErr := h.Queries.RevertEffectEventUndo(r.Context(), db.RevertEffectEventUndoParams{ID: id, WorkspaceID: wsUUID}); revertErr != nil {
			slog.Error("undo internal effect: revert claim", "error", revertErr, "effect_id", uuidToString(id))
		}
		if errors.Is(undoErr, errInternalNotUndoable) {
			writeErrorCode(w, http.StatusUnprocessableEntity, "not_undoable", "không hoàn tác được")
			return
		}
		slog.Error("undo internal effect", "error", undoErr, "effect_id", uuidToString(id), "kind", spec.Kind)
		writeError(w, http.StatusInternalServerError, "the undo failed; the effect is unchanged")
		return
	}

	updated, err := h.Queries.MarkEffectEventUndone(r.Context(), db.MarkEffectEventUndoneParams{ID: id, WorkspaceID: wsUUID})
	if err != nil || updated == 0 {
		slog.Error("undo internal effect: mark undone", "error", err, "updated", updated, "effect_id", uuidToString(id))
		writeError(w, http.StatusInternalServerError, "the undo ran but the effect could not be marked undone")
		return
	}
	undoEventID := ""
	if undoID, err := h.Queries.InsertEffectEvent(r.Context(), db.InsertEffectEventParams{
		WorkspaceID:   wsUUID,
		MemberID:      member.UserID,
		Source:        ev.Source,
		Target:        ev.Target,
		EffectClass:   ev.EffectClass,
		Reversibility: ev.Reversibility,
		Decision:      connectedapps.PolicyAllow,
		Status:        "executed",
		Summary:       truncateEffectSummary("Hoàn tác " + uuidToString(id)),
	}); err != nil {
		slog.Error("undo internal effect: record undo event", "error", err, "effect_id", uuidToString(id))
	} else {
		undoEventID = uuidToString(undoID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"effect_id":     uuidToString(id),
		"status":        "undone",
		"undo_event_id": undoEventID,
	})
}

func (h *Handler) applyInternalUndo(ctx context.Context, wsUUID pgtype.UUID, member db.Member, spec internalUndoSpec, targetID pgtype.UUID) error {
	switch spec.Kind {
	case InternalUndoIssueDescription:
		issue, err := h.Queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{ID: targetID, WorkspaceID: wsUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errInternalNotUndoable
		}
		if err != nil {
			return err
		}
		content, err := h.Queries.GetIssueDescriptionRevisionContent(ctx, db.GetIssueDescriptionRevisionContentParams{
			IssueID: targetID, Revision: spec.Revision,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errInternalNotUndoable
		}
		if err != nil {
			return err
		}
		_, err = h.Queries.UpdateIssue(ctx, db.UpdateIssueParams{
			ID:                 issue.ID,
			Description:        pgtype.Text{String: content, Valid: true},
			AssigneeType:       issue.AssigneeType,
			AssigneeID:         issue.AssigneeID,
			Position:           pgtype.Float8{Float64: issue.Position, Valid: true},
			StartDate:          issue.StartDate,
			DueDate:            issue.DueDate,
			ParentIssueID:      issue.ParentIssueID,
			ProjectID:          issue.ProjectID,
			Stage:              issue.Stage,
			DuplicateOfIssueID: issue.DuplicateOfIssueID,
		})
		return err

	case InternalUndoAgentInstruction:
		if spec.Revision <= 0 {
			return errInternalNotUndoable
		}
		_, _, err := service.RestoreAgentInstructionRevision(ctx, h.Queries, wsUUID, targetID, int32(spec.Revision), service.GovernanceActor{
			Type: service.GovernanceActorMember, ID: member.UserID,
		})
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, service.ErrInstructionsUnchanged) {
			return errInternalNotUndoable
		}
		return err

	case InternalUndoKnowledgeDraft:
		link, err := h.Queries.GetKnowledgeLink(ctx, db.GetKnowledgeLinkParams{ID: targetID, WorkspaceID: wsUUID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && link.State != service.KnowledgeLinkDraft) {
			return errInternalNotUndoable
		}
		if err != nil {
			return err
		}
		if h.Knowledge != nil && link.WeknoraKnowledgeID != "" {
			return h.Knowledge.DiscardDocument(ctx, wsUUID, targetID)
		}
		_, err = h.Queries.DeleteKnowledgeLink(ctx, db.DeleteKnowledgeLinkParams{ID: targetID, WorkspaceID: wsUUID})
		return err
	}
	return errInternalNotUndoable
}

func validDeclaredClass(class string) bool {
	switch class {
	case connectedapps.EffectRead, connectedapps.EffectWriteInternal, connectedapps.EffectWrite,
		connectedapps.EffectSend, connectedapps.EffectShare, connectedapps.EffectDelete, connectedapps.EffectSpend:
		return true
	}
	return false
}

func validDeclaredReversibility(rev string) bool {
	switch rev {
	case connectedapps.ReversibilityReversible, connectedapps.ReversibilityCompensable, connectedapps.ReversibilityIrreversible:
		return true
	}
	return false
}

func (h *Handler) durableEffectsEnabled(ctx context.Context) bool {
	if svc, ok := h.durable.Effects.(*service.EffectService); ok && svc != nil && svc.Flags != nil {
		return featureflags.DurableEffectsEnabled(ctx, svc.Flags)
	}
	return featureflags.DurableEffectsEnabled(ctx, h.FeatureFlags)
}

func truncateEffectSummary(s string) string {
	if r := []rune(s); len(r) > 500 {
		return string(r[:500])
	}
	return s
}

// effectHiddenFrom reports whether the effect was taken by a task on an issue in a project the
// caller cannot see. An effect with no task belongs to no project. A hidden effect is answered as
// a missing one, and a project that cannot be resolved is treated as hidden.
func (h *Handler) effectHiddenFrom(r *http.Request, workspaceID string, ev db.EffectEvent) bool {
	if !ev.TaskID.Valid {
		return false
	}
	task, err := h.Queries.GetAgentTask(r.Context(), ev.TaskID)
	if err != nil || !task.IssueID.Valid {
		return false
	}
	projectID, err := h.Queries.GetIssueProjectID(r.Context(), task.IssueID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	if err != nil {
		return true
	}
	hidden, err := h.hiddenProjects(r, workspaceID)
	if err != nil {
		return true
	}
	return !hidden.CanSeeProject(projectID)
}
