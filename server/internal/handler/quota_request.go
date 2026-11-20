package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// quotaGates are the gates an owner may ask to raise (spec R6 3c.5). A gate
// outside this list is a 400.
var quotaGates = map[string]entitlement.GateName{
	"issue_count":         entitlement.GateIssueCount,
	"autopilot_runs":      entitlement.GateAutopilotRuns,
	"agents":              entitlement.GateAgents,
	"runtimes":            entitlement.GateRuntimes,
	"storage_bytes":       entitlement.GateStorageBytes,
	"api_rate_per_minute": entitlement.GateAPIRatePerMinute,
	"member_groups":       entitlement.GateMemberGroups,
}

// CreateQuotaRequestRequest is the body of POST /api/workspaces/{id}/quota-requests.
type CreateQuotaRequestRequest struct {
	Gate           string `json:"gate"`
	RequestedValue int64  `json:"requested_value"`
	Reason         string `json:"reason"`
}

// QuotaRequestResponse is one request as the owner sees it.
type QuotaRequestResponse struct {
	ID             string  `json:"id"`
	Gate           string  `json:"gate"`
	CurrentValue   *int64  `json:"current_value"`
	RequestedValue int64   `json:"requested_value"`
	ApprovedValue  *int64  `json:"approved_value"`
	State          string  `json:"state"`
	Reason         string  `json:"reason"`
	DecisionReason string  `json:"decision_reason"`
	CreatedAt      string  `json:"created_at"`
	DecidedAt      *string `json:"decided_at"`
}

// CreateQuotaRequest files an owner's request to raise a plan gate. The value
// must exceed the gate's effective limit, and one pending request per gate is
// allowed. The platform decides it; nothing changes until then.
func (h *Handler) CreateQuotaRequest(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	wsID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	var req CreateQuotaRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	gate, known := quotaGates[strings.TrimSpace(req.Gate)]
	reason := strings.TrimSpace(req.Reason)
	var fields []map[string]string
	if !known {
		fields = append(fields, map[string]string{"field": "gate", "message": "unknown gate"})
	}
	if n := len([]rune(reason)); n < 1 || n > 500 {
		fields = append(fields, map[string]string{"field": "reason", "message": "reason must be 1 to 500 characters"})
	}
	policy := h.resolvePlanGate(r.Context(), wsID, gate)
	if known && (!policy.Enforced || req.RequestedValue <= policy.Limit) {
		fields = append(fields, map[string]string{"field": "requested_value", "message": "must exceed the current limit"})
	}
	if len(fields) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":  "invalid quota request",
			"code":   "quota_request_invalid",
			"fields": fields,
		})
		return
	}

	currentValue := pgtype.Int8{Int64: policy.Limit, Valid: true}
	tenantID, err := h.Queries.GetTenantIDByWorkspace(r.Context(), wsID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to create quota request")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		tenantID = pgtype.UUID{}
	}
	user, err := h.Queries.GetUser(r.Context(), util.MustParseUUID(requestUserID(r)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create quota request")
		return
	}
	row, err := h.Queries.InsertQuotaRequest(r.Context(), db.InsertQuotaRequestParams{
		WorkspaceID:      wsID,
		TenantID:         tenantID,
		Gate:             string(gate),
		CurrentValue:     currentValue,
		RequestedValue:   req.RequestedValue,
		Reason:           reason,
		RequestedBy:      user.ID,
		RequestedByLabel: user.Email,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "a request for this gate is already pending",
			"code":  "quota_request_pending",
		})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create quota request")
		return
	}
	if err := h.recordAuditWith(r, h.Queries, audit.Entry{
		WorkspaceID: workspaceID,
		TenantID:    uuidToString(tenantID),
		Action:      audit.ActionQuotaRequestCreated,
		TargetType:  "quota_request",
		TargetID:    uuidToString(row.ID),
		TargetLabel: string(gate),
		Detail:      map[string]any{"gate": string(gate), "requested_value": req.RequestedValue, "current_value": policy.Limit},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create quota request")
		return
	}
	writeJSON(w, http.StatusCreated, quotaRequestToResponse(row))
}

// ListQuotaRequests returns the workspace's own quota requests, newest first.
func (h *Handler) ListQuotaRequests(w http.ResponseWriter, r *http.Request) {
	wsID, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	rows, err := h.Queries.ListQuotaRequestsForWorkspace(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list quota requests")
		return
	}
	out := make([]QuotaRequestResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, quotaRequestToResponse(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"quota_requests": out})
}

func quotaRequestToResponse(row db.QuotaRequest) QuotaRequestResponse {
	resp := QuotaRequestResponse{
		ID:             uuidToString(row.ID),
		Gate:           row.Gate,
		RequestedValue: row.RequestedValue,
		State:          row.State,
		Reason:         row.Reason,
		DecisionReason: row.DecisionReason,
		CreatedAt:      row.CreatedAt.Time.Format(time.RFC3339),
	}
	if row.CurrentValue.Valid {
		v := row.CurrentValue.Int64
		resp.CurrentValue = &v
	}
	if row.ApprovedValue.Valid {
		v := row.ApprovedValue.Int64
		resp.ApprovedValue = &v
	}
	if row.DecidedAt.Valid {
		s := row.DecidedAt.Time.Format(time.RFC3339)
		resp.DecidedAt = &s
	}
	return resp
}
