package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/auth"
)

// TestShadowTaskMutationReturns403ShadowTask: a shadow run's token may read, but a request that
// would change workspace data answers 403 shadow_task (durable spec 9.4 (d)). A task that is not a
// shadow run keeps the ordinary behaviour with the same token shape.
func TestShadowTaskMutationReturns403ShadowTask(t *testing.T) {
	if testPool == nil {
		t.Skip("no database connection")
	}
	ctx := context.Background()
	var agentID, runtimeID string
	if err := testPool.QueryRow(ctx, `
		SELECT id::text, runtime_id::text FROM agent
		WHERE workspace_id = $1 AND runtime_id IS NOT NULL AND archived_at IS NULL
		ORDER BY created_at LIMIT 1`, testWorkspaceID).Scan(&agentID, &runtimeID); err != nil {
		t.Fatalf("load integration-test agent: %v", err)
	}

	tokenFor := func(t *testing.T, contextJSON string) string {
		t.Helper()
		var taskID string
		if err := testPool.QueryRow(ctx, `
			INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority, context)
			VALUES ($1, $2, 'queued', 0, $3::jsonb) RETURNING id::text`,
			agentID, runtimeID, contextJSON).Scan(&taskID); err != nil {
			t.Fatalf("insert task: %v", err)
		}
		token, err := auth.GenerateAgentTaskToken()
		if err != nil {
			t.Fatalf("generate task token: %v", err)
		}
		var tokenID string
		if err := testPool.QueryRow(ctx, `
			INSERT INTO task_token (token_hash, task_id, agent_id, workspace_id, user_id, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id::text`,
			auth.HashToken(token), taskID, agentID, testWorkspaceID, testUserID, time.Now().Add(time.Hour)).Scan(&tokenID); err != nil {
			t.Fatalf("insert task token: %v", err)
		}
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM task_token WHERE id = $1`, tokenID)
			_, _ = testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID)
		})
		return token
	}
	do := func(t *testing.T, token, method, path string) (int, string) {
		t.Helper()
		var body io.Reader
		if method == http.MethodPost {
			body = bytes.NewReader([]byte(`{}`))
		}
		req, err := http.NewRequest(method, testServer.URL+path, body)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("perform request: %v", err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}

	shadowToken := tokenFor(t, `{"durable_kind":"shadow","shadow_pair_id":"00000000-0000-0000-0000-000000000001"}`)
	path := "/api/agents/" + agentID + "/dossier"

	status, body := do(t, shadowToken, http.MethodPost, path)
	if status != http.StatusForbidden || !bytes.Contains([]byte(body), []byte(`"code":"shadow_task"`)) {
		t.Errorf("a shadow run's POST: status %d body %s, want 403 with code shadow_task", status, body)
	}
	if status, body := do(t, shadowToken, http.MethodGet, path); bytes.Contains([]byte(body), []byte("shadow_task")) {
		t.Errorf("a shadow run's GET was refused as a change: status %d body %s", status, body)
	}

	ordinaryToken := tokenFor(t, `{}`)
	if status, body := do(t, ordinaryToken, http.MethodPost, path); bytes.Contains([]byte(body), []byte("shadow_task")) {
		t.Errorf("an ordinary task's POST was refused as a shadow run: status %d body %s", status, body)
	}
}
