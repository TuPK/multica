package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/planlimit"
)

// resolvePlanGate reads one quota gate for the workspace. Call it before the
// creating transaction begins: a policy fetch is a network call. With no
// entitlement provider (no MULTICA_CLOUD_URL) the gate is off.
func (h *Handler) resolvePlanGate(ctx context.Context, workspaceID pgtype.UUID, gate entitlement.GateName) planlimit.Policy {
	return planlimit.Resolve(ctx, h.Entitlements, workspaceID, gate)
}

// writePlanLimitReached answers a refused creation with 402 and the body of
// spec 10. It returns false when err is not a plan limit.
func writePlanLimitReached(w http.ResponseWriter, err error) bool {
	var reached *planlimit.ReachedError
	if !errors.As(err, &reached) {
		return false
	}
	writeJSON(w, http.StatusPaymentRequired, reached.Body())
	return true
}
