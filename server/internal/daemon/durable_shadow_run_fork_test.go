//go:build unix

package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

// TestShadowRunExecutesInATemporaryDirectoryThatIsRemoved runs a shadow task through runTask with
// a fake agent. The agent works in a temporary directory, not in the daemon's workspace root, and
// the directory is gone when the run ends. Nothing for the task is left in the configured root.
func TestShadowRunExecutesInATemporaryDirectoryThatIsRemoved(t *testing.T) {
	fakeBin := filepath.Join(t.TempDir(), "droid")
	attemptFile := filepath.Join(t.TempDir(), "attempts")
	writeTestExecutable(t, fakeBin, []byte(`#!/bin/sh
cat >/dev/null
printf '%s\n' "$PWD" >> "$DROID_ATTEMPTS"
printf '%s\n' '{"type":"system","subtype":"init","session_id":"shadow-session"}' '{"type":"completion","finalText":"done"}'
`))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer srv.Close()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	workspacesRoot := t.TempDir()
	d := &Daemon{
		client: NewClient(srv.URL), logger: logger,
		workspaces: make(map[string]*workspaceState), activeEnvRoots: make(map[string]int),
		runtimeIndex: map[string]Runtime{"rt-shadow": {ID: "rt-shadow", Provider: "droid"}},
		cfg: Config{WorkspacesRoot: workspacesRoot, AgentTimeout: 10 * time.Second, ServerBaseURL: srv.URL,
			Agents: map[string]AgentEntry{"droid": {Path: fakeBin}}},
	}
	task := Task{
		ID: "task-shadow-run", WorkspaceID: "ws-shadow", RuntimeID: "rt-shadow", IssueID: "issue-shadow",
		AgentID: "agent-shadow", AuthToken: "mat_shadow_fixture", Shadow: true, OmitSkills: true,
		Agent: &AgentData{ID: "agent-shadow", Name: "shadow-agent", CustomEnv: map[string]string{"DROID_ATTEMPTS": attemptFile}},
	}

	if _, err := d.runTask(context.Background(), task, "droid", 0, logger); err != nil {
		t.Fatalf("runTask: %v; logs:\n%s", err, logs.String())
	}

	raw, err := os.ReadFile(attemptFile)
	if err != nil {
		t.Fatalf("the agent did not run: %v; logs:\n%s", err, logs.String())
	}
	workDir := strings.TrimSpace(strings.Split(string(raw), "\n")[0])
	if strings.HasPrefix(workDir, workspacesRoot) {
		t.Fatalf("the shadow agent ran in the workspace root: %s", workDir)
	}
	if !strings.Contains(workDir, "shadow-run-") {
		t.Fatalf("the shadow agent ran in %s, not a shadow run directory", workDir)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("the shadow run's directory %s survives the run (stat err %v)", workDir, err)
	}
	entries, err := os.ReadDir(workspacesRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), task.ID) || strings.Contains(e.Name(), "task-shadow") {
			t.Fatalf("the shadow run left %s in the configured workspace root", e.Name())
		}
	}
}

// TestShadowTaskFromTheClaimResponseRunsThroughRunTask is the claim path: the batch claim request
// advertises shadow-v1, the server answers with a shadow task, and the daemon runs the task it took
// from that response in a temporary directory. The task is not built by hand.
func TestShadowTaskFromTheClaimResponseRunsThroughRunTask(t *testing.T) {
	fakeBin := filepath.Join(t.TempDir(), "droid")
	attemptFile := filepath.Join(t.TempDir(), "attempts")
	writeTestExecutable(t, fakeBin, []byte(`#!/bin/sh
cat >/dev/null
printf '%s\n' "$PWD" >> "$DROID_ATTEMPTS"
printf '%s\n' '{"type":"system","subtype":"init","session_id":"shadow-session"}' '{"type":"completion","finalText":"done"}'
`))
	claimBody, err := json.Marshal(map[string]any{"tasks": []any{map[string]any{
		"id": "task-shadow-claimed", "agent_id": "agent-shadow", "runtime_id": "rt-shadow",
		"issue_id": "issue-shadow", "workspace_id": "ws-shadow", "auth_token": "mat_shadow_fixture",
		"shadow": true, "omit_skills": true,
		"agent": map[string]any{"id": "agent-shadow", "name": "shadow-agent",
			"custom_env": map[string]string{"DROID_ATTEMPTS": attemptFile}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	claimHeaders := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/daemon/tasks/claim" {
			claimHeaders <- r.Header.Get("X-Client-Capabilities")
			_, _ = w.Write(claimBody)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	workspacesRoot := t.TempDir()
	d := &Daemon{
		client: NewClient(srv.URL), logger: logger,
		workspaces: make(map[string]*workspaceState), activeEnvRoots: make(map[string]int),
		runtimeIndex: map[string]Runtime{"rt-shadow": {ID: "rt-shadow", Provider: "droid"}},
		cfg: Config{WorkspacesRoot: workspacesRoot, AgentTimeout: 10 * time.Second, ServerBaseURL: srv.URL,
			Agents: map[string]AgentEntry{"droid": {Path: fakeBin}}},
	}
	d.client.SetToken("tok")

	tasks, err := d.client.ClaimTasks(context.Background(), "daemon-x", []string{"rt-shadow"}, 1)
	if err != nil {
		t.Fatalf("ClaimTasks: %v", err)
	}
	if caps := <-claimHeaders; !strings.Contains(caps, protocol.DaemonCapabilityShadowV1) {
		t.Fatalf("the claim sent X-Client-Capabilities %q, without %s", caps, protocol.DaemonCapabilityShadowV1)
	}
	if len(tasks) != 1 || !tasks[0].Shadow || !tasks[0].OmitSkills {
		t.Fatalf("claimed %+v, want one shadow task with omit_skills", tasks)
	}

	if _, err := d.runTask(context.Background(), *tasks[0], "droid", 0, logger); err != nil {
		t.Fatalf("runTask: %v; logs:\n%s", err, logs.String())
	}

	raw, err := os.ReadFile(attemptFile)
	if err != nil {
		t.Fatalf("the agent did not run: %v; logs:\n%s", err, logs.String())
	}
	workDir := strings.TrimSpace(strings.Split(string(raw), "\n")[0])
	if strings.HasPrefix(workDir, workspacesRoot) || !strings.Contains(workDir, "shadow-run-") {
		t.Fatalf("the claimed shadow task ran in %s, not a temporary shadow run directory", workDir)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("the shadow run's directory %s survives the run (stat err %v)", workDir, err)
	}
}
