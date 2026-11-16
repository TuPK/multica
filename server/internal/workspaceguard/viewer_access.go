package workspaceguard

import "strings"

// RoleViewer is the read-only workspace role (spec 6.6).
const RoleViewer = "viewer"

// viewerWrites is the allow-list of writes a viewer may make: personal state
// only. Each key is "METHOD route template" as chi reports it, with the trailing
// slash removed by normalize. Every other write by a viewer is refused.
var viewerWrites = map[string]bool{
	"POST /api/inbox/{id}/read":             true,
	"POST /api/inbox/issues/{issueId}/read": true,
	"POST /api/inbox/mark-all-read":         true,
	"PATCH /api/notification-preferences":   true,
	"PUT /api/notification-preferences":     true,
	"POST /api/pins":                        true,
	"PUT /api/pins/reorder":                 true,
	"DELETE /api/pins/{itemType}/{itemId}":  true,
	"POST /api/workspaces/{id}/leave":       true,
}

// IsReadMethod reports whether a method only reads. Every other method is a
// write, and a viewer may not make one that is not on the allow-list.
func IsReadMethod(method string) bool { return isRead(method) }

// ViewerAllowsWrite reports whether a viewer may make this write. A request
// served by no known route (an empty pattern) is refused.
func ViewerAllowsWrite(method, routePattern string) bool {
	if routePattern == "" {
		return false
	}
	return viewerWrites[strings.ToUpper(method)+" "+normalize(routePattern)]
}
