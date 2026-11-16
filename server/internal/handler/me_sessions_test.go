package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// These tests write platform_account for the shared handler-test user. They
// run only against the dedicated *_test database named by
// PLATFORM_TEST_DATABASE_URL, which must equal DATABASE_URL.
func requirePlatformTestDatabase(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" || dsn != os.Getenv("DATABASE_URL") {
		t.Skip("PLATFORM_TEST_DATABASE_URL not set or differs from DATABASE_URL")
	}
}

// recordingAccountCache is the guard's cache, with the deletions it saw.
type recordingAccountCache struct {
	mu      sync.Mutex
	deleted []string
}

func (*recordingAccountCache) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, nil
}
func (*recordingAccountCache) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (c *recordingAccountCache) Del(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleted = append(c.deleted, key)
	return nil
}

type queriesAccountStore struct{ q *db.Queries }

func (s queriesAccountStore) AccountState(ctx context.Context, userID string) (accountguard.Record, bool, error) {
	row, err := s.q.AccountState(ctx, parseUUID(userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return accountguard.Record{}, false, nil
	}
	if err != nil {
		return accountguard.Record{}, false, err
	}
	rec := accountguard.Record{Status: accountguard.State(row.Status)}
	if row.SessionsValidAfter.Valid {
		rec.SessionsValidAfter = row.SessionsValidAfter.Time
	}
	return rec, true, nil
}

func resetPlatformAccount(t *testing.T) {
	t.Helper()
	clean := func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM platform_account WHERE user_id = $1::uuid`, testUserID)
	}
	clean()
	t.Cleanup(clean)
}

func revokeAllRequest() *http.Request {
	return newRequest(http.MethodPost, "/api/me/sessions/revoke-all", nil)
}

func decodeRevokeAll(t *testing.T, resp *testutil.Response) RevokeAllSessionsResponse {
	t.Helper()
	var out RevokeAllSessionsResponse
	resp.Want(http.StatusOK).JSON(&out)
	return out
}

func TestRevokeAllSessionsWritesMarkAndReturnsASessionThatSurvivesIt(t *testing.T) {
	requirePlatformTestDatabase(t)
	resetPlatformAccount(t)
	cache := &recordingAccountCache{}
	guard := accountguard.New(queriesAccountStore{testHandler.Queries}, cache)

	resp := testutil.Call(t, testHandler.RevokeAllSessions(guard), revokeAllRequest())
	out := decodeRevokeAll(t, resp)

	revokedAt, err := time.Parse(time.RFC3339, out.RevokedAt)
	if err != nil {
		t.Fatalf("revoked_at %q is not RFC 3339: %v", out.RevokedAt, err)
	}
	if revokedAt.Location() != time.UTC || revokedAt.Nanosecond() != 0 {
		t.Errorf("revoked_at %q is not whole-second UTC", out.RevokedAt)
	}

	row, err := testHandler.Queries.AccountState(context.Background(), parseUUID(testUserID))
	if err != nil {
		t.Fatalf("read platform_account: %v", err)
	}
	if row.Status != "active" {
		t.Errorf("a new row must start active, got %q", row.Status)
	}
	if !row.SessionsValidAfter.Valid || !row.SessionsValidAfter.Time.Equal(revokedAt) {
		t.Errorf("sessions_valid_after = %v, want %v", row.SessionsValidAfter, revokedAt)
	}

	if len(cache.deleted) != 1 {
		t.Errorf("guard cache invalidated %d times, want 1", len(cache.deleted))
	}

	claims, err := auth.ParseSessionToken(out.Token)
	if err != nil {
		t.Fatalf("returned token does not parse: %v", err)
	}
	if got, _ := auth.SessionAuthTime(claims); got != revokedAt.Unix() {
		t.Errorf("new token auth_time = %d, want revoked_at %d", got, revokedAt.Unix())
	}
	var sawCookie bool
	for _, c := range resp.Result().Cookies() {
		if c.Name == auth.AuthCookieName && c.Value == out.Token {
			sawCookie = true
		}
	}
	if !sawCookie {
		t.Error("the new token was not set as the auth cookie")
	}

	// End to end through the middleware: the new session passes, a session
	// from before the mark is refused.
	protected := middleware.Auth(testHandler.Queries, nil, nil, nil, guard)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	call := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		return rec
	}
	if rec := call(out.Token); rec.Code != http.StatusOK {
		t.Errorf("the replacement session was refused: %d %s", rec.Code, rec.Body.String())
	}
	older := jwt.MapClaims{
		"sub": testUserID, "iat": revokedAt.Unix() - 30, auth.AuthTimeClaim: revokedAt.Unix() - 30,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	oldToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, older).SignedString(auth.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	rec := call(oldToken)
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusUnauthorized || body.Code != "session_revoked" {
		t.Errorf("a session from before the mark got %d %s, want 401 session_revoked", rec.Code, rec.Body.String())
	}
}

// RevokeSessionsForUser touches sessions_valid_after and nothing else; a
// pending-deletion account stays pending-deletion.
func TestRevokeAllSessionsKeepsAnExistingAccountState(t *testing.T) {
	requirePlatformTestDatabase(t)
	resetPlatformAccount(t)
	ctx := context.Background()
	if _, err := testHandler.Queries.SetAccountState(ctx, db.SetAccountStateParams{
		UserID: parseUUID(testUserID), Status: "pending_deletion", Reason: "requested by user",
		DeletionScheduledAt: pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("seed platform_account: %v", err)
	}

	decodeRevokeAll(t, testutil.Call(t, testHandler.RevokeAllSessions(nil), revokeAllRequest()))

	row, err := testHandler.Queries.AccountState(ctx, parseUUID(testUserID))
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "pending_deletion" {
		t.Errorf("status = %q, want pending_deletion left alone", row.Status)
	}
	var reason string
	var scheduled pgtype.Timestamptz
	if err := testPool.QueryRow(ctx, `SELECT reason, deletion_scheduled_at FROM platform_account WHERE user_id = $1::uuid`, testUserID).Scan(&reason, &scheduled); err != nil {
		t.Fatal(err)
	}
	if reason != "requested by user" || !scheduled.Valid {
		t.Errorf("reason/deletion_scheduled_at were overwritten: %q %v", reason, scheduled)
	}
	if !row.SessionsValidAfter.Valid {
		t.Error("sessions_valid_after was not set")
	}
}

// Behind RequireHumanActor, an agent task token or cloud-node PAT is refused
// before anything is written.
func TestRevokeAllSessionsRefusesMachineCredentials(t *testing.T) {
	requirePlatformTestDatabase(t)
	resetPlatformAccount(t)
	h := RequireHumanActor(testHandler.RevokeAllSessions(nil))
	for _, source := range []string{"task_token", "cloud_pat"} {
		req := revokeAllRequest()
		req.Header.Set("X-Actor-Source", source)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", source, rec.Code)
		}
	}
	if _, err := testHandler.Queries.AccountState(context.Background(), parseUUID(testUserID)); err == nil {
		t.Error("a refused call still wrote platform_account")
	}
}

type allowAllMembership struct{}

func (allowAllMembership) IsMember(context.Context, string, string) (bool, error) { return true, nil }
func (allowAllMembership) Members(_ context.Context, ids []realtime.MembershipIdentity) (map[realtime.MembershipIdentity]bool, error) {
	out := make(map[realtime.MembershipIdentity]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// The user's open sockets are closed, so a signed-out device stops receiving
// events instead of waiting for its next HTTP call to fail.
func TestRevokeAllSessionsDisconnectsTheUsersWebSocket(t *testing.T) {
	requirePlatformTestDatabase(t)
	resetPlatformAccount(t)

	hub := realtime.NewHub()
	go hub.Run()
	mc := allowAllMembership{}
	hub.SetMembershipChecker(mc)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		realtime.HandleWebSocket(hub, mc, nil, nil, nil, w, r)
	}))
	defer server.Close()

	prevHub := testHandler.Hub
	testHandler.Hub = hub
	t.Cleanup(func() { testHandler.Hub = prevHub })

	token, err := testHandler.issueJWT(db.User{ID: parseUUID(testUserID), Email: handlerTestEmail, Name: handlerTestName}, []string{"otp"})
	if err != nil {
		t.Fatal(err)
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws?workspace_id="+testWorkspaceID, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	authMsg, _ := json.Marshal(map[string]any{"type": "auth", "payload": map[string]string{"token": token}})
	if err := conn.WriteMessage(websocket.TextMessage, authMsg); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, ack, err := conn.ReadMessage(); err != nil || !strings.Contains(string(ack), "auth_ack") {
		t.Fatalf("auth_ack = %s (err %v)", ack, err)
	}
	// The hub registers the client after the ack is written.
	time.Sleep(100 * time.Millisecond)

	decodeRevokeAll(t, testutil.Call(t, testHandler.RevokeAllSessions(nil), revokeAllRequest()))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("the socket stayed open after sign-out of all devices")
	}
}
