package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// effectStatusRecorder keeps the status a handler wrote, so the effect event can
// record whether the request succeeded.
type effectStatusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *effectStatusRecorder) WriteHeader(status int) {
	rec.status = status
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *effectStatusRecorder) Write(b []byte) (int, error) {
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	return rec.ResponseWriter.Write(b)
}

// EffectCLIMiddleware logs each mutating request that comes from the multica CLI as
// one write_internal effect (durable spec 4.5, source cli). A request the policy
// blocks gets 403. It is a no-op for other clients, for reads, and for requests
// that are not a workspace member's: those reach the handler, which refuses them.
// It runs inside the authenticated group, so the user is known. A request sent with
// an agent task's X-Task-ID is the agent's own write, and the event carries its
// task and agent. The declare route is not gated here: it answers with the decision
// and records its own event.
func (h *Handler) EffectCLIMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		platform, _, _ := middleware.ClientMetadataFromContext(r.Context())
		if h.durable.Effects == nil || platform != "cli" || !isMutatingMethod(r.Method) || r.URL.Path == effectDeclarePath {
			next.ServeHTTP(w, r)
			return
		}
		ws, err := util.ParseUUID(h.resolveWorkspaceID(r))
		if err != nil || !ws.Valid {
			next.ServeHTTP(w, r)
			return
		}
		userID, err := util.ParseUUID(requestUserID(r))
		if err != nil || !userID.Valid {
			next.ServeHTTP(w, r)
			return
		}
		if _, err := h.Queries.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{
			UserID: userID, WorkspaceID: ws,
		}); err != nil {
			next.ServeHTTP(w, r)
			return
		}
		req := connectedapps.EffectRequest{
			WS:            ws,
			MemberID:      userID,
			Source:        connectedapps.SourceCLI,
			Target:        r.Method + " " + r.URL.Path,
			Class:         connectedapps.EffectWriteInternal,
			Reversibility: connectedapps.ReversibilityReversible,
			Summary:       "multica CLI: " + r.Method + " " + r.URL.Path,
			Prior:         connectedapps.PolicyAllow,
		}
		if task, ok := h.taskFromRequestHeader(r); ok {
			req.TaskID, req.AgentID = task.ID, task.AgentID
		}
		ctx := r.Context()
		ticket, err := h.durable.Effects.Gate(ctx, req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not record the effect of this request")
			return
		}
		if ticket.Decision == connectedapps.PolicyBlock {
			writeError(w, http.StatusForbidden, "this request is blocked by the effect policy")
			return
		}
		rec := &effectStatusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		outcome := "executed"
		if rec.status >= http.StatusBadRequest {
			outcome = "failed"
		}
		if err := h.durable.Effects.Finish(ctx, ws, ticket.EventID, outcome, nil); err != nil {
			slog.Error("effect settle failed", "error", err, "outcome", outcome)
		}
	})
}

// isMutatingMethod reports whether a request method can change data.
// effectDeclarePath is the route an agent declares an effect on (router.go, /api/effects).
const effectDeclarePath = "/api/effects/declare"

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// pluginWriteGate logs a write made through the Action API before it runs (durable
// spec 4.5, source plugin_action). A blocked write gets 403 and allowed is false.
// Otherwise the caller writes through the returned writer and calls settle when the
// handler is done, which records executed for a 2xx or 3xx answer and failed for
// anything else.
func (h *Handler) pluginWriteGate(w http.ResponseWriter, r *http.Request, ws pgtype.UUID, target string) (rec *effectStatusRecorder, settle func(), allowed bool) {
	if h.durable.Effects == nil {
		rec = &effectStatusRecorder{ResponseWriter: w}
		return rec, func() {}, true
	}
	ctx := r.Context()
	ticket, err := h.durable.Effects.Gate(ctx, connectedapps.EffectRequest{
		WS:            ws,
		Source:        connectedapps.SourcePluginAction,
		Target:        truncateEffectTarget(target),
		Class:         connectedapps.EffectWriteInternal,
		Reversibility: connectedapps.ReversibilityReversible,
		Summary:       "plugin action: " + target,
		Prior:         connectedapps.PolicyAllow,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record the effect of this request")
		return nil, nil, false
	}
	if ticket.Decision == connectedapps.PolicyBlock {
		writeError(w, http.StatusForbidden, "this request is blocked by the effect policy")
		return nil, nil, false
	}
	rec = &effectStatusRecorder{ResponseWriter: w}
	settle = func() {
		outcome := "executed"
		if rec.status >= http.StatusBadRequest {
			outcome = "failed"
		}
		if err := h.durable.Effects.Finish(ctx, ws, ticket.EventID, outcome, nil); err != nil {
			slog.Error("effect settle failed", "error", err, "outcome", outcome)
		}
	}
	return rec, settle, true
}

// truncateEffectTarget keeps a target within the effect log's length limit.
func truncateEffectTarget(s string) string {
	if r := []rune(s); len(r) > 300 {
		return string(r[:300])
	}
	return s
}

// gateWebhookDispatch logs a webhook delivery that is about to start an autopilot
// run (durable spec 4.5, source webhook). The run acts through the gates itself, so
// this event records the start; its decision is not enforced here. The returned
// func settles the event once the dispatch has returned.
func (h *Handler) gateWebhookDispatch(ctx context.Context, ws pgtype.UUID) func(dispatched bool) {
	if h.durable.Effects == nil {
		return func(bool) {}
	}
	ticket, err := h.durable.Effects.Gate(ctx, connectedapps.EffectRequest{
		WS:            ws,
		Source:        connectedapps.SourceWebhook,
		Target:        "autopilot webhook",
		Class:         connectedapps.EffectWriteInternal,
		Reversibility: connectedapps.ReversibilityReversible,
		Summary:       "webhook delivery started an autopilot run",
		Prior:         connectedapps.PolicyAllow,
	})
	if err != nil {
		slog.Error("effect gate failed for webhook dispatch", "error", err)
		return func(bool) {}
	}
	return func(dispatched bool) {
		outcome := "failed"
		if dispatched {
			outcome = "executed"
		}
		if err := h.durable.Effects.Finish(ctx, ws, ticket.EventID, outcome, nil); err != nil {
			slog.Error("effect settle failed", "error", err, "outcome", outcome)
		}
	}
}
