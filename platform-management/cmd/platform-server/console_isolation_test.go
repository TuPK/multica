package main

import (
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/console"
	"github.com/multica-ai/multica/platform-management/internal/pricechange"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// TestConsoleRoutesDoNotReadWorkspaceContent is the route-table half of AC46
// (spec 7b): the console reads a tenant's account, plan, usage, orders and
// staff notes, and no route under /api/platform reads the content of its
// workspace. A new route that names such content, or a new resource under a
// tenant, fails here until the spec allows it.
func TestConsoleRoutesDoNotReadWorkspaceContent(t *testing.T) {
	var patterns []string
	patterns = append(patterns, console.Patterns()...)
	patterns = append(patterns, subscription.ConsolePatterns()...)
	patterns = append(patterns, pricechange.Patterns...)
	if len(patterns) == 0 {
		t.Fatal("no console route patterns")
	}

	content := []string{
		"issue", "comment", "agent", "file", "attachment", "message", "chat",
		"project", "skill", "squad", "runtime", "task", "document", "knowledge",
		"inbox", "label", "channel", "autopilot", "pin",
	}
	tenantResources := map[string]bool{
		"status": true, "usage": true, "audit-events": true, "notes": true,
		"subscription": true, "billing-profile": true,
		// POST /tenants/{id}/delete-now (spec R6 section 6.5) deletes the tenant's
		// workspaces through Multica's deletion path; it reads no workspace content.
		"delete-now": true,
	}
	for _, pattern := range patterns {
		rest, ok := strings.CutPrefix(pattern, "/api/platform/")
		if !ok {
			t.Errorf("pattern %q is outside /api/platform", pattern)
			continue
		}
		segments := strings.Split(rest, "/")
		for _, seg := range segments {
			if strings.HasPrefix(seg, "{") {
				continue
			}
			for _, word := range content {
				if seg == word || seg == word+"s" {
					t.Errorf("pattern %q names workspace content (%q)", pattern, word)
				}
			}
		}
		if segments[0] == "tenants" && len(segments) > 2 && !tenantResources[segments[2]] {
			t.Errorf("pattern %q reads a tenant resource %q that the spec does not list", pattern, segments[2])
		}
	}
}
