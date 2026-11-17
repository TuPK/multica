package workspaceguard

import "strings"

// CodeGuestForbidden refuses a guest on a route outside the guest allow-list,
// with HTTP 403 (spec 7.1 rule 9, D53).
const CodeGuestForbidden = "guest_forbidden"

// guestAllows is the closed allow-list of routes a guest (member.is_guest) may
// use, reads included (spec 7.1 rule 9). Each key is "METHOD route template" as
// chi reports it, with the trailing slash removed by normalize. Every route
// that is not listed is refused with CodeGuestForbidden.
//
// The list names routes, not projects. The handlers narrow issue and comment
// reads to the projects granted to the guest (P41.3 owns that narrowing); until
// it lands, a guest sees whatever issue lists the handler returns. The member's
// own role (member or viewer) still applies after this check, so a guest
// viewer writes nothing.
//
// Deliberately closed: the workspace itself and its member list (emails), every
// agent, label, status, workflow and project route, and the issue
// sub-resources not named below (children, attachments, labels, reactions,
// subscribers, read marks, wakeups, flow, pull requests, metadata, properties).
var guestAllows = map[string]bool{
	// Issues.
	"GET /api/issues":                  true,
	"POST /api/issues":                 true,
	"GET /api/issues/search":           true,
	"POST /api/issues/query":           true,
	"POST /api/issues/table/groups":    true,
	"POST /api/issues/table/rows":      true,
	"POST /api/issues/table/facets":    true,
	"GET /api/issues/grouped":          true,
	"GET /api/issues/{id}":             true,
	"PUT /api/issues/{id}":             true,
	"GET /api/issues/{id}/comments":    true,
	"POST /api/issues/{id}/comments":   true,
	"PUT /api/comments/{commentId}":    true,
	"DELETE /api/comments/{commentId}": true,

	// Inbox.
	"GET /api/inbox":                        true,
	"GET /api/inbox/archived":               true,
	"GET /api/inbox/archived/page":          true,
	"GET /api/inbox/archived/facets":        true,
	"GET /api/inbox/unread-count":           true,
	"GET /api/inbox/unread-summary":         true,
	"POST /api/inbox/mark-all-read":         true,
	"POST /api/inbox/archive-all":           true,
	"POST /api/inbox/archive-all-read":      true,
	"POST /api/inbox/archive-completed":     true,
	"POST /api/inbox/issues/{issueId}/read": true,
	"POST /api/inbox/{id}/read":             true,
	"POST /api/inbox/{id}/unread":           true,
	"POST /api/inbox/{id}/archive":          true,
	"POST /api/inbox/{id}/unarchive":        true,

	// Personal preferences.
	"GET /api/pins":                        true,
	"POST /api/pins":                       true,
	"PUT /api/pins/reorder":                true,
	"DELETE /api/pins/{itemType}/{itemId}": true,
	"GET /api/notification-preferences":    true,
	"PATCH /api/notification-preferences":  true,
	"PUT /api/notification-preferences":    true,
	"GET /api/issue-view-preferences":      true,
	"PUT /api/issue-view-preferences":      true,

	// Leaving the workspace.
	"POST /api/workspaces/{id}/leave": true,
}

// GuestAllows reports whether a guest may use this route. A request served by
// no known route (an empty pattern) is refused.
func GuestAllows(method, routePattern string) bool {
	if routePattern == "" {
		return false
	}
	return guestAllows[strings.ToUpper(method)+" "+normalize(routePattern)]
}
