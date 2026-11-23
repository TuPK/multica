// Package workspaceguard enforces a workspace's state (read_only, suspended,
// pending_deletion) on the request path and at task claim time. The platform
// writes the state; this package reads it through workspacestate and decides.
//
// Decide is the decision table of the platform-management spec, section 5.3.
// It is a pure function of the status and the route, so the allow-lists below
// are the only place that names Delegora routes for this purpose.
package workspaceguard

import (
	"net/http"
	"strings"

	"github.com/multica-ai/multica/platform-management/workspacestate"
)

// Refusal codes. Clients branch on them, so they are part of the API. All are
// returned with HTTP 403 except CodeStateUnavailable, which is 503.
const (
	CodeReadOnly         = "workspace_read_only"
	CodeSuspended        = "workspace_suspended"
	CodePendingDeletion  = "workspace_pending_deletion"
	CodeStateUnavailable = "workspace_state_unavailable"
)

// Policy refusal codes (spec 6.4), returned with HTTP 403. They refuse one
// workspace only; the user still reaches their other workspaces.
const (
	CodeMFARequired    = "mfa_required"
	CodeReauthRequired = "reauth_required"
	CodeSSORequired    = "sso_required"
)

// Personal access token refusal codes (spec 6.5), returned with HTTP 403.
const (
	CodeTokenReadOnly          = "token_read_only"
	CodeTokenWorkspaceMismatch = "token_workspace_mismatch"
)

// CodeViewerReadOnly refuses a write by a viewer, with HTTP 403 (spec 6.6).
const CodeViewerReadOnly = "viewer_read_only"

// Decision is the verdict on one request.
type Decision struct {
	// Allowed reports whether the request may proceed.
	Allowed bool
	// Code is the refusal code when Allowed is false, and empty otherwise.
	Code string
	// Roles, when set on an allowed decision, limits the allow-list entry to
	// members holding one of these roles. A member with another role is refused
	// with CodeFor(status).
	Roles []string
}

// CodeFor returns the refusal code of a status, or "" for active. A status this
// build does not know is treated as suspended, the same as the account guard.
func CodeFor(status string) string {
	switch workspacestate.Status(status) {
	case workspacestate.Active:
		return ""
	case workspacestate.ReadOnly:
		return CodeReadOnly
	case workspacestate.PendingDeletion:
		return CodePendingDeletion
	default:
		return CodeSuspended
	}
}

const subscriptionsPrefix = "/api/cloud-subscriptions"

var (
	ownerAdmin = []string{"owner", "admin"}
	owner      = []string{"owner"}
)

// Decide applies the decision table to a request. routePattern is the chi
// route template of the leaf route that serves the request; the caller
// resolves it, because a middleware mounted ahead of a sub-router sees only the
// mount prefix. An empty pattern matches no allow-list entry.
func Decide(status, method, routePattern string) Decision {
	if workspacestate.Status(status) == workspacestate.Active {
		return Decision{Allowed: true}
	}
	pattern := normalize(routePattern)
	refuse := Decision{Code: CodeFor(status)}

	if workspacestate.Status(status) == workspacestate.ReadOnly {
		// The export routes are owner-only even for reads, so they come before
		// the read shortcut.
		if exportRoute(method, pattern) {
			return Decision{Allowed: true, Roles: owner}
		}
		if isRead(method) || readOnlyAllows(method, pattern) {
			return Decision{Allowed: true}
		}
		return refuse
	}

	// suspended, pending_deletion, and any status this build does not know.
	if roles, ok := restrictedAllows(method, pattern); ok {
		return Decision{Allowed: true, Roles: roles}
	}
	return refuse
}

// normalize drops the trailing slash chi keeps on a Route("/{id}") leaf of "/".
func normalize(pattern string) string {
	if len(pattern) > 1 {
		return strings.TrimSuffix(pattern, "/")
	}
	return pattern
}

func isRead(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

func underSubscriptions(pattern string) bool {
	return pattern == subscriptionsPrefix || strings.HasPrefix(pattern, subscriptionsPrefix+"/")
}

// exportRoute reports whether the request is one of the three workspace-export
// routes of the platform spec 5.11: create, list, and read one export.
func exportRoute(method, pattern string) bool {
	switch {
	case method == http.MethodPost && pattern == "/api/workspaces/{id}/exports":
		return true
	case method == http.MethodGet && (pattern == "/api/workspaces/{id}/exports" || pattern == "/api/workspaces/{id}/exports/{exportId}"):
		return true
	}
	return false
}

// archiveRoutes are the non-DELETE routes that delete or archive a resource.
// Every DELETE route is a deletion and needs no entry here.
var archiveRoutes = map[string]bool{
	"POST /api/issues/batch-delete":                            true,
	"POST /api/agents/{id}/archive":                            true,
	"POST /api/runtimes/{runtimeId}/unbind-agents-and-delete":  true,
	"POST /api/runtimes/{runtimeId}/archive-agents-and-delete": true,
	"PATCH /api/chat/sessions/{sessionId}/archive":             true,
}

// readOnlyAllows is the write allow-list while read_only: subscription routes,
// leaving the workspace, and deleting or archiving resources. Deleting covers
// removing a member, revoking an invitation or share link, and deleting the
// workspace itself, all of which are DELETE routes.
func readOnlyAllows(method, pattern string) bool {
	if pattern == "" {
		return false
	}
	switch {
	case underSubscriptions(pattern):
		return true
	case method == http.MethodDelete:
		return true
	case method == http.MethodPost && pattern == "/api/workspaces/{id}/leave":
		return true
	}
	return archiveRoutes[method+" "+pattern]
}

// restrictedAllows is the allow-list while suspended or pending_deletion. The
// returned roles limit the entry; nil means any member.
func restrictedAllows(method, pattern string) (roles []string, ok bool) {
	switch {
	case underSubscriptions(pattern):
		return ownerAdmin, true
	case isRead(method) && pattern == "/api/workspaces/{id}":
		return nil, true
	case method == http.MethodPost && pattern == "/api/workspaces/{id}/leave":
		return nil, true
	case method == http.MethodDelete && pattern == "/api/workspaces/{id}/deletion":
		return owner, true
	case exportRoute(method, pattern):
		return owner, true
	case isRead(method) && (pattern == "/api/company/export" || pattern == "/api/hub/export"):
		return owner, true
	}
	return nil, false
}

// DecideForMember is Decide for one member's request. daemonPath reports that
// the request came through the daemon routes: a daemon request is always
// allowed, so a running task can report its result in any state. Otherwise the
// result is Decide's, and an allowed decision that names Roles is refused,
// with the status's code, for a member whose role is not among them.
func DecideForMember(status, method, routePattern, role string, daemonPath bool) Decision {
	if daemonPath {
		return Decision{Allowed: true}
	}
	d := Decide(status, method, routePattern)
	if !d.Allowed || len(d.Roles) == 0 {
		return d
	}
	for _, r := range d.Roles {
		if r == role {
			return Decision{Allowed: true}
		}
	}
	return Decision{Code: CodeFor(status)}
}

// Message is the error text that goes with a refusal code. The workspace
// middleware and the handlers share it so both write the same body.
func Message(code string) string {
	switch code {
	case CodeReadOnly:
		return "workspace is read-only"
	case CodePendingDeletion:
		return "workspace is pending deletion"
	case CodeStateUnavailable:
		return "workspace state unavailable"
	case CodeMFARequired:
		return "this workspace requires two-step sign-in"
	case CodeReauthRequired:
		return "this workspace requires a recent sign-in"
	case CodeSSORequired:
		return "this workspace requires single sign-on"
	case CodeTokenReadOnly:
		return "this token is read-only"
	case CodeTokenWorkspaceMismatch:
		return "this token is bound to a different workspace"
	case CodeViewerReadOnly:
		return "viewers can only read this workspace"
	case CodeGuestForbidden:
		return "guests cannot use this route"
	default:
		return "workspace is suspended"
	}
}
