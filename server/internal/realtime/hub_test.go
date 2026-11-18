package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/server/internal/auth"
)

const testWorkspaceID = "test-workspace"
const testUserID = "test-user"

// mockMembershipChecker always returns true.
type mockMembershipChecker struct{}

func (m *mockMembershipChecker) IsMember(_ context.Context, _, _ string) (bool, error) {
	return true, nil
}

func (*mockMembershipChecker) Members(_ context.Context, identities []MembershipIdentity) (map[MembershipIdentity]bool, error) {
	members := make(map[MembershipIdentity]bool, len(identities))
	for _, identity := range identities {
		members[identity] = true
	}
	return members, nil
}

func makeTestToken(t *testing.T) string {
	return makeTestTokenForUser(t, testUserID, "")
}

func makeTestTokenForUser(t *testing.T, userID, email string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": testUserID,
	}
	if userID != "" {
		claims["sub"] = userID
	}
	if email != "" {
		claims["email"] = email
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(auth.JWTSecret())
	if err != nil {
		t.Fatalf("failed to sign test JWT: %v", err)
	}
	return signed
}

type staticPATResolver map[string]string

func (s staticPATResolver) ResolveToken(_ context.Context, token string) (auth.PATEntry, bool) {
	userID, ok := s[token]
	return auth.PATEntry{UserID: userID}, ok
}

func TestAuthenticateTokenRejectsTemporarilyDisabledJWTUser(t *testing.T) {
	token := makeTestTokenForUser(t, "514492f7-b30f-4147-bd33-c0e8ce5d6d4f", "")

	uid, _, errMsg := authenticateToken(token, nil, context.Background(), "")
	if uid != "" {
		t.Fatalf("expected no user ID, got %q", uid)
	}
	if !strings.Contains(errMsg, "account disabled") {
		t.Fatalf("expected account disabled error, got %q", errMsg)
	}
}

func TestAuthenticateTokenRejectsTemporarilyDisabledPATUser(t *testing.T) {
	uid, _, errMsg := authenticateToken("mul_disabled", staticPATResolver{
		"mul_disabled": "1d542296-17c6-484a-9914-dcee589be116",
	}, context.Background(), "")
	if uid != "" {
		t.Fatalf("expected no user ID, got %q", uid)
	}
	if !strings.Contains(errMsg, "account disabled") {
		t.Fatalf("expected account disabled error, got %q", errMsg)
	}
}

func newTestHub(t *testing.T) (*Hub, *httptest.Server) {
	t.Helper()
	hub := NewHub()
	go hub.Run()

	mc := &mockMembershipChecker{}
	hub.SetMembershipChecker(mc)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(hub, mc, nil, nil, nil, w, r)
	})
	server := httptest.NewServer(mux)
	return hub, server
}

func connectWS(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	token := makeTestToken(t)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?workspace_id=" + testWorkspaceID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect WebSocket: %v", err)
	}
	authMsg, _ := json.Marshal(map[string]any{
		"type":    "auth",
		"payload": map[string]string{"token": token},
	})
	if err := conn.WriteMessage(websocket.TextMessage, authMsg); err != nil {
		t.Fatalf("failed to send auth message: %v", err)
	}
	// Read auth_ack before returning the connection.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, ack, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read auth_ack: %v", err)
	}
	if !strings.Contains(string(ack), "auth_ack") {
		t.Fatalf("expected auth_ack, got %s", ack)
	}
	conn.SetReadDeadline(time.Time{})
	return conn
}

type failingScopeAuthorizer struct{}

func (failingScopeAuthorizer) AuthorizeScope(context.Context, string, string, string, string) (bool, error) {
	return false, errors.New("database unavailable")
}

func TestClientHandleSubscribeReportsLookupFailure(t *testing.T) {
	hub := NewHub()
	hub.SetAuthorizer(failingScopeAuthorizer{})
	client := &Client{
		hub:           hub,
		send:          make(chan []byte, 1),
		userID:        testUserID,
		workspaceID:   testWorkspaceID,
		subscriptions: make(map[scopeKey]bool),
	}

	client.handleSubscribe(ScopeTask, "task-id")

	select {
	case raw := <-client.send:
		var frame struct {
			Type    string            `json:"type"`
			Payload map[string]string `json:"payload"`
		}
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatalf("unmarshal subscribe error: %v", err)
		}
		if frame.Type != "subscribe_error" {
			t.Fatalf("frame type = %q, want subscribe_error", frame.Type)
		}
		if got := frame.Payload["error"]; got != "lookup_failed" {
			t.Fatalf("error = %q, want lookup_failed", got)
		}
		if got := frame.Payload["scope"]; got != ScopeTask {
			t.Fatalf("scope = %q, want %q", got, ScopeTask)
		}
		if got := frame.Payload["id"]; got != "task-id" {
			t.Fatalf("id = %q, want task-id", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscribe_error")
	}

	if len(client.subscriptions) != 0 {
		t.Fatalf("lookup failure must not subscribe client, got %d subscriptions", len(client.subscriptions))
	}
}

// A workspace-member room carries one member's private content in one
// workspace. A connection joins only its own room, at connect time, and no
// subscribe frame can add another one.
func TestWorkspaceMemberScopeReachesOnlyThatMembersWorkspaceConnection(t *testing.T) {
	hub := NewHub()
	newClient := func(userID, workspaceID string) *Client {
		c := &Client{hub: hub, send: make(chan []byte, 4), userID: userID, workspaceID: workspaceID}
		hub.registerClient(c)
		return c
	}
	owner := newClient("owner", "ws-x")
	ownerElsewhere := newClient("owner", "ws-y")
	peer := newClient("peer", "ws-x")

	peer.handleSubscribe(ScopeWorkspaceMember, WorkspaceMemberScopeID("ws-x", "owner"))
	select {
	case raw := <-peer.send:
		if !strings.Contains(string(raw), "subscribe_error") {
			t.Fatalf("peer subscribe reply = %s, want subscribe_error", raw)
		}
	default:
		t.Fatal("peer subscribe to another member's room was not refused")
	}

	hub.BroadcastToScope(ScopeWorkspaceMember, WorkspaceMemberScopeID("ws-x", "owner"), []byte(`{"type":"chat:done"}`))
	if len(owner.send) != 1 {
		t.Fatalf("owner's ws-x connection got %d frames, want 1", len(owner.send))
	}
	if len(ownerElsewhere.send) != 0 || len(peer.send) != 0 {
		t.Fatalf("frame reached ws-y owner (%d) or peer (%d)", len(ownerElsewhere.send), len(peer.send))
	}
}

// totalClients counts all currently registered clients.
func totalClients(hub *Hub) int {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.clients)
}

func TestHub_ClientRegistration(t *testing.T) {
	hub, server := newTestHub(t)
	defer server.Close()

	conn := connectWS(t, server)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	count := totalClients(hub)
	if count != 1 {
		t.Fatalf("expected 1 client, got %d", count)
	}
}

func TestHub_Broadcast(t *testing.T) {
	hub, server := newTestHub(t)
	defer server.Close()

	conn1 := connectWS(t, server)
	defer conn1.Close()
	conn2 := connectWS(t, server)
	defer conn2.Close()

	time.Sleep(50 * time.Millisecond)

	msg := []byte(`{"type":"issue:created","data":"test"}`)
	hub.Broadcast(msg)

	conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, received1, err := conn1.ReadMessage()
	if err != nil {
		t.Fatalf("client 1 read error: %v", err)
	}
	if string(received1) != string(msg) {
		t.Fatalf("client 1: expected %s, got %s", msg, received1)
	}

	conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, received2, err := conn2.ReadMessage()
	if err != nil {
		t.Fatalf("client 2 read error: %v", err)
	}
	if string(received2) != string(msg) {
		t.Fatalf("client 2: expected %s, got %s", msg, received2)
	}
}

func TestHub_ClientDisconnect(t *testing.T) {
	hub, server := newTestHub(t)
	defer server.Close()

	conn := connectWS(t, server)

	time.Sleep(50 * time.Millisecond)

	countBefore := totalClients(hub)
	if countBefore != 1 {
		t.Fatalf("expected 1 client before disconnect, got %d", countBefore)
	}

	conn.Close()
	time.Sleep(100 * time.Millisecond)

	countAfter := totalClients(hub)
	if countAfter != 0 {
		t.Fatalf("expected 0 clients after disconnect, got %d", countAfter)
	}
}

func TestHub_BroadcastToMultipleClients(t *testing.T) {
	hub, server := newTestHub(t)
	defer server.Close()

	const numClients = 5
	conns := make([]*websocket.Conn, numClients)
	for i := 0; i < numClients; i++ {
		conns[i] = connectWS(t, server)
		defer conns[i].Close()
	}

	time.Sleep(50 * time.Millisecond)

	count := totalClients(hub)
	if count != numClients {
		t.Fatalf("expected %d clients, got %d", numClients, count)
	}

	msg := []byte(`{"type":"test","count":5}`)
	hub.Broadcast(msg)

	for i, conn := range conns {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, received, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("client %d read error: %v", i, err)
		}
		if string(received) != string(msg) {
			t.Fatalf("client %d: expected %s, got %s", i, msg, received)
		}
	}
}

func TestHub_MultipleBroadcasts(t *testing.T) {
	hub, server := newTestHub(t)
	defer server.Close()

	conn := connectWS(t, server)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	messages := []string{
		`{"type":"issue:created"}`,
		`{"type":"issue:updated"}`,
		`{"type":"issue:deleted"}`,
	}

	for _, msg := range messages {
		hub.Broadcast([]byte(msg))
	}

	for i, expected := range messages {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, received, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("message %d read error: %v", i, err)
		}
		if string(received) != expected {
			t.Fatalf("message %d: expected %s, got %s", i, expected, received)
		}
	}
}

// TestHandleWebSocket_ClientIdentityFromQuery verifies that client_platform,
// client_version, and client_os query params on the WS upgrade URL are read
// by the handler and surfaced to the access log. Browsers cannot set custom
// headers on WS upgrades, so this query-param channel is the only way to
// preserve the same observability dimensions HTTP clients get via X-Client-*.
func TestHandleWebSocket_ClientIdentityFromQuery(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	handler := slog.NewJSONHandler(&lockedWriter{w: &buf, mu: &mu}, &slog.HandlerOptions{Level: slog.LevelDebug})
	prevDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(prevDefault) })

	_, server := newTestHub(t)
	defer server.Close()

	token := makeTestToken(t)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") +
		"/ws?workspace_id=" + testWorkspaceID +
		"&client_platform=desktop&client_version=1.2.3&client_os=macos"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	authMsg, _ := json.Marshal(map[string]any{
		"type":    "auth",
		"payload": map[string]string{"token": token},
	})
	if err := conn.WriteMessage(websocket.TextMessage, authMsg); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read auth_ack: %v", err)
	}

	// Wait briefly for the "websocket connected" log line to be flushed.
	deadline := time.Now().Add(2 * time.Second)
	var found map[string]any
	for time.Now().Before(deadline) {
		mu.Lock()
		raw := buf.String()
		mu.Unlock()
		for _, line := range strings.Split(raw, "\n") {
			if line == "" {
				continue
			}
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			if msg, _ := entry["msg"].(string); msg == "websocket connected" {
				found = entry
				break
			}
		}
		if found != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if found == nil {
		t.Fatalf("did not observe \"websocket connected\" log entry; buffered logs:\n%s", buf.String())
	}
	if got, _ := found["client_platform"].(string); got != "desktop" {
		t.Errorf("client_platform = %q, want %q", got, "desktop")
	}
	if got, _ := found["client_version"].(string); got != "1.2.3" {
		t.Errorf("client_version = %q, want %q", got, "1.2.3")
	}
	if got, _ := found["client_os"].(string); got != "macos" {
		t.Errorf("client_os = %q, want %q", got, "macos")
	}
}

// lockedWriter is a thread-safe writer used to capture concurrent slog output.
type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

type failingWSWriter struct {
	err error
}

func (f failingWSWriter) WriteMessage(int, []byte) error {
	return f.err
}

func TestWriteWSAuthFrameLogsWriteErrors(t *testing.T) {
	var buf bytes.Buffer
	prevDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prevDefault) })

	ok := writeWSAuthFrame(
		failingWSWriter{err: errors.New("write blocked")},
		[]byte(`{"error":"invalid token"}`),
		"auth_error",
		"workspace_id", testWorkspaceID,
	)

	if ok {
		t.Fatal("expected writeWSAuthFrame to report failed write")
	}
	logs := buf.String()
	if !strings.Contains(logs, "ws: failed to send auth frame") {
		t.Fatalf("expected auth frame write failure log, got:\n%s", logs)
	}
	if !strings.Contains(logs, "write blocked") {
		t.Fatalf("expected write error in log, got:\n%s", logs)
	}
	if !strings.Contains(logs, "auth_error") {
		t.Fatalf("expected frame kind in log, got:\n%s", logs)
	}
	if !strings.Contains(logs, testWorkspaceID) {
		t.Fatalf("expected workspace id in log, got:\n%s", logs)
	}
}

func TestCheckOrigin(t *testing.T) {
	prev := allowedWSOrigins.Load().([]string)
	SetAllowedOrigins([]string{
		"http://localhost:3000",
		"https://multica.ai",
	})
	t.Cleanup(func() { SetAllowedOrigins(prev) })

	prevProxies := trustedProxies.Load().([]netip.Prefix)
	SetTrustedProxies([]netip.Prefix{
		netip.MustParsePrefix("127.0.0.1/32"),
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	})
	t.Cleanup(func() { SetTrustedProxies(prevProxies) })

	cases := []struct {
		name       string
		host       string
		origin     string
		fwdHost    string
		remoteAddr string
		want       bool
	}{
		{"empty origin allowed", "api.multica.ai", "", "", "1.2.3.4:5678", true},
		{"same-origin allowed (native client default)", "localhost:8080", "http://localhost:8080", "", "1.2.3.4:5678", true},
		{"same-origin allowed (https)", "api.multica.ai", "https://api.multica.ai", "", "1.2.3.4:5678", true},
		{"same-origin allowed (case-insensitive host, RFC 7230)", "API.Multica.AI", "https://api.multica.ai", "", "1.2.3.4:5678", true},
		{"whitelisted origin allowed (web cross-origin)", "localhost:8080", "http://localhost:3000", "", "1.2.3.4:5678", true},
		{"whitelisted origin allowed (prod web)", "api.multica.ai", "https://multica.ai", "", "1.2.3.4:5678", true},
		{"unknown origin rejected (CSWSH defense)", "api.multica.ai", "https://evil.com", "", "1.2.3.4:5678", false},
		{"different port rejected", "localhost:8080", "http://localhost:9999", "", "1.2.3.4:5678", false},
		{"X-Forwarded-Host from trusted proxy matches origin", "internal.proxy", "https://multica.ai", "multica.ai", "127.0.0.1:5678", true},
		{"X-Forwarded-Host from trusted proxy case-insensitive", "internal.proxy", "https://Multica.AI", "multica.ai", "10.0.0.1:5678", true},
		{"X-Forwarded-Host from untrusted source rejected", "internal.proxy", "https://example.com", "example.com", "1.2.3.4:5678", false},
		{"X-Forwarded-Host from trusted proxy but evil origin rejected", "internal.proxy", "https://evil.com", "multica.ai", "127.0.0.1:5678", false},
		{"X-Forwarded-Host present but origin matches direct Host", "multica.ai", "https://multica.ai", "other.host", "1.2.3.4:5678", true},
		{"X-Forwarded-Host spoofed by attacker rejected", "internal.proxy", "https://evil.com", "evil.com", "1.2.3.4:5678", false},
		{"X-Forwarded-Host from trusted CIDR range matches origin", "internal.proxy", "https://multica.ai", "multica.ai", "10.5.6.7:5678", true},
		{"X-Forwarded-Host from trusted IPv6 proxy matches origin", "internal.proxy", "https://multica.ai", "multica.ai", "[::1]:5678", true},
		{"X-Forwarded-Host comma list uses first (client-facing) value", "internal.proxy", "https://multica.ai", "multica.ai, proxy.internal", "127.0.0.1:5678", true},
		{"X-Forwarded-Host comma list ignores trailing values", "internal.proxy", "https://staging.multica.ai", "proxy.internal, staging.multica.ai", "127.0.0.1:5678", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/ws", nil)
			r.Host = tc.host
			r.RemoteAddr = tc.remoteAddr
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.fwdHost != "" {
				r.Header.Set("X-Forwarded-Host", tc.fwdHost)
			}
			if got := checkOrigin(r); got != tc.want {
				t.Fatalf("checkOrigin(host=%q, origin=%q, X-Forwarded-Host=%q, remoteAddr=%q) = %v, want %v", tc.host, tc.origin, tc.fwdHost, tc.remoteAddr, got, tc.want)
			}
		})
	}
}

// waitFor polls cond until it holds or the deadline expires. Hub registration
// and the metrics bump both happen off the connection's own goroutine, so
// asserting on them right after a read would race.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The token-auth path reads its first frame before the caller has presented
// any credential, so the read limit has to be in place by then (#6210).
func TestHandleWebSocket_RejectsOversizedFrameBeforeAuth(t *testing.T) {
	hub, server := newTestHub(t)
	defer server.Close()

	before := M.InboundTooLargeTotal.Load()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?workspace_id=" + testWorkspaceID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect WebSocket: %v", err)
	}
	defer conn.Close()

	// Write errors are expected here: the server rejects the frame from its
	// declared length and closes before the payload is drained.
	_ = conn.WriteMessage(websocket.TextMessage, bytes.Repeat([]byte("a"), inboundReadLimit+1))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
		t.Fatalf("read after oversized pre-auth frame = %v, want close code %d", err, websocket.CloseMessageTooBig)
	}

	waitFor(t, "inbound_too_large_total to increment", func() bool {
		return M.InboundTooLargeTotal.Load()-before == 1
	})
	if n := totalClients(hub); n != 0 {
		t.Fatalf("oversized pre-auth frame registered %d clients, want 0", n)
	}
}

func TestReadPump_RejectsOversizedFrameAfterAuth(t *testing.T) {
	hub, server := newTestHub(t)
	defer server.Close()

	conn := connectWS(t, server)
	defer conn.Close()

	waitFor(t, "client registration", func() bool { return totalClients(hub) == 1 })
	before := M.InboundTooLargeTotal.Load()

	oversized, err := json.Marshal(map[string]any{
		"type": "ping",
		"pad":  string(bytes.Repeat([]byte("a"), inboundReadLimit)),
	})
	if err != nil {
		t.Fatalf("marshal oversized frame: %v", err)
	}
	_ = conn.WriteMessage(websocket.TextMessage, oversized)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
		t.Fatalf("read after oversized frame = %v, want close code %d", err, websocket.CloseMessageTooBig)
	}

	waitFor(t, "inbound_too_large_total to increment", func() bool {
		return M.InboundTooLargeTotal.Load()-before == 1
	})
	waitFor(t, "client to be unregistered", func() bool { return totalClients(hub) == 0 })
}

// The limit must not clip legitimate traffic: real frames are ~1 KiB, so
// anything comfortably below the cap has to keep working.
func TestReadPump_AcceptsFrameUnderReadLimit(t *testing.T) {
	_, server := newTestHub(t)
	defer server.Close()

	conn := connectWS(t, server)
	defer conn.Close()

	frame, err := json.Marshal(map[string]any{
		"type": "ping",
		"pad":  string(bytes.Repeat([]byte("a"), inboundReadLimit/2)),
	})
	if err != nil {
		t.Fatalf("marshal ping frame: %v", err)
	}
	if len(frame) >= inboundReadLimit {
		t.Fatalf("test frame is %d bytes, not under the %d byte limit", len(frame), inboundReadLimit)
	}
	if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
		t.Fatalf("write ping: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if !strings.Contains(string(raw), "pong") {
		t.Fatalf("got %s, want a pong frame", raw)
	}
}

func TestDisconnectUserClosesEveryWorkspaceConnectionOfThatUser(t *testing.T) {
	hub := NewHub()
	newClient := func(userID, workspaceID string) *Client {
		c := &Client{hub: hub, send: make(chan []byte, 4), userID: userID, workspaceID: workspaceID}
		hub.registerClient(c)
		return c
	}
	first := newClient("target", "ws-x")
	second := newClient("target", "ws-y")
	other := newClient("other", "ws-x")

	hub.DisconnectUser("target")

	for name, c := range map[string]*Client{"first": first, "second": second} {
		if !c.stopped.Load() {
			t.Errorf("%s connection of the target user was not stopped", name)
		}
		hub.mu.RLock()
		registered := hub.clients[c]
		hub.mu.RUnlock()
		if registered {
			t.Errorf("%s connection of the target user is still registered", name)
		}
	}
	if other.stopped.Load() {
		t.Error("a connection of another user was stopped")
	}
	hub.mu.RLock()
	registered := hub.clients[other]
	hub.mu.RUnlock()
	if !registered {
		t.Error("a connection of another user was removed")
	}
}

func TestDisconnectUserClosesTheSocket(t *testing.T) {
	hub, server := newTestHub(t)
	defer server.Close()
	conn := connectWS(t, server)
	defer conn.Close()
	waitFor(t, "client registration", func() bool {
		hub.mu.RLock()
		defer hub.mu.RUnlock()
		return len(hub.clients) == 1
	})

	hub.DisconnectUser(testUserID)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("read succeeded after DisconnectUser; want the socket closed")
	}
}

func TestDisconnectUserIgnoresEmptyUserAndNilHub(t *testing.T) {
	hub := NewHub()
	anonymous := &Client{hub: hub, send: make(chan []byte, 1), workspaceID: "ws-x"}
	hub.registerClient(anonymous)

	hub.DisconnectUser("")
	(*Hub)(nil).DisconnectUser("target")

	if anonymous.stopped.Load() {
		t.Fatal("an empty user ID matched a connection with no user")
	}
}

type fakeAccountStore struct {
	status accountguard.State
	after  time.Time
	found  bool
	err    error
	mu     sync.Mutex
	calls  int
}

func (s *fakeAccountStore) AccountState(context.Context, string) (accountguard.Record, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return accountguard.Record{Status: s.status, SessionsValidAfter: s.after}, s.found, s.err
}

func (s *fakeAccountStore) lookups() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type noAccountCache struct{}

func (noAccountCache) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (noAccountCache) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (noAccountCache) Del(context.Context, string) error                        { return nil }

func newGuardedTestServer(t *testing.T, store *fakeAccountStore) *httptest.Server {
	t.Helper()
	hub := NewHub()
	go hub.Run()
	mc := &mockMembershipChecker{}
	hub.SetMembershipChecker(mc)
	guard := accountguard.New(store, noAccountCache{})
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(hub, mc, nil, nil, guard, w, r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func guardedWSURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?workspace_id=" + testWorkspaceID
}

func TestHandleWebSocketRefusesNonActiveAccountOnCookieUpgrade(t *testing.T) {
	cases := []struct {
		name       string
		store      *fakeAccountStore
		wantStatus int
		wantCode   string
	}{
		{"suspended", &fakeAccountStore{found: true, status: accountguard.Suspended}, http.StatusForbidden, "account_suspended"},
		{"deleted", &fakeAccountStore{found: true, status: accountguard.Deleted}, http.StatusForbidden, "account_suspended"},
		{"pending deletion", &fakeAccountStore{found: true, status: accountguard.PendingDeletion}, http.StatusForbidden, "account_pending_deletion"},
		{"guard error", &fakeAccountStore{err: errors.New("database unavailable")}, http.StatusServiceUnavailable, "account_state_unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newGuardedTestServer(t, tc.store)
			header := http.Header{"Cookie": []string{auth.AuthCookieName + "=" + makeTestToken(t)}}

			conn, resp, err := websocket.DefaultDialer.Dial(guardedWSURL(server), header)
			if err == nil {
				conn.Close()
				t.Fatal("upgrade succeeded for a non-active account")
			}
			if resp == nil || resp.StatusCode != tc.wantStatus {
				t.Fatalf("response = %v, want status %d", resp, tc.wantStatus)
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.Code != tc.wantCode {
				t.Fatalf("body code = %q (err %v), want %q", body.Code, err, tc.wantCode)
			}
		})
	}
}

func TestHandleWebSocketAllowsActiveAccountOnCookieUpgrade(t *testing.T) {
	for _, store := range []*fakeAccountStore{
		{found: true, status: accountguard.Active},
		{}, // no platform_account row
	} {
		server := newGuardedTestServer(t, store)
		header := http.Header{"Cookie": []string{auth.AuthCookieName + "=" + makeTestToken(t)}}
		conn, _, err := websocket.DefaultDialer.Dial(guardedWSURL(server), header)
		if err != nil {
			t.Fatalf("upgrade refused for an active account: %v", err)
		}
		conn.Close()
	}
}

// A token sent as the first frame is authenticated after the upgrade, so the
// refusal arrives as an auth_error frame instead of an HTTP status.
func TestHandleWebSocketRefusesNonActiveAccountOnFirstMessageAuth(t *testing.T) {
	store := &fakeAccountStore{found: true, status: accountguard.Suspended}
	server := newGuardedTestServer(t, store)
	conn, _, err := websocket.DefaultDialer.Dial(guardedWSURL(server), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	authMsg, _ := json.Marshal(map[string]any{"type": "auth", "payload": map[string]string{"token": makeTestToken(t)}})
	if err := conn.WriteMessage(websocket.TextMessage, authMsg); err != nil {
		t.Fatalf("send auth: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, frame, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read auth response: %v", err)
	}
	var body struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(frame, &body) != nil || body.Code != "account_suspended" {
		t.Fatalf("frame = %s, want an account_suspended error", frame)
	}
	if strings.Contains(string(frame), "auth_ack") {
		t.Fatal("a suspended account was acknowledged")
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("connection stayed open after the refusal")
	}
}

// An anonymous connection has no user to look up. The guard must not be asked
// about one, whatever the first frame turns out to be.
func TestHandleWebSocketDoesNotConsultGuardForAnonymousConnection(t *testing.T) {
	store := &fakeAccountStore{found: true, status: accountguard.Suspended}
	server := newGuardedTestServer(t, store)
	conn, _, err := websocket.DefaultDialer.Dial(guardedWSURL(server), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"auth","payload":{"token":"not-a-token"}}`)); err != nil {
		t.Fatalf("send auth: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, frame, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(frame), "invalid token") {
		t.Fatalf("frame = %s (err %v), want the invalid token error", frame, err)
	}
	if got := store.lookups(); got != 0 {
		t.Fatalf("guard consulted %d times for an unauthenticated connection", got)
	}
}

func tokenWithAuthTime(t *testing.T, authTime int64) string {
	t.Helper()
	claims := jwt.MapClaims{"sub": testUserID, "iat": authTime + 1000, "auth_time": authTime}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(auth.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestHandleWebSocketRefusesRevokedSessionOnCookieUpgrade(t *testing.T) {
	mark := time.Unix(1_800_000_000, 0)
	store := &fakeAccountStore{found: true, status: accountguard.Active, after: mark}
	server := newGuardedTestServer(t, store)
	header := http.Header{"Cookie": []string{auth.AuthCookieName + "=" + tokenWithAuthTime(t, mark.Unix()-1)}}

	conn, resp, err := websocket.DefaultDialer.Dial(guardedWSURL(server), header)
	if err == nil {
		conn.Close()
		t.Fatal("upgrade succeeded for a revoked session")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("response = %v, want 401", resp)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.Code != "session_revoked" {
		t.Fatalf("body code = %q (err %v), want session_revoked", body.Code, err)
	}
	var cleared bool
	for _, c := range resp.Cookies() {
		if c.Name == auth.AuthCookieName && c.Value == "" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("the revoked session's auth cookie was not cleared")
	}
}

func TestHandleWebSocketAllowsSessionAtOrAfterTheMark(t *testing.T) {
	mark := time.Unix(1_800_000_000, 0)
	server := newGuardedTestServer(t, &fakeAccountStore{found: true, status: accountguard.Active, after: mark})
	header := http.Header{"Cookie": []string{auth.AuthCookieName + "=" + tokenWithAuthTime(t, mark.Unix())}}
	conn, _, err := websocket.DefaultDialer.Dial(guardedWSURL(server), header)
	if err != nil {
		t.Fatalf("a session issued at the mark was refused: %v", err)
	}
	conn.Close()
}

func TestHandleWebSocketRefusesRevokedSessionOnFirstMessageAuth(t *testing.T) {
	mark := time.Unix(1_800_000_000, 0)
	server := newGuardedTestServer(t, &fakeAccountStore{found: true, status: accountguard.Active, after: mark})
	conn, _, err := websocket.DefaultDialer.Dial(guardedWSURL(server), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	authMsg, _ := json.Marshal(map[string]any{"type": "auth", "payload": map[string]string{"token": tokenWithAuthTime(t, mark.Unix()-1)}})
	if err := conn.WriteMessage(websocket.TextMessage, authMsg); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, frame, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(frame), `"code":"session_revoked"`) {
		t.Fatalf("frame = %s (err %v), want a session_revoked error", frame, err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("connection stayed open after the refusal")
	}
}

// A PAT is not a session; the mark does not apply to it.
func TestHandleWebSocketSessionRevokedDoesNotApplyToPAT(t *testing.T) {
	store := &fakeAccountStore{found: true, status: accountguard.Active, after: time.Now().Add(time.Hour)}
	hub := NewHub()
	go hub.Run()
	mc := &mockMembershipChecker{}
	hub.SetMembershipChecker(mc)
	guard := accountguard.New(store, noAccountCache{})
	pr := staticPATResolver{"mul_ok": testUserID}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(hub, mc, pr, nil, guard, w, r)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial(guardedWSURL(server), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	authMsg, _ := json.Marshal(map[string]any{"type": "auth", "payload": map[string]string{"token": "mul_ok"}})
	_ = conn.WriteMessage(websocket.TextMessage, authMsg)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, ack, err := conn.ReadMessage(); err != nil || !strings.Contains(string(ack), "auth_ack") {
		t.Fatalf("PAT connection refused: %s (err %v)", ack, err)
	}
}
