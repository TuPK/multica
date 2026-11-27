package daemon

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

// TestShadowClaimAdvertisesCapability: the daemon's claim requests carry shadow-v1, so the server
// may give it a shadow run (durable spec 9.4, D30). A daemon without it is never given one.
func TestShadowClaimAdvertisesCapability(t *testing.T) {
	for name, caps := range map[string]string{
		"websocket": daemonClientCapabilities(),
		"http":      daemonHTTPClientCapabilities(),
	} {
		if !strings.Contains(caps, protocol.DaemonCapabilityShadowV1) {
			t.Errorf("%s capabilities %q do not advertise %s", name, caps, protocol.DaemonCapabilityShadowV1)
		}
	}

	var gotCapabilities string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCapabilities = r.Header.Get("X-Client-Capabilities")
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tasks":[]}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	c.SetToken("tok")
	if _, err := c.ClaimTasks(context.Background(), "daemon-x", []string{"rt-a"}, 1); err != nil {
		t.Fatalf("ClaimTasks: %v", err)
	}
	if !strings.Contains(gotCapabilities, protocol.DaemonCapabilityShadowV1) {
		t.Errorf("the claim request sent X-Client-Capabilities %q, without %s", gotCapabilities, protocol.DaemonCapabilityShadowV1)
	}
}

// TestShadowClaimFieldsReachTheDaemon: a claimed shadow task carries shadow and omit_skills, and
// a task without them is an ordinary run.
func TestShadowClaimFieldsReachTheDaemon(t *testing.T) {
	var shadow, plain Task
	if err := json.Unmarshal([]byte(`{"id":"t1","shadow":true,"omit_skills":true}`), &shadow); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"id":"t2"}`), &plain); err != nil {
		t.Fatal(err)
	}
	if !shadow.Shadow || !shadow.OmitSkills {
		t.Errorf("shadow task = %+v, want shadow and omit_skills", shadow)
	}
	if plain.Shadow || plain.OmitSkills {
		t.Errorf("ordinary task = %+v, want neither flag", plain)
	}
}

// TestShadowRunUsesATemporaryWorkspaceRoot: a shadow run's environment goes under a fresh temporary
// directory, and cleanup removes it when the run ends. An ordinary run keeps the configured root.
func TestShadowRunUsesATemporaryWorkspaceRoot(t *testing.T) {
	configured := t.TempDir()

	root, cleanup, err := shadowWorkspaceRoot(Task{Shadow: true}, configured)
	if err != nil {
		t.Fatalf("shadowWorkspaceRoot: %v", err)
	}
	if root == configured || filepath.Dir(root) != os.TempDir() {
		t.Fatalf("shadow root %q is not a fresh directory under %s", root, os.TempDir())
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("shadow root does not exist during the run: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "scratch.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("shadow root %q survives the run: %v", root, err)
	}

	kept, noop, err := shadowWorkspaceRoot(Task{}, configured)
	if err != nil || kept != configured {
		t.Fatalf("ordinary task root = %q (%v), want the configured %q", kept, err, configured)
	}
	noop()
	if _, err := os.Stat(configured); err != nil {
		t.Fatalf("an ordinary run's cleanup removed the configured root: %v", err)
	}
}

// TestShadowRunNeverReusesAWorktree: a shadow run takes no prior environment, so it never claims a
// worktree; an ordinary run may reuse one.
func TestShadowRunNeverReusesAWorktree(t *testing.T) {
	if workspaceReuseAllowed(Task{Shadow: true}) {
		t.Error("a shadow run may reuse a prior environment")
	}
	if !workspaceReuseAllowed(Task{}) {
		t.Error("an ordinary run may not reuse its prior environment")
	}
}
