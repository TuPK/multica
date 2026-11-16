package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
	"github.com/multica-ai/multica/server/internal/workspacepolicy"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// WithPATEntry records the personal access token that authenticated a request,
// so the workspace middleware can check the token's workspace binding against
// the workspace it resolves. Auth and DaemonAuth call it for a mul_ token.
func WithPATEntry(ctx context.Context, e auth.PATEntry) context.Context {
	return context.WithValue(ctx, ctxKeyPAT, e)
}

// PATEntryFromContext returns the personal access token of the request, if it
// was authenticated by one.
func PATEntryFromContext(ctx context.Context) (auth.PATEntry, bool) {
	e, ok := ctx.Value(ctxKeyPAT).(auth.PATEntry)
	return e, ok
}

// patEntryFromRow is the cache entry of a token row.
func patEntryFromRow(pat db.PersonalAccessToken) auth.PATEntry {
	e := auth.PATEntry{UserID: uuidToString(pat.UserID), ReadOnly: pat.ReadOnly}
	if pat.WorkspaceID.Valid {
		e.WorkspaceID = uuidToString(pat.WorkspaceID)
	}
	return e
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// unresolvedWorkspace stands for a selector that names no workspace this server
// can resolve. It never equals a token's binding.
const unresolvedWorkspace = "unresolved"

// requestedWorkspaces returns every workspace the request names: the slug and id
// selectors the workspace middleware reads, and the id of a /api/workspaces/{id}
// URL. All of them are returned, not the one the middleware would pick, so a
// request that names the token's workspace in one place and another workspace
// in a second place is refused.
func requestedWorkspaces(r *http.Request, queries *db.Queries) []string {
	var out []string
	byID := func(s string) {
		if s == "" {
			return
		}
		id, err := util.ParseUUID(s)
		if err != nil {
			out = append(out, unresolvedWorkspace)
			return
		}
		out = append(out, util.UUIDToString(id))
	}
	bySlug := func(s string) {
		if s == "" {
			return
		}
		if queries == nil {
			out = append(out, unresolvedWorkspace)
			return
		}
		ws, err := queries.GetWorkspaceBySlug(r.Context(), s)
		if err != nil {
			out = append(out, unresolvedWorkspace)
			return
		}
		out = append(out, util.UUIDToString(ws.ID))
	}

	bySlug(r.URL.Query().Get("workspace_slug"))
	bySlug(r.Header.Get("X-Workspace-Slug"))
	byID(r.URL.Query().Get("workspace_id"))
	byID(r.Header.Get("X-Workspace-ID"))
	if rest, ok := strings.CutPrefix(r.URL.Path, "/api/workspaces/"); ok {
		seg, _, _ := strings.Cut(rest, "/")
		if _, err := util.ParseUUID(seg); err == nil {
			byID(seg)
		} else {
			bySlug(seg)
		}
	}
	return out
}

// enforcePATRestrictions applies the restrictions of a personal access token
// (spec 6.5) and reports whether the request may proceed; when it may not, the
// response has been written. It returns the request carrying the token in its
// context. A token with no restriction passes untouched.
//
// A read-only token is refused on every method but GET, HEAD and OPTIONS. A
// token bound to a workspace is refused when the request names another
// workspace, and when it names none, except for GET /api/me.
func enforcePATRestrictions(w http.ResponseWriter, r *http.Request, queries *db.Queries, e auth.PATEntry) (*http.Request, bool) {
	r = r.WithContext(WithPATEntry(r.Context(), e))
	if e.ReadOnly && !isSafeMethod(r.Method) {
		slog.Warn("auth: read-only token refused", "path", r.URL.Path, "method", r.Method, "user_id", e.UserID)
		writeCodedError(w, http.StatusForbidden, workspaceguard.CodeTokenReadOnly, workspaceguard.Message(workspaceguard.CodeTokenReadOnly))
		return r, false
	}
	if e.WorkspaceID == "" {
		return r, true
	}
	allowNone := r.Method == http.MethodGet && strings.TrimSuffix(r.URL.Path, "/") == "/api/me"
	named := requestedWorkspaces(r, queries)
	if len(named) == 0 {
		named = []string{""}
	}
	for _, id := range named {
		code := workspaceguard.CheckWorkspaceAccess(workspacepolicy.Default(), workspaceguard.Session{
			Machine:          true,
			TokenWorkspaceID: e.WorkspaceID,
			WorkspaceID:      id,
			AllowNoWorkspace: allowNone,
		}, time.Now())
		if code != "" {
			slog.Warn("auth: workspace-bound token refused",
				"path", r.URL.Path, "method", r.Method, "user_id", e.UserID, "token_workspace_id", e.WorkspaceID, "code", code)
			writeCodedError(w, http.StatusForbidden, code, workspaceguard.Message(code))
			return r, false
		}
	}
	return r, true
}

// guardTokenWorkspace is the workspace middleware's check of a token binding
// against the workspace it resolved, which covers a route that names the
// workspace only in its URL pattern. It reports whether the request may proceed;
// when it may not, the response has been written.
func guardTokenWorkspace(w http.ResponseWriter, r *http.Request, workspaceID string) bool {
	e, ok := PATEntryFromContext(r.Context())
	if !ok || e.WorkspaceID == "" {
		return true
	}
	code := workspaceguard.CheckWorkspaceAccess(workspacepolicy.Default(), workspaceguard.Session{
		Machine:          true,
		TokenWorkspaceID: e.WorkspaceID,
		WorkspaceID:      workspaceID,
	}, time.Now())
	if code == "" {
		return true
	}
	slog.Warn("workspace: workspace-bound token refused",
		"path", r.URL.Path, "method", r.Method, "workspace_id", workspaceID, "code", code)
	writeCodedError(w, http.StatusForbidden, code, workspaceguard.Message(code))
	return false
}
