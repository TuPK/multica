package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The stateless check of a manifest (spec 7.1, 10, 16.4): the one route of the
// authoring family an agent may call. It reads no draft and writes nothing.

const (
	flowValidateLimit  = 30
	flowValidateWindow = time.Minute
)

// flowActorLimiter is a fixed-window count per actor. It lives in the memory of one
// server, so the limit is per instance; it only bounds how much validation work one
// actor can ask for.
type flowActorLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	seen   map[string]flowActorWindow
}

type flowActorWindow struct {
	start time.Time
	count int
}

func newFlowActorLimiter(limit int, window time.Duration) *flowActorLimiter {
	return &flowActorLimiter{limit: limit, window: window, seen: map[string]flowActorWindow{}}
}

// allow counts a call of the actor and reports whether it is within the limit.
func (l *flowActorLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.seen) > 4096 {
		for k, w := range l.seen {
			if now.Sub(w.start) >= l.window {
				delete(l.seen, k)
			}
		}
	}
	w := l.seen[key]
	if now.Sub(w.start) >= l.window {
		w = flowActorWindow{start: now}
	}
	w.count++
	l.seen[key] = w
	return w.count <= l.limit
}

var flowValidateLimiter = newFlowActorLimiter(flowValidateLimit, flowValidateWindow)

type validateFlowManifestRequest struct {
	Manifest json.RawMessage `json:"manifest"`
	SquadID  *string         `json:"squad_id"`
}

// ValidateFlowManifest checks a manifest the way a draft is checked, and answers 200
// with every issue it finds. With squad_id the roles are checked against the roster of
// that squad; without it only what does not depend on a squad is.
func (h *Handler) ValidateFlowManifest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !featureflags.FlowAuthoringEnabled(ctx, h.FeatureFlags) {
		writeErrorCode(w, http.StatusNotFound, "flow_authoring_disabled", "flow authoring is not enabled")
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, uuidToString(member.UserID), workspaceID)
	if !flowValidateLimiter.allow(workspaceID+":"+actorType+":"+actorID, time.Now()) {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(flowValidateWindow.Seconds())))
		writeErrorCode(w, http.StatusTooManyRequests, "flow_validate_rate_limited", "too many checks: wait a moment")
		return
	}

	var req validateFlowManifestRequest
	if ok, tooLarge := readFlowBody(w, r, &req); !ok {
		if tooLarge {
			writeFlowManifestInvalid(w, flowManifestIssue("a manifest is at most %d bytes", issueworkflow.DocMaxBytes))
		}
		return
	}
	manifest, issue := readFlowManifestEnvelope(req.Manifest)
	if issue != nil {
		writeFlowManifestInvalid(w, *issue)
		return
	}

	a := flowAuthoring{member: member, workspaceID: workspaceID, wsUUID: wsUUID}
	roster := issueworkflow.Roster{}
	if req.SquadID != nil && *req.SquadID != "" {
		squadID, ok := parseUUIDOrBadRequest(w, *req.SquadID, "squad id")
		if !ok {
			return
		}
		squad, err := h.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: squadID, WorkspaceID: wsUUID})
		if err != nil {
			writeError(w, http.StatusNotFound, "squad not found")
			return
		}
		a.squad = squad
		if roster, err = h.squadRoster(ctx, h.Queries, squad, true); err != nil {
			writeFlowError(w, r, err)
			return
		}
	}
	vctx, err := h.flowValidateContextWith(ctx, h.Queries, a, roster, false)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	// A manifest carries no people; one that does is checked as an import would read it.
	doc := []byte(manifest.Doc)
	if unbound, err := stripBind(doc); err == nil {
		doc = unbound
	}
	writeJSON(w, http.StatusOK, newFlowValidation(0, doc, vctx))
}

func writeFlowManifestInvalid(w http.ResponseWriter, issue issueworkflow.Issue) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": issue.Message, "code": "flow_manifest_invalid", "issues": []issueworkflow.Issue{issue},
	})
}
