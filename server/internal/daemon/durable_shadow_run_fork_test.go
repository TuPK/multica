//go:build unix

package daemon

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
