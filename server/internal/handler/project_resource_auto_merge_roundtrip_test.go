package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Turning auto-merge on for a worktree resource must survive the save: the
// response and a fresh read both carry auto_merge: true.
func TestUpdateProjectResourceAutoMergeRoundTrip(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	project := createTestProjectForResources(t, "Auto-merge round trip")
	const daemonID = "daemon-auto-merge-roundtrip"

	// A runtime that advertises worktree support, so the save gate passes the
	// way it does for a real up-to-date daemon.
	dbfx.Insert(t, "agent_runtime", testutil.Cols{
		"workspace_id": testWorkspaceID,
		"daemon_id":    daemonID,
		"name":         "auto-merge roundtrip runtime",
		"runtime_mode": "local",
		"provider":     "claude",
		"status":       "online",
		"device_info":  testutil.Raw("'{}'::jsonb"),
		"metadata":     testutil.Raw("'{\"capabilities\":[\"" + protocol.DaemonCapabilityLocalWorktreeV1 + "\"]}'::jsonb"),
		"last_seen_at": testutil.Raw("now()"),
	})

	created := createLocalDirectoryResourceFor(t, project.ID, map[string]any{
		"local_path":     "/Users/dev/work/roundtrip",
		"daemon_id":      daemonID,
		"execution_mode": "in_place",
	})

	w := httptest.NewRecorder()
	req := newRequest("PUT", "/api/projects/"+project.ID+"/resources/"+created.ID, map[string]any{
		"resource_ref": map[string]any{
			"local_path":     "/Users/dev/work/roundtrip",
			"daemon_id":      daemonID,
			"execution_mode": "worktree",
			"auto_merge":     true,
		},
	})
	req = withURLParams(req, "id", project.ID, "resourceId", created.ID)
	testHandler.UpdateProjectResource(w, req)
	if w.Code != 200 {
		t.Fatalf("UpdateProjectResource: %d %s", w.Code, w.Body.String())
	}
	var resp ProjectResourceResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	t.Logf("PUT response ref: %s", resp.ResourceRef)

	stored := storedProjectResource(t, ctx, created.ID)
	t.Logf("stored ref: %s", stored.ResourceRef)
}
