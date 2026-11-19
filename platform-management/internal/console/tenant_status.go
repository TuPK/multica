package console

import (
	"context"
	"errors"
	"net/http"

	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/tenantstatus"
)

// TenantStatuses locks and unlocks a whole tenant (spec R6 section 3c.9, M52).
// *tenantstatus.Service is one.
type TenantStatuses interface {
	SetStatus(ctx context.Context, by tenantstatus.Actor, tenantID, status, reason string) ([]string, error)
}

// setTenantStatus locks a tenant (every workspace of it) or unlocks it (M52).
// system_admin, support and the admin of the tenant may; the rest get the flat 404.
func (h *Handler) setTenantStatus(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionTenantStatus)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if !tenantVisible(scope, id) {
		writeError(w, notFound())
		return
	}
	var body struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if perr := decodeBody(w, r, &body); perr != nil {
		writeError(w, perr)
		return
	}
	if !tenantstatus.Valid(body.Status) {
		writeError(w, badRequest("invalid_status", "status must be active, read_only or suspended"))
		return
	}
	reason, perr := cleanReason(body.Reason)
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	changed, err := h.TenantStatuses.SetStatus(r.Context(), tenantstatus.Actor{ID: a.ID, Label: a.Email}, id, body.Status, reason)
	if errors.Is(err, tenantstatus.ErrNotFound) {
		writeError(w, notFound())
		return
	}
	if err != nil {
		h.fail(w, r, "set tenant status", err)
		return
	}
	if changed == nil {
		changed = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": body.Status, "workspace_ids": changed})
}
