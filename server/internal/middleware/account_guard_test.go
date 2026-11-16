package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	redismock "github.com/go-redis/redismock/v9"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/server/internal/auth"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const guardTestUserID = "11111111-1111-4111-8111-111111111111"

// fakeAccountStore is the accountguard.Store the decision table runs against.
type fakeAccountStore struct {
	rec   accountguard.Record
	found bool
	err   error
	calls int
}

func (s *fakeAccountStore) AccountState(context.Context, string) (accountguard.Record, bool, error) {
	s.calls++
	return s.rec, s.found, s.err
}

// noAccountCache makes every guard lookup reach the store.
type noAccountCache struct{}

func (noAccountCache) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (noAccountCache) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (noAccountCache) Del(context.Context, string) error                        { return nil }

func guardOver(store *fakeAccountStore) *accountguard.Guard {
	return accountguard.New(store, noAccountCache{})
}

// taskTokenRows answers GetTaskTokenByHash with a row bound to guardTestUserID.
type taskTokenRows struct{ db.DBTX }

func (taskTokenRows) QueryRow(context.Context, string, ...any) pgx.Row { return taskTokenRow{} }

type taskTokenRow struct{ pgx.Row }

func (taskTokenRow) Scan(dest ...any) error {
	// Column order of GetTaskTokenByHash: id, token_hash, task_id, agent_id,
	// workspace_id, user_id, expires_at, created_at.
	user, ok := dest[5].(*pgtype.UUID)
	if !ok {
		return errors.New("unexpected GetTaskTokenByHash column layout")
	}
	return user.Scan(guardTestUserID)
}

func (taskTokenRows) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func fleetReturningOwner(t *testing.T, ownerID string) *auth.CloudPATVerifier {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"valid":true,"owner_id":"` + ownerID + `","instance_id":"i-01"}`))
	}))
	t.Cleanup(srv.Close)
	return auth.NewCloudPATVerifier(auth.CloudPATVerifierConfig{FleetBaseURL: srv.URL})
}

func sessionJWT(t *testing.T, extra jwt.MapClaims) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": guardTestUserID,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}
	return generateToken(claims, auth.JWTSecret())
}

// guardCase is one token kind: the middleware that authenticates it and a
// request that authenticates as guardTestUserID.
type guardCase struct {
	name  string
	build func(t *testing.T, guard *accountguard.Guard, path string) (func(http.Handler) http.Handler, *http.Request)
}

func guardCases() []guardCase {
	bearer := func(method, path, token string) *http.Request {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		return req
	}
	patCache := func(userID string) *auth.PATCache {
		rdb, mock := redismock.NewClientMock()
		mock.Regexp().ExpectGet(`^mul:auth:pat:v2:`).SetVal(`{"user_id":"` + userID + `"}`)
		return auth.NewPATCache(rdb)
	}
	daemonCache := func(t *testing.T, ownerID string) *auth.DaemonTokenCache {
		raw, err := json.Marshal(auth.DaemonTokenIdentity{WorkspaceID: "ws-1", DaemonID: "daemon-1", OwnerID: ownerID})
		if err != nil {
			t.Fatal(err)
		}
		rdb, mock := redismock.NewClientMock()
		mock.Regexp().ExpectGet(`^mul:auth:daemon:v2:`).SetVal(string(raw))
		return auth.NewDaemonTokenCache(rdb)
	}
	return []guardCase{
		{"auth/jwt", func(t *testing.T, g *accountguard.Guard, path string) (func(http.Handler) http.Handler, *http.Request) {
			return Auth(nil, nil, nil, nil, g), bearer("GET", path, sessionJWT(t, nil))
		}},
		{"auth/pat", func(t *testing.T, g *accountguard.Guard, path string) (func(http.Handler) http.Handler, *http.Request) {
			return Auth(nil, patCache(guardTestUserID), nil, nil, g), bearer("GET", path, "mul_guard_test")
		}},
		{"auth/task_token", func(t *testing.T, g *accountguard.Guard, path string) (func(http.Handler) http.Handler, *http.Request) {
			return Auth(db.New(taskTokenRows{}), nil, nil, nil, g), bearer("GET", path, "mat_guard_test")
		}},
		{"auth/cloud_pat", func(t *testing.T, g *accountguard.Guard, path string) (func(http.Handler) http.Handler, *http.Request) {
			return Auth(nil, nil, fleetReturningOwner(t, guardTestUserID), nil, g), bearer("GET", path, "mcn_guard_test")
		}},
		{"daemon/daemon_token", func(t *testing.T, g *accountguard.Guard, path string) (func(http.Handler) http.Handler, *http.Request) {
			return DaemonAuth(nil, nil, daemonCache(t, guardTestUserID), nil, g), bearer("GET", path, "mdt_guard_test")
		}},
		{"daemon/jwt", func(t *testing.T, g *accountguard.Guard, path string) (func(http.Handler) http.Handler, *http.Request) {
			return DaemonAuth(nil, nil, nil, nil, g), bearer("GET", path, sessionJWT(t, nil))
		}},
		{"daemon/pat", func(t *testing.T, g *accountguard.Guard, path string) (func(http.Handler) http.Handler, *http.Request) {
			return DaemonAuth(nil, patCache(guardTestUserID), nil, nil, g), bearer("GET", path, "mul_guard_test")
		}},
		{"daemon/cloud_pat", func(t *testing.T, g *accountguard.Guard, path string) (func(http.Handler) http.Handler, *http.Request) {
			return DaemonAuth(nil, nil, nil, fleetReturningOwner(t, guardTestUserID), g), bearer("GET", path, "mcn_guard_test")
		}},
	}
}

type guardResult struct {
	status int
	code   string
	served bool
}

func serve(t *testing.T, mw func(http.Handler) http.Handler, req *http.Request) (guardResult, *httptest.ResponseRecorder) {
	t.Helper()
	res := guardResult{}
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res.served = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)
	res.status = rec.Code
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	res.code = body.Code
	return res, rec
}

// Every token kind, every account state, one table. A kind that skips the
// guard shows up as a 200 where the table wants a refusal.
func TestAccountGuardDecisionTable(t *testing.T) {
	states := []struct {
		name       string
		store      fakeAccountStore
		wantStatus int
		wantCode   string
	}{
		{"active", fakeAccountStore{found: true, rec: accountguard.Record{Status: accountguard.Active}}, http.StatusOK, ""},
		{"no row", fakeAccountStore{}, http.StatusOK, ""},
		{"suspended", fakeAccountStore{found: true, rec: accountguard.Record{Status: accountguard.Suspended}}, http.StatusForbidden, "account_suspended"},
		{"deleted", fakeAccountStore{found: true, rec: accountguard.Record{Status: accountguard.Deleted}}, http.StatusForbidden, "account_suspended"},
		{"pending_deletion", fakeAccountStore{found: true, rec: accountguard.Record{Status: accountguard.PendingDeletion}}, http.StatusForbidden, "account_pending_deletion"},
		{"guard error", fakeAccountStore{err: errors.New("database unavailable")}, http.StatusServiceUnavailable, "account_state_unavailable"},
	}
	for _, kind := range guardCases() {
		for _, state := range states {
			t.Run(kind.name+"/"+state.name, func(t *testing.T) {
				store := state.store
				mw, req := kind.build(t, guardOver(&store), "/api/issues")
				got, _ := serve(t, mw, req)
				if got.status != state.wantStatus || got.code != state.wantCode {
					t.Fatalf("status/code = %d/%q, want %d/%q", got.status, got.code, state.wantStatus, state.wantCode)
				}
				if got.served != (state.wantStatus == http.StatusOK) {
					t.Fatalf("handler served = %v for status %d", got.served, state.wantStatus)
				}
				if store.calls != 1 {
					t.Fatalf("guard consulted %d times, want 1", store.calls)
				}
			})
		}
	}
}

// A nil guard is today's behavior: nothing is consulted, nothing is refused.
func TestAccountGuardNilGuardPassesEveryKind(t *testing.T) {
	for _, kind := range guardCases() {
		t.Run(kind.name, func(t *testing.T) {
			mw, req := kind.build(t, nil, "/api/issues")
			if got, _ := serve(t, mw, req); got.status != http.StatusOK {
				t.Fatalf("status = %d, want 200", got.status)
			}
		})
	}
}

// Daemon tokens bound to no user (managed by Qoder) have no account to check.
func TestAccountGuardSkipsDaemonTokenWithoutOwner(t *testing.T) {
	store := &fakeAccountStore{found: true, rec: accountguard.Record{Status: accountguard.Suspended}}
	raw, _ := json.Marshal(auth.DaemonTokenIdentity{WorkspaceID: "ws-1", DaemonID: "qoder-managed-1"})
	rdb, mock := redismock.NewClientMock()
	mock.Regexp().ExpectGet(`^mul:auth:daemon:v2:`).SetVal(string(raw))

	req := httptest.NewRequest("GET", "/api/daemon/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer mdt_unowned")
	got, _ := serve(t, DaemonAuth(nil, nil, auth.NewDaemonTokenCache(rdb), nil, guardOver(store)), req)

	if got.status != http.StatusOK || !got.served {
		t.Fatalf("status = %d served = %v, want an unowned daemon token to pass", got.status, got.served)
	}
	if store.calls != 0 {
		t.Fatalf("guard consulted %d times for a token with no owner", store.calls)
	}
}

// pending_deletion is refused everywhere except the three routes a user needs
// to see the notice, cancel the deletion, or sign out. Matching is on the chi
// route template, so a path that merely starts the same does not qualify.
func TestAccountGuardPendingDeletionExemptRoutes(t *testing.T) {
	pending := func() *fakeAccountStore {
		return &fakeAccountStore{found: true, rec: accountguard.Record{Status: accountguard.PendingDeletion}}
	}
	cases := []struct {
		method, path string
		want         int
	}{
		{"GET", "/api/me", http.StatusOK},
		{"DELETE", "/api/me/deletion", http.StatusOK},
		{"POST", "/auth/logout", http.StatusOK},
		{"PATCH", "/api/me", http.StatusForbidden},
		{"GET", "/api/me/deletion", http.StatusForbidden},
		{"DELETE", "/api/me", http.StatusForbidden},
		{"GET", "/auth/logout", http.StatusForbidden},
		{"GET", "/api/me/onboarding", http.StatusForbidden},
		{"GET", "/api/issues", http.StatusForbidden},
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Use(Auth(nil, nil, nil, nil, guardOver(pending())))
		r.Get("/api/me", ok)
		r.Patch("/api/me", ok)
		r.Delete("/api/me", ok)
		r.Get("/api/me/deletion", ok)
		r.Delete("/api/me/deletion", ok)
		r.Get("/api/me/onboarding", ok)
		r.Post("/auth/logout", ok)
		r.Get("/auth/logout", ok)
		r.Get("/api/issues", ok)
	})
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+sessionJWT(t, nil))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// The exemption is for pending_deletion only. A suspended account gets no
// route, including the three above.
func TestAccountGuardSuspendedHasNoExemptRoute(t *testing.T) {
	store := &fakeAccountStore{found: true, rec: accountguard.Record{Status: accountguard.Suspended}}
	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Use(Auth(nil, nil, nil, nil, guardOver(store)))
		r.Get("/api/me", func(w http.ResponseWriter, r *http.Request) { t.Fatal("served a suspended account") })
	})
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+sessionJWT(t, nil))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func revokedAfter(t time.Time) *fakeAccountStore {
	return &fakeAccountStore{found: true, rec: accountguard.Record{Status: accountguard.Active, SessionsValidAfter: t}}
}

func TestAuthSessionRevokedCheck(t *testing.T) {
	validAfter := time.Unix(1_800_000_000, 0)
	at := func(sec int64) float64 { return float64(sec) }
	cases := []struct {
		name   string
		store  *fakeAccountStore
		claims jwt.MapClaims
		want   int
	}{
		{"auth_time before the mark", revokedAfter(validAfter), jwt.MapClaims{"auth_time": at(validAfter.Unix() - 1)}, http.StatusUnauthorized},
		{"auth_time equal to the mark passes", revokedAfter(validAfter), jwt.MapClaims{"auth_time": at(validAfter.Unix())}, http.StatusOK},
		{"auth_time after the mark", revokedAfter(validAfter), jwt.MapClaims{"auth_time": at(validAfter.Unix() + 60)}, http.StatusOK},
		{"iat stands in for a missing auth_time: before", revokedAfter(validAfter), jwt.MapClaims{"iat": at(validAfter.Unix() - 1)}, http.StatusUnauthorized},
		{"iat stands in for a missing auth_time: after", revokedAfter(validAfter), jwt.MapClaims{"iat": at(validAfter.Unix() + 1)}, http.StatusOK},
		{"auth_time wins over a newer iat", revokedAfter(validAfter), jwt.MapClaims{"auth_time": at(validAfter.Unix() - 100), "iat": at(validAfter.Unix() + 100)}, http.StatusUnauthorized},
		{"whole seconds: a sub-second mark does not revoke the same second", revokedAfter(validAfter.Add(900 * time.Millisecond)), jwt.MapClaims{"auth_time": at(validAfter.Unix())}, http.StatusOK},
		{"no mark", &fakeAccountStore{found: true, rec: accountguard.Record{Status: accountguard.Active}}, jwt.MapClaims{"auth_time": at(1)}, http.StatusOK},
		{"no row", &fakeAccountStore{}, jwt.MapClaims{"auth_time": at(1)}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := tc.claims
			if _, hasIat := claims["iat"]; !hasIat {
				// A newer iat than the mark, so only auth_time can explain a refusal.
				claims["iat"] = at(tc.store.rec.SessionsValidAfter.Unix() + 1)
			}
			req := httptest.NewRequest("GET", "/api/issues", nil)
			req.Header.Set("Authorization", "Bearer "+sessionJWT(t, claims))
			got, _ := serve(t, Auth(nil, nil, nil, nil, guardOver(tc.store)), req)
			if got.status != tc.want {
				t.Fatalf("status = %d, want %d", got.status, tc.want)
			}
			if tc.want == http.StatusUnauthorized && got.code != "session_revoked" {
				t.Fatalf("code = %q, want session_revoked", got.code)
			}
		})
	}
}

func TestAuthSessionRevokedClearsCookieAndDoesNotRenew(t *testing.T) {
	validAfter := time.Now().Add(-time.Minute)
	// Inside the renewal window, so an unrevoked token would be re-issued.
	token := generateToken(jwt.MapClaims{
		"sub":       guardTestUserID,
		"exp":       time.Now().Add(auth.AuthRenewThreshold() / 2).Unix(),
		"iat":       time.Now().Add(-time.Hour).Unix(),
		"auth_time": time.Now().Add(-time.Hour).Unix(),
		"sid":       "sid-revoked",
	}, auth.JWTSecret())

	got, rec := serve(t, Auth(nil, nil, nil, nil, guardOver(revokedAfter(validAfter))), cookieRequest(http.MethodGet, token))

	if got.status != http.StatusUnauthorized || got.code != "session_revoked" || got.served {
		t.Fatalf("got %+v, want 401 session_revoked and no handler call", got)
	}
	var cleared bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.AuthCookieName {
			if c.Value != "" || c.MaxAge >= 0 {
				t.Fatalf("auth cookie is %q (MaxAge %d), want it cleared", c.Value, c.MaxAge)
			}
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("the revoked session's auth cookie was not cleared")
	}
}

// A suspended account is refused as suspended even when its session is also
// revoked: the guard runs first, so the client sees the more specific state.
func TestAuthGuardRunsBeforeSessionRevokedCheck(t *testing.T) {
	store := &fakeAccountStore{found: true, rec: accountguard.Record{
		Status:             accountguard.Suspended,
		SessionsValidAfter: time.Now().Add(time.Hour),
	}}
	req := httptest.NewRequest("GET", "/api/issues", nil)
	req.Header.Set("Authorization", "Bearer "+sessionJWT(t, jwt.MapClaims{"auth_time": time.Now().Add(-time.Hour).Unix()}))
	got, _ := serve(t, Auth(nil, nil, nil, nil, guardOver(store)), req)
	if got.status != http.StatusForbidden || got.code != "account_suspended" {
		t.Fatalf("got %+v, want 403 account_suspended", got)
	}
}

// sessions_valid_after revokes sessions. A PAT is a separate credential the
// user revokes on its own, so the mark never applies to it.
func TestAuthSessionRevokedDoesNotApplyToPAT(t *testing.T) {
	rdb, mock := redismock.NewClientMock()
	mock.Regexp().ExpectGet(`^mul:auth:pat:v2:`).SetVal(`{"user_id":"` + guardTestUserID + `"}`)
	req := httptest.NewRequest("GET", "/api/issues", nil)
	req.Header.Set("Authorization", "Bearer mul_unaffected")
	got, _ := serve(t, Auth(nil, auth.NewPATCache(rdb), nil, nil, guardOver(revokedAfter(time.Now().Add(time.Hour)))), req)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want 200", got.status)
	}
}

// Renewal keeps the original login time, so the next request is still judged
// against the login that started the session.
func TestAuthRenewalKeepsAuthTime(t *testing.T) {
	loggedIn := time.Now().Add(-48 * time.Hour).Unix()
	token := generateToken(jwt.MapClaims{
		"sub":       guardTestUserID,
		"exp":       time.Now().Add(auth.AuthRenewThreshold() / 2).Unix(),
		"iat":       time.Now().Add(-time.Hour).Unix(),
		"auth_time": loggedIn,
		"sid":       "sid-renew",
	}, auth.JWTSecret())
	store := &fakeAccountStore{found: true, rec: accountguard.Record{Status: accountguard.Active, SessionsValidAfter: time.Unix(loggedIn, 0)}}

	got, rec := serve(t, Auth(nil, nil, nil, nil, guardOver(store)), cookieRequest(http.MethodGet, token))
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want 200", got.status)
	}
	renewed := renewedAuthCookie(rec)
	if renewed == "" {
		t.Fatal("expected a renewed auth cookie")
	}
	claims, err := auth.ParseSessionToken(renewed)
	if err != nil {
		t.Fatalf("renewed token does not parse: %v", err)
	}
	if got, _ := auth.SessionAuthTime(claims); got != loggedIn {
		t.Fatalf("renewed auth_time = %d, want %d", got, loggedIn)
	}
}

// DaemonAuth accepts a JWT too, so the "sign out of all devices" mark has to
// hold there as well.
func TestDaemonAuthSessionRevokedCheck(t *testing.T) {
	mark := time.Unix(1_800_000_000, 0)
	cases := []struct {
		name     string
		authTime int64
		want     int
	}{
		{"before the mark", mark.Unix() - 1, http.StatusUnauthorized},
		{"equal to the mark", mark.Unix(), http.StatusOK},
		{"after the mark", mark.Unix() + 1, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/daemon/workspaces", nil)
			req.Header.Set("Authorization", "Bearer "+sessionJWT(t, jwt.MapClaims{"auth_time": tc.authTime, "iat": mark.Unix() + 100}))
			got, _ := serve(t, DaemonAuth(nil, nil, nil, nil, guardOver(revokedAfter(mark))), req)
			if got.status != tc.want || (tc.want == http.StatusUnauthorized && got.code != "session_revoked") {
				t.Fatalf("got %+v, want status %d", got, tc.want)
			}
			if got.served != (tc.want == http.StatusOK) {
				t.Fatalf("served = %v for status %d", got.served, got.status)
			}
		})
	}
}

func TestDaemonAuthSessionRevokedDoesNotApplyToPAT(t *testing.T) {
	rdb, mock := redismock.NewClientMock()
	mock.Regexp().ExpectGet(`^mul:auth:pat:v2:`).SetVal(`{"user_id":"` + guardTestUserID + `"}`)
	req := httptest.NewRequest("GET", "/api/daemon/workspaces", nil)
	req.Header.Set("Authorization", "Bearer mul_unaffected")
	got, _ := serve(t, DaemonAuth(nil, auth.NewPATCache(rdb), nil, nil, guardOver(revokedAfter(time.Now().Add(time.Hour)))), req)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want 200", got.status)
	}
}
