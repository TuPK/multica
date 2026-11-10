package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// createAgentCopyingSecrets posts a create that copies the secrets of
// sourceID, as the caller userID, and returns the recorder.
func createAgentCopyingSecrets(t *testing.T, userID, name, sourceID string, extra map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body := map[string]any{
		"name":                       name,
		"runtime_id":                 testRuntimeID,
		"visibility":                 "private",
		"max_concurrent_tasks":       1,
		"copy_secrets_from_agent_id": sourceID,
	}
	for k, v := range extra {
		body[k] = v
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE workspace_id = $1 AND name = $2`, testWorkspaceID, name)
	})
	w := httptest.NewRecorder()
	testHandler.CreateAgent(w, newRequestAs(userID, http.MethodPost, "/api/agents", body))
	return w
}

// secretsOfAgentNamed reads the stored custom_env and mcp_config of the agent
// with the given name in the test workspace.
func secretsOfAgentNamed(t *testing.T, name string) (customEnv, mcpConfig string) {
	t.Helper()
	if err := testPool.QueryRow(context.Background(), `
		SELECT custom_env::text, COALESCE(mcp_config::text, '')
		FROM agent WHERE workspace_id = $1 AND name = $2
	`, testWorkspaceID, name).Scan(&customEnv, &mcpConfig); err != nil {
		t.Fatalf("read copied agent %q: %v", name, err)
	}
	return customEnv, mcpConfig
}

// makeTestRuntimeUsableByMembers opens the shared test runtime to every
// member for the test's duration, so a plain-member owner can create an agent
// on it. The original visibility is restored on cleanup.
func makeTestRuntimeUsableByMembers(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	var previous string
	if err := testPool.QueryRow(ctx, `SELECT visibility FROM agent_runtime WHERE id = $1`, testRuntimeID).Scan(&previous); err != nil {
		t.Fatalf("read test runtime visibility: %v", err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE agent_runtime SET visibility = 'public' WHERE id = $1`, testRuntimeID); err != nil {
		t.Fatalf("open test runtime: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE agent_runtime SET visibility = $2 WHERE id = $1`, testRuntimeID, previous)
	})
}

func setSourceMcpConfig(t *testing.T, agentID string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE agent SET mcp_config = '{"servers":{"docs":{"token":"mcp-token"}}}' WHERE id = $1`, agentID); err != nil {
		t.Fatalf("set source mcp_config: %v", err)
	}
}

// The agent owner copies the source's env and MCP config. The values are
// stored on the copy, and the audit row names the keys without the values.
func TestCreateAgent_CopySecrets_OwnerCopiesEnvAndMcp(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	sourceID, ownerUserID := agentEnvOwnerFixture(t, "copy-secrets-source", "copy-secrets-owner@multica.test")
	setSourceMcpConfig(t, sourceID)
	makeTestRuntimeUsableByMembers(t)

	const cloneName = "copy-secrets-owner-clone"
	w := createAgentCopyingSecrets(t, ownerUserID, cloneName, sourceID, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateAgent copying secrets: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	env, mcp := secretsOfAgentNamed(t, cloneName)
	if !strings.Contains(env, `"API_KEY"`) || !strings.Contains(env, "secret-value") {
		t.Errorf("copied custom_env = %s; want the source's API_KEY", env)
	}
	if !strings.Contains(mcp, "mcp-token") {
		t.Errorf("copied mcp_config = %s; want the source's MCP config", mcp)
	}

	var details string
	if err := testPool.QueryRow(ctx, `
		SELECT details::text FROM activity_log
		WHERE workspace_id = $1 AND action = 'agent_secrets_copied'
		  AND details->>'source_agent_id' = $2
		ORDER BY created_at DESC LIMIT 1
	`, testWorkspaceID, sourceID).Scan(&details); err != nil {
		t.Fatalf("expected agent_secrets_copied activity row: %v", err)
	}
	if !strings.Contains(details, `"API_KEY"`) {
		t.Errorf("audit details should name the copied key, got: %s", details)
	}
	if strings.Contains(details, "secret-value") || strings.Contains(details, "mcp-token") {
		t.Errorf("audit details must NOT contain secret values, got: %s", details)
	}

	// custom_env is never serialized on agent responses (only its key count),
	// so its value must not appear there. mcp_config follows the existing
	// response rules and is not asserted here.
	if strings.Contains(w.Body.String(), "secret-value") {
		t.Errorf("CreateAgent response leaked a custom_env value: %s", w.Body.String())
	}
}

// A plain member who cannot manage the source gets 403 and no agent is created.
func TestCreateAgent_CopySecrets_UnrelatedMemberForbidden(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	sourceID, _ := agentEnvOwnerFixture(t, "copy-secrets-forbidden-source", "copy-secrets-target-owner@multica.test")
	strangerID := createPermissionTestMember(t, "copy-secrets-stranger@multica.test")

	const cloneName = "copy-secrets-forbidden-clone"
	w := createAgentCopyingSecrets(t, strangerID, cloneName, sourceID, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a member who cannot manage the source, got %d: %s", w.Code, w.Body.String())
	}
	var count int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM agent WHERE workspace_id = $1 AND name = $2`, testWorkspaceID, cloneName).Scan(&count); err != nil {
		t.Fatalf("count agents: %v", err)
	}
	if count != 0 {
		t.Errorf("refused copy still created %d agent(s)", count)
	}
}

// A custom_env the request sends itself wins over the source's, while the
// MCP config it does not send is still copied.
func TestCreateAgent_CopySecrets_ExplicitFieldWins(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	sourceID, ownerUserID := agentEnvOwnerFixture(t, "copy-secrets-explicit-source", "copy-secrets-explicit-owner@multica.test")
	setSourceMcpConfig(t, sourceID)
	makeTestRuntimeUsableByMembers(t)

	const cloneName = "copy-secrets-explicit-clone"
	w := createAgentCopyingSecrets(t, ownerUserID, cloneName, sourceID, map[string]any{
		"custom_env": map[string]string{"OTHER": "typed-value"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateAgent: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	env, mcp := secretsOfAgentNamed(t, cloneName)
	var got map[string]string
	if err := json.Unmarshal([]byte(env), &got); err != nil {
		t.Fatalf("decode copied env: %v", err)
	}
	if len(got) != 1 || got["OTHER"] != "typed-value" {
		t.Errorf("copied custom_env = %v; want only the explicit OTHER key", got)
	}
	if !strings.Contains(mcp, "mcp-token") {
		t.Errorf("copied mcp_config = %s; want the source's MCP config", mcp)
	}
}

// With the workspace's always-redact policy on, mcp_config is not copied
// because the copy would hand the secret to a place that policy hides it from.
func TestCreateAgent_CopySecrets_AlwaysRedactSkipsMcp(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	sourceID, ownerUserID := agentEnvOwnerFixture(t, "copy-secrets-redact-source", "copy-secrets-redact-owner@multica.test")
	setSourceMcpConfig(t, sourceID)
	makeTestRuntimeUsableByMembers(t)

	var previous string
	if err := testPool.QueryRow(ctx, `SELECT COALESCE(settings::text, '{}') FROM workspace WHERE id = $1`, testWorkspaceID).Scan(&previous); err != nil {
		t.Fatalf("read workspace settings: %v", err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE workspace SET settings = '{"always_redact_env": true}' WHERE id = $1`, testWorkspaceID); err != nil {
		t.Fatalf("enable always-redact: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `UPDATE workspace SET settings = $2::jsonb WHERE id = $1`, testWorkspaceID, previous)
	})

	const cloneName = "copy-secrets-redact-clone"
	w := createAgentCopyingSecrets(t, ownerUserID, cloneName, sourceID, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateAgent: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	env, mcp := secretsOfAgentNamed(t, cloneName)
	if !strings.Contains(env, "secret-value") {
		t.Errorf("copied custom_env = %s; env is not governed by always-redact and should copy", env)
	}
	if strings.Contains(mcp, "mcp-token") {
		t.Errorf("copied mcp_config = %s; always-redact must keep it out of the copy", mcp)
	}
}

// An agent acting for its owner cannot copy secrets out of another agent.
func TestCreateAgent_CopySecrets_AgentActorRejected(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	sourceID, ownerUserID := agentEnvOwnerFixture(t, "copy-secrets-actor-source", "copy-secrets-actor-owner@multica.test")

	hostAgentID := createHandlerTestAgent(t, "copy-secrets-actor-host", nil)
	if _, err := testPool.Exec(ctx, `UPDATE agent SET owner_id = $1 WHERE id = $2`, ownerUserID, hostAgentID); err != nil {
		t.Fatalf("assign host agent owner: %v", err)
	}
	hostTaskID := createHandlerTestTaskForAgent(t, hostAgentID)

	const cloneName = "copy-secrets-actor-clone"
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE workspace_id = $1 AND name = $2`, testWorkspaceID, cloneName)
	})
	req := newRequestAs(ownerUserID, http.MethodPost, "/api/agents", map[string]any{
		"name":                       cloneName,
		"runtime_id":                 testRuntimeID,
		"visibility":                 "private",
		"max_concurrent_tasks":       1,
		"copy_secrets_from_agent_id": sourceID,
	})
	req.Header.Set("X-Agent-ID", hostAgentID)
	req.Header.Set("X-Task-ID", hostTaskID)
	w := httptest.NewRecorder()
	testHandler.CreateAgent(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 from an agent actor, got %d: %s", w.Code, w.Body.String())
	}
}
