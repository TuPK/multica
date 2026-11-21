package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/workspacestate"
	"github.com/multica-ai/multica/server/internal/membergroup"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
	"github.com/multica-ai/multica/server/internal/workspacepolicy"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Context keys for workspace-scoped request data.
type contextKey int

const (
	ctxKeyWorkspaceID contextKey = iota
	ctxKeyMember
	ctxKeyWorkspaceState
	ctxKeyWorkspacePolicy
	ctxKeySession
	ctxKeyPAT
)

// MemberFromContext returns the workspace member injected by the workspace middleware.
func MemberFromContext(ctx context.Context) (db.Member, bool) {
	m, ok := ctx.Value(ctxKeyMember).(db.Member)
	return m, ok
}

// WorkspaceIDFromContext returns the workspace ID injected by the workspace middleware.
func WorkspaceIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyWorkspaceID).(string)
	return id
}

// SetMemberContext injects workspace ID and member into the context.
// This is useful for handlers that resolve the workspace from an entity lookup
// and want to share the member with downstream code.
func SetMemberContext(ctx context.Context, workspaceID string, member db.Member) context.Context {
	ctx = context.WithValue(ctx, ctxKeyWorkspaceID, workspaceID)
	ctx = context.WithValue(ctx, ctxKeyMember, member)
	return ctx
}

// errWorkspaceNotFound is returned when a slug was provided but doesn't match
// any workspace. This lets the middleware distinguish "no identifier provided"
// (400) from "identifier provided but invalid" (404).
var errWorkspaceNotFound = errors.New("workspace not found")

// ResolveWorkspaceIDFromRequest returns the workspace UUID for an HTTP
// request using the same priority order as the workspace middleware. This is
// the single source of truth for "which workspace is this request targeting?",
// shared by middleware-protected routes (via context fast path) and
// middleware-less routes (e.g. /api/upload-file) that must resolve the slug
// themselves.
//
// Priority:
//  1. task-token binding (X-Actor-Source == "task_token") — authoritative,
//     server-set, cannot be re-negotiated by the client (MUL-2600)
//  2. middleware-injected context (fast path for middleware-protected routes)
//  3. X-Workspace-Slug header → GetWorkspaceBySlug → UUID (post-refactor frontend)
//  4. ?workspace_slug query → GetWorkspaceBySlug → UUID
//  5. X-Workspace-ID header (CLI/daemon compat)
//  6. ?workspace_id query (CLI/daemon compat)
//
// Returns "" when no identifier was provided OR a slug was provided but
// doesn't resolve to any workspace. Callers that need to distinguish "no
// identifier" (400) from "invalid slug" (404) should use the middleware's
// internal resolver instead — this helper collapses both cases to "" for
// simpler handler-level checks.
func ResolveWorkspaceIDFromRequest(r *http.Request, queries *db.Queries) string {
	// A mat_ task token is bound to exactly one workspace by the token
	// row. Auth middleware writes that workspace into X-Workspace-ID
	// after stripping any client-supplied X-Actor-Source. Any other
	// workspace identifier on the request (slug header/query, ID
	// query, URL param) is the agent trying to widen its blast
	// radius — ignore it. Explicit selectors that conflict with the
	// binding were already rejected by Auth (checkTaskTokenWorkspaceTarget,
	// GitHub #9403); this branch is the backstop.
	if r.Header.Get("X-Actor-Source") == "task_token" {
		return r.Header.Get("X-Workspace-ID")
	}
	if id := WorkspaceIDFromContext(r.Context()); id != "" {
		return id
	}
	if slug := r.Header.Get("X-Workspace-Slug"); slug != "" {
		if ws, err := queries.GetWorkspaceBySlug(r.Context(), slug); err == nil {
			return util.UUIDToString(ws.ID)
		}
	}
	if slug := r.URL.Query().Get("workspace_slug"); slug != "" {
		if ws, err := queries.GetWorkspaceBySlug(r.Context(), slug); err == nil {
			return util.UUIDToString(ws.ID)
		}
	}
	if id := r.Header.Get("X-Workspace-ID"); id != "" {
		return id
	}
	return r.URL.Query().Get("workspace_id")
}

// workspaceResolver extracts a workspace UUID from the request.
// Returns ("", nil) if no workspace identifier was provided at all.
// Returns ("", errWorkspaceNotFound) if a slug was provided but doesn't exist.
// Returns (uuid, nil) on success.
type workspaceResolver func(r *http.Request) (string, error)

// resolveWorkspaceUUID builds a resolver that accepts slug-first identification.
//
// Priority:
//  1. task-token binding (X-Actor-Source == "task_token") — authoritative,
//     server-set; the agent cannot widen its workspace scope by passing a
//     different slug/id (MUL-2600)
//  2. X-Workspace-Slug header / ?workspace_slug query → GetWorkspaceBySlug → UUID
//  3. X-Workspace-ID header / ?workspace_id query → UUID directly (CLI/daemon compat)
//
// TODO: cache slug→UUID lookup (slug is immutable, safe to cache with short TTL)
func resolveWorkspaceUUID(queries *db.Queries) workspaceResolver {
	return func(r *http.Request) (string, error) {
		// Task-token-authenticated requests must operate on the
		// token's bound workspace. The auth middleware wrote that ID
		// into X-Workspace-ID; nothing the agent can put on the wire
		// (slug header/query, id query, URL param) can override it.
		if r.Header.Get("X-Actor-Source") == "task_token" {
			id := r.Header.Get("X-Workspace-ID")
			if id == "" {
				return "", errWorkspaceNotFound
			}
			return id, nil
		}
		// Slug path (preferred — frontend sends this after the URL refactor)
		if slug := r.URL.Query().Get("workspace_slug"); slug != "" {
			ws, err := queries.GetWorkspaceBySlug(r.Context(), slug)
			if err != nil {
				return "", errWorkspaceNotFound
			}
			return util.UUIDToString(ws.ID), nil
		}
		if slug := r.Header.Get("X-Workspace-Slug"); slug != "" {
			ws, err := queries.GetWorkspaceBySlug(r.Context(), slug)
			if err != nil {
				return "", errWorkspaceNotFound
			}
			return util.UUIDToString(ws.ID), nil
		}
		// UUID fallback (CLI, daemon, legacy clients)
		if id := r.URL.Query().Get("workspace_id"); id != "" {
			return id, nil
		}
		if id := r.Header.Get("X-Workspace-ID"); id != "" {
			return id, nil
		}
		return "", nil
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(`{"error":"` + msg + `"}`))
}

// RequireWorkspaceMember resolves the workspace from slug (preferred) or UUID
// (fallback), validates membership, and injects the member and workspace ID
// into the request context.
func RequireWorkspaceMember(queries *db.Queries) func(http.Handler) http.Handler {
	return buildMiddleware(queries, resolveWorkspaceUUID(queries), nil, "")
}

// RequireWorkspaceRole is like RequireWorkspaceMember but additionally checks
// that the member has one of the specified roles.
func RequireWorkspaceRole(queries *db.Queries, roles ...string) func(http.Handler) http.Handler {
	return buildMiddleware(queries, resolveWorkspaceUUID(queries), roles, "")
}

// RequireWorkspaceMemberFromURL resolves the workspace ID from a chi URL
// parameter, validates membership, and injects into context.
func RequireWorkspaceMemberFromURL(queries *db.Queries, param string) func(http.Handler) http.Handler {
	return buildMiddleware(queries, func(r *http.Request) (string, error) {
		id := chi.URLParam(r, param)
		if id == "" {
			return "", nil
		}
		return id, nil
	}, nil, "")
}

// RequireWorkspaceRoleFromURL is like RequireWorkspaceMemberFromURL but
// additionally checks that the member has one of the specified roles.
func RequireWorkspaceRoleFromURL(queries *db.Queries, param string, roles ...string) func(http.Handler) http.Handler {
	return buildMiddleware(queries, func(r *http.Request) (string, error) {
		id := chi.URLParam(r, param)
		if id == "" {
			return "", nil
		}
		return id, nil
	}, roles, "")
}

// RequireWorkspacePermission is like RequireWorkspaceMember but additionally
// requires the permission key (spec 7b.2). owner and admin pass without a group
// lookup; any other role passes only when one of its groups grants key.
func RequireWorkspacePermission(queries *db.Queries, key string) func(http.Handler) http.Handler {
	return buildMiddleware(queries, resolveWorkspaceUUID(queries), nil, key)
}

// RequireWorkspacePermissionFromURL is RequireWorkspacePermission with the
// workspace taken from a chi URL parameter.
func RequireWorkspacePermissionFromURL(queries *db.Queries, param, key string) func(http.Handler) http.Handler {
	return buildMiddleware(queries, func(r *http.Request) (string, error) {
		return chi.URLParam(r, param), nil
	}, nil, key)
}

// guardPermission reports whether member holds key, and writes the refusal when
// not. The groups are read on every request, so a removal from a group applies
// to the next one.
func guardPermission(w http.ResponseWriter, r *http.Request, queries *db.Queries, member db.Member, key string) bool {
	if member.Role == "owner" || member.Role == "admin" {
		return true
	}
	access, err := membergroup.New(queries).Access(r.Context(), member.WorkspaceID, member.UserID)
	if err != nil {
		slog.Warn("workspace: permission lookup failed",
			"path", r.URL.Path, "workspace_id", util.UUIDToString(member.WorkspaceID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check permissions")
		return false
	}
	if membergroup.Has(access.Permissions, key) {
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": "insufficient permissions", "code": "permission_denied", "permission": key,
	})
	return false
}

// buildMiddleware checks the role list and, when permission is set, the
// permission key. Both are optional.
//
// It is not inlined so that every workspace guard shares one closure code
// pointer, which the route-table tests use to tell a workspace guard from other
// middleware.
//
//go:noinline
func buildMiddleware(queries *db.Queries, resolve workspaceResolver, roles []string, permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			workspaceID, resolveErr := resolve(r)
			if resolveErr != nil {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			if workspaceID == "" {
				writeError(w, http.StatusBadRequest, "workspace_id or workspace_slug is required")
				return
			}

			// A personal access token bound to another workspace stops here, even
			// when Auth could not tell which workspace the route names.
			if !guardTokenWorkspace(w, r, workspaceID) {
				return
			}

			// Final task-token binding check: even when the workspace
			// was resolved from a chi URL parameter
			// (RequireWorkspaceMemberFromURL), the agent must not be
			// allowed to operate on a workspace other than the one
			// stamped into its task token. This is the catch-all
			// behind resolveWorkspaceUUID's earlier check. MUL-2600.
			if r.Header.Get("X-Actor-Source") == "task_token" {
				bound := r.Header.Get("X-Workspace-ID")
				if bound == "" || workspaceID != bound {
					writeError(w, http.StatusForbidden, "task token is bound to a different workspace")
					return
				}
			}

			userID := r.Header.Get("X-User-ID")
			if userID == "" {
				writeError(w, http.StatusUnauthorized, "user not authenticated")
				return
			}

			userUUID, err := util.ParseUUID(userID)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "user not authenticated")
				return
			}
			wsUUID, err := util.ParseUUID(workspaceID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid workspace_id")
				return
			}
			member, err := queries.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{
				UserID:      userUUID,
				WorkspaceID: wsUUID,
			})
			if err != nil {
				writeError(w, http.StatusNotFound, "workspace not found")
				return
			}
			if member.Role == "guest" && !GuestMayRequest(r) {
				RefuseGuest(w, r)
				return
			}

			// A guest reaches only the allow-list (spec 7.1 rule 9). This runs
			// before the role check so every other route reports guest_forbidden.
			if member.IsGuest && !workspaceguard.GuestAllows(r.Method, leafRoutePattern(r)) {
				writeCodedError(w, http.StatusForbidden, workspaceguard.CodeGuestForbidden, workspaceguard.Message(workspaceguard.CodeGuestForbidden))
				return
			}

			if len(roles) > 0 {
				allowed := false
				for _, role := range roles {
					if member.Role == role {
						allowed = true
						break
					}
				}
				if !allowed {
					writeError(w, http.StatusForbidden, "insufficient permissions")
					return
				}
			}

			// The key check precedes the policy guard so a viewer who holds the
			// key is still refused a write by the viewer rule (spec 7b.2, rule 7).
			if permission != "" && !guardPermission(w, r, queries, member, permission) {
				return
			}

			// The session policies come first: they refuse a caller whose
			// session is not good enough for this workspace before the state
			// of the workspace is looked at.
			if !guardWorkspacePolicy(w, r, workspaceID, member) {
				return
			}
			if !guardWorkspaceState(w, r, member, wsUUID) {
				return
			}

			ctx := SetMemberContext(r.Context(), workspaceID, member)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// WithWorkspaceState makes guard the source of workspace state for every
// workspace middleware below it. The router mounts it once, ahead of all
// routes. A nil guard, or a router that does not mount it, skips the check;
// that is for tests only.
func WithWorkspaceState(guard *workspacestate.Guard) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if guard == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxKeyWorkspaceState, guard)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// WorkspaceStateFromContext returns the guard that WithWorkspaceState mounted,
// or nil when none was. Handlers that authorize a member themselves read the
// state through it, so they and the workspace middleware share one source.
func WorkspaceStateFromContext(ctx context.Context) *workspacestate.Guard {
	guard, _ := ctx.Value(ctxKeyWorkspaceState).(*workspacestate.Guard)
	return guard
}

// PolicyReader returns the policy in force for a workspace: the defaults when
// the workspace has none or its plan lacks the feature.
type PolicyReader interface {
	Policy(ctx context.Context, workspaceID string) (workspacepolicy.Policy, error)
}

// SSOReader is the optional second half of a PolicyReader: it says whether
// require_sso binds a member. A reader that lacks it never refuses for SSO.
type SSOReader interface {
	SSOAccess(ctx context.Context, workspaceID, userID string) (workspaceguard.SSOAccess, error)
}

// WithWorkspacePolicy makes reader the source of workspace policies for every
// workspace middleware below it. A nil reader skips the check; that is for
// tests only.
func WithWorkspacePolicy(reader PolicyReader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if reader == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxKeyWorkspacePolicy, reader)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type sessionFacts struct {
	amr           []string
	authTime      int64
	authTimeKnown bool
}

// WithSession records how the caller's session token says they signed in. Auth
// calls it for a session JWT and for nothing else, so a request whose context
// carries no session came in on a machine credential.
func WithSession(ctx context.Context, amr []string, authTime int64, authTimeKnown bool) context.Context {
	return context.WithValue(ctx, ctxKeySession, sessionFacts{amr: amr, authTime: authTime, authTimeKnown: authTimeKnown})
}

// requestSession returns the session of a request for the policy check. A
// request is a machine credential when it carries no session, came through the
// daemon routes, or was authenticated by a task token or a cloud node token.
func requestSession(r *http.Request) workspaceguard.Session {
	facts, ok := r.Context().Value(ctxKeySession).(sessionFacts)
	if !ok || DaemonAuthPathFromContext(r.Context()) != "" || r.Header.Get("X-Actor-Source") != "" {
		return workspaceguard.Session{Machine: true}
	}
	return workspaceguard.Session{AMR: facts.amr, AuthTime: facts.authTime, AuthTimeKnown: facts.authTimeKnown}
}

// guardWorkspacePolicy applies the session policies of the workspace (spec
// 6.4) and the viewer rule (spec 6.6), and reports whether the request may
// proceed; when it may not, the response has been written. Machine credentials
// pass the session policies without reading the policy, but a viewer's write is
// refused for every credential. A policy that cannot be read refuses a session,
// as the state guard does. With no policy reader only the viewer rule applies.
func guardWorkspacePolicy(w http.ResponseWriter, r *http.Request, workspaceID string, member db.Member) bool {
	reader, _ := r.Context().Value(ctxKeyWorkspacePolicy).(PolicyReader)
	session := requestSession(r)
	session.Role = member.Role
	session.Method = r.Method
	if member.Role == workspaceguard.RoleViewer {
		session.RoutePattern = leafRoutePattern(r)
	}
	policy := workspacepolicy.Default()
	if reader != nil && !session.Machine {
		var err error
		policy, err = reader.Policy(r.Context(), workspaceID)
		if err != nil {
			slog.Warn("workspace: policy unavailable",
				"path", r.URL.Path, "workspace_id", workspaceID, "error", err)
			writeCodedError(w, http.StatusServiceUnavailable, workspaceguard.CodeStateUnavailable, workspaceguard.Message(workspaceguard.CodeStateUnavailable))
			return false
		}
	}
	if policy.RequireSSO && !session.Machine {
		// The SSO facts cost two reads, so only a workspace that requires SSO
		// pays for them. A failed read refuses, as an unreadable policy does.
		session.IsOwner = member.Role == "owner"
		if ssoReader, ok := reader.(SSOReader); ok && !session.IsOwner {
			access, err := ssoReader.SSOAccess(r.Context(), workspaceID, util.UUIDToString(member.UserID))
			if err != nil {
				slog.Warn("workspace: sso access unavailable",
					"path", r.URL.Path, "workspace_id", workspaceID, "error", err)
				writeCodedError(w, http.StatusServiceUnavailable, workspaceguard.CodeStateUnavailable, workspaceguard.Message(workspaceguard.CodeStateUnavailable))
				return false
			}
			session.SSOConnectionID, session.SSOCovered = access.ConnectionID, access.Covered
		}
	}
	code := workspaceguard.CheckWorkspaceAccess(policy, session, time.Now())
	if code == "" {
		return true
	}
	slog.Warn("workspace: request refused by policy",
		"path", r.URL.Path, "method", r.Method, "workspace_id", workspaceID, "code", code)
	writeCodedError(w, http.StatusForbidden, code, workspaceguard.Message(code))
	return false
}

// guardWorkspaceState applies the workspace-state decision table to a member's
// request and reports whether it may proceed; when it may not, the response has
// been written. An active workspace passes without resolving the route.
func guardWorkspaceState(w http.ResponseWriter, r *http.Request, member db.Member, workspace pgtype.UUID) bool {
	guard := WorkspaceStateFromContext(r.Context())
	if guard == nil {
		return true
	}
	workspaceID := util.UUIDToString(workspace)
	rec, err := guard.State(r.Context(), workspaceID)
	if err != nil {
		slog.Warn("workspace: state unavailable",
			"path", r.URL.Path, "workspace_id", workspaceID, "error", err)
		writeCodedError(w, http.StatusServiceUnavailable, workspaceguard.CodeStateUnavailable, workspaceguard.Message(workspaceguard.CodeStateUnavailable))
		return false
	}
	if rec.Status == workspacestate.Active {
		return true
	}
	status := string(rec.Status)
	d := workspaceguard.Decide(status, r.Method, leafRoutePattern(r))
	if d.Allowed && roleListed(member.Role, d.Roles) {
		return true
	}
	code := d.Code
	if d.Allowed {
		code = workspaceguard.CodeFor(status)
	}
	slog.Warn("workspace: request refused by state",
		"path", r.URL.Path, "method", r.Method, "workspace_id", workspaceID, "status", status, "code", code)
	writeCodedError(w, http.StatusForbidden, code, workspaceguard.Message(code))
	return false
}

// roleListed reports whether role passes an allow-list entry's role limit. An
// empty limit admits every member.
func roleListed(role string, roles []string) bool {
	if len(roles) == 0 {
		return true
	}
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// leafRoutePattern returns the chi route template of the route that serves r.
// A workspace middleware that wraps a sub-router mount runs before the
// sub-router has matched, so rctx.RoutePattern() would be only the mount prefix
// (for example /api/issues/*). Find walks the whole tree for the same method
// and path. It returns "" when nothing matches, which no allow-list admits.
func leafRoutePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil || rctx.Routes == nil {
		return ""
	}
	path := r.URL.RawPath
	if path == "" {
		path = r.URL.Path
	}
	return rctx.Routes.Find(chi.NewRouteContext(), r.Method, path)
}
