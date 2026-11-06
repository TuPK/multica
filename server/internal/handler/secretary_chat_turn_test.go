package handler

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

// makeSecretaryAgent names the chat's agent the workspace's Secretary for one test.
func makeSecretaryAgent(t *testing.T, agentID string) {
	t.Helper()
	dbfx.Exec(t, `UPDATE workspace SET secretary_agent_id = $1 WHERE id = $2`, agentID, testWorkspaceID)
	t.Cleanup(func() {
		dbfx.Exec(t, `UPDATE workspace SET secretary_agent_id = NULL WHERE id = $1`, testWorkspaceID)
	})
}

// The Secretary's own output in a chat turn is never the reply: a turn that did
// not reply gets the no_response outcome, not its text.
func TestCompleteTask_SecretaryChatTurnWritesNoOutputWithoutAReply(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID, sessionID, _, _ := setupDirectChatSession(t, ctx, "secretary output gate")
	makeSecretaryAgent(t, agentID)
	taskID := sendDirectChat(t, ctx, agentID, sessionID, "Is the launch on track?")
	markTaskRunning(t, ctx, taskID)

	if _, err := testHandler.TaskService.CompleteTask(ctx, parseUUID(taskID), completeResult(t, "Unchecked answer the boss must not see."), "", "", "", false, "", ""); err != nil {
		t.Fatalf("complete task: %v", err)
	}

	rows := assistantRows(t, ctx, sessionID)
	if len(rows) != 1 {
		t.Fatalf("expected exactly one assistant outcome, got %d", len(rows))
	}
	if rows[0].MessageKind != protocol.ChatMessageKindNoResponse {
		t.Fatalf("expected no_response, got %q", rows[0].MessageKind)
	}
	if rows[0].Content == "Unchecked answer the boss must not see." {
		t.Fatal("the Secretary's own output reached the chat")
	}
}

// A Secretary turn that replied through `chat reply` adds no row of its own.
func TestCompleteTask_SecretaryChatTurnAddsNothingAfterAReply(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID, sessionID, _, _ := setupDirectChatSession(t, ctx, "secretary reply gate")
	makeSecretaryAgent(t, agentID)
	taskID := sendDirectChat(t, ctx, agentID, sessionID, "Is the launch on track?")
	markTaskRunning(t, ctx, taskID)

	dbfx.Insert(t, "chat_message", map[string]any{
		"chat_session_id": sessionID,
		"role":            "assistant",
		"content":         "Asked the Release lead; answers by Friday.",
	})
	if _, err := testHandler.TaskService.CompleteTask(ctx, parseUUID(taskID), completeResult(t, "Stdout that must not be written."), "", "", "", false, "", ""); err != nil {
		t.Fatalf("complete task: %v", err)
	}

	rows := assistantRows(t, ctx, sessionID)
	if len(rows) != 1 || rows[0].Content != "Asked the Release lead; answers by Friday." {
		t.Fatalf("expected only the reply, got %+v", rows)
	}
}
