package subscription

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

const profilePath = "/api/v1/subscriptions/" + wsID + "/billing-profile"

type profileAPIEnv struct {
	members  *fakeMembers
	profiles *fakeProfiles
	h        http.Handler
}

func newProfileAPIEnv(t *testing.T) *profileAPIEnv {
	t.Helper()
	e := &profileAPIEnv{
		members: &fakeMembers{roles: map[string]string{
			wsID + "/" + ownerID: "owner", wsID + "/" + adminID: "admin", wsID + "/" + memberID: "member",
			wsID + "/" + viewerID: "viewer",
		}},
		profiles: newFakeProfiles(t),
	}
	api := NewProfileAPI(ProfileAPIDeps{
		Service: NewProfileService(e.profiles), Members: e.members,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	e.h = api.Handler(trust.New(trust.DefaultCIDRs()).RequireUser)
	return e
}

const viewerID = "aaaaaaaa-1111-0000-0000-000000000005"

func (e *profileAPIEnv) put(path, user string, body any) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != nil {
		raw, ok := body.([]byte)
		if !ok {
			var err error
			if raw, err = json.Marshal(body); err != nil {
				panic(err)
			}
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(http.MethodPut, path, rd)
	req.RemoteAddr = "10.0.0.7:4000"
	if user != "" {
		req.Header.Set(trust.UserHeader, user)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func bodyOf(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return m
}

func wantRefusal(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) map[string]any {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d: %s", rec.Code, status, rec.Body)
	}
	m := bodyOf(t, rec)
	if m["code"] != code {
		t.Fatalf("code = %v, want %q: %s", m["code"], code, rec.Body)
	}
	return m
}

func businessBody() map[string]any {
	return map[string]any{
		"account_kind": "business", "legal_name": "Acme Co", "tax_code": "0123456789",
		"address": "1 Main St", "invoice_email": "ap@acme.test",
	}
}

func TestProfileAPIChangesToBusinessAndWritesTheAuditRow(t *testing.T) {
	e := newProfileAPIEnv(t)
	rec := e.put(profilePath, ownerID, businessBody())
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	m := bodyOf(t, rec)
	if m["account_kind"] != "business" || m["legal_name"] != "Acme Co" || m["tax_code"] != "0123456789" ||
		m["address"] != "1 Main St" || m["invoice_email"] != "ap@acme.test" || m["verified"] != false || m["changed"] != true {
		t.Errorf("body = %v", m)
	}
	if _, ok := m["verified_by"]; ok {
		t.Error("the body carries verified_by")
	}
	stored := e.profiles.profiles[wsID]
	if stored.AccountKind != AccountBusiness || stored.UpdatedBy != ownerID {
		t.Errorf("stored = %+v", stored)
	}
	if len(e.profiles.audits) != 1 {
		t.Fatalf("audit rows = %d, want 1", len(e.profiles.audits))
	}
	a := e.profiles.audits[0]
	if a.Action != audit.ActionWorkspaceAccountKindChanged || a.WorkspaceID != wsID ||
		a.ActorType != audit.ActorMember || a.ActorID != ownerID {
		t.Errorf("audit = %+v", a)
	}
}

func TestProfileAPIAdminMayChangeTheKind(t *testing.T) {
	e := newProfileAPIEnv(t)
	if rec := e.put(profilePath, adminID, businessBody()); rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
}

func TestProfileAPIRefusesAMemberAndAViewer(t *testing.T) {
	for name, user := range map[string]string{"member": memberID, "viewer": viewerID} {
		e := newProfileAPIEnv(t)
		wantRefusal(t, e.put(profilePath, user, businessBody()), 403, "forbidden")
		if len(e.profiles.audits) != 0 || len(e.profiles.profiles) != 0 || e.profiles.txs != 0 {
			t.Errorf("%s: a refused caller changed something: %d audits, %d profiles, %d transactions",
				name, len(e.profiles.audits), len(e.profiles.profiles), e.profiles.txs)
		}
	}
}

func TestProfileAPIHidesTheWorkspaceFromANonMember(t *testing.T) {
	e := newProfileAPIEnv(t)
	wantRefusal(t, e.put(profilePath, strangID, businessBody()), 404, "not_found")
	if e.profiles.txs != 0 {
		t.Error("a non-member opened a transaction")
	}
}

func TestProfileAPINeedsATrustedUser(t *testing.T) {
	e := newProfileAPIEnv(t)
	if rec := e.put(profilePath, "", businessBody()); rec.Code != 401 {
		t.Errorf("no X-User-ID: status %d, want 401", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPut, profilePath, strings.NewReader(`{"account_kind":"business"}`))
	req.RemoteAddr = "203.0.113.9:4000"
	req.Header.Set(trust.UserHeader, ownerID)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code == 200 {
		t.Error("an untrusted peer was served")
	}
}

func TestProfileAPIValidatesTheBody(t *testing.T) {
	long := strings.Repeat("x", 1025)
	cases := []struct {
		name   string
		body   any
		status int
		code   string
		fields []any
	}{
		{"not json", []byte("nope"), 400, "billing_profile_invalid", nil},
		{"no kind", map[string]any{"legal_name": "Acme"}, 400, "billing_profile_invalid", []any{"account_kind"}},
		{"unknown kind", map[string]any{"account_kind": "enterprise"}, 400, "billing_profile_invalid", []any{"account_kind"}},
		{"a field of the wrong type", map[string]any{"account_kind": "business", "legal_name": 7}, 400, "billing_profile_invalid", nil},
		{"tax code with letters", withField(businessBody(), "tax_code", "01234abcde"), 400, "billing_profile_invalid", []any{"tax_code"}},
		{"tax code of 9 digits", withField(businessBody(), "tax_code", "012345678"), 400, "billing_profile_invalid", []any{"tax_code"}},
		{"address too long", withField(businessBody(), "address", long), 400, "billing_profile_invalid", []any{"address"}},
		{"legal name too long", withField(businessBody(), "legal_name", long), 400, "billing_profile_invalid", []any{"legal_name"}},
		{"email without an address", withField(businessBody(), "invoice_email", "not an email"), 400, "billing_profile_invalid", []any{"invoice_email"}},
		{"email with a display name", withField(businessBody(), "invoice_email", "Acme <ap@acme.test>"), 400, "billing_profile_invalid", []any{"invoice_email"}},
		{"missing fields", map[string]any{"account_kind": "business", "legal_name": "Acme"}, 409, "billing_profile_incomplete", []any{"tax_code", "address", "invoice_email"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newProfileAPIEnv(t)
			m := wantRefusal(t, e.put(profilePath, ownerID, c.body), c.status, c.code)
			if c.fields != nil {
				got, _ := m["fields"].([]any)
				if len(got) != len(c.fields) {
					t.Fatalf("fields = %v, want %v", m["fields"], c.fields)
				}
				for i := range got {
					if got[i] != c.fields[i] {
						t.Fatalf("fields = %v, want %v", m["fields"], c.fields)
					}
				}
			}
			if len(e.profiles.profiles) != 0 || len(e.profiles.audits) != 0 {
				t.Error("a refused body wrote something")
			}
		})
	}
}

func withField(m map[string]any, k string, v any) map[string]any {
	m[k] = v
	return m
}

func TestProfileAPIOfABadWorkspaceIDIsNotFound(t *testing.T) {
	e := newProfileAPIEnv(t)
	wantRefusal(t, e.put("/api/v1/subscriptions/acme/billing-profile", ownerID, businessBody()), 404, "not_found")
}

func TestProfileAPIRefusesGoingBackOnABusinessOnlyPlan(t *testing.T) {
	e := newProfileAPIEnv(t)
	e.profiles.profiles[wsID] = BillingProfile{WorkspaceID: wsID, AccountKind: AccountBusiness}
	e.profiles.subs[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "business", Status: StatusActive, Seats: 5}
	m := wantRefusal(t, e.put(profilePath, ownerID, map[string]any{"account_kind": "individual"}), 409, "plan_audience_mismatch")
	if m["plan_key"] != "business" || m["account_kind"] != "individual" {
		t.Errorf("body = %v", m)
	}
	if len(e.profiles.audits) != 0 {
		t.Error("a refused change wrote an audit row")
	}
}

func TestProfileAPIOfTheSameKindChangesNothing(t *testing.T) {
	e := newProfileAPIEnv(t)
	rec := e.put(profilePath, ownerID, map[string]any{"account_kind": "individual"})
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if m := bodyOf(t, rec); m["account_kind"] != "individual" || m["changed"] != false {
		t.Errorf("body = %v", m)
	}
	if len(e.profiles.audits) != 0 || len(e.profiles.profiles) != 0 {
		t.Error("an unchanged kind wrote something")
	}
}

func TestProfileAPIDoesNotLeakAnInternalError(t *testing.T) {
	e := newProfileAPIEnv(t)
	e.members.err = io.ErrUnexpectedEOF
	rec := e.put(profilePath, ownerID, businessBody())
	wantRefusal(t, rec, 500, "internal")
	if strings.Contains(rec.Body.String(), "EOF") {
		t.Errorf("the cause leaked: %s", rec.Body)
	}
}
