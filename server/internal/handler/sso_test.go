package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/mfa"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/seatcapacity"
	"github.com/multica-ai/multica/server/internal/sso"
	"github.com/multica-ai/multica/server/internal/sso/ssotest"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
)

const ssoTestPublicURL = "https://api.example.test"

// ssoEnv is a workspace with one verified domain, an active connection to the
// fake provider, and a router that serves the connection and sign-in routes.
type ssoEnv struct {
	t       *testing.T
	idp     *ssotest.IdP
	ws      string
	domain  string
	connID  string
	handler *SSOHandler
	router  http.Handler
}

func ssoRouter(s *SSOHandler) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireWorkspaceRoleFromURL(testHandler.Queries, "id", "owner"))
			s.ConnectionRoutes(r)
		})
	})
	r.Route("/auth/sso", func(r chi.Router) { s.AuthRoutes(r) })
	return r
}

func newSSOEnv(t *testing.T) *ssoEnv {
	t.Helper()
	t.Setenv(mfa.EnvKey, base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	t.Setenv("MULTICA_PUBLIC_URL", ssoTestPublicURL)
	t.Setenv("MULTICA_APP_URL", "https://app.example.test")

	idp := ssotest.New(t)
	ws := domainWorkspace(t)
	domain := uniqueDomain("sso")
	verifiedDomain(t, ws, domain)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM sso_connection WHERE workspace_id = $1`, ws)
	})
	s := NewSSOHandler(testHandler, sso.Client{HTTP: idp.HTTPClient()})
	s.AllowInsecureIssuer = true
	env := &ssoEnv{t: t, idp: idp, ws: ws, domain: domain, handler: s, router: ssoRouter(s)}
	idp.Email = env.newEmail()

	rec := env.putConnection(map[string]any{
		"issuer": idp.Issuer(), "client_id": idp.ClientID, "client_secret": idp.ClientSecret, "default_role": "member",
	})
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("connect the provider: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.ID == "" {
		t.Fatalf("connection response has no id: %s", rec.Body.String())
	}
	env.connID = body.ID
	return env
}

func (e *ssoEnv) newEmail() string {
	email := "person-" + uuid.NewString()[:8] + "@" + e.domain
	e.t.Cleanup(func() {
		ctx := context.Background()
		testPool.Exec(ctx, `DELETE FROM member WHERE user_id IN (SELECT id FROM "user" WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM seat_capacity_outbox WHERE user_id IN (SELECT id FROM "user" WHERE email = $1)`, email)
		testPool.Exec(ctx, `DELETE FROM "user" WHERE email = $1`, email)
	})
	return email
}

// register creates the Multica user for the IdP email. Sign-in only admits
// accounts platform management registered, so a test that expects the person
// to join or sign in needs the account first.
func (e *ssoEnv) register() string {
	return dbfx.User(e.t, "SSO person", e.idp.Email)
}

func (e *ssoEnv) serve(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func (e *ssoEnv) connectionPath() string { return "/api/workspaces/" + e.ws + "/sso/connection" }

func (e *ssoEnv) putConnection(body map[string]any) *httptest.ResponseRecorder {
	return e.serve(newRequest(http.MethodPut, e.connectionPath(), body))
}

// startLogin opens /auth/sso/start, then plays the browser at the provider.
// It returns the callback URL and the pending-login cookie.
func (e *ssoEnv) startLogin() (callback *url.URL, pending *http.Cookie) {
	e.t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/auth/sso/start?connection="+e.connID+"&next=/acme/issues", nil)
	rec := e.serve(req)
	if rec.Code != http.StatusFound {
		e.t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sso.PendingCookie {
			pending = c
		}
	}
	if pending == nil || !pending.HttpOnly || pending.SameSite != http.SameSiteLaxMode || pending.Value == "" {
		e.t.Fatalf("start must set an HttpOnly SameSite=Lax %s cookie, got %+v", sso.PendingCookie, pending)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, e.idp.Issuer()) || !strings.Contains(loc, "redirect_uri="+url.QueryEscape(ssoTestPublicURL+"/auth/sso/callback")) {
		e.t.Fatalf("start redirects to %q, want the provider with the callback %s/auth/sso/callback", loc, ssoTestPublicURL)
	}
	cb, err := url.Parse(e.idp.Authorize(e.t, loc))
	if err != nil {
		e.t.Fatal(err)
	}
	return cb, pending
}

// callback sends the provider's redirect back to the server.
func (e *ssoEnv) callback(cb *url.URL, pending *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/auth/sso/callback?"+cb.RawQuery, nil)
	if pending != nil {
		req.AddCookie(pending)
	}
	return e.serve(req)
}

func (e *ssoEnv) login() *httptest.ResponseRecorder {
	cb, pending := e.startLogin()
	return e.callback(cb, pending)
}

func authCookie(rec *httptest.ResponseRecorder) string {
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.AuthCookieName && c.Value != "" {
			return c.Value
		}
	}
	return ""
}

func ssoErrorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusFound {
		t.Fatalf("callback answered %d %s, want a 302 back to the sign-in page", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return loc.Query().Get("sso_error")
}

func (e *ssoEnv) refused(rec *httptest.ResponseRecorder, wantErr, email string) {
	e.t.Helper()
	if got := ssoErrorOf(e.t, rec); got == "" || (wantErr != "" && got != wantErr) {
		e.t.Fatalf("sso_error = %q, want %q (Location %s)", got, wantErr, rec.Header().Get("Location"))
	}
	if authCookie(rec) != "" {
		e.t.Fatal("a refused sign-in set a session cookie")
	}
	if email != "" && dbfx.Count(e.t, `SELECT count(*) FROM "user" WHERE email = $1`, email) != 0 {
		e.t.Fatalf("a refused sign-in created the user %s", email)
	}
}

func memberRole(t *testing.T, ws, email string) string {
	t.Helper()
	var role string
	err := testPool.QueryRow(context.Background(),
		`SELECT m.role FROM member m JOIN "user" u ON u.id = m.user_id WHERE m.workspace_id = $1 AND u.email = $2`, ws, email).Scan(&role)
	if err != nil {
		return ""
	}
	return role
}

// Contract test 1 and spec AC16: a good sign-in finds or creates the user and
// issues a session whose amr names the connection.
func TestSSOLoginIssuesASessionWithTheConnectionInAMR(t *testing.T) {
	env := newSSOEnv(t)
	env.register()
	rec := env.login()
	if rec.Code != http.StatusFound {
		t.Fatalf("callback: %d %s", rec.Code, rec.Body.String())
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Query().Get("sso_error") != "" || loc.Host != "app.example.test" || loc.Path != "/acme/issues" {
		t.Fatalf("callback redirects to %s, want https://app.example.test/acme/issues", loc)
	}
	token := authCookie(rec)
	if token == "" {
		t.Fatal("no auth cookie")
	}
	claims, err := auth.ParseSessionToken(token)
	if err != nil {
		t.Fatalf("session token: %v", err)
	}
	amr := auth.SessionAMR(claims)
	if len(amr) != 1 || amr[0] != "sso:"+env.connID {
		t.Fatalf("amr = %v, want [sso:%s]", amr, env.connID)
	}
	var userID string
	if err := testPool.QueryRow(context.Background(), `SELECT id FROM "user" WHERE email = $1`, env.idp.Email).Scan(&userID); err != nil {
		t.Fatalf("the user was not created: %v", err)
	}
	if claims["sub"] != userID {
		t.Fatalf("session sub = %v, want the user %s", claims["sub"], userID)
	}
	var cleared bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == sso.PendingCookie && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("the pending-login cookie was not cleared")
	}
}

func TestSSOLoginFindsAnExistingUser(t *testing.T) {
	env := newSSOEnv(t)
	existing := dbfx.User(t, "Existing", env.idp.Email)
	rec := env.login()
	claims, err := auth.ParseSessionToken(authCookie(rec))
	if err != nil {
		t.Fatalf("session token: %v (%s)", err, rec.Header().Get("Location"))
	}
	if claims["sub"] != existing {
		t.Fatalf("session sub = %v, want the existing user %s", claims["sub"], existing)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM "user" WHERE email = $1`, env.idp.Email); n != 1 {
		t.Fatalf("%d users with the email, want 1", n)
	}
}

// Contract test 3: a new user is added with the connection's default role.
func TestSSOLoginAddsTheMemberWithTheDefaultRole(t *testing.T) {
	env := newSSOEnv(t)
	env.register()
	if rec := env.putConnection(map[string]any{"issuer": env.idp.Issuer(), "client_id": env.idp.ClientID, "default_role": "admin"}); rec.Code != http.StatusOK {
		t.Fatalf("set the role: %d %s", rec.Code, rec.Body.String())
	}
	env.login()
	if got := memberRole(t, env.ws, env.idp.Email); got != "admin" {
		t.Fatalf("role = %q, want admin", got)
	}

	// A second sign-in does not change the role or add a second membership.
	env.login()
	if n := dbfx.Count(t, `SELECT count(*) FROM member m JOIN "user" u ON u.id = m.user_id WHERE m.workspace_id = $1 AND u.email = $2`, env.ws, env.idp.Email); n != 1 {
		t.Fatalf("%d memberships after two sign-ins, want 1", n)
	}
}

func TestSSOLoginKeepsTheRoleOfAnExistingMember(t *testing.T) {
	env := newSSOEnv(t)
	uid := dbfx.User(t, "Owner by SSO", env.idp.Email)
	dbfx.Member(t, env.ws, uid, "owner")
	env.login()
	if got := memberRole(t, env.ws, env.idp.Email); got != "owner" {
		t.Fatalf("role = %q, want owner kept", got)
	}
}

// Contract test 3, second half: with no seat left the join is refused with the
// existing seat error, and no membership or session results.
func TestSSOLoginRefusesTheJoinWhenNoSeatIsLeft(t *testing.T) {
	env := newSSOEnv(t)
	env.register()
	useSeatCapacity(t, &shareJoinCapacityStub{claimDecision: &seatcapacity.Decision{Managed: true, Allowed: false}})
	rec := env.login()
	env.refused(rec, "seat_capacity_full", "")
	if got := memberRole(t, env.ws, env.idp.Email); got != "" {
		t.Fatalf("the user became a %s member with no seat", got)
	}
}

// Contract test 2 and AC17: each of these is refused, and nothing is created.
func TestSSOLoginRefusesWhatAC17Lists(t *testing.T) {
	t.Run("wrong state", func(t *testing.T) {
		env := newSSOEnv(t)
		cb, pending := env.startLogin()
		q := cb.Query()
		q.Set("state", "state-of-someone-elses-login")
		cb.RawQuery = q.Encode()
		env.refused(env.callback(cb, pending), "", env.idp.Email)
	})
	t.Run("wrong nonce", func(t *testing.T) {
		env := newSSOEnv(t)
		env.idp.WrongNonce = true
		env.refused(env.login(), "", env.idp.Email)
	})
	t.Run("bad signature", func(t *testing.T) {
		env := newSSOEnv(t)
		env.idp.BadSignature = true
		env.refused(env.login(), "", env.idp.Email)
	})
	t.Run("email not verified", func(t *testing.T) {
		env := newSSOEnv(t)
		env.idp.EmailVerified = false
		env.refused(env.login(), "email_not_verified", env.idp.Email)
	})
	t.Run("no pending-login cookie", func(t *testing.T) {
		env := newSSOEnv(t)
		cb, _ := env.startLogin()
		env.refused(env.callback(cb, nil), "", env.idp.Email)
	})
	t.Run("cookie of another login", func(t *testing.T) {
		env := newSSOEnv(t)
		cb, _ := env.startLogin()
		_, otherPending := env.startLogin()
		env.refused(env.callback(cb, otherPending), "", env.idp.Email)
	})
	t.Run("provider returns an error", func(t *testing.T) {
		env := newSSOEnv(t)
		_, pending := env.startLogin()
		cb, _ := url.Parse("/auth/sso/callback?error=access_denied&state=x")
		env.refused(env.callback(cb, pending), "", env.idp.Email)
	})
}

// Spec 6.3 and AC16 as the spec words it: the email's domain must be a verified
// domain of this workspace, or a workspace could claim anyone's account.
func TestSSOLoginRefusesAnEmailOutsideTheVerifiedDomains(t *testing.T) {
	t.Run("another domain", func(t *testing.T) {
		env := newSSOEnv(t)
		env.idp.Email = "victim-" + uuid.NewString()[:8] + "@" + uniqueDomain("elsewhere")
		env.refused(env.login(), "domain_not_allowed", env.idp.Email)
	})
	t.Run("a domain that is claimed and not verified", func(t *testing.T) {
		env := newSSOEnv(t)
		pending := uniqueDomain("pending")
		dbfx.Exec(t, `INSERT INTO sso_domain (workspace_id, domain, token) VALUES ($1, $2, 'unproven')`, env.ws, pending)
		env.idp.Email = "x-" + uuid.NewString()[:8] + "@" + pending
		env.refused(env.login(), "domain_not_allowed", env.idp.Email)
	})
	t.Run("a domain verified by another workspace", func(t *testing.T) {
		env := newSSOEnv(t)
		other := domainWorkspace(t)
		theirs := uniqueDomain("theirs")
		verifiedDomain(t, other, theirs)
		env.idp.Email = "x-" + uuid.NewString()[:8] + "@" + theirs
		env.refused(env.login(), "domain_not_allowed", env.idp.Email)
	})
	t.Run("a subdomain of a verified domain", func(t *testing.T) {
		env := newSSOEnv(t)
		env.idp.Email = "x-" + uuid.NewString()[:8] + "@sub." + env.domain
		env.refused(env.login(), "domain_not_allowed", env.idp.Email)
	})
}

func TestSSOLoginRefusesADisabledConnection(t *testing.T) {
	env := newSSOEnv(t)
	cb, pending := env.startLogin()
	if rec := env.putConnection(map[string]any{"issuer": env.idp.Issuer(), "client_id": env.idp.ClientID, "status": "disabled"}); rec.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body.String())
	}
	env.refused(env.callback(cb, pending), "", env.idp.Email)
	rec := env.serve(httptest.NewRequest(http.MethodGet, "/auth/sso/start?connection="+env.connID, nil))
	if strings.HasPrefix(rec.Header().Get("Location"), env.idp.Issuer()) || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("start sent the browser to the provider of a disabled connection: %d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestSSODiscover(t *testing.T) {
	env := newSSOEnv(t)
	discover := func(email string) map[string]any {
		t.Helper()
		rec := env.serve(newRequest(http.MethodPost, "/auth/sso/discover", map[string]string{"email": email}))
		if rec.Code != http.StatusOK {
			t.Fatalf("discover %s: %d %s", email, rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	got := discover(env.idp.Email)
	if got["sso"] != true || got["start_url"] != ssoTestPublicURL+"/auth/sso/start?connection="+env.connID {
		t.Fatalf("discover on the verified domain = %v, want sso true with the start url", got)
	}
	if got := discover("someone@" + uniqueDomain("unknown")); got["sso"] != false || got["start_url"] != nil {
		t.Fatalf("discover on an unknown domain = %v, want sso false", got)
	}
	if got := discover("not-an-email"); got["sso"] != false {
		t.Fatalf("discover on a malformed email = %v, want sso false", got)
	}
	env.putConnection(map[string]any{"issuer": env.idp.Issuer(), "client_id": env.idp.ClientID, "status": "disabled"})
	if got := discover(env.idp.Email); got["sso"] != false {
		t.Fatalf("discover with a disabled connection = %v, want sso false", got)
	}
}

func TestSSOConnectionRoutes(t *testing.T) {
	env := newSSOEnv(t)
	env.register()

	t.Run("get never returns the secret and says whether one is set", func(t *testing.T) {
		rec := env.serve(newRequest(http.MethodGet, env.connectionPath(), nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
		}
		var got map[string]any
		json.Unmarshal(rec.Body.Bytes(), &got)
		if got["client_secret_set"] != true || got["issuer"] != env.idp.Issuer() || got["default_role"] != "member" || got["status"] != "active" ||
			got["redirect_uri"] != ssoTestPublicURL+"/auth/sso/callback" {
			t.Fatalf("connection = %v", got)
		}
		for key := range got {
			if strings.Contains(key, "secret") && key != "client_secret_set" {
				t.Fatalf("the response has the field %q", key)
			}
		}
	})

	t.Run("the secret is stored encrypted", func(t *testing.T) {
		var stored []byte
		dbfx.QueryRow(t, `SELECT client_secret_enc FROM sso_connection WHERE workspace_id = $1`, env.ws).Scan(&stored)
		if len(stored) == 0 || bytes.Contains(stored, []byte(env.idp.ClientSecret)) {
			t.Fatalf("client_secret_enc is empty or holds the plaintext (%d bytes)", len(stored))
		}
	})

	t.Run("an update without a secret keeps the stored one", func(t *testing.T) {
		if rec := env.putConnection(map[string]any{"issuer": env.idp.Issuer(), "client_id": env.idp.ClientID, "default_role": "admin"}); rec.Code != http.StatusOK {
			t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
		}
		if rec := env.login(); authCookie(rec) == "" {
			t.Fatalf("sign-in after an update without a secret failed: %s", rec.Header().Get("Location"))
		}
	})

	t.Run("one connection per workspace", func(t *testing.T) {
		if n := dbfx.Count(t, `SELECT count(*) FROM sso_connection WHERE workspace_id = $1`, env.ws); n != 1 {
			t.Fatalf("%d connections after create and update, want 1", n)
		}
	})

	t.Run("bad input is refused", func(t *testing.T) {
		for name, body := range map[string]map[string]any{
			"role owner":          {"issuer": env.idp.Issuer(), "client_id": "c", "default_role": "owner"},
			"unknown status":      {"issuer": env.idp.Issuer(), "client_id": "c", "status": "paused"},
			"no client id":        {"issuer": env.idp.Issuer()},
			"not a url":           {"issuer": "not a url", "client_id": "c"},
			"issuer without oidc": {"issuer": env.idp.Issuer() + "/missing", "client_id": "c"},
		} {
			if rec := env.putConnection(body); rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s: %d %s, want a 4xx refusal", name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("a plain http issuer is refused in production", func(t *testing.T) {
		strict := NewSSOHandler(testHandler, sso.Client{HTTP: env.idp.HTTPClient()})
		rec := httptest.NewRecorder()
		ssoRouter(strict).ServeHTTP(rec, newRequest(http.MethodPut, env.connectionPath(), map[string]any{"issuer": env.idp.Issuer(), "client_id": "c"}))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("http issuer: %d %s, want 400", rec.Code, rec.Body.String())
		}
	})

	t.Run("delete removes it", func(t *testing.T) {
		if rec := env.serve(newRequest(http.MethodDelete, env.connectionPath(), nil)); rec.Code != http.StatusNoContent {
			t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
		}
		if rec := env.serve(newRequest(http.MethodGet, env.connectionPath(), nil)); rec.Code != http.StatusNotFound {
			t.Fatalf("get after delete: %d, want 404", rec.Code)
		}
	})
}

// The connection is tied to a verified domain: with none, it cannot be made.
func TestSSOConnectionNeedsAVerifiedDomain(t *testing.T) {
	env := newSSOEnv(t)
	bare := domainWorkspace(t)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM sso_connection WHERE workspace_id = $1`, bare)
	})
	put := func() *httptest.ResponseRecorder {
		return env.serve(newRequest(http.MethodPut, "/api/workspaces/"+bare+"/sso/connection",
			map[string]any{"issuer": env.idp.Issuer(), "client_id": env.idp.ClientID, "client_secret": env.idp.ClientSecret}))
	}

	if rec := put(); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "domain_not_verified") {
		t.Fatalf("no domain: %d %s, want 422 domain_not_verified", rec.Code, rec.Body.String())
	}
	dbfx.Exec(t, `INSERT INTO sso_domain (workspace_id, domain, token) VALUES ($1, $2, 'unproven')`, bare, uniqueDomain("unproven"))
	if rec := put(); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unverified domain: %d %s, want 422", rec.Code, rec.Body.String())
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM sso_connection WHERE workspace_id = $1`, bare); n != 0 {
		t.Fatalf("%d connections stored for a workspace with no verified domain", n)
	}
	dbfx.Exec(t, `UPDATE sso_domain SET verified_at = now() WHERE workspace_id = $1`, bare)
	if rec := put(); rec.Code != http.StatusCreated {
		t.Fatalf("verified domain: %d %s, want 201", rec.Code, rec.Body.String())
	}
}

// Contract test 5: the secret is in no response and no log line, including the
// failure paths where the provider echoes it back.
func TestSSOClientSecretIsNeverInAResponseOrALog(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	env := newSSOEnv(t)
	secret := env.idp.ClientSecret
	var seen []string
	note := func(rec *httptest.ResponseRecorder) *httptest.ResponseRecorder {
		seen = append(seen, rec.Body.String())
		for k, v := range rec.Header() {
			seen = append(seen, k+": "+strings.Join(v, ","))
		}
		return rec
	}

	note(env.putConnection(map[string]any{"issuer": env.idp.Issuer(), "client_id": env.idp.ClientID, "client_secret": secret}))
	note(env.serve(newRequest(http.MethodGet, env.connectionPath(), nil)))
	note(env.serve(newRequest(http.MethodPost, "/auth/sso/discover", map[string]string{"email": env.idp.Email})))
	note(env.login())

	env.idp.TokenError = true // the provider echoes the secret it was sent
	note(env.login())
	env.idp.TokenError = false
	env.idp.BadSignature = true
	note(env.login())
	// A bad update fails at the provider check; the secret is in its request.
	note(env.putConnection(map[string]any{"issuer": env.idp.Issuer() + "/missing", "client_id": "c", "client_secret": secret}))

	for _, s := range seen {
		if strings.Contains(s, secret) {
			t.Fatalf("the client secret is in a response: %q", s)
		}
	}
	if logs.Len() == 0 {
		t.Fatal("no log lines were captured; the check proves nothing")
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatalf("the client secret is in the log:\n%s", logs.String())
	}
}

func ssoPolicyRouter(connID string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.WithWorkspacePolicy(workspaceguard.NewPolicyStore(testHandler.Queries, nil)))
	r.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Use(middleware.RequireWorkspaceMemberFromURL(testHandler.Queries, "id"))
		r.Get("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	})
	return r
}

// Contract test 4 and AC17: with require_sso, a member who came in another way
// is refused in this workspace, and the owner is not.
func TestSSORequireSSORefusesOtherSignInMethods(t *testing.T) {
	env := newSSOEnv(t)
	dbfx.Exec(t, `INSERT INTO workspace_policy (workspace_id, require_sso) VALUES ($1, true)`, env.ws)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM workspace_policy WHERE workspace_id = $1`, env.ws)
	})

	memberEmail := env.newEmail()
	member := dbfx.User(t, "On the domain", memberEmail)
	dbfx.Member(t, env.ws, member, "member")
	outsider := dbfx.User(t, "Contractor", "contractor-"+uuid.NewString()[:8]+"@"+uniqueDomain("contractor"))
	dbfx.Member(t, env.ws, outsider, "member")
	h := ssoPolicyRouter(env.connID)

	serve := func(userID string, amr ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+env.ws+"/", nil)
		req.Header.Set("X-User-ID", userID)
		req = req.WithContext(middleware.WithSession(req.Context(), amr, time.Now().Unix(), true))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	wantRefused := func(rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"sso_required"`) {
			t.Fatalf("%d %s, want 403 sso_required", rec.Code, rec.Body.String())
		}
	}
	wantPass := func(rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%d %s, want 204", rec.Code, rec.Body.String())
		}
	}

	wantRefused(serve(member, "otp"))
	wantRefused(serve(member, "google"))
	wantRefused(serve(member, "otp", "mfa"))
	wantRefused(serve(member, "sso:"+uuid.NewString()))
	wantPass(serve(member, "sso:"+env.connID))
	wantPass(serve(member, "sso:"+env.connID, "mfa"))
	wantPass(serve(testUserID, "otp")) // the owner of the workspace
	wantPass(serve(outsider, "otp"))   // a guest outside the verified domains

	// The refusal is for this workspace only.
	other := domainWorkspace(t)
	dbfx.Member(t, other, member, "member")
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+other+"/", nil)
	req.Header.Set("X-User-ID", member)
	req = req.WithContext(middleware.WithSession(req.Context(), []string{"otp"}, time.Now().Unix(), true))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	wantPass(rec)

	// With the connection disabled there is no way in through SSO, so nobody is locked out.
	env.putConnection(map[string]any{"issuer": env.idp.Issuer(), "client_id": env.idp.ClientID, "status": "disabled"})
	wantPass(serve(member, "otp"))
}

// Deleting a workspace removes its SSO rows.
func TestSSODeletingAWorkspaceRemovesItsSSORows(t *testing.T) {
	env := newSSOEnv(t)
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := testHandler.deleteWorkspaceData(ctx, tx, parseUUID(env.ws)); err != nil {
		t.Fatalf("deleteWorkspaceData: %v", err)
	}
	for _, table := range []string{"sso_connection", "sso_domain"} {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, env.ws).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s rows after the workspace was deleted: %d, %v", table, n, err)
		}
	}
}
