package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/entitlement/entitlementtest"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/sso"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// fakeTXT answers TXT lookups from a map; a name it does not hold is a
// "not found" answer, as a real resolver gives. Nothing here touches the network.
type fakeTXT struct {
	records map[string][]string
	err     error
	asked   []string
}

func (f *fakeTXT) LookupTXT(_ context.Context, name string) ([]string, error) {
	f.asked = append(f.asked, name)
	if f.err != nil {
		return nil, f.err
	}
	recs, ok := f.records[name]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	return recs, nil
}

type domainBody struct {
	Domain     string  `json:"domain"`
	TXTName    string  `json:"txt_name"`
	TXTValue   string  `json:"txt_value"`
	VerifiedAt *string `json:"verified_at"`
}

// domainWorkspace is a fresh workspace owned by the suite's user, and the
// sso_domain rows it gets are removed with it.
func domainWorkspace(t *testing.T) string {
	t.Helper()
	ws := planLimitWorkspace(t)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM sso_domain WHERE workspace_id = $1`, ws)
	})
	return ws
}

func uniqueDomain(label string) string {
	return label + "-" + uuid.NewString()[:8] + ".example.test"
}

// verifiedDomain stores domain as verified for ws, as a finished verification
// leaves it.
func verifiedDomain(t *testing.T, ws, domain string) {
	t.Helper()
	dbfx.Exec(t, `INSERT INTO sso_domain (workspace_id, domain, token, verified_at) VALUES ($1, $2, 'seeded', now())`, ws, domain)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM sso_domain WHERE workspace_id = $1 AND domain = $2`, ws, domain)
	})
}

func domainRouter(lookup sso.TXTLookup) http.Handler {
	d := NewSSODomainHandler(testHandler, lookup)
	r := chi.NewRouter()
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspaceRoleFromURL(testHandler.Queries, "id", "owner"))
			d.Routes(r)
		})
	})
	return r
}

func doDomain(t *testing.T, lookup sso.TXTLookup, method, ws, userID, path string, body any) *testutil.Response {
	t.Helper()
	req := newRequest(method, "/api/workspaces/"+ws+"/sso/domains"+path, body)
	req.Header.Set("X-User-ID", userID)
	req.Header.Set("X-Workspace-ID", ws)
	return testutil.Call(t, domainRouter(lookup).ServeHTTP, req)
}

func addDomain(t *testing.T, ws, domain string) domainBody {
	t.Helper()
	var out domainBody
	doDomain(t, &fakeTXT{}, http.MethodPost, ws, testUserID, "", map[string]any{"domain": domain}).Want(http.StatusCreated).JSON(&out)
	return out
}

// setSSOFeature says whether the plan of ws includes sso. Cloud sends `off` for
// a plan with the feature and `enforce` with limit 0 for one without.
func setSSOFeature(t *testing.T, ws string, available bool) {
	t.Helper()
	stub := entitlementtest.New()
	if !available {
		zero := 0
		stub.Set(uuid.MustParse(ws), entitlement.GateSSO, entitlement.Decision{
			Gate: entitlement.Gate{Action: entitlement.ActionEnforce, Limit: &zero},
		})
	}
	prior := testHandler.Entitlements
	testHandler.Entitlements = stub
	t.Cleanup(func() { testHandler.Entitlements = prior })
}

// ---- contract item 1 ------------------------------------------------------

func TestDomainAlreadyVerifiedByAnotherWorkspaceIsRefused(t *testing.T) {
	a, b := domainWorkspace(t), domainWorkspace(t)
	domain := uniqueDomain("taken")
	verifiedDomain(t, a, domain)

	wantPolicyCode(t, doDomain(t, &fakeTXT{}, http.MethodPost, b, testUserID, "", map[string]any{"domain": domain}),
		http.StatusConflict, "domain_already_verified")
	if n := dbfx.Count(t, `SELECT count(*) FROM sso_domain WHERE workspace_id = $1`, b); n != 0 {
		t.Fatalf("a refused add left %d rows", n)
	}
}

func TestDomainVerifiedByAnotherWorkspaceWhileThisOneWasPending(t *testing.T) {
	a, b := domainWorkspace(t), domainWorkspace(t)
	domain := uniqueDomain("race")
	claimB := addDomain(t, b, domain)
	claimA := addDomain(t, a, domain)

	doDomain(t, &fakeTXT{records: map[string][]string{claimA.TXTName: {claimA.TXTValue}}},
		http.MethodPost, a, testUserID, "/"+domain+"/verify", nil).Want(http.StatusOK)

	lookup := &fakeTXT{records: map[string][]string{claimB.TXTName: {claimB.TXTValue}}}
	wantPolicyCode(t, doDomain(t, lookup, http.MethodPost, b, testUserID, "/"+domain+"/verify", nil),
		http.StatusConflict, "domain_already_verified")
	if n := dbfx.Count(t, `SELECT count(*) FROM sso_domain WHERE domain = $1 AND verified_at IS NOT NULL`, domain); n != 1 {
		t.Fatalf("%d verified rows for %s, want exactly 1", n, domain)
	}
}

// ---- contract item 2 ------------------------------------------------------

func TestDomainPublicEmailDomainIsRefused(t *testing.T) {
	ws := domainWorkspace(t)
	for _, domain := range []string{"gmail.com", "Gmail.com", "outlook.com", "mail.yahoo.com"} {
		wantPolicyCode(t, doDomain(t, &fakeTXT{}, http.MethodPost, ws, testUserID, "", map[string]any{"domain": domain}),
			http.StatusBadRequest, "public_email_domain")
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM sso_domain WHERE workspace_id = $1`, ws); n != 0 {
		t.Fatalf("a refused add left %d rows", n)
	}
}

func TestDomainInvalidDomainIsRefused(t *testing.T) {
	ws := domainWorkspace(t)
	for _, domain := range []string{"", "localhost", "a b.com", "a@example.com", "example.com/x"} {
		wantPolicyCode(t, doDomain(t, &fakeTXT{}, http.MethodPost, ws, testUserID, "", map[string]any{"domain": domain}),
			http.StatusBadRequest, "invalid_domain")
	}
}

// ---- contract item 3 ------------------------------------------------------

func TestDomainWrongOrMissingTXTDoesNotVerifyAndTheRightOneDoes(t *testing.T) {
	ws := domainWorkspace(t)
	domain := uniqueDomain("txt")
	claim := addDomain(t, ws, domain)
	if claim.VerifiedAt != nil || claim.TXTName == "" || claim.TXTValue == "" {
		t.Fatalf("a new claim = %+v, want pending with the record to publish", claim)
	}
	verified := func() int {
		return dbfx.Count(t, `SELECT count(*) FROM sso_domain WHERE workspace_id = $1 AND domain = $2 AND verified_at IS NOT NULL`, ws, domain)
	}

	for label, lookup := range map[string]*fakeTXT{
		"no record":        {},
		"a wrong token":    {records: map[string][]string{claim.TXTName: {sso.ChallengeValue("not-the-token")}}},
		"the wrong name":   {records: map[string][]string{domain: {claim.TXTValue}}},
		"an unrelated TXT": {records: map[string][]string{claim.TXTName: {"v=spf1 -all"}}},
	} {
		wantPolicyCode(t, doDomain(t, lookup, http.MethodPost, ws, testUserID, "/"+domain+"/verify", nil),
			http.StatusUnprocessableEntity, "txt_record_not_found")
		if verified() != 0 {
			t.Fatalf("%s verified the domain", label)
		}
	}

	wantPolicyCode(t, doDomain(t, &fakeTXT{err: errors.New("resolver down")}, http.MethodPost, ws, testUserID, "/"+domain+"/verify", nil),
		http.StatusBadGateway, "dns_lookup_failed")
	if verified() != 0 {
		t.Fatal("a resolver failure verified the domain")
	}

	right := &fakeTXT{records: map[string][]string{claim.TXTName: {"v=spf1 -all", claim.TXTValue}}}
	var got domainBody
	doDomain(t, right, http.MethodPost, ws, testUserID, "/"+domain+"/verify", nil).Want(http.StatusOK).JSON(&got)
	if got.VerifiedAt == nil || verified() != 1 {
		t.Fatalf("the right record did not verify: %+v", got)
	}
	if len(right.asked) != 1 || right.asked[0] != claim.TXTName {
		t.Fatalf("asked %v, want only %q", right.asked, claim.TXTName)
	}
}

func TestDomainAddingAgainKeepsTheToken(t *testing.T) {
	ws := domainWorkspace(t)
	domain := uniqueDomain("again")
	first := addDomain(t, ws, domain)
	var second domainBody
	doDomain(t, &fakeTXT{}, http.MethodPost, ws, testUserID, "", map[string]any{"domain": domain}).Want(http.StatusCreated).JSON(&second)
	if second.TXTValue != first.TXTValue {
		t.Fatalf("token changed from %q to %q", first.TXTValue, second.TXTValue)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM sso_domain WHERE workspace_id = $1`, ws); n != 1 {
		t.Fatalf("%d rows, want 1", n)
	}
}

func TestDomainTokenIsPerWorkspaceAndDomain(t *testing.T) {
	a, b := domainWorkspace(t), domainWorkspace(t)
	d1, d2 := uniqueDomain("tok1"), uniqueDomain("tok2")
	seen := map[string]bool{}
	for _, claim := range []domainBody{addDomain(t, a, d1), addDomain(t, a, d2), addDomain(t, b, d1)} {
		if seen[claim.TXTValue] {
			t.Fatalf("token %q was issued twice", claim.TXTValue)
		}
		seen[claim.TXTValue] = true
	}
}

func TestDomainRemoveDropsTheClaim(t *testing.T) {
	ws := domainWorkspace(t)
	domain := uniqueDomain("gone")
	addDomain(t, ws, domain)
	doDomain(t, &fakeTXT{}, http.MethodDelete, ws, testUserID, "/"+domain, nil).Want(http.StatusNoContent)
	if n := dbfx.Count(t, `SELECT count(*) FROM sso_domain WHERE workspace_id = $1`, ws); n != 0 {
		t.Fatalf("%d rows left", n)
	}
	doDomain(t, &fakeTXT{}, http.MethodDelete, ws, testUserID, "/"+domain, nil).Want(http.StatusNotFound)
}

// ---- contract item 4 ------------------------------------------------------

func TestDomainReadableByTheOwnerOnly(t *testing.T) {
	ws, other := domainWorkspace(t), domainWorkspace(t)
	domain, otherDomain := uniqueDomain("mine"), uniqueDomain("theirs")
	addDomain(t, ws, domain)
	addDomain(t, other, otherDomain)
	admin := dbfx.User(t, "domain admin", testutil.UniqueEmail("domain-admin"))
	dbfx.Member(t, ws, admin, "admin")
	member := dbfx.User(t, "domain member", testutil.UniqueEmail("domain-member"))
	dbfx.Member(t, ws, member, "member")
	outsider := dbfx.User(t, "domain outsider", testutil.UniqueEmail("domain-outsider"))

	var list struct {
		Domains []domainBody `json:"domains"`
	}
	doDomain(t, &fakeTXT{}, http.MethodGet, ws, testUserID, "", nil).Want(http.StatusOK).JSON(&list)
	if len(list.Domains) != 1 || list.Domains[0].Domain != domain {
		t.Fatalf("owner list = %+v, want only %s", list.Domains, domain)
	}
	for label, user := range map[string]string{"admin": admin, "member": member, "outsider": outsider} {
		for _, call := range []struct{ method, path string }{
			{http.MethodGet, ""},
			{http.MethodPost, ""},
			{http.MethodPost, "/" + domain + "/verify"},
			{http.MethodDelete, "/" + domain},
		} {
			resp := doDomain(t, &fakeTXT{}, call.method, ws, user, call.path, map[string]any{"domain": uniqueDomain("x")})
			if resp.Code != http.StatusForbidden && resp.Code != http.StatusNotFound {
				t.Errorf("%s %s as %s = %d, want 403 or 404", call.method, call.path, label, resp.Code)
			}
		}
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM sso_domain WHERE workspace_id = $1`, ws); n != 1 {
		t.Fatalf("refused calls changed the rows: %d, want 1", n)
	}
}

func TestDomainPolicyAcceptsOnlyVerifiedDomains(t *testing.T) {
	ws, other := policyWorkspace(t), policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	for _, w := range []string{ws, other} {
		w := w
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM sso_domain WHERE workspace_id = $1`, w) })
	}
	mine, pending, theirs := uniqueDomain("mine"), uniqueDomain("pending"), uniqueDomain("theirs")
	verifiedDomain(t, ws, mine)
	dbfx.Exec(t, `INSERT INTO sso_domain (workspace_id, domain, token) VALUES ($1, $2, 'pending')`, ws, pending)
	verifiedDomain(t, other, theirs)

	for label, domains := range map[string][]string{
		"an unknown domain":                {uniqueDomain("unknown")},
		"a pending domain":                 {pending},
		"another workspace's verified one": {theirs},
		"one verified and one pending":     {mine, pending},
	} {
		wantPolicyCode(t, doPolicy(t, http.MethodPut, ws, testUserID, map[string]any{"allowed_invite_domains": domains}),
			http.StatusBadRequest, "domain_not_verified")
		if n := dbfx.Count(t, `SELECT count(*) FROM workspace_policy WHERE workspace_id = $1`, ws); n != 0 {
			t.Fatalf("%s: a refused write stored a policy", label)
		}
	}

	got := putPolicy(t, ws, map[string]any{"allowed_invite_domains": []string{" " + mine + " "}})
	if len(got.AllowedInviteDomains) != 1 || got.AllowedInviteDomains[0] != mine {
		t.Fatalf("a verified domain was not accepted: %+v", got)
	}
	// Other fields and an empty list never need a verified domain.
	putPolicy(t, ws, map[string]any{"require_mfa": true})
	if got := putPolicy(t, ws, map[string]any{"allowed_invite_domains": []string{}}); len(got.AllowedInviteDomains) != 0 {
		t.Fatalf("an empty list was not accepted: %+v", got)
	}
}

// ---- the sso plan feature -------------------------------------------------

func TestDomainAddAndVerifyNeedTheSSOPlanFeature(t *testing.T) {
	ws := domainWorkspace(t)
	domain := uniqueDomain("plan")
	claim := addDomain(t, ws, domain)
	setSSOFeature(t, ws, false)

	for _, call := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "", map[string]any{"domain": uniqueDomain("plan2")}},
		{http.MethodPost, "/" + domain + "/verify", nil},
	} {
		lookup := &fakeTXT{records: map[string][]string{claim.TXTName: {claim.TXTValue}}}
		resp := doDomain(t, lookup, call.method, ws, testUserID, call.path, call.body)
		resp.Want(http.StatusPaymentRequired)
		var body codedBody
		resp.JSON(&body)
		if body.Code != "plan_feature_unavailable" || body.Gate != "sso" {
			t.Fatalf("%s %s: body = %s", call.method, call.path, resp.Text())
		}
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM sso_domain WHERE workspace_id = $1 AND verified_at IS NOT NULL`, ws); n != 0 {
		t.Fatal("a refused verify verified the domain")
	}
	// Reading and removing stay open so an owner can clean up.
	doDomain(t, &fakeTXT{}, http.MethodGet, ws, testUserID, "", nil).Want(http.StatusOK)
	doDomain(t, &fakeTXT{}, http.MethodDelete, ws, testUserID, "/"+domain, nil).Want(http.StatusNoContent)
}
