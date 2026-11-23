package console

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/lifecycle"
	"github.com/multica-ai/multica/platform-management/internal/quota"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Quota requests (spec R6 section 3c.5, QT7): a request to raise a workspace's
// plan gate, created by staff for a workspace (M48), read by scope (M46), and
// approved or denied by system_admin only (M47).

// QuotaRequests is the quota service of the console. *quota.Service is one.
type QuotaRequests interface {
	Create(ctx context.Context, by quota.Actor, workspaceID, tenantID, gate string, requested int64, reason string) (quota.Request, error)
	Approve(ctx context.Context, by quota.Actor, id string, value *int64, reason string) (quota.Request, error)
	Deny(ctx context.Context, by quota.Actor, id, reason string) (quota.Request, error)
}

// QuotaSearch is one page of the quota request list.
type QuotaSearch struct {
	State  string
	Cursor *Cursor
	Limit  int
	Scope  Scope
}

var quotaStates = []string{quota.StatePending, quota.StateApproved, quota.StateDenied, "canceled"}

func quotaJSON(r quota.Request) map[string]any {
	out := map[string]any{
		"id": r.ID, "workspace_id": r.WorkspaceID, "tenant_id": r.TenantID, "gate": r.Gate,
		"requested_value": r.RequestedValue, "state": r.State, "reason": r.Reason,
		"decision_reason": r.DecisionReason, "requested_by": r.RequestedBy, "requested_by_label": r.RequestedByLabel,
		"decided_by": r.DecidedBy, "created_at": ts(r.CreatedAt),
	}
	if r.CurrentValue != nil {
		out["current_value"] = *r.CurrentValue
	} else {
		out["current_value"] = nil
	}
	if r.ApprovedValue != nil {
		out["approved_value"] = *r.ApprovedValue
	} else {
		out["approved_value"] = nil
	}
	if r.DecidedAt != nil {
		out["decided_at"] = ts(*r.DecidedAt)
	} else {
		out["decided_at"] = nil
	}
	return out
}

// failQuota answers a quota refusal with its status and code, not found with 404,
// and anything else as fail does.
func (h *Handler) failQuota(w http.ResponseWriter, r *http.Request, op string, err error) {
	var qe *quota.Error
	switch {
	case errors.As(err, &qe):
		body := map[string]any{"error": qe.Message, "code": qe.Code}
		for k, v := range qe.Extra {
			body[k] = v
		}
		writeJSON(w, qe.Status, body)
	case errors.Is(err, quota.ErrNotFound):
		writeError(w, notFound())
	default:
		h.fail(w, r, op, err)
	}
}

// listQuotaRequests returns one page of quota requests, newest first (M46).
func (h *Handler) listQuotaRequests(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionQuotaRequestsRead)
	if !ok {
		return
	}
	q := r.URL.Query()
	p, perr := parseListParams(q)
	if perr != nil {
		writeError(w, perr)
		return
	}
	state := q.Get("state")
	if state != "" && !containsString(quotaStates, state) {
		writeError(w, badRequest("invalid_state", "state must be pending, approved, denied or canceled"))
		return
	}
	rows, err := h.Reader.ListQuotaRequests(r.Context(), QuotaSearch{State: state, Cursor: p.cursor, Limit: p.limit, Scope: scope})
	if err != nil {
		h.fail(w, r, "list quota requests", err)
		return
	}
	rows, next := nextCursor(rows, p.limit, func(x quota.Request) (time.Time, string) { return x.CreatedAt, x.ID })
	out := make([]map[string]any, len(rows))
	for i, x := range rows {
		out[i] = quotaJSON(x)
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": out, "next_cursor": next})
}

// getQuotaRequest returns one quota request the scope may see (M46).
func (h *Handler) getQuotaRequest(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionQuotaRequestsRead)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	row, found, err := h.Reader.GetQuotaRequest(r.Context(), id)
	if err != nil {
		h.fail(w, r, "get quota request", err)
		return
	}
	if !found || !tenantVisible(scope, row.TenantID) {
		writeError(w, notFound())
		return
	}
	writeJSON(w, http.StatusOK, quotaJSON(row))
}

// createQuotaRequest asks for a gate of a workspace to be raised (M48).
func (h *Handler) createQuotaRequest(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionQuotaRequestCreate)
	if !ok {
		return
	}
	wsID, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if !h.visibleWorkspace(w, r, scope, wsID) {
		return
	}
	var body struct {
		Gate           string `json:"gate"`
		RequestedValue int64  `json:"requested_value"`
		Reason         string `json:"reason"`
	}
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	tenant, err := h.Reader.WorkspaceTenant(r.Context(), wsID)
	if err != nil {
		h.fail(w, r, "workspace tenant", err)
		return
	}
	req, err := h.Quota.Create(r.Context(), quota.Actor{ID: a.ID, Label: a.Email}, wsID, tenant, body.Gate, body.RequestedValue, body.Reason)
	if err != nil {
		h.failQuota(w, r, "create quota request", err)
		return
	}
	writeJSON(w, http.StatusCreated, quotaJSON(req))
}

// approveQuotaRequest approves a pending request, optionally at a lower value (M47).
func (h *Handler) approveQuotaRequest(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.scopeFor(w, r, staff.ActionQuotaRequestDecide); !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	var body struct {
		Value  *int64 `json:"value"`
		Reason string `json:"reason"`
	}
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, err := h.Quota.Approve(r.Context(), quota.Actor{ID: a.ID, Label: a.Email}, id, body.Value, body.Reason)
	if err != nil {
		h.failQuota(w, r, "approve quota request", err)
		return
	}
	h.notifyRequester(r, req, "approved")
	writeJSON(w, http.StatusOK, quotaJSON(req))
}

// denyQuotaRequest closes a pending request with a reason (M47).
func (h *Handler) denyQuotaRequest(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.scopeFor(w, r, staff.ActionQuotaRequestDecide); !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, err := h.Quota.Deny(r.Context(), quota.Actor{ID: a.ID, Label: a.Email}, id, body.Reason)
	if err != nil {
		h.failQuota(w, r, "deny quota request", err)
		return
	}
	h.notifyRequester(r, req, "denied")
	writeJSON(w, http.StatusOK, quotaJSON(req))
}

// notifyRequester emails the requester the decision. The decision is already
// written, so a failed send is logged and does not fail the request.
func (h *Handler) notifyRequester(r *http.Request, req quota.Request, decision string) {
	if h.QuotaMail == nil {
		return
	}
	u, found, err := h.Reader.GetUser(r.Context(), req.RequestedBy)
	if err != nil || !found || u.Email == "" {
		if err != nil {
			h.Log.Error("console: quota result lookup failed", "request", req.ID, "error", err)
		}
		return
	}
	subject := fmt.Sprintf("Delegora: your %s increase request was %s", req.Gate, decision)
	body := fmt.Sprintf("Your request to raise %s to %d in workspace %s was %s.\n", req.Gate, req.RequestedValue, req.WorkspaceID, decision)
	if req.DecisionReason != "" {
		body += "\nReason: " + req.DecisionReason + "\n"
	}
	if err := h.QuotaMail.Send(r.Context(), lifecycle.Message{To: []string{u.Email}, Subject: subject, Body: body}); err != nil {
		h.Log.Error("console: quota result email failed", "request", req.ID, "error", err)
	}
}
