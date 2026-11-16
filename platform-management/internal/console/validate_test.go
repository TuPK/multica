package console

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

func wantAPIError(t *testing.T, err *apiError, status int, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want %d %s", status, code)
	}
	if err.Status != status || err.Code != code {
		t.Fatalf("error = %d %s, want %d %s", err.Status, err.Code, status, code)
	}
}

func TestCleanReason(t *testing.T) {
	for _, in := range []string{"", " ", "\t\n ", "\u00a0\u2003"} {
		_, err := cleanReason(in)
		wantAPIError(t, err, 400, "reason_required")
	}
	got, err := cleanReason("  owner asked by phone \n")
	if err != nil || got != "owner asked by phone" {
		t.Fatalf("cleanReason = %q, %v", got, err)
	}
	if _, err := cleanReason(strings.Repeat("é", maxReasonChars)); err != nil {
		t.Fatalf("a reason of %d characters: %v", maxReasonChars, err)
	}
	_, err = cleanReason(strings.Repeat("é", maxReasonChars+1))
	wantAPIError(t, err, 400, "reason_too_long")
	_, err = cleanReason("a\x00b")
	wantAPIError(t, err, 400, "invalid_body")
}

func TestCleanNoteBody(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t"} {
		_, err := cleanNoteBody(in)
		wantAPIError(t, err, 400, "body_required")
	}
	got, err := cleanNoteBody("  called the owner \n")
	if err != nil || got != "called the owner" {
		t.Fatalf("cleanNoteBody = %q, %v", got, err)
	}
	// The limit counts characters, not bytes.
	if _, err := cleanNoteBody(strings.Repeat("语", maxNoteChars)); err != nil {
		t.Fatalf("a note of %d characters: %v", maxNoteChars, err)
	}
	_, err = cleanNoteBody(strings.Repeat("语", maxNoteChars+1))
	wantAPIError(t, err, 400, "body_too_long")
	_, err = cleanNoteBody("a\x00b")
	wantAPIError(t, err, 400, "invalid_body")
	if maxNoteChars != 4000 {
		t.Fatalf("maxNoteChars = %d, the contract says 4000", maxNoteChars)
	}
}

func TestParseTenantStatus(t *testing.T) {
	for in, want := range map[string]workspacestate.Status{
		"active":           workspacestate.Active,
		"read_only":        workspacestate.ReadOnly,
		"suspended":        workspacestate.Suspended,
		"pending_deletion": workspacestate.PendingDeletion,
	} {
		got, err := parseTenantStatus(in)
		if err != nil || got != want {
			t.Errorf("parseTenantStatus(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "Active", "deleted", "read-only", " active", "active "} {
		_, err := parseTenantStatus(in)
		wantAPIError(t, err, 400, "invalid_status")
	}
}

func TestParseAccountStatus(t *testing.T) {
	for _, in := range []string{"active", "suspended"} {
		got, err := parseAccountStatus(in)
		if err != nil || string(got) != in {
			t.Errorf("parseAccountStatus(%q) = %q, %v", in, got, err)
		}
	}
	// The other lifecycle states belong to account deletion, not to the console.
	for _, in := range []string{"", "pending_deletion", "deleted", "read_only", "Suspended"} {
		_, err := parseAccountStatus(in)
		wantAPIError(t, err, 400, "invalid_status")
	}
}

func TestParseStaffRole(t *testing.T) {
	for _, in := range []string{"support", "billing", "admin"} {
		got, err := parseStaffRole(in)
		if err != nil || string(got) != in {
			t.Errorf("parseStaffRole(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "root", "Admin", "owner"} {
		_, err := parseStaffRole(in)
		wantAPIError(t, err, 400, "invalid_role")
	}
}

func TestParseEmail(t *testing.T) {
	got, err := parseEmail("  Ops@Example.COM ")
	if err != nil || got != "ops@example.com" {
		t.Fatalf("parseEmail = %q, %v", got, err)
	}
	for _, in := range []string{"", "ops", "ops@", "@example.com", "Ops <ops@example.com>", "a@b.c, d@e.f", strings.Repeat("a", 250) + "@example.com"} {
		_, err := parseEmail(in)
		wantAPIError(t, err, 400, "invalid_email")
	}
}

func TestLikePattern(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		none bool
	}{
		{in: "", none: true},
		{in: "   ", none: true},
		{in: "acme", want: `%acme%`},
		{in: "  Acme Co ", want: `%Acme Co%`},
		{in: `50%_off\`, want: `%50\%\_off\\%`},
	} {
		got, err := likePattern(tc.in)
		if err != nil {
			t.Fatalf("likePattern(%q): %v", tc.in, err)
		}
		if tc.none != (got == nil) || (got != nil && *got != tc.want) {
			t.Errorf("likePattern(%q) = %v, want %q (none=%v)", tc.in, got, tc.want, tc.none)
		}
	}
	_, err := likePattern("a\x00b")
	wantAPIError(t, err, 400, "invalid_filter")
}

func TestParseListParams(t *testing.T) {
	p, err := parseListParams(url.Values{})
	if err != nil || p.limit != 50 || p.cursor != nil {
		t.Fatalf("defaults = %+v, %v", p, err)
	}
	p, err = parseListParams(url.Values{"limit": {"100"}})
	if err != nil || p.limit != 100 {
		t.Fatalf("limit 100 = %+v, %v", p, err)
	}
	for _, in := range []string{"0", "101", "-1", "x", ""} {
		_, err := parseListParams(url.Values{"limit": {in}})
		wantAPIError(t, err, 400, "invalid_filter")
	}
	for _, in := range []string{"", "!!", "bm90LWEtY3Vyc29y"} {
		_, err := parseListParams(url.Values{"cursor": {in}})
		wantAPIError(t, err, 400, "invalid_cursor")
	}
	at := time.Date(2030, 1, 2, 3, 4, 5, 6000, time.UTC)
	id := "11111111-2222-3333-4444-555555555555"
	p, err = parseListParams(url.Values{"cursor": {encodeCursor(at, id)}})
	if err != nil || p.cursor == nil || !p.cursor.CreatedAt.Equal(at) || p.cursor.ID != id {
		t.Fatalf("cursor round trip = %+v, %v", p.cursor, err)
	}
}

func TestParseAuditParams(t *testing.T) {
	p, err := parseAuditParams(url.Values{
		"action":   {"staff.added"},
		"actor_id": {"abc"},
		"from":     {"2030-01-01T00:00:00Z"},
		"to":       {"2030-02-01T00:00:00+02:00"},
		"limit":    {"7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.action == nil || *p.action != "staff.added" || p.actorID == nil || *p.actorID != "abc" || p.limit != 7 {
		t.Fatalf("params = %+v", p)
	}
	if p.from == nil || p.to == nil || p.to.Location() != time.UTC {
		t.Fatalf("times = %v %v", p.from, p.to)
	}

	for name, q := range map[string]url.Values{
		"unknown action":  {"action": {"workspace.exploded"}},
		"empty actor":     {"actor_id": {""}},
		"bad from":        {"from": {"yesterday"}},
		"bad to":          {"to": {"2030-01-01"}},
		"from after to":   {"from": {"2030-02-01T00:00:00Z"}, "to": {"2030-01-01T00:00:00Z"}},
		"limit too large": {"limit": {"500"}},
	} {
		_, err := parseAuditParams(q)
		if err == nil || err.Status != 400 || err.Code != "invalid_filter" {
			t.Errorf("%s: error = %v, want 400 invalid_filter", name, err)
		}
	}
	_, err = parseAuditParams(url.Values{"cursor": {"x"}})
	wantAPIError(t, err, 400, "invalid_cursor")
}

// Every action the console writes can be used as a filter.
func TestAuditActionFilterAcceptsEveryActionTheConsoleWrites(t *testing.T) {
	for _, a := range []string{
		"workspace.status_changed", "account.status_changed", "account.sessions_revoked",
		"staff.added", "staff.removed", "note.added", "member.removed", "auth.login",
	} {
		if !isAuditAction(a) {
			t.Errorf("%s is not accepted as a filter", a)
		}
	}
}

// The filter also takes the actions the other staff routes write (plan,
// subscription and billing) and the R2 actions Multica writes, so a staff
// member can find them in the cross-workspace log. The names come from the
// audit package: a literal such as an agent action would read as a table name
// to the isolation scan.
func TestAuditActionFilterAcceptsTheR2AndBillingActions(t *testing.T) {
	for _, a := range []string{
		audit.ActionSubscriptionChanged, audit.ActionPlanChanged,
		audit.ActionBillingOrderCreated, audit.ActionBillingOrderPaid,
		audit.ActionBillingOrderExpired, audit.ActionBillingOrderCanceled,
		audit.ActionIntegrationConnected, audit.ActionIntegrationDisconnected,
		audit.ActionPluginInstalled, audit.ActionPluginUninstalled, audit.ActionPluginEnabled,
		audit.ActionPluginDisabled, audit.ActionPluginConfigured, audit.ActionPluginTokenRotated,
		audit.ActionMCPServerCreated, audit.ActionMCPServerUpdated, audit.ActionMCPServerDeleted,
		audit.ActionRuntimeProfileCreated, audit.ActionRuntimeProfileUpdated, audit.ActionRuntimeProfileDeleted,
		audit.ActionAgentCreated, audit.ActionAgentArchived, audit.ActionAgentPermissionChanged, audit.ActionAgentEnvUpdated,
		audit.ActionRuntimeVisibilityChanged, audit.ActionRuntimeDeleted,
		audit.ActionWorkspaceExportRequested, audit.ActionWorkspaceExportCompleted, audit.ActionWorkspaceExportFailed,
		audit.ActionWorkspaceDeletionScheduled, audit.ActionWorkspaceDeletionCancelled,
	} {
		if !isAuditAction(a) {
			t.Errorf("%s is not accepted as a filter", a)
		}
	}
}

// The filter reads the audit package's catalogue, so every action in it is
// accepted and nothing else is.
func TestAuditActionFilterIsTheAuditCatalog(t *testing.T) {
	if len(auditActions) != len(audit.Catalog) {
		t.Errorf("filter holds %d actions, the catalogue %d", len(auditActions), len(audit.Catalog))
	}
	for _, a := range audit.Catalog {
		if !isAuditAction(a) {
			t.Errorf("%s is in the catalogue and not accepted", a)
		}
	}
	if isAuditAction("nope") {
		t.Error("an action outside the catalogue is accepted")
	}
}

// The role matrix of contract section 2, from the route table.
func TestRouteRoles(t *testing.T) {
	anyStaff := []staff.Role(nil)
	supportAdmin := []staff.Role{staff.RoleSupport, staff.RoleAdmin}
	admin := []staff.Role{staff.RoleAdmin}
	billingAdmin := []staff.Role{staff.RoleBilling, staff.RoleAdmin}
	want := map[string][]staff.Role{
		"/api/platform/tenants":                    anyStaff,
		"/api/platform/tenants/{id}":               anyStaff,
		"/api/platform/tenants/{id}/status":        supportAdmin,
		"/api/platform/tenants/{id}/audit-events":  anyStaff,
		"/api/platform/tenants/{id}/notes":         anyStaff,
		"/api/platform/users":                      anyStaff,
		"/api/platform/users/{id}":                 anyStaff,
		"/api/platform/users/{id}/status":          supportAdmin,
		"/api/platform/users/{id}/revoke-sessions": supportAdmin,
		"/api/platform/users/{id}/mfa-reset":       supportAdmin,
		"/api/platform/staff":                      admin,
		"/api/platform/staff/{user_id}":            admin,
		"/api/platform/audit-events":               anyStaff,
		"/api/platform/overview":                   anyStaff,
		"/api/platform/orders":                     billingAdmin,
		"/api/platform/orders/{id}/confirm":        billingAdmin,
		"/api/platform/orders/{id}/cancel":         billingAdmin,
	}
	got := map[string][]staff.Role{}
	for _, r := range routeTable() {
		got[r.pattern] = r.roles
	}
	if len(got) != len(want) {
		t.Fatalf("route table has %d patterns, want %d", len(got), len(want))
	}
	for pattern, roles := range want {
		g, ok := got[pattern]
		if !ok {
			t.Errorf("%s is not routed", pattern)
			continue
		}
		if strings.Join(roleNames(g), ",") != strings.Join(roleNames(roles), ",") {
			t.Errorf("%s roles = %v, want %v", pattern, g, roles)
		}
	}
}

func roleNames(rs []staff.Role) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}
