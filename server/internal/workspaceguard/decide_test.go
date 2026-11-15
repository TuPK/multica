package workspaceguard

import (
	"reflect"
	"testing"
)

var (
	wantOwnerAdmin = []string{"owner", "admin"}
	wantOwnerOnly  = []string{"owner"}
)

const (
	routeWorkspace      = "/api/workspaces/{id}"
	routeLeave          = "/api/workspaces/{id}/leave"
	routeMemberRemove   = "/api/workspaces/{id}/members/{memberId}"
	routeInvitation     = "/api/workspaces/{id}/invitations/{invitationId}"
	routeShareLink      = "/api/workspaces/{id}/share-links/{linkId}"
	routeSubSummary     = "/api/cloud-subscriptions/summary"
	routeSubCheckout    = "/api/cloud-subscriptions/checkout-sessions"
	routeCompanyExport  = "/api/company/export"
	routeHubExport      = "/api/hub/export"
	routeIssues         = "/api/issues"
	routeIssue          = "/api/issues/{id}"
	routeBatchDelete    = "/api/issues/batch-delete"
	routeAgentArchive   = "/api/agents/{id}/archive"
	routeRuntimeUnbind  = "/api/runtimes/{runtimeId}/unbind-agents-and-delete"
	routeRuntimeArchive = "/api/runtimes/{runtimeId}/archive-agents-and-delete"
	routeChatArchive    = "/api/chat/sessions/{sessionId}/archive"
	routeAgentRestore   = "/api/agents/{id}/restore"
	routeInboxArchive   = "/api/inbox/{id}/archive"
)

var (
	allMethods   = []string{"GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE"}
	readMethods  = []string{"GET", "HEAD", "OPTIONS"}
	writeMethods = []string{"POST", "PUT", "PATCH", "DELETE"}
)

// route is one corpus entry: the pattern a request is matched on, and for each
// refusing status the methods the allow-list lets through and with which roles.
type route struct {
	pattern string
	// readOnly lists the methods allowed while read_only, beyond the reads that
	// read_only always allows and the DELETE that any resolved route allows.
	readOnly []string
	// restricted lists the methods allowed while suspended and pending_deletion,
	// and the roles that may use them (nil: any member).
	restricted map[string][]string
}

func methodsRoles(roles []string, methods ...string) map[string][]string {
	m := map[string][]string{}
	for _, method := range methods {
		m[method] = roles
	}
	return m
}

// corpus is the contract table of section 5, written out per route. Anything
// not named here is refused in every non-active status (except reads while
// read_only).
var corpus = []route{
	{pattern: routeWorkspace, restricted: methodsRoles(nil, "GET", "HEAD", "OPTIONS")},
	{pattern: routeLeave, readOnly: []string{"POST"}, restricted: methodsRoles(nil, "POST")},
	{pattern: routeMemberRemove},
	{pattern: routeInvitation},
	{pattern: routeShareLink},
	{pattern: routeSubSummary, readOnly: allMethods, restricted: methodsRoles(wantOwnerAdmin, allMethods...)},
	{pattern: routeSubCheckout, readOnly: allMethods, restricted: methodsRoles(wantOwnerAdmin, allMethods...)},
	{pattern: routeCompanyExport, restricted: methodsRoles(wantOwnerOnly, "GET", "HEAD", "OPTIONS")},
	{pattern: routeHubExport, restricted: methodsRoles(wantOwnerOnly, "GET", "HEAD", "OPTIONS")},
	{pattern: routeIssues},
	{pattern: routeIssue},
	{pattern: routeBatchDelete, readOnly: []string{"POST"}},
	{pattern: routeAgentArchive, readOnly: []string{"POST"}},
	{pattern: routeRuntimeUnbind, readOnly: []string{"POST"}},
	{pattern: routeRuntimeArchive, readOnly: []string{"POST"}},
	{pattern: routeChatArchive, readOnly: []string{"PATCH"}},
	{pattern: routeAgentRestore},
	{pattern: routeInboxArchive},
	{pattern: "/api/not/a/route"},
	{pattern: ""},
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func TestDecideMatrix(t *testing.T) {
	for _, rt := range corpus {
		for _, method := range allMethods {
			isRead := contains(readMethods, method)

			// active passes everything and needs no route.
			if d := Decide("active", method, rt.pattern); !d.Allowed || d.Code != "" || len(d.Roles) != 0 {
				t.Errorf("active %s %q = %+v, want a plain allow", method, rt.pattern, d)
			}

			// read_only: reads pass; a write passes only from the list.
			d := Decide("read_only", method, rt.pattern)
			wantAllow := isRead || contains(rt.readOnly, method) || (method == "DELETE" && rt.pattern != "")
			if d.Allowed != wantAllow {
				t.Errorf("read_only %s %q allowed = %v, want %v", method, rt.pattern, d.Allowed, wantAllow)
			}
			if !wantAllow && d.Code != "workspace_read_only" {
				t.Errorf("read_only %s %q code = %q, want workspace_read_only", method, rt.pattern, d.Code)
			}
			if wantAllow && (d.Code != "" || len(d.Roles) != 0) {
				t.Errorf("read_only %s %q allow carries %+v, want no code and no roles", method, rt.pattern, d)
			}

			// suspended and pending_deletion: only the restricted list passes.
			for status, code := range map[string]string{
				"suspended":        "workspace_suspended",
				"pending_deletion": "workspace_pending_deletion",
			} {
				d := Decide(status, method, rt.pattern)
				roles, listed := rt.restricted[method]
				if d.Allowed != listed {
					t.Errorf("%s %s %q allowed = %v, want %v", status, method, rt.pattern, d.Allowed, listed)
					continue
				}
				if !listed {
					if d.Code != code {
						t.Errorf("%s %s %q code = %q, want %q", status, method, rt.pattern, d.Code, code)
					}
					continue
				}
				if !reflect.DeepEqual(d.Roles, roles) && !(len(d.Roles) == 0 && len(roles) == 0) {
					t.Errorf("%s %s %q roles = %v, want %v", status, method, rt.pattern, d.Roles, roles)
				}
			}
		}
	}
}

func TestDecideRefusalCodeIsTheStatusCode(t *testing.T) {
	for status, code := range map[string]string{
		"read_only":        "workspace_read_only",
		"suspended":        "workspace_suspended",
		"pending_deletion": "workspace_pending_deletion",
	} {
		if got := CodeFor(status); got != code {
			t.Errorf("CodeFor(%q) = %q, want %q", status, got, code)
		}
	}
	if CodeFor("active") != "" {
		t.Errorf("CodeFor(active) = %q, want empty", CodeFor("active"))
	}
}

func TestDecideUnknownStatusRefusesAsSuspended(t *testing.T) {
	for _, status := range []string{"", "deleted", "ACTIVE", "archived"} {
		for _, method := range allMethods {
			d := Decide(status, method, routeIssues)
			if d.Allowed || d.Code != "workspace_suspended" {
				t.Errorf("status %q %s = %+v, want a workspace_suspended refusal", status, method, d)
			}
		}
		// An unknown status gets the suspended allow-list, not read_only's.
		if d := Decide(status, "GET", routeWorkspace); !d.Allowed {
			t.Errorf("status %q GET %s = %+v, want the suspended allow-list entry", status, routeWorkspace, d)
		}
		if d := Decide(status, "DELETE", routeIssue); d.Allowed {
			t.Errorf("status %q DELETE %s allowed; only read_only may delete", status, routeIssue)
		}
	}
}

func TestDecideEveryNamedRouteOfTheContract(t *testing.T) {
	// Each route the contract names for read_only, by method.
	readOnly := []struct{ method, pattern string }{
		{"POST", routeSubCheckout}, {"GET", routeSubSummary},
		{"DELETE", routeMemberRemove}, {"POST", routeLeave},
		{"DELETE", routeInvitation}, {"DELETE", routeShareLink},
		{"DELETE", routeWorkspace}, {"DELETE", routeIssue},
		{"POST", routeAgentArchive}, {"PATCH", routeChatArchive},
	}
	for _, c := range readOnly {
		if d := Decide("read_only", c.method, c.pattern); !d.Allowed {
			t.Errorf("read_only %s %s refused: %+v", c.method, c.pattern, d)
		}
	}
	// And for suspended and pending_deletion.
	restricted := []struct {
		method, pattern string
		roles           []string
	}{
		{"GET", routeWorkspace, nil},
		{"POST", routeSubCheckout, wantOwnerAdmin},
		{"GET", routeSubSummary, wantOwnerAdmin},
		{"POST", routeLeave, nil},
		{"GET", routeCompanyExport, wantOwnerOnly},
		{"GET", routeHubExport, wantOwnerOnly},
	}
	for _, status := range []string{"suspended", "pending_deletion"} {
		for _, c := range restricted {
			d := Decide(status, c.method, c.pattern)
			if !d.Allowed {
				t.Errorf("%s %s %s refused: %+v", status, c.method, c.pattern, d)
				continue
			}
			if !(len(d.Roles) == 0 && len(c.roles) == 0) && !reflect.DeepEqual(d.Roles, c.roles) {
				t.Errorf("%s %s %s roles = %v, want %v", status, c.method, c.pattern, d.Roles, c.roles)
			}
		}
	}
}

func TestDecideTrailingSlashIsIgnored(t *testing.T) {
	// chi reports a Route("/{id}") with a "/" leaf as ".../{id}/".
	if d := Decide("read_only", "DELETE", "/api/workspaces/{id}/"); !d.Allowed {
		t.Errorf("DELETE /api/workspaces/{id}/ refused while read_only: %+v", d)
	}
	if d := Decide("suspended", "GET", "/api/workspaces/{id}/"); !d.Allowed {
		t.Errorf("GET /api/workspaces/{id}/ refused while suspended: %+v", d)
	}
}

func TestDecideSubscriptionPrefixIsExact(t *testing.T) {
	for _, p := range []string{
		"/api/cloud-subscriptions-other", "/api/cloud-subscriptionsx/summary",
		"/api/cloud-billing/balance", "/api/x/api/cloud-subscriptions/summary",
	} {
		if d := Decide("read_only", "POST", p); d.Allowed {
			t.Errorf("read_only POST %q allowed", p)
		}
		if d := Decide("suspended", "GET", p); d.Allowed {
			t.Errorf("suspended GET %q allowed", p)
		}
	}
}

func TestDecideAWildcardRouteMatchesNothingMoreThanItNames(t *testing.T) {
	// A mount-level pattern such as /api/issues/* is not a leaf; the middleware
	// resolves leaves, so a bare mount pattern gets no allow-list entry.
	if d := Decide("read_only", "POST", "/api/issues/*"); d.Allowed {
		t.Errorf("POST /api/issues/* allowed while read_only: %+v", d)
	}
	if d := Decide("read_only", "POST", "/api/cloud-subscriptions/*"); !d.Allowed {
		t.Errorf("POST /api/cloud-subscriptions/* refused while read_only: %+v", d)
	}
}

func TestDecideAnUnlistedRouteIsRefusedInEveryNonActiveStatus(t *testing.T) {
	for _, status := range []string{"read_only", "suspended", "pending_deletion"} {
		for _, method := range writeMethods {
			if method == "DELETE" && status == "read_only" {
				continue // deleting resources is allowed while read_only
			}
			if d := Decide(status, method, routeIssues); d.Allowed {
				t.Errorf("%s %s %s allowed", status, method, routeIssues)
			}
		}
	}
	for _, status := range []string{"suspended", "pending_deletion"} {
		for _, method := range readMethods {
			if d := Decide(status, method, routeIssues); d.Allowed {
				t.Errorf("%s %s %s allowed", status, method, routeIssues)
			}
		}
	}
}

func TestMessageIsStablePerCode(t *testing.T) {
	for code, want := range map[string]string{
		CodeReadOnly:         "workspace is read-only",
		CodeSuspended:        "workspace is suspended",
		CodePendingDeletion:  "workspace is pending deletion",
		CodeStateUnavailable: "workspace state unavailable",
	} {
		if got := Message(code); got != want {
			t.Errorf("Message(%q) = %q, want %q", code, got, want)
		}
	}
}
