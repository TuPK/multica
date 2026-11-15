package workspaceguard

import (
	"os"
	"strings"
	"testing"
)

const routerFile = "../../cmd/server/router.go"

func readRouter(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(routerFile)
	if err != nil {
		t.Fatalf("read router: %v", err)
	}
	return string(b)
}

// block returns the text of the func literal that starts at opener, up to its
// closing brace. Chi path parameters are balanced inside their string literals,
// so counting braces is enough here.
func block(t *testing.T, src, opener string) string {
	t.Helper()
	start := strings.Index(src, opener)
	if start < 0 {
		t.Fatalf("router has no %q", opener)
	}
	depth := 0
	for i := start + len(opener) - 1; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1]
			}
		}
	}
	t.Fatalf("no end of the block that starts at %q", opener)
	return ""
}

// The daemon routes are how a running task reports its result, so they must not
// pass the workspace state check (contract section 2). That check lives only in
// the workspace middleware, so the block must not mount it.
func TestDaemonRoutesDoNotMountTheWorkspaceMiddleware(t *testing.T) {
	daemon := block(t, readRouter(t), `r.Route("/api/daemon", func(r chi.Router) {`)
	if !strings.Contains(daemon, "h.ReportTaskMessages") || !strings.Contains(daemon, "h.ClaimTaskByRuntime") {
		t.Fatalf("the extracted block is not the daemon route block:\n%.200s", daemon)
	}
	for _, banned := range []string{"RequireWorkspaceMember", "RequireWorkspaceRole", "WithWorkspaceState"} {
		if strings.Contains(daemon, banned) {
			t.Errorf("the /api/daemon routes mount %s; a running task could no longer report in a non-active workspace", banned)
		}
	}
}

// The gate is always set in the router: a router that forgot either line would
// run every workspace as active.
func TestRouterWiresTheWorkspaceStateIn(t *testing.T) {
	src := readRouter(t)
	for _, want := range []string{
		"h.TaskService.WorkspaceGate = workspaceguard.NewClaimGate(",
		"r.Use(middleware.WithWorkspaceState(",
	} {
		if strings.Count(src, want) != 1 {
			t.Errorf("router.go must contain %q exactly once", want)
		}
	}
	// The guard is mounted before any route is registered: chi rejects a Use
	// after a route, and the workspace middleware reads it from the context.
	use := strings.Index(src, "r.Use(middleware.WithWorkspaceState(")
	firstRoute := strings.Index(src, `r.Get("/health"`)
	if use < 0 || firstRoute < 0 || use > firstRoute {
		t.Errorf("WithWorkspaceState is not mounted ahead of the first route")
	}
}
