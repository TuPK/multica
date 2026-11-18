package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/multica-ai/multica/server/internal/auth"
)

const otherWorkspaceID = "other-workspace"

type entryPATResolver map[string]auth.PATEntry

func (s entryPATResolver) ResolveToken(_ context.Context, token string) (auth.PATEntry, bool) {
	e, ok := s[token]
	return e, ok
}

func patServer(t *testing.T, hub *Hub, pr PATResolver) *httptest.Server {
	t.Helper()
	mc := &mockMembershipChecker{}
	hub.SetMembershipChecker(mc)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(hub, mc, pr, nil, nil, w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func patWSURL(server *httptest.Server, workspaceID string) string {
	return "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?workspace_id=" + workspaceID
}

// dialPAT performs the first-message auth and returns the first frame.
func dialPAT(t *testing.T, server *httptest.Server, workspaceID, token string) (*websocket.Conn, string) {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(patWSURL(server, workspaceID), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	msg, _ := json.Marshal(map[string]any{"type": "auth", "payload": map[string]string{"token": token}})
	if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, frame, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("no first frame: %v", err)
	}
	return conn, string(frame)
}

func TestWSPATBoundToOtherWorkspaceIsRefused(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	pr := entryPATResolver{"mul_bound": {UserID: testUserID, WorkspaceID: testWorkspaceID}}
	server := patServer(t, hub, pr)

	// First-message path.
	conn, frame := dialPAT(t, server, otherWorkspaceID, "mul_bound")
	if strings.Contains(frame, "auth_ack") || !strings.Contains(frame, `"code":"token_workspace_mismatch"`) {
		t.Fatalf("frame = %s, want a token_workspace_mismatch auth error", frame)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("connection stayed open after the refusal")
	}

	// Cookie path.
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/ws?workspace_id="+otherWorkspaceID, nil)
	req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: "mul_bound"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cookie path status = %d, want 403", resp.StatusCode)
	}

	// The bound workspace is accepted.
	_, frame = dialPAT(t, server, testWorkspaceID, "mul_bound")
	if !strings.Contains(frame, "auth_ack") {
		t.Fatalf("bound workspace frame = %s, want auth_ack", frame)
	}
}

func TestWSReadOnlyPATReadsButSendsNoWrites(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	pr := entryPATResolver{"mul_ro": {UserID: testUserID, WorkspaceID: testWorkspaceID, ReadOnly: true}}
	server := patServer(t, hub, pr)

	conn, frame := dialPAT(t, server, testWorkspaceID, "mul_ro")
	if !strings.Contains(frame, "auth_ack") {
		t.Fatalf("frame = %s, want auth_ack", frame)
	}
	waitFor(t, "workspace subscription", func() bool { return hub.HasLocalSubscribers(ScopeWorkspace, testWorkspaceID) })

	hub.BroadcastToWorkspace(testWorkspaceID, []byte(`{"type":"comment:created","payload":{"body":"x"}}`))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, msg, err := conn.ReadMessage(); err != nil || !strings.Contains(string(msg), "comment:created") {
		t.Fatalf("event = %s (err %v), want comment:created", msg, err)
	}

	// An unknown frame is a no-op: no reply, no subscription change.
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"issue:create","payload":{"title":"x"}}`)); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, msg, err := conn.ReadMessage(); err == nil {
		t.Fatalf("unknown frame produced a reply: %s", msg)
	}
	if !hub.HasLocalSubscribers(ScopeWorkspace, testWorkspaceID) {
		t.Fatal("workspace subscription changed after an unknown frame")
	}
}
