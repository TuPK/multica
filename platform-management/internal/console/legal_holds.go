package console

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/multica-ai/multica/platform-management/internal/legalhold"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Legal holds (spec R6 section 3c.7, QT11, M50): only system_admin places, releases
// and sees them. The other roles meet a hold only as a 409 legal_hold on a deletion.

// LegalHolds places, releases and lists holds. *legalhold.Service is one.
type LegalHolds interface {
	Place(ctx context.Context, by legalhold.Actor, kind, scopeID, tenantID, reason, caseRef string) (legalhold.Hold, error)
	Release(ctx context.Context, by legalhold.Actor, id, tenantID string) (legalhold.Hold, error)
	List(ctx context.Context, limit int) ([]legalhold.Hold, error)
}

func holdJSON(x legalhold.Hold) map[string]any {
	return map[string]any{
		"id": x.ID, "scope_kind": x.ScopeKind, "scope_id": x.ScopeID, "reason": x.Reason, "case_ref": x.CaseRef,
		"placed_by": x.PlacedBy, "released": x.Released, "released_by": x.ReleasedBy,
	}
}

// failLegalHold answers a hold refusal of the service with its status and code.
func (h *Handler) failLegalHold(w http.ResponseWriter, r *http.Request, op string, err error) {
	var he *legalhold.Error
	switch {
	case errors.As(err, &he):
		writeJSON(w, he.Status, map[string]any{"error": he.Message, "code": he.Code})
	case errors.Is(err, legalhold.ErrNotFound):
		writeError(w, &apiError{Status: http.StatusNotFound, Code: legalhold.CodeNotFound, Message: "legal hold not found"})
	default:
		h.fail(w, r, op, err)
	}
}

// listLegalHolds returns the holds, live ones first (M50).
func (h *Handler) listLegalHolds(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.scopeFor(w, r, staff.ActionLegalHoldManage); !ok {
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			writeError(w, badRequest("invalid_limit", "limit must be 1 to 500"))
			return
		}
		limit = n
	}
	rows, err := h.LegalHolds.List(r.Context(), limit)
	if err != nil {
		h.fail(w, r, "list legal holds", err)
		return
	}
	out := make([]map[string]any, len(rows))
	for i, x := range rows {
		out[i] = holdJSON(x)
	}
	writeJSON(w, http.StatusOK, map[string]any{"legal_holds": out})
}

// placeLegalHold puts a hold on a tenant, a workspace or an account (M50).
func (h *Handler) placeLegalHold(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.scopeFor(w, r, staff.ActionLegalHoldManage); !ok {
		return
	}
	var body struct {
		ScopeKind string `json:"scope_kind"`
		ScopeID   string `json:"scope_id"`
		Reason    string `json:"reason"`
		CaseRef   string `json:"case_ref"`
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
	tenant := ""
	switch body.ScopeKind {
	case legalhold.KindTenant:
		tenant = body.ScopeID
	case legalhold.KindWorkspace:
		if tenant, err = h.Reader.WorkspaceTenant(r.Context(), body.ScopeID); err != nil {
			h.fail(w, r, "workspace tenant", err)
			return
		}
	}
	hold, err := h.LegalHolds.Place(r.Context(), legalhold.Actor{ID: a.ID, Label: a.Email}, body.ScopeKind, body.ScopeID, tenant, body.Reason, body.CaseRef)
	if err != nil {
		h.failLegalHold(w, r, "place legal hold", err)
		return
	}
	writeJSON(w, http.StatusCreated, holdJSON(hold))
}

// releaseLegalHold lifts a live hold (M50).
func (h *Handler) releaseLegalHold(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.scopeFor(w, r, staff.ActionLegalHoldManage); !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	a, err := h.currentActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	hold, err := h.LegalHolds.Release(r.Context(), legalhold.Actor{ID: a.ID, Label: a.Email}, id, "")
	if err != nil {
		h.failLegalHold(w, r, "release legal hold", err)
		return
	}
	writeJSON(w, http.StatusOK, holdJSON(hold))
}

// hideHoldID removes legal_hold_id from a refusal's body unless the caller is
// system_admin (spec R6 section 3c.7, rule 6).
func hideHoldID(body map[string]any, role staff.Role) {
	if role != staff.RoleSystemAdmin {
		delete(body, "legal_hold_id")
	}
}
