package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/entitlement/entitlementtest"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// policyWorkspace is a fresh workspace owned by the suite's user. Its policy
// row and audit rows are removed with it.
func policyWorkspace(t *testing.T) string {
	t.Helper()
	ws := planLimitWorkspace(t)
	useInvitationRateLimiters(t, InvitationRateLimiters{
		Actor:     &stubInvitationRateLimiter{allowed: true},
		Workspace: &stubInvitationRateLimiter{allowed: true},
		Recipient: &stubInvitationRateLimiter{allowed: true},
	})
	t.Cleanup(func() {
		ctx := context.Background()
		testPool.Exec(ctx, `DELETE FROM workspace_policy WHERE workspace_id = $1`, ws)
		testPool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1`, ws)
		testPool.Exec(ctx, `DELETE FROM workspace_invitation WHERE workspace_id = $1`, ws)
		testPool.Exec(ctx, `DELETE FROM workspace_share_link WHERE workspace_id = $1`, ws)
	})
	return ws
}

// setPolicyFeature says whether the plan of ws includes workspace_policies.
// Cloud sends `off` for a plan with the feature and `enforce` with limit 0 for
// one without.
func setPolicyFeature(t *testing.T, ws string, available bool) {
	t.Helper()
	stub := entitlementtest.New()
	if !available {
		zero := 0
		stub.Set(uuid.MustParse(ws), entitlement.GateWorkspacePolicies, entitlement.Decision{
			Gate: entitlement.Gate{Action: entitlement.ActionEnforce, Limit: &zero},
		})
	}
	prior := testHandler.Entitlements
	testHandler.Entitlements = stub
	t.Cleanup(func() { testHandler.Entitlements = prior })
}

// policyRouter mounts the policy routes with the role checks router.go uses.
func policyRouter() http.Handler {
	r := chi.NewRouter()
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.With(middleware.RequireWorkspaceRoleFromURL(testHandler.Queries, "id", "owner", "admin")).Get("/policy", testHandler.GetWorkspacePolicy)
		r.With(middleware.RequireWorkspaceRoleFromURL(testHandler.Queries, "id", "owner")).Put("/policy", testHandler.UpdateWorkspacePolicy)
	})
	return r
}

func doPolicy(t *testing.T, method, ws, userID string, body any) *testutil.Response {
	t.Helper()
	req := newRequest(method, "/api/workspaces/"+ws+"/policy", body)
	req.Header.Set("X-User-ID", userID)
	req.Header.Set("X-Workspace-ID", ws)
	return testutil.Call(t, policyRouter().ServeHTTP, req)
}

type policyBody struct {
	AllowedInviteDomains []string `json:"allowed_invite_domains"`
	ShareLinksEnabled    bool     `json:"share_links_enabled"`
	RequireSSO           bool     `json:"require_sso"`
	RequireMFA           bool     `json:"require_mfa"`
	SessionMaxAgeHours   int32    `json:"session_max_age_hours"`
	UpdatedBy            *string  `json:"updated_by"`
	UpdatedAt            *string  `json:"updated_at"`
}

type codedBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
	Gate  string `json:"gate"`
}

func wantPolicyCode(t *testing.T, resp *testutil.Response, status int, code string) {
	t.Helper()
	resp.Want(status)
	var body codedBody
	resp.JSON(&body)
	if body.Code != code || body.Error == "" {
		t.Fatalf("body = %s, want code %q with a message", resp.Text(), code)
	}
}

func putPolicy(t *testing.T, ws string, body map[string]any) policyBody {
	t.Helper()
	var out policyBody
	doPolicy(t, http.MethodPut, ws, testUserID, body).Want(http.StatusOK).JSON(&out)
	return out
}

func inviteRequest(ws, email string) *http.Request {
	req := newRequest("POST", "/api/workspaces/"+ws+"/members", CreateMemberRequest{Email: email, Role: "member"})
	req = withURLParam(req, "id", ws)
	req.Header.Set("X-Workspace-ID", ws)
	return req
}

func createLinkRequest(ws string) *http.Request {
	req := newRequest("POST", "/api/workspaces/"+ws+"/share-links", CreateShareLinkRequest{Role: "member"})
	req = withURLParam(req, "id", ws)
	req.Header.Set("X-Workspace-ID", ws)
	return req
}

func joinRequest(userID, code string) *http.Request {
	req := newRequest("POST", "/api/share-links/join", JoinByShareLinkRequest{Code: code})
	req.Header.Set("X-User-ID", userID)
	return req
}

func activeLinks(t *testing.T, ws string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM workspace_share_link WHERE workspace_id = $1 AND is_active`, ws)
}

// ---- the routes -----------------------------------------------------------

func TestPolicyReadReturnsDefaultsWhenNoRowExists(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	var got policyBody
	doPolicy(t, http.MethodGet, ws, testUserID, nil).Want(http.StatusOK).JSON(&got)
	if got.AllowedInviteDomains == nil || len(got.AllowedInviteDomains) != 0 || !got.ShareLinksEnabled ||
		got.RequireMFA || got.RequireSSO || got.SessionMaxAgeHours != 0 || got.UpdatedBy != nil {
		t.Fatalf("policy = %+v, want the defaults with an empty (not null) domain list", got)
	}
}

func TestPolicyWriteStoresAndReadsBack(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	verifiedDomain(t, ws, "example.com")
	put := putPolicy(t, ws, map[string]any{
		"allowed_invite_domains": []string{" Example.COM "},
		"share_links_enabled":    true,
		"require_mfa":            true,
		"require_sso":            true,
		"session_max_age_hours":  12,
	})
	var got policyBody
	doPolicy(t, http.MethodGet, ws, testUserID, nil).Want(http.StatusOK).JSON(&got)
	if len(got.AllowedInviteDomains) != 1 || got.AllowedInviteDomains[0] != "example.com" ||
		!got.RequireMFA || !got.RequireSSO || got.SessionMaxAgeHours != 12 || !got.ShareLinksEnabled {
		t.Fatalf("read back %+v after writing %+v", got, put)
	}
	if got.UpdatedBy == nil || *got.UpdatedBy != testUserID || got.UpdatedAt == nil {
		t.Fatalf("updated_by/updated_at = %v / %v, want the writer and a time", got.UpdatedBy, got.UpdatedAt)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace_policy WHERE workspace_id = $1`, ws); n != 1 {
		t.Fatalf("%d policy rows, want 1", n)
	}
}

func TestPolicyOmittedFieldsKeepTheirValue(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	verifiedDomain(t, ws, "example.com")
	putPolicy(t, ws, map[string]any{"require_mfa": true, "session_max_age_hours": 6})
	got := putPolicy(t, ws, map[string]any{"allowed_invite_domains": []string{"example.com"}})
	if !got.RequireMFA || got.SessionMaxAgeHours != 6 || len(got.AllowedInviteDomains) != 1 {
		t.Fatalf("policy = %+v, want the earlier fields kept", got)
	}
}

func TestPolicyRoles(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	admin := dbfx.User(t, "policy admin", testutil.UniqueEmail("policy-admin"))
	dbfx.Member(t, ws, admin, "admin")
	member := dbfx.User(t, "policy member", testutil.UniqueEmail("policy-member"))
	dbfx.Member(t, ws, member, "member")

	doPolicy(t, http.MethodGet, ws, admin, nil).Want(http.StatusOK)
	doPolicy(t, http.MethodGet, ws, testUserID, nil).Want(http.StatusOK)
	doPolicy(t, http.MethodGet, ws, member, nil).Want(http.StatusForbidden)
	doPolicy(t, http.MethodPut, ws, admin, map[string]any{"require_mfa": true}).Want(http.StatusForbidden)
	doPolicy(t, http.MethodPut, ws, member, map[string]any{"require_mfa": true}).Want(http.StatusForbidden)
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace_policy WHERE workspace_id = $1`, ws); n != 0 {
		t.Fatalf("a refused write left %d rows", n)
	}
}

func TestPolicyWriteValidation(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	for name, body := range map[string]map[string]any{
		"negative age":     {"session_max_age_hours": -1},
		"age above a year": {"session_max_age_hours": 8761},
		"bad domain":       {"allowed_invite_domains": []string{"not a domain"}},
		"an address":       {"allowed_invite_domains": []string{"a@example.com"}},
	} {
		t.Run(name, func(t *testing.T) {
			doPolicy(t, http.MethodPut, ws, testUserID, body).Want(http.StatusBadRequest)
		})
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace_policy WHERE workspace_id = $1`, ws); n != 0 {
		t.Fatalf("invalid writes left %d rows", n)
	}
}

// ---- plan gate ------------------------------------------------------------

func TestPolicyWriteIs402WithoutThePlanFeature(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, false)
	resp := doPolicy(t, http.MethodPut, ws, testUserID, map[string]any{"require_mfa": true})
	wantPolicyCode(t, resp, http.StatusPaymentRequired, "plan_feature_unavailable")
	var body codedBody
	resp.JSON(&body)
	if body.Gate != "workspace_policies" {
		t.Fatalf("gate = %q, want workspace_policies", body.Gate)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace_policy WHERE workspace_id = $1`, ws); n != 0 {
		t.Fatalf("a 402 write left %d rows", n)
	}
}

func TestPolicyReadWithoutThePlanFeatureReturnsDefaults(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	putPolicy(t, ws, map[string]any{"require_mfa": true, "share_links_enabled": false})
	setPolicyFeature(t, ws, false)
	var got policyBody
	doPolicy(t, http.MethodGet, ws, testUserID, nil).Want(http.StatusOK).JSON(&got)
	if got.RequireMFA || !got.ShareLinksEnabled {
		t.Fatalf("policy = %+v, want the defaults while the plan lacks the feature", got)
	}
}

// Enforcement follows the plan: a stored policy is not enforced once the plan
// loses the feature, and applies again when the feature returns.
func TestStoredPolicyIsNotEnforcedWithoutThePlanFeature(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	verifiedDomain(t, ws, "example.com")
	putPolicy(t, ws, map[string]any{"allowed_invite_domains": []string{"example.com"}, "share_links_enabled": false})

	setPolicyFeature(t, ws, false)
	testutil.Call(t, testHandler.CreateInvitation, inviteRequest(ws, testutil.UniqueEmail("anyone"))).Want(http.StatusCreated)
	testutil.Call(t, testHandler.CreateShareLink, createLinkRequest(ws)).Want(http.StatusCreated)

	setPolicyFeature(t, ws, true)
	wantPolicyCode(t, testutil.Call(t, testHandler.CreateShareLink, createLinkRequest(ws)), http.StatusForbidden, "share_links_disabled")
}

// ---- allowed_invite_domains -----------------------------------------------

func TestInvitationRefusesADomainOutsideThePolicy(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	verifiedDomain(t, ws, "example.test")
	putPolicy(t, ws, map[string]any{"allowed_invite_domains": []string{"example.test"}})

	for _, email := range []string{"a@other.org", "a@evilexample.test", "a@example.test.evil.io"} {
		wantPolicyCode(t, testutil.Call(t, testHandler.CreateInvitation, inviteRequest(ws, email)), http.StatusForbidden, "invite_domain_not_allowed")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace_invitation WHERE workspace_id = $1`, ws); n != 0 {
		t.Fatalf("%d invitations were created for refused domains", n)
	}
	for _, email := range []string{"a@example.test", "b@Dev.Example.TEST"} {
		testutil.Call(t, testHandler.CreateInvitation, inviteRequest(ws, email)).Want(http.StatusCreated)
	}
}

func TestInvitationAnyDomainWhenTheListIsEmpty(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	putPolicy(t, ws, map[string]any{"require_mfa": true})
	testutil.Call(t, testHandler.CreateInvitation, inviteRequest(ws, testutil.UniqueEmail("open"))).Want(http.StatusCreated)
}

func TestInvitationOtherWorkspaceIsUnaffected(t *testing.T) {
	restricted := policyWorkspace(t)
	other := policyWorkspace(t)
	setPolicyFeature(t, restricted, true)
	verifiedDomain(t, restricted, "example.test")
	putPolicy(t, restricted, map[string]any{"allowed_invite_domains": []string{"example.test"}})
	testutil.Call(t, testHandler.CreateInvitation, inviteRequest(other, "a@other.org")).Want(http.StatusCreated)
}

func TestJoinByShareLinkChecksTheJoinersDomain(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	link := createTestShareLink(t, ws, "member", 0, 0)
	verifiedDomain(t, ws, "example.test")
	putPolicy(t, ws, map[string]any{"allowed_invite_domains": []string{"example.test"}})

	outsider := dbfx.User(t, "outsider", "outsider-"+uuid.NewString()[:8]+"@other.org")
	wantPolicyCode(t, testutil.Call(t, testHandler.JoinByShareLink, joinRequest(outsider, link.Code)), http.StatusForbidden, "invite_domain_not_allowed")
	if n := dbfx.Count(t, `SELECT count(*) FROM member WHERE workspace_id = $1 AND user_id = $2`, ws, outsider); n != 0 {
		t.Fatalf("the refused joiner became a member")
	}
	if n := dbfx.Count(t, `SELECT use_count FROM workspace_share_link WHERE id = $1`, uuidToString(link.ID)); n != 0 {
		t.Fatalf("a refused join used the link %d times", n)
	}

	insider := dbfx.User(t, "insider", "insider-"+uuid.NewString()[:8]+"@example.test")
	testutil.Call(t, testHandler.JoinByShareLink, joinRequest(insider, link.Code)).Want(http.StatusOK)
}

// ---- share_links_enabled --------------------------------------------------

func TestShareLinksDisabledRefusesCreateAndJoin(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	putPolicy(t, ws, map[string]any{"share_links_enabled": false})
	// Inserted after the policy write, which deactivates the links that exist.
	link := createTestShareLink(t, ws, "member", 0, 0)

	wantPolicyCode(t, testutil.Call(t, testHandler.CreateShareLink, createLinkRequest(ws)), http.StatusForbidden, "share_links_disabled")

	joiner := dbfx.User(t, "joiner", testutil.UniqueEmail("joiner"))
	wantPolicyCode(t, testutil.Call(t, testHandler.JoinByShareLink, joinRequest(joiner, link.Code)), http.StatusForbidden, "share_links_disabled")
}

func TestTurningShareLinksOffDeactivatesActiveLinksOfThatWorkspaceOnly(t *testing.T) {
	ws := policyWorkspace(t)
	other := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	createTestShareLink(t, ws, "member", 0, 0)
	createTestShareLink(t, other, "member", 0, 0)
	if activeLinks(t, ws) != 1 || activeLinks(t, other) != 1 {
		t.Fatal("fixture: want one active link in each workspace")
	}

	putPolicy(t, ws, map[string]any{"share_links_enabled": false})
	if n := activeLinks(t, ws); n != 0 {
		t.Fatalf("%d active links remain in the workspace that turned them off", n)
	}
	if n := activeLinks(t, other); n != 1 {
		t.Fatalf("the other workspace has %d active links, want 1", n)
	}
}

func TestShareLinksTurnedBackOnAllowCreation(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	putPolicy(t, ws, map[string]any{"share_links_enabled": false})
	putPolicy(t, ws, map[string]any{"share_links_enabled": true})
	testutil.Call(t, testHandler.CreateShareLink, createLinkRequest(ws)).Want(http.StatusCreated)
}

// A write that leaves the links on does not touch them.
func TestWritingOtherFieldsKeepsActiveLinks(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	createTestShareLink(t, ws, "member", 0, 0)
	putPolicy(t, ws, map[string]any{"require_mfa": true})
	if n := activeLinks(t, ws); n != 1 {
		t.Fatalf("%d active links, want 1", n)
	}
}

// ---- audit ----------------------------------------------------------------

type policyAuditRow struct {
	Actor  string
	Target string
	Detail map[string]map[string]map[string]any
}

func policyAuditRows(t *testing.T, ws string) []policyAuditRow {
	t.Helper()
	rows, err := testPool.Query(context.Background(),
		`SELECT actor_id::text, target_id, detail FROM audit_event WHERE workspace_id = $1 AND action = 'policy.updated' ORDER BY created_at, id`, ws)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []policyAuditRow
	for rows.Next() {
		var r policyAuditRow
		var raw []byte
		if err := rows.Scan(&r.Actor, &r.Target, &raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &r.Detail); err != nil {
			t.Fatalf("detail %s: %v", raw, err)
		}
		out = append(out, r)
	}
	return out
}

func TestPolicyWriteAuditsOldAndNewOfChangedFields(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	verifiedDomain(t, ws, "example.com")
	putPolicy(t, ws, map[string]any{"require_mfa": true, "session_max_age_hours": 8})
	putPolicy(t, ws, map[string]any{"session_max_age_hours": 24, "allowed_invite_domains": []string{"example.com"}})

	rows := policyAuditRows(t, ws)
	if len(rows) != 2 {
		t.Fatalf("%d policy.updated rows, want 2", len(rows))
	}
	for _, r := range rows {
		if r.Actor != testUserID || r.Target != ws {
			t.Errorf("actor %s target %s, want the writer and the workspace", r.Actor, r.Target)
		}
	}
	first := rows[0].Detail["changes"]
	if first["require_mfa"]["old"] != false || first["require_mfa"]["new"] != true ||
		first["session_max_age_hours"]["old"] != float64(0) || first["session_max_age_hours"]["new"] != float64(8) {
		t.Errorf("first changes = %v", first)
	}
	second := rows[1].Detail["changes"]
	if _, has := second["require_mfa"]; has {
		t.Errorf("second row names an unchanged field: %v", second)
	}
	if second["session_max_age_hours"]["old"] != float64(8) || second["session_max_age_hours"]["new"] != float64(24) {
		t.Errorf("second changes = %v", second)
	}
	doms, _ := second["allowed_invite_domains"]["new"].([]any)
	oldDoms, _ := second["allowed_invite_domains"]["old"].([]any)
	if len(doms) != 1 || doms[0] != "example.com" || oldDoms == nil || len(oldDoms) != 0 {
		t.Errorf("domain change = %v", second["allowed_invite_domains"])
	}
}

func TestPolicyWriteWithNothingChangedWritesNoAuditRow(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	putPolicy(t, ws, map[string]any{"require_mfa": true})
	putPolicy(t, ws, map[string]any{"require_mfa": true})
	if n := len(policyAuditRows(t, ws)); n != 1 {
		t.Fatalf("%d audit rows, want 1", n)
	}
}

func TestRefusedPolicyWriteWritesNoAuditRow(t *testing.T) {
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, false)
	doPolicy(t, http.MethodPut, ws, testUserID, map[string]any{"require_mfa": true}).Want(http.StatusPaymentRequired)
	if n := len(policyAuditRows(t, ws)); n != 0 {
		t.Fatalf("%d audit rows after a 402", n)
	}
}
