package daemon

import (
	"fmt"
	"os"
)

// A shadow run (durable spec 9.4 (c)) is a candidate model working beside a seat. The daemon
// runs it in a temporary workspace root that goes with the run, never reuses a prior worktree or
// environment, and never claims one, so it cannot touch the user's working copy.

// shadowWorkspaceRoot returns the workspace root a task's environment is prepared under, and the
// cleanup that removes it when the run ends. Any other task keeps the configured root, and its
// cleanup does nothing.
func shadowWorkspaceRoot(task Task, configured string) (string, func(), error) {
	if !task.Shadow {
		return configured, func() {}, nil
	}
	root, err := os.MkdirTemp("", "shadow-run-")
	if err != nil {
		return "", func() {}, fmt.Errorf("shadow run directory: %w", err)
	}
	return root, func() { _ = os.RemoveAll(root) }, nil
}

// workspaceReuseAllowed reports whether a task may reuse a prior run's environment. A shadow run
// never does.
func workspaceReuseAllowed(task Task) bool {
	return !task.Shadow
}
