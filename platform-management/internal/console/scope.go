package console

import (
	"context"
	"net/http"

	"github.com/multica-ai/multica/platform-management/internal/platformauth"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// scopeFor returns the Scope of the signed-in staff member for action, from the
// identity the middleware built for this request: the staff row was read from
// the database then, so a changed role or tenant applies to the next request
// (spec R6 section 3.5, rule 3). It answers the flat 404 of an unknown route,
// and returns false, when the role has no access to the action. The route
// table already admits only roles with access; this keeps the handlers safe when
// they are reached another way (rule 4).
func (h *Handler) scopeFor(w http.ResponseWriter, r *http.Request, action string) (Scope, bool) {
	id, ok := platformauth.IdentityFrom(r.Context())
	if !ok {
		http.NotFound(w, r)
		return Scope{}, false
	}
	switch staff.Allowed(id.Role, action) {
	case staff.All:
		return AllScope, true
	case staff.Own:
		// An admin with no tenant, or with one that is gone, has a scope that
		// matches nothing (rule 5).
		return Scope{TenantID: id.TenantID}, true
	default:
		http.NotFound(w, r)
		return Scope{}, false
	}
}

// workspaceVisible reports whether the workspace is inside scope. A workspace
// outside it reads as one that does not exist, and the handler answers
// notFound() for both, byte for byte (rule 1).
func (h *Handler) workspaceVisible(ctx context.Context, scope Scope, workspaceID string) (bool, error) {
	if scope.All {
		return true, nil
	}
	return h.Reader.WorkspaceInScope(ctx, workspaceID, scope)
}
