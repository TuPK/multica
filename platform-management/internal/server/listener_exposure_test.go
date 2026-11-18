package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/trust"
)

const internalPort = "8095"

// The console listener is the only one a browser or the public side reaches.
// The internal routes live on the API listener, so the console must refuse
// them without forwarding, and the API listener must serve them.
func TestInternalRoutesAreNotServedOnTheConsoleListener(t *testing.T) {
	_, console, hits := newListeners(t, HealthHandler(), "")
	internal := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	apiWithInternal := httptest.NewServer(NewAPIHandler(trust.New(trust.DefaultCIDRs()), HealthHandler(), http.NotFoundHandler(),
		Route{Pattern: "GET /api/v1/internal/entitlement-policies/{workspaceId}", Handler: internal}))
	t.Cleanup(apiWithInternal.Close)

	const path = "/api/v1/internal/entitlement-policies/00000000-0000-0000-0000-000000000001"
	if code, _ := get(t, apiWithInternal.URL, path); code != http.StatusNoContent {
		t.Fatalf("internal listener: %s status = %d, want 204", path, code)
	}
	for _, p := range []string{path, "/api/v1/internal/", "/api/v1/internal/anything"} {
		if code, _ := get(t, console.URL, p); code != http.StatusNotFound {
			t.Errorf("console listener: %s status = %d, want 404", p, code)
		}
	}
	if hits.Load() != 0 {
		t.Fatal("an internal path reached the upstream through the console")
	}
}

// composeFiles are the compose files at the repository root and under deploy/.
func composeFiles(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..", "..")
	var files []string
	for _, pattern := range []string{
		filepath.Join(root, "docker-compose*.yml"),
		filepath.Join(root, "docker-compose*.yaml"),
		filepath.Join(root, "deploy", "*", "*compose*.y*ml"),
		filepath.Join(root, "deploy", "*compose*.y*ml"),
	} {
		m, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) == 0 {
		t.Fatal("no compose files found")
	}
	return files
}

// EXPOSE only documents a port. The internal port must not be published, and
// any compose mapping for it must bind a loopback address.
func TestDockerfileDoesNotPublishInternalPort(t *testing.T) {
	df, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(df), "EXPOSE") || !strings.Contains(string(df), internalPort) {
		t.Fatalf("Dockerfile no longer mentions %s; update this test", internalPort)
	}
	for i, line := range strings.Split(string(df), "\n") {
		if strings.Contains(strings.ToUpper(line), "PUBLISH") && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			t.Errorf("Dockerfile line %d publishes a port: %q", i+1, line)
		}
	}

	mapping := regexp.MustCompile(`^\s*-\s*["']?([^"'#]+)`)
	for _, f := range composeFiles(t) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			m := mapping.FindStringSubmatch(line)
			if m == nil || !strings.Contains(m[1], internalPort) || strings.Contains(m[1], "=") {
				continue
			}
			spec := strings.TrimSpace(m[1])
			if !strings.HasPrefix(spec, "127.") && !strings.HasPrefix(spec, "[::1]") {
				t.Errorf("%s:%d publishes the internal port %s beyond loopback: %q", f, i+1, internalPort, spec)
			}
		}
	}
}
