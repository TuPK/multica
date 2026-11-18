package realtime

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// memberClient registers a connection through registerClient, as a connect
// does, so the rooms it joins are the production ones.
func memberClient(hub *Hub, workspaceID, userID string, guest bool) *Client {
	c := &Client{send: make(chan []byte, 8), workspaceID: workspaceID, userID: userID, guest: guest}
	hub.registerClient(c)
	return c
}

func gotFrame(c *Client) bool {
	select {
	case <-c.send:
		return true
	case <-time.After(20 * time.Millisecond):
		return false
	}
}

type recordedPublish struct{ scopeType, scopeID, exclude string }

type recordingPublisher struct{ published []recordedPublish }

func (p *recordingPublisher) PublishWithID(scopeType, scopeID, exclude string, _ []byte, _ string) error {
	p.published = append(p.published, recordedPublish{scopeType, scopeID, exclude})
	return nil
}

// A per-person event published through the relay must reach the same people
// on the other side: the publish path keeps scope and scope id, and the
// stream path delivers a stored envelope to the recipient's member room and
// to nobody else in the workspace.
func TestRelayKeepsRecipients(t *testing.T) {
	const ws = "workspace-1"
	scopeID := WorkspaceMemberScopeID(ws, "alice")
	frame := []byte(`{"type":"issue:updated","payload":{}}`)

	t.Run("publish path", func(t *testing.T) {
		hub := NewHub()
		alice := memberClient(hub, ws, "alice", false)
		bob := memberClient(hub, ws, "bob", false)
		relay := &recordingPublisher{}

		newDualWriteBroadcaster(hub, relay).BroadcastToScope(ScopeWorkspaceMember, scopeID, frame)

		if len(relay.published) != 1 || relay.published[0] != (recordedPublish{ScopeWorkspaceMember, scopeID, ""}) {
			t.Fatalf("relay saw %+v, want one publish to %s/%s", relay.published, ScopeWorkspaceMember, scopeID)
		}
		if !gotFrame(alice) {
			t.Error("recipient missed the local fast path")
		}
		if gotFrame(bob) {
			t.Error("another member of the workspace received a per-person event")
		}
	})

	t.Run("stream path", func(t *testing.T) {
		// The frame crosses Redis as an XADD envelope and is read back by the
		// relay on another node.
		sent := newEnvelope("node-a", ScopeWorkspaceMember, scopeID, "", frame, "evt-1")
		msg := redis.XMessage{ID: "1-0", Values: envelopeRedisValues(sent)}
		got, ok := envelopeFromXMessage(msg)
		if !ok || got.Scope != ScopeWorkspaceMember || got.ScopeID != scopeID {
			t.Fatalf("envelope lost its recipient: %+v ok=%v", got, ok)
		}

		hub := NewHub()
		alice := memberClient(hub, ws, "alice", false)
		bob := memberClient(hub, ws, "bob", false)
		guest := memberClient(hub, ws, "guest", true)
		deliverEnvelope(hub, nil, nil, got)

		if !gotFrame(alice) {
			t.Error("recipient did not receive the relayed event")
		}
		if gotFrame(bob) || gotFrame(guest) {
			t.Error("relayed per-person event reached someone else in the workspace")
		}
	})
}

// A guest is never in the workspace room; they are in their own user and
// member rooms only.
func TestGuestClientJoinsNoWorkspaceRoom(t *testing.T) {
	hub := NewHub()
	guest := memberClient(hub, "workspace-1", "guest", true)
	member := memberClient(hub, "workspace-1", "member", false)

	hub.BroadcastToWorkspace("workspace-1", []byte(`{"type":"agent:status"}`))
	if gotFrame(guest) {
		t.Error("guest received a workspace-room frame")
	}
	if !gotFrame(member) {
		t.Error("member missed a workspace-room frame")
	}
	hub.SendToUser("guest", []byte(`{"type":"inbox:new"}`))
	hub.BroadcastToScope(ScopeWorkspaceMember, WorkspaceMemberScopeID("workspace-1", "guest"), []byte(`{"type":"issue:created"}`))
	if !gotFrame(guest) || !gotFrame(guest) {
		t.Error("guest missed a frame addressed to their user or member room")
	}
}

// DisconnectGuestFlagChange closes the user's connections to that workspace
// and no one else's, without a removal notice.
func TestDisconnectGuestFlagChangeClosesOnlyThatUsersWorkspaceConnections(t *testing.T) {
	hub := NewHub()
	target := memberClient(hub, "workspace-1", "alice", false)
	otherWorkspace := memberClient(hub, "workspace-2", "alice", false)
	otherUser := memberClient(hub, "workspace-1", "bob", false)

	hub.DisconnectGuestFlagChange("workspace-1", "alice")

	if !target.stopped.Load() {
		t.Error("the user's connection to the workspace stayed open")
	}
	if otherWorkspace.stopped.Load() || otherUser.stopped.Load() {
		t.Error("an unrelated connection was closed")
	}
}
