package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Work outcomes, acceptance criteria and checks over HTTP (durable spec 2.7). Reads
// are for members and agents. An agent writes criteria, and records a check only through
// the verification task it was given; a person records checks as a member. Overrides are
// for owners and admins.

const (
	outcomePageMax     = 100
	outcomePageDefault = 50
)

// OutcomeResponse is one work outcome. Verifications is set on the detail view only.
type OutcomeResponse struct {
	ID               string                 `json:"id"`
	IssueID          string                 `json:"issue_id"`
	AgentID          *string                `json:"agent_id"`
	MemberID         *string                `json:"member_id"`
	SquadID          *string                `json:"squad_id"`
	WorkClass        string                 `json:"work_class"`
	VerificationMode string                 `json:"verification_mode"`
	Sampled          bool                   `json:"sampled"`
	Verdict          string                 `json:"verdict"`
	Severity         string                 `json:"severity"`
	FailCause        string                 `json:"fail_cause"`
	SettleAt         string                 `json:"settle_at"`
	SettledAt        *string                `json:"settled_at"`
	SupersededBy     *string                `json:"superseded_by"`
	CreatedAt        string                 `json:"created_at"`
	Verifications    []VerificationResponse `json:"verifications,omitempty"`
}

// VerificationResponse is one check on an outcome.
type VerificationResponse struct {
	ID           string          `json:"id"`
	OutcomeID    string          `json:"outcome_id"`
	CriterionID  *string         `json:"criterion_id"`
	VerifierType string          `json:"verifier_type"`
	VerifierID   *string         `json:"verifier_id"`
	TaskID       *string         `json:"task_id"`
	Result       string          `json:"result"`
	Severity     string          `json:"severity"`
	Note         string          `json:"note"`
	Evidence     json.RawMessage `json:"evidence"`
	CreatedAt    string          `json:"created_at"`
}

// CriterionResponse is one acceptance criterion.
type CriterionResponse struct {
	ID            string          `json:"id"`
	IssueID       string          `json:"issue_id"`
	Position      int32           `json:"position"`
	Text          string          `json:"text"`
	CheckKind     string          `json:"check_kind"`
	CheckSpec     json.RawMessage `json:"check_spec"`
	Source        string          `json:"source"`
	CreatedByType string          `json:"created_by_type"`
	CreatedByID   *string         `json:"created_by_id"`
	ArchivedAt    *string         `json:"archived_at"`
	CreatedAt     string          `json:"created_at"`
}

// OutcomeListResponse is one page of outcomes. NextCursor is empty on the last page.
type OutcomeListResponse struct {
	Outcomes   []OutcomeResponse `json:"outcomes"`
	NextCursor string            `json:"next_cursor"`
}

// OutcomeSummaryGroup is one row of the summary.
type OutcomeSummaryGroup struct {
	Key        string `json:"key"`
	Total      int64  `json:"total"`
	Pass       int64  `json:"pass"`
	Partial    int64  `json:"partial"`
	Fail       int64  `json:"fail"`
	Unverified int64  `json:"unverified"`
	Pending    int64  `json:"pending"`
}

// outcomeCaller is who is asking. An agent carries its task, which a check must name.
type outcomeCaller struct {
	actor  service.GovernanceActor
	task   pgtype.UUID
	admin  bool
	member db.Member
	agent  bool
}

// outcomeCallerFor resolves the caller of an outcome route. A task_token or an agent
// header makes an agent; anything else is a member, who must belong to the workspace.
func (h *Handler) outcomeCallerFor(w http.ResponseWriter, r *http.Request, workspaceID string) (outcomeCaller, bool) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return outcomeCaller{}, false
	}
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	if actorType == "agent" {
		agentID, ok := parseUUIDOrBadRequest(w, actorID, "actor id")
		if !ok {
			return outcomeCaller{}, false
		}
		task := pgtype.UUID{}
		if raw := r.Header.Get("X-Task-ID"); raw != "" {
			parsed, err := util.ParseUUID(raw)
			if err != nil {
				writeErrorCode(w, http.StatusForbidden, "not_verification_task", "the task header is not a task id")
				return outcomeCaller{}, false
			}
			task = parsed
		}
		return outcomeCaller{actor: service.GovernanceActor{Type: service.GovernanceActorAgent, ID: agentID}, task: task, agent: true}, true
	}
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return outcomeCaller{}, false
	}
	return outcomeCaller{
		actor:  service.GovernanceActor{Type: service.GovernanceActorMember, ID: member.UserID},
		admin:  roleAllowed(member.Role, "owner", "admin"),
		member: member,
	}, true
}

// outcomeService returns the verification service over the handler's queries.
func (h *Handler) outcomeService() *service.VerificationService {
	jury := service.NewJuryService(h.Queries, h.TxStarter, h.TaskService)
	jury.Flags = h.FeatureFlags
	return &service.VerificationService{
		Queries: h.Queries, TxStarter: h.TxStarter, Tasks: h.TaskService,
		Jury: jury, Flags: h.FeatureFlags,
	}
}

// writeOutcomeError maps a service error to its status and machine code (spec 2.7).
// It reports whether err was handled.
func writeOutcomeError(w http.ResponseWriter, err error) bool {
	type mapped struct {
		status int
		code   string
	}
	table := []struct {
		target error
		mapped
	}{
		{service.ErrCriterionLimit, mapped{http.StatusConflict, "criterion_limit"}},
		{service.ErrCriterionInvalid, mapped{http.StatusBadRequest, "criterion_invalid"}},
		{service.ErrVerificationInvalid, mapped{http.StatusBadRequest, "verification_invalid"}},
		{service.ErrCriterionNotFound, mapped{http.StatusNotFound, "criterion_not_found"}},
		{service.ErrOutcomeNotFound, mapped{http.StatusNotFound, "outcome_not_found"}},
		{service.ErrOutcomeSettled, mapped{http.StatusConflict, "outcome_settled"}},
		{service.ErrOutcomeNotSettled, mapped{http.StatusConflict, "outcome_not_settled"}},
		{service.ErrCriterionChecked, mapped{http.StatusConflict, "criterion_checked"}},
		{service.ErrOverrideWindowClosed, mapped{http.StatusConflict, "override_window_closed"}},
		{service.ErrVerifierIsDoer, mapped{http.StatusForbidden, "verifier_is_doer"}},
		{service.ErrNotVerificationTask, mapped{http.StatusForbidden, "not_verification_task"}},
		{service.ErrTrackModeInvalid, mapped{http.StatusBadRequest, "verification_mode_invalid"}},
		{service.ErrTrackAgentNotFound, mapped{http.StatusNotFound, "agent_not_found"}},
		{service.ErrOutcomesOff, mapped{http.StatusServiceUnavailable, "durable_outcomes_off"}},
	}
	for _, row := range table {
		if errors.Is(err, row.target) {
			writeErrorCode(w, row.status, row.code, err.Error())
			return true
		}
	}
	return false
}

func writeOutcomeFailure(w http.ResponseWriter, r *http.Request, what string, err error) {
	if writeOutcomeError(w, err) {
		return
	}
	slog.Error(what, append(logger.RequestAttrs(r), "error", err)...)
	writeError(w, http.StatusInternalServerError, "failed to "+what)
}

func outcomeResponse(o db.WorkOutcome) OutcomeResponse {
	return OutcomeResponse{
		ID:               uuidToString(o.ID),
		IssueID:          uuidToString(o.IssueID),
		AgentID:          uuidPtr(o.AgentID),
		MemberID:         uuidPtr(o.MemberID),
		SquadID:          uuidPtr(o.SquadID),
		WorkClass:        o.WorkClass,
		VerificationMode: o.VerificationMode,
		Sampled:          o.Sampled,
		Verdict:          o.Verdict,
		Severity:         o.Severity,
		FailCause:        o.FailCause,
		SettleAt:         timestampToString(o.SettleAt),
		SettledAt:        timestampPtr(o.SettledAt),
		SupersededBy:     uuidPtr(o.SupersededBy),
		CreatedAt:        timestampToString(o.CreatedAt),
	}
}

func verificationResponse(v db.OutcomeVerification) VerificationResponse {
	evidence := json.RawMessage(v.Evidence)
	if len(evidence) == 0 {
		evidence = json.RawMessage("[]")
	}
	return VerificationResponse{
		ID:           uuidToString(v.ID),
		OutcomeID:    uuidToString(v.OutcomeID),
		CriterionID:  uuidPtr(v.CriterionID),
		VerifierType: v.VerifierType,
		VerifierID:   uuidPtr(v.VerifierID),
		TaskID:       uuidPtr(v.TaskID),
		Result:       v.Result,
		Severity:     v.Severity,
		Note:         v.Note,
		Evidence:     evidence,
		CreatedAt:    timestampToString(v.CreatedAt),
	}
}

func criterionResponse(c db.AcceptanceCriterion) CriterionResponse {
	spec := json.RawMessage(c.CheckSpec)
	if len(spec) == 0 {
		spec = json.RawMessage("{}")
	}
	return CriterionResponse{
		ID:            uuidToString(c.ID),
		IssueID:       uuidToString(c.IssueID),
		Position:      c.Position,
		Text:          c.Text,
		CheckKind:     c.CheckKind,
		CheckSpec:     spec,
		Source:        c.Source,
		CreatedByType: c.CreatedByType,
		CreatedByID:   uuidPtr(c.CreatedByID),
		ArchivedAt:    timestampPtr(c.ArchivedAt),
		CreatedAt:     timestampToString(c.CreatedAt),
	}
}

// timestampPtr is the RFC 3339 time, or nil when the column is NULL.
func timestampPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := timestampToString(t)
	return &s
}

// issueForOutcomeRoute loads the issue named in the path, by id or identifier,
// enforcing project visibility through loadIssueForUser.
func (h *Handler) issueForOutcomeRoute(w http.ResponseWriter, r *http.Request, _ string) (db.Issue, bool) {
	return h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
}

// ListWorkOutcomes returns outcomes newest first, filtered and paged by cursor.
func (h *Handler) ListWorkOutcomes(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.outcomeCallerFor(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	hidden, ok := h.requestHiddenProjects(w, r, workspaceID)
	if !ok {
		return
	}
	q := r.URL.Query()
	params := db.ListWorkOutcomesParams{
		WorkspaceID:      wsUUID,
		HiddenProjectIds: hidden.Param(),
		RowLimit:         outcomePageDefault,
	}
	for name, dst := range map[string]*pgtype.UUID{"agent_id": &params.AgentID, "squad_id": &params.SquadID} {
		if raw := q.Get(name); raw != "" {
			id, ok := parseUUIDOrBadRequest(w, raw, name)
			if !ok {
				return
			}
			*dst = id
		}
	}
	if raw := q.Get("work_class"); raw != "" {
		params.WorkClass = pgtype.Text{String: raw, Valid: true}
	}
	if raw := q.Get("verdict"); raw != "" {
		switch raw {
		case "pending", "pass", "partial", "fail", "unverified":
		default:
			writeError(w, http.StatusBadRequest, "verdict must be pending, pass, partial, fail or unverified")
			return
		}
		params.Verdict = pgtype.Text{String: raw, Valid: true}
	}
	if raw := q.Get("since"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be an RFC 3339 time")
			return
		}
		params.Since = pgtype.Timestamptz{Time: t, Valid: true}
	}
	if raw := q.Get("cursor"); raw != "" {
		at, id, err := decodeOutcomeCursor(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "cursor is not valid")
			return
		}
		params.BeforeAt = pgtype.Timestamptz{Time: at, Valid: true}
		params.BeforeID = id
	}
	if raw := q.Get("limit"); raw != "" {
		var n int
		if _, err := fmt.Sscanf(raw, "%d", &n); err != nil || n < 1 || n > outcomePageMax {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("limit must be from 1 to %d", outcomePageMax))
			return
		}
		params.RowLimit = int32(n)
	}

	rows, err := h.Queries.ListWorkOutcomes(r.Context(), params)
	if err != nil {
		writeOutcomeFailure(w, r, "list outcomes", err)
		return
	}
	out := OutcomeListResponse{Outcomes: make([]OutcomeResponse, 0, len(rows))}
	for _, o := range rows {
		out.Outcomes = append(out.Outcomes, outcomeResponse(o))
	}
	if len(rows) == int(params.RowLimit) && len(rows) > 0 {
		last := rows[len(rows)-1]
		out.NextCursor = encodeOutcomeCursor(last.CreatedAt.Time, last.ID)
	}
	writeJSON(w, http.StatusOK, out)
}

// encodeOutcomeCursor and decodeOutcomeCursor carry the (created_at, id) of the last row.
func encodeOutcomeCursor(at time.Time, id pgtype.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + uuidToString(id)))
}

func decodeOutcomeCursor(raw string) (time.Time, pgtype.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, pgtype.UUID{}, err
	}
	at, id, ok := strings.Cut(string(b), "|")
	if !ok {
		return time.Time{}, pgtype.UUID{}, errors.New("cursor has no separator")
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return time.Time{}, pgtype.UUID{}, err
	}
	uid, err := util.ParseUUID(id)
	if err != nil {
		return time.Time{}, pgtype.UUID{}, err
	}
	return t, uid, nil
}

// GetWorkOutcome returns one outcome with its checks.
func (h *Handler) GetWorkOutcome(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.outcomeCallerFor(w, r, workspaceID); !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "outcome id")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	o, err := h.Queries.GetWorkOutcomeInWorkspace(r.Context(), db.GetWorkOutcomeInWorkspaceParams{ID: id, WorkspaceID: wsUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "outcome_not_found", "outcome not found")
		return
	}
	if err != nil {
		writeOutcomeFailure(w, r, "read outcome", err)
		return
	}
	if !h.requireIssueIDVisible(w, r, workspaceID, o.IssueID, http.StatusNotFound, "outcome not found") {
		return
	}
	checks, err := h.Queries.ListOutcomeVerifications(r.Context(), db.ListOutcomeVerificationsParams{OutcomeID: id, WorkspaceID: wsUUID})
	if err != nil {
		writeOutcomeFailure(w, r, "read outcome checks", err)
		return
	}
	resp := outcomeResponse(o)
	resp.Verifications = make([]VerificationResponse, 0, len(checks))
	for _, v := range checks {
		resp.Verifications = append(resp.Verifications, verificationResponse(v))
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetIssueOutcome returns the current outcome of an issue, 404 when there is none.
func (h *Handler) GetIssueOutcome(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.outcomeCallerFor(w, r, workspaceID); !ok {
		return
	}
	issue, ok := h.issueForOutcomeRoute(w, r, workspaceID)
	if !ok {
		return
	}
	o, err := h.Queries.GetCurrentWorkOutcomeForIssue(r.Context(), db.GetCurrentWorkOutcomeForIssueParams{WorkspaceID: issue.WorkspaceID, IssueID: issue.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "outcome_not_found", "the issue has no outcome yet")
		return
	}
	if err != nil {
		writeOutcomeFailure(w, r, "read issue outcome", err)
		return
	}
	writeJSON(w, http.StatusOK, outcomeResponse(o))
}

// ListIssueCriteria returns the criteria of an issue, archived ones included.
func (h *Handler) ListIssueCriteria(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.outcomeCallerFor(w, r, workspaceID); !ok {
		return
	}
	issue, ok := h.issueForOutcomeRoute(w, r, workspaceID)
	if !ok {
		return
	}
	rows, err := h.outcomeService().ListCriteria(r.Context(), issue.WorkspaceID, issue.ID)
	if err != nil {
		writeOutcomeFailure(w, r, "list criteria", err)
		return
	}
	out := make([]CriterionResponse, 0, len(rows))
	for _, c := range rows {
		out = append(out, criterionResponse(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"criteria": out})
}

// CreateCriterionRequest is a criterion a member or an agent writes.
type CreateCriterionRequest struct {
	Text      string         `json:"text"`
	CheckKind string         `json:"check_kind"`
	CheckSpec map[string]any `json:"check_spec"`
}

// CreateIssueCriterion adds a criterion. The source is agent when an agent writes it, so
// that the agent cannot check its own criterion.
func (h *Handler) CreateIssueCriterion(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	caller, ok := h.outcomeCallerFor(w, r, workspaceID)
	if !ok {
		return
	}
	issue, ok := h.issueForOutcomeRoute(w, r, workspaceID)
	if !ok {
		return
	}
	var req CreateCriterionRequest
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	c, err := h.outcomeService().AddCriterion(r.Context(), issue.WorkspaceID, issue.ID, caller.actor, service.CriterionInput{
		Text: req.Text, CheckKind: req.CheckKind, CheckSpec: req.CheckSpec,
	})
	if err != nil {
		writeOutcomeFailure(w, r, "add criterion", err)
		return
	}
	writeJSON(w, http.StatusCreated, criterionResponse(c))
}

// UpdateCriterionRequest changes a criterion's text, or archives it.
type UpdateCriterionRequest struct {
	Text     *string `json:"text"`
	Archived *bool   `json:"archived"`
}

// UpdateIssueCriterion edits or archives a criterion. The creator, an owner or an admin
// may do it; the service refuses it once the outcome has settled or been checked.
func (h *Handler) UpdateIssueCriterion(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	caller, ok := h.outcomeCallerFor(w, r, workspaceID)
	if !ok {
		return
	}
	issue, ok := h.issueForOutcomeRoute(w, r, workspaceID)
	if !ok {
		return
	}
	cid, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "cid"), "criterion id")
	if !ok {
		return
	}
	var req UpdateCriterionRequest
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	if req.Text == nil && req.Archived == nil {
		writeError(w, http.StatusBadRequest, "nothing to change: send text or archived")
		return
	}
	current, err := h.Queries.GetAcceptanceCriterion(r.Context(), db.GetAcceptanceCriterionParams{ID: cid, WorkspaceID: issue.WorkspaceID, IssueID: issue.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "criterion_not_found", "criterion not found")
		return
	}
	if err != nil {
		writeOutcomeFailure(w, r, "load criterion", err)
		return
	}
	creator := caller.actor.ID.Valid && current.CreatedByID == caller.actor.ID && current.CreatedByType == caller.actor.Type
	if !creator && !(caller.admin && !caller.agent) {
		writeError(w, http.StatusForbidden, "only the criterion's creator, an owner or an admin can change it")
		return
	}
	c, err := h.outcomeService().UpdateCriterion(r.Context(), issue.WorkspaceID, issue.ID, cid, caller.actor, service.CriterionPatch{
		Text:    req.Text,
		Archive: req.Archived != nil && *req.Archived,
	})
	if err != nil {
		writeOutcomeFailure(w, r, "update criterion", err)
		return
	}
	writeJSON(w, http.StatusOK, criterionResponse(c))
}

// RecordVerificationRequest is one check on an outcome.
type RecordVerificationRequest struct {
	CriterionID *string         `json:"criterion_id"`
	Result      string          `json:"result"`
	Severity    string          `json:"severity"`
	Note        string          `json:"note"`
	Evidence    json.RawMessage `json:"evidence"`
}

// RecordOutcomeVerification writes one check. An agent names the criterion, and its task
// header names the task it was given for that criterion.
func (h *Handler) RecordOutcomeVerification(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	caller, ok := h.outcomeCallerFor(w, r, workspaceID)
	if !ok {
		return
	}
	outcomeID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "outcome id")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	var req RecordVerificationRequest
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	if o, err := h.Queries.GetWorkOutcomeInWorkspace(r.Context(), db.GetWorkOutcomeInWorkspaceParams{ID: outcomeID, WorkspaceID: wsUUID}); err == nil {
		if !h.requireIssueIDVisible(w, r, workspaceID, o.IssueID, http.StatusNotFound, "outcome not found") {
			return
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeOutcomeFailure(w, r, "read outcome", err)
		return
	}
	in := service.VerificationInput{
		Result: req.Result, Severity: req.Severity, Note: req.Note, Evidence: req.Evidence, TaskID: caller.task,
	}
	if req.CriterionID != nil && *req.CriterionID != "" {
		cid, ok := parseUUIDOrBadRequest(w, *req.CriterionID, "criterion_id")
		if !ok {
			return
		}
		in.CriterionID = cid
	}
	v, err := h.outcomeService().Record(r.Context(), wsUUID, caller.actor, outcomeID, in)
	if err != nil {
		writeOutcomeFailure(w, r, "record check", err)
		return
	}
	if !caller.agent {
		h.answerAttention(r.Context(), "outcome_check", outcomeID)
	}
	writeJSON(w, http.StatusCreated, verificationResponse(v))
}

// OverrideRequest is a person's new conclusion on a settled outcome.
type OverrideRequest struct {
	Result   string `json:"result"`
	Severity string `json:"severity"`
	Note     string `json:"note"`
}

// OverrideOutcome changes a settled conclusion within the override window. Only an owner
// or an admin, as a person, may do it.
func (h *Handler) OverrideOutcome(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	caller, ok := h.outcomeCallerFor(w, r, workspaceID)
	if !ok {
		return
	}
	if caller.agent || !caller.admin {
		writeErrorCode(w, http.StatusForbidden, "forbidden", "only the owner or an admin can change a settled outcome")
		return
	}
	outcomeID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "outcome id")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	var req OverrideRequest
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	if o, err := h.Queries.GetWorkOutcomeInWorkspace(r.Context(), db.GetWorkOutcomeInWorkspaceParams{ID: outcomeID, WorkspaceID: wsUUID}); err == nil {
		if !h.requireIssueIDVisible(w, r, workspaceID, o.IssueID, http.StatusNotFound, "outcome not found") {
			return
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeOutcomeFailure(w, r, "read outcome", err)
		return
	}
	o, err := h.outcomeService().Override(r.Context(), wsUUID, caller.actor, outcomeID, service.VerificationInput{
		Result: req.Result, Severity: req.Severity, Note: req.Note,
	})
	if err != nil {
		writeOutcomeFailure(w, r, "override outcome", err)
		return
	}
	writeJSON(w, http.StatusOK, outcomeResponse(o))
}

// SummarizeWorkOutcomes groups the outcomes of the last week or month by agent, squad or
// work class.
func (h *Handler) SummarizeWorkOutcomes(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.outcomeCallerFor(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	q := r.URL.Query()
	groupBy := q.Get("group_by")
	switch groupBy {
	case "agent", "squad", "work_class":
	case "":
		groupBy = "work_class"
	default:
		writeError(w, http.StatusBadRequest, "group_by must be agent, squad or work_class")
		return
	}
	window := 7 * 24 * time.Hour
	switch q.Get("period") {
	case "", "week":
	case "month":
		window = 30 * 24 * time.Hour
	default:
		writeError(w, http.StatusBadRequest, "period must be week or month")
		return
	}
	hidden, ok := h.requestHiddenProjects(w, r, workspaceID)
	if !ok {
		return
	}
	rows, err := h.Queries.SummarizeWorkOutcomes(r.Context(), db.SummarizeWorkOutcomesParams{
		GroupBy: groupBy, WorkspaceID: wsUUID, Since: pgtype.Timestamptz{Time: time.Now().Add(-window), Valid: true},
		HiddenProjectIds: hidden.Param(),
	})
	if err != nil {
		writeOutcomeFailure(w, r, "summarize outcomes", err)
		return
	}
	groups := make([]OutcomeSummaryGroup, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, OutcomeSummaryGroup{
			Key: row.GroupKey, Total: row.Total, Pass: row.Passed, Partial: row.Partial,
			Fail: row.Failed, Unverified: row.Unverified, Pending: row.Pending,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"group_by": groupBy, "groups": groups})
}

// applyDurableClaimBrief adds the outcome blocks to a claim's instructions (durable spec 2.8).
// The blocks come from service.DurableBriefingBlocks, which owns what each task is shown.
func (h *Handler) applyDurableClaimBrief(ctx context.Context, task db.AgentTaskQueue, resp *AgentTaskResponse) {
	if resp.Agent == nil {
		return
	}
	blocks := service.DurableBriefingBlocks(ctx, h.Queries, h.FeatureFlags, task)
	if len(blocks) == 0 {
		return
	}
	sections := make([]string, 0, len(blocks))
	for _, b := range blocks {
		sections = append(sections, b.Markdown())
	}
	brief := strings.Join(sections, "\n\n")
	if strings.TrimSpace(resp.Agent.Instructions) == "" {
		resp.Agent.Instructions = brief
	} else {
		resp.Agent.Instructions = resp.Agent.Instructions + "\n\n" + brief
	}
}
