//go:build unix

package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/pkg/agent"
)

// TestAdapterRunsUnderThePiBackend runs the adapter the way the daemon runs a custom
// runtime whose protocol_family is pi: through the pi backend, which builds the
// command line, writes the prompt to stdin and reads the event stream. The adapter
// is built from this package and the worker is a shell script.
func TestAdapterRunsUnderThePiBackend(t *testing.T) {
	dir := t.TempDir()
	adapter := filepath.Join(dir, "multica-ocf-adapter")
	if out, err := exec.Command("go", "build", "-o", adapter, ".").CombinedOutput(); err != nil {
		t.Fatalf("build the adapter: %v\n%s", err, out)
	}

	t.Run("completed run", func(t *testing.T) {
		worker := writeWorker(t, "cat > /dev/null\necho 'weekly report drafted'\necho 'figures checked'\n")
		result := runUnderPi(t, adapter, dir, worker)
		if result.Status != "completed" {
			t.Fatalf("status = %q, want completed; error: %s", result.Status, result.Error)
		}
		for _, want := range []string{"weekly report drafted", "figures checked"} {
			if !strings.Contains(result.Output, want) {
				t.Fatalf("output lacks %q: %q", want, result.Output)
			}
		}
	})

	t.Run("failed worker", func(t *testing.T) {
		worker := writeWorker(t, "cat > /dev/null\necho 'partial work'\nexit 3\n")
		result := runUnderPi(t, adapter, dir, worker)
		if result.Status != "failed" {
			t.Fatalf("status = %q, want failed; output: %q", result.Status, result.Output)
		}
		if result.Error == "" {
			t.Fatal("a failed run carries no error message")
		}
	})
}

func writeWorker(t *testing.T, body string) string {
	t.Helper()
	worker := filepath.Join(t.TempDir(), "worker.sh")
	if err := os.WriteFile(worker, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write the worker: %v", err)
	}
	return worker
}

func runUnderPi(t *testing.T, adapter, dir, worker string) agent.Result {
	t.Helper()
	backend, err := agent.New("pi", agent.Config{ExecutablePath: adapter, Logger: slog.Default()})
	if err != nil {
		t.Fatalf("new pi backend: %v", err)
	}
	session, err := backend.Execute(context.Background(), "Write the weekly report", agent.ExecOptions{
		Cwd:        dir,
		Timeout:    2 * time.Minute,
		CustomArgs: []string{"--worker", worker},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	select {
	case result := <-session.Result:
		return result
	case <-time.After(3 * time.Minute):
		t.Fatal("timeout waiting for the pi backend result")
		return agent.Result{}
	}
}
