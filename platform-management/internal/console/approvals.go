package console

import (
	"net/http"

	"github.com/multica-ai/multica/platform-management/internal/deletionrequest"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Two-person approval (spec R6 section 3c.6, M49): a system_admin other than the
// requester approves or rejects an immediate deletion that waits for approval.
// The list shows the requests awaiting one.

// listApprovals returns the deletion requests awaiting approval (M49).
func (h *Handler) listApprovals(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.scopeFor(w, r, staff.ActionApprovalsDecide); !ok {
		return
	}
	rows, err := h.Deletions.List(r.Context(), deletionrequest.Filter{State: deletionrequest.StateAwaitingApproval})
	if err != nil {
		h.fail(w, r, "list approvals", err)
		return
	}
	out := make([]deletionRequestJSON, len(rows))
	for i, x := range rows {
		out[i] = toDeletionRequestJSON(x)
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": out})
}

// approveApproval approves an awaiting request and runs its phase 1 (M49).
func (h *Handler) approveApproval(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.scopeFor(w, r, staff.ActionApprovalsDecide); !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	by, err := h.deletionActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, err := h.Deletions.Approve(r.Context(), by, id)
	if err != nil {
		h.failDeletion(w, r, "approve deletion", err)
		return
	}
	writeJSON(w, http.StatusOK, deleteAnswer{RequestID: req.ID, State: req.State})
}

// rejectApproval cancels an awaiting request (M49).
func (h *Handler) rejectApproval(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.scopeFor(w, r, staff.ActionApprovalsDecide); !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	by, err := h.deletionActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, err := h.Deletions.Reject(r.Context(), by, id)
	if err != nil {
		h.failDeletion(w, r, "reject deletion", err)
		return
	}
	writeJSON(w, http.StatusOK, deleteAnswer{RequestID: req.ID, State: req.State})
}
