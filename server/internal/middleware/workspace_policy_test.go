package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/workspacepolicy"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const policyOtherWorkspace = "44444444-4444-4444-8444-444444444444"

type fakePolicyReader struct {
	byWorkspace map[string]workspacepolicy.Policy
	err         error
	calls       int
}

func (f *fakePolicyReader) Policy(_ context.Context, id string) (workspacepolicy.Policy, error) {
	f.calls++
	if f.err != nil {
		return workspacepolicy.Policy{}, f.err
	}
	if p, ok := f.byWorkspace[id]; ok {
		return p, nil
	}
	return workspacepolicy.Default(), nil
}

func policyRouter(reader PolicyReader) http.Handler {
	queries := db.New(memberDB{role: "member"})
	r := chi.NewRouter()
	r.Use(WithWorkspacePolicy(reader))
	r.Group(func(r chi.Router) {
		r.Use(RequireWorkspaceMember(queries))
		r.Get("/api/issues/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	})
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Use(RequireWorkspaceMemberFromURL(queries, "id"))
		r.Get("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	})
	return r
}

type policyReq struct {
	workspace string
	ctx       func(context.Context) context.Context
	headers   map[string]string
}

func servePolicy(t *testing.T, h http.Handler, in policyReq) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/issues/", nil)
	req.Header.Set("X-User-ID", stateTestUser)
	req.Header.Set("X-Workspace-ID", in.workspace)
	for k, v := range in.headers {
		req.Header.Set(k, v)
	}
	if in.ctx != nil {
		req = req.WithContext(in.ctx(req.Context()))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func session(amr []string, authTime time.Time) func(context.Context) context.Context {
	return func(ctx context.Context) context.Context {
		return WithSession(ctx, amr, authTime.Unix(), true)
	}
}

func TestWorkspacePolicyRefusesOnlyThatWorkspace(t *testing.T) {
	reader := &fakePolicyReader{byWorkspace: map[string]workspacepolicy.Policy{
		stateTestWorkspace: {ShareLinksEnabled: true, RequireMFA: true},
	}}
	h := policyRouter(reader)
	otp := session([]string{"otp"}, time.Now())

	rec := servePolicy(t, h, policyReq{workspace: stateTestWorkspace, ctx: otp})
	if rec.Code != http.StatusForbidden || codeOf(t, rec) != "mfa_required" {
		t.Fatalf("workspace with require_mfa: %d %s, want 403 mfa_required", rec.Code, rec.Body.String())
	}
	rec = servePolicy(t, h, policyReq{workspace: policyOtherWorkspace, ctx: otp})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("another workspace of the same session: %d %s, want it served", rec.Code, rec.Body.String())
	}
	rec = servePolicy(t, h, policyReq{workspace: stateTestWorkspace, ctx: session([]string{"otp", "mfa"}, time.Now())})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("session with mfa: %d %s, want it served", rec.Code, rec.Body.String())
	}
}

func TestWorkspacePolicySessionAge(t *testing.T) {
	reader := &fakePolicyReader{byWorkspace: map[string]workspacepolicy.Policy{
		stateTestWorkspace: {ShareLinksEnabled: true, SessionMaxAgeHours: 8},
	}}
	h := policyRouter(reader)
	old := session([]string{"otp"}, time.Now().Add(-9*time.Hour))
	rec := servePolicy(t, h, policyReq{workspace: stateTestWorkspace, ctx: old})
	if rec.Code != http.StatusForbidden || codeOf(t, rec) != "reauth_required" {
		t.Fatalf("old session: %d %s, want 403 reauth_required", rec.Code, rec.Body.String())
	}
	rec = servePolicy(t, h, policyReq{workspace: policyOtherWorkspace, ctx: old})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("old session in a workspace without a limit: %d, want it served", rec.Code)
	}
	rec = servePolicy(t, h, policyReq{workspace: stateTestWorkspace, ctx: session([]string{"otp"}, time.Now().Add(-time.Hour))})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("fresh session: %d, want it served", rec.Code)
	}
}

func TestWorkspacePolicyAppliesOnURLBoundRoutes(t *testing.T) {
	reader := &fakePolicyReader{byWorkspace: map[string]workspacepolicy.Policy{
		stateTestWorkspace: {ShareLinksEnabled: true, RequireMFA: true},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+stateTestWorkspace+"/", nil)
	req.Header.Set("X-User-ID", stateTestUser)
	req = req.WithContext(WithSession(req.Context(), []string{"otp"}, time.Now().Unix(), true))
	rec := httptest.NewRecorder()
	policyRouter(reader).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || codeOf(t, rec) != "mfa_required" {
		t.Fatalf("URL-bound route: %d %s, want 403 mfa_required", rec.Code, rec.Body.String())
	}
}

func TestWorkspacePolicyExemptsMachineCredentials(t *testing.T) {
	strict := workspacepolicy.Policy{ShareLinksEnabled: true, RequireMFA: true, SessionMaxAgeHours: 1}
	reader := &fakePolicyReader{byWorkspace: map[string]workspacepolicy.Policy{stateTestWorkspace: strict}}
	h := policyRouter(reader)
	staleOTP := session([]string{"otp"}, time.Now().Add(-100*time.Hour))

	cases := map[string]policyReq{
		"personal access token (no session on the request)": {workspace: stateTestWorkspace},
		"daemon token": {workspace: stateTestWorkspace, ctx: func(c context.Context) context.Context {
			return WithDaemonContext(staleOTP(c), stateTestWorkspace, "d1")
		}},
		"task token (agent)": {workspace: stateTestWorkspace, ctx: staleOTP, headers: map[string]string{"X-Actor-Source": "task_token"}},
		"cloud PAT":          {workspace: stateTestWorkspace, ctx: staleOTP, headers: map[string]string{"X-Actor-Source": "cloud_pat"}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			// A task token is bound to its workspace by the header Auth sets.
			if in.headers["X-Actor-Source"] == "task_token" {
				in.headers["X-Workspace-ID"] = stateTestWorkspace
			}
			rec := servePolicy(t, h, in)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("%d %s, want it served", rec.Code, rec.Body.String())
			}
		})
	}
	// The same strict policy refuses a human session, so the cases above pass
	// because of the exemption and not because the policy is lax.
	rec := servePolicy(t, h, policyReq{workspace: stateTestWorkspace, ctx: staleOTP})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("human session: %d, want 403", rec.Code)
	}
}

func TestWorkspacePolicyReaderIsSkippedWhenNothingToCheck(t *testing.T) {
	reader := &fakePolicyReader{}
	h := policyRouter(reader)
	servePolicy(t, h, policyReq{workspace: stateTestWorkspace})
	if reader.calls != 0 {
		t.Fatalf("policy read %d times for a request with no session", reader.calls)
	}
}

func TestWorkspacePolicyUnavailableFailsClosedForSessions(t *testing.T) {
	reader := &fakePolicyReader{err: errors.New("db down")}
	h := policyRouter(reader)
	rec := servePolicy(t, h, policyReq{workspace: stateTestWorkspace, ctx: session([]string{"otp"}, time.Now())})
	if rec.Code != http.StatusServiceUnavailable || codeOf(t, rec) != "workspace_state_unavailable" {
		t.Fatalf("%d %s, want 503 workspace_state_unavailable", rec.Code, rec.Body.String())
	}
}

func TestWorkspacePolicyNilReaderSkipsTheCheck(t *testing.T) {
	h := policyRouter(nil)
	rec := servePolicy(t, h, policyReq{workspace: stateTestWorkspace, ctx: session([]string{"otp"}, time.Now().Add(-1000*time.Hour))})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("%d, want it served without a reader", rec.Code)
	}
}
