package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/trust"
)

func okCheck(name string) Check {
	return Check{Name: name, Ping: func(context.Context) error { return nil }}
}

func failCheck(name string) Check {
	return Check{Name: name, Ping: func(context.Context) error { return errors.New("connection refused to secret-host:5432") }}
}

func get(t *testing.T, base, path string) (int, string) {
	t.Helper()
	return do(t, http.MethodGet, base, path)
}

func do(t *testing.T, method, base, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

type healthBody struct {
	Status string   `json:"status"`
	Failed []string `json:"failed"`
}

func decodeHealth(t *testing.T, body string) healthBody {
	t.Helper()
	var hb healthBody
	if err := json.Unmarshal([]byte(body), &hb); err != nil {
		t.Fatalf("body %q is not JSON: %v", body, err)
	}
	return hb
}

// newListeners builds the two handlers the way the service wires them, with a
// fake upstream standing in for the API listener on the console side.
func newListeners(t *testing.T, health http.Handler, dist string) (api, console *httptest.Server, upstreamHits *atomic.Int32) {
	t.Helper()
	upstreamHits = new(atomic.Int32)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
		w.Header().Set("X-Upstream-Path", r.URL.Path)
		w.Header().Set("X-Upstream-Query", r.URL.RawQuery)
		w.Header().Set("X-Upstream-Forwarded-For", r.Header.Get("X-Forwarded-For"))
		w.WriteHeader(http.StatusTeapot)
		io.WriteString(w, "from upstream")
	}))
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	api = httptest.NewServer(NewAPIHandler(trust.New(trust.DefaultCIDRs()), health, http.NotFoundHandler()))
	t.Cleanup(api.Close)
	console = httptest.NewServer(NewConsoleHandler(health, target, dist))
	t.Cleanup(console.Close)
	return api, console, upstreamHits
}

func TestHealthzReportsOKOnBothListeners(t *testing.T) {
	api, console, _ := newListeners(t, HealthHandler(okCheck("database"), okCheck("redis")), "")
	for name, srv := range map[string]*httptest.Server{"api": api, "console": console} {
		code, body := get(t, srv.URL, "/healthz")
		if code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", name, code, body)
		}
		if hb := decodeHealth(t, body); hb.Status != "ok" {
			t.Fatalf("%s: body = %s", name, body)
		}
	}
}

func TestHealthzNamesTheFailedDependency(t *testing.T) {
	cases := []struct {
		name   string
		checks []Check
		failed []string
	}{
		{"database", []Check{failCheck("database"), okCheck("redis")}, []string{"database"}},
		{"redis", []Check{okCheck("database"), failCheck("redis")}, []string{"redis"}},
		{"both", []Check{failCheck("database"), failCheck("redis")}, []string{"database", "redis"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api, console, _ := newListeners(t, HealthHandler(tc.checks...), "")
			for name, srv := range map[string]*httptest.Server{"api": api, "console": console} {
				code, body := get(t, srv.URL, "/healthz")
				if code != http.StatusServiceUnavailable {
					t.Fatalf("%s: status = %d, body = %s", name, code, body)
				}
				hb := decodeHealth(t, body)
				if hb.Status != "unavailable" || strings.Join(hb.Failed, ",") != strings.Join(tc.failed, ",") {
					t.Fatalf("%s: body = %s", name, body)
				}
				if strings.Contains(body, "secret-host") {
					t.Fatalf("%s: body leaks the dependency error: %s", name, body)
				}
			}
		})
	}
}

func TestHealthzTimesOutASlowDependency(t *testing.T) {
	slow := Check{Name: "redis", Ping: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	srv := httptest.NewServer(newHealthHandler(20*time.Millisecond, slow))
	t.Cleanup(srv.Close)
	code, body := get(t, srv.URL, "/healthz")
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "redis") {
		t.Fatalf("status = %d, body = %s", code, body)
	}
}

func TestConsoleProxiesPlatformAPIToUpstream(t *testing.T) {
	_, console, hits := newListeners(t, HealthHandler(), "")
	req, _ := http.NewRequest(http.MethodPost, console.URL+"/api/platform/x?a=1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusTeapot || string(body) != "from upstream" {
		t.Fatalf("status = %d, body = %q", resp.StatusCode, body)
	}
	if got := resp.Header.Get("X-Upstream-Path"); got != "/api/platform/x" {
		t.Fatalf("upstream path = %q", got)
	}
	if got := resp.Header.Get("X-Upstream-Query"); got != "a=1" {
		t.Fatalf("upstream query = %q", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream hits = %d", hits.Load())
	}
}

func TestConsoleSetsForwardedForFromTheConnection(t *testing.T) {
	_, console, _ := newListeners(t, HealthHandler(), "")
	req, _ := http.NewRequest(http.MethodGet, console.URL+"/api/platform/x", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.9") // a client must not choose its own address
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("X-Upstream-Forwarded-For"); got != "127.0.0.1" {
		t.Fatalf("forwarded for = %q, want 127.0.0.1", got)
	}
}

func TestConsoleKeepsInternalAndOtherAPIPathsOffTheUpstream(t *testing.T) {
	_, console, hits := newListeners(t, HealthHandler(), "")
	for _, path := range []string{
		"/api/v1/internal/x",
		"/api/v1/internal/",
		"/api/other",
		"/api/",
		"/api",
		"/api/platform", // the prefix itself is not a route
		"/api/platformx/y",
		"/api/platform/../v1/internal/x",
		"/api/platform/%2e%2e/v1/internal/x",
		"/api/platform/%2e%2e/%2e%2e/api/v1/internal/x",
		"/api/platform/..%2fv1/internal/x",
		"/api/platform/./x",
		"/api//v1/internal/x",
	} {
		// A path that needs cleaning is redirected to its clean form, which
		// is then refused; neither answer may come from the upstream.
		code, _ := get(t, console.URL, path)
		if code != http.StatusNotFound && (code < 300 || code > 399) {
			t.Errorf("%s: status = %d, want 404 or a redirect", path, code)
		}
		if hits.Load() != 0 {
			t.Fatalf("%s reached the upstream", path)
		}
	}
	for _, path := range []string{"/api/v1/internal/x", "/api/other"} {
		if code, _ := get(t, console.URL, path); code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, code)
		}
	}
}

func TestAPIServesOnlyHealthzAndThePlatformPrefix(t *testing.T) {
	platform := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	api := httptest.NewServer(NewAPIHandler(trust.New(trust.DefaultCIDRs()), HealthHandler(), platform))
	t.Cleanup(api.Close)
	for path, want := range map[string]int{
		"/api/platform/x":    http.StatusTeapot,
		"/api/platform/me":   http.StatusTeapot,
		"/api/v1/internal/x": http.StatusNotFound,
		"/api/platformx/y":   http.StatusNotFound,
		"/api/other":         http.StatusNotFound,
		"/":                  http.StatusNotFound,
		"/healthz":           http.StatusOK,
	} {
		if code, _ := get(t, api.URL, path); code != want {
			t.Errorf("%s: status = %d, want %d", path, code, want)
		}
	}
}

// writeDist builds a console build with the given files and returns its
// directory.
func writeDist(t *testing.T, files map[string]string) string {
	t.Helper()
	dist := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dist, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dist
}

// distFiles is a small build: the entry page, one hashed asset, one other file,
// a dot file and a directory with no index.
var distFiles = map[string]string{
	"index.html":       "<h1>console</h1>",
	"assets/app.js":    "console.log(1)",
	"favicon.svg":      "<svg/>",
	".gitkeep":         "",
	"empty/.keep":      "",
	"assets/sub/a.css": "a{}",
}

func getResp(t *testing.T, method, url string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func TestConsoleServesTheBuildFromDist(t *testing.T) {
	_, console, _ := newListeners(t, HealthHandler(), writeDist(t, distFiles))
	if code, body := get(t, console.URL, "/"); code != http.StatusOK || body != "<h1>console</h1>" {
		t.Fatalf("/: status = %d, body = %q", code, body)
	}
	if code, body := get(t, console.URL, "/index.html"); code != http.StatusOK || body != "<h1>console</h1>" {
		t.Fatalf("/index.html: status = %d, body = %q", code, body)
	}
	if code, body := get(t, console.URL, "/assets/app.js"); code != http.StatusOK || body != "console.log(1)" {
		t.Fatalf("/assets/app.js: status = %d, body = %q", code, body)
	}
	if code, body := get(t, console.URL, "/assets/sub/a.css"); code != http.StatusOK || body != "a{}" {
		t.Fatalf("/assets/sub/a.css: status = %d, body = %q", code, body)
	}
	if code, body := get(t, console.URL, "/favicon.svg"); code != http.StatusOK || body != "<svg/>" {
		t.Fatalf("/favicon.svg: status = %d, body = %q", code, body)
	}
	// A dot file is never served, and never answered with the page either.
	for _, path := range []string{"/.gitkeep", "/empty/.keep", "/.env", "/assets/.hidden"} {
		if code, body := get(t, console.URL, path); code != http.StatusNotFound {
			t.Errorf("%s: status = %d, body = %q, want 404", path, code, body)
		}
	}
}

func TestConsoleNeverServesAFileOutsideTheBuild(t *testing.T) {
	base := t.TempDir()
	dist := filepath.Join(base, "dist")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<h1>console</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "secret.txt"), []byte("outside the build"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, console, _ := newListeners(t, HealthHandler(), dist)
	for _, path := range []string{"/../secret.txt", "/%2e%2e/secret.txt", "/assets/../../secret.txt", "/..%2fsecret.txt"} {
		_, body := get(t, console.URL, path)
		if strings.Contains(body, "outside the build") {
			t.Errorf("%s served a file outside the build: %q", path, body)
		}
	}
}

func TestConsoleAnswersPathsThatAreNotFilesWithTheIndex(t *testing.T) {
	_, console, _ := newListeners(t, HealthHandler(), writeDist(t, distFiles))
	for _, path := range []string{"/tenants", "/tenants/0b0f6f3e", "/users/u1/", "/missing", "/empty/", "/missing.js", "/assetsx"} {
		resp, body := getResp(t, http.MethodGet, console.URL+path)
		if resp.StatusCode != http.StatusOK || body != "<h1>console</h1>" {
			t.Errorf("%s: status = %d, body = %q, want the index", path, resp.StatusCode, body)
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want no-cache", path, got)
		}
		if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Errorf("%s: Content-Type = %q, want text/html", path, got)
		}
	}
}

func TestConsoleIndexRevalidatesOnEveryLoad(t *testing.T) {
	_, console, _ := newListeners(t, HealthHandler(), writeDist(t, distFiles))
	for _, path := range []string{"/", "/index.html"} {
		resp, _ := getResp(t, http.MethodGet, console.URL+path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, resp.StatusCode)
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want no-cache", path, got)
		}
	}
}

func TestConsoleCachesAssetsForGood(t *testing.T) {
	_, console, _ := newListeners(t, HealthHandler(), writeDist(t, distFiles))
	const want = "public, max-age=31536000, immutable"
	for _, path := range []string{"/assets/app.js", "/assets/sub/a.css"} {
		resp, _ := getResp(t, http.MethodGet, console.URL+path)
		if got := resp.Header.Get("Cache-Control"); got != want {
			t.Errorf("%s: Cache-Control = %q, want %q", path, got, want)
		}
	}
	if got := resp(t, console.URL+"/favicon.svg").Header.Get("Cache-Control"); got == want {
		t.Errorf("a file outside /assets/ must not be cached as immutable, got %q", got)
	}
}

func resp(t *testing.T, url string) *http.Response {
	t.Helper()
	r, _ := getResp(t, http.MethodGet, url)
	return r
}

func TestConsoleMissingAssetIsA404NotTheIndex(t *testing.T) {
	_, console, _ := newListeners(t, HealthHandler(), writeDist(t, distFiles))
	for _, path := range []string{"/assets/missing.js", "/assets/", "/assets", "/assets/sub/"} {
		resp, body := getResp(t, http.MethodGet, console.URL+path)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status = %d, body = %q, want 404", path, resp.StatusCode, body)
		}
		if strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
			t.Errorf("%s: a 404 must not be cached as immutable, got %q", path, resp.Header.Get("Cache-Control"))
		}
	}
}

func TestConsoleKeepsTheAPINotFoundRuleWithABuild(t *testing.T) {
	_, console, hits := newListeners(t, HealthHandler(), writeDist(t, distFiles))
	for _, path := range []string{"/api/", "/api/v1/internal/x", "/api/unknown"} {
		resp, body := getResp(t, http.MethodGet, console.URL+path)
		if resp.StatusCode != http.StatusNotFound || strings.Contains(body, "console") {
			t.Errorf("%s: status = %d, body = %q, want the 404 of an unknown API path", path, resp.StatusCode, body)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("upstream was reached %d times by paths outside /api/platform/", n)
	}
	// /api/platform/ still goes to the API, not to the page.
	resp, body := getResp(t, http.MethodGet, console.URL+"/api/platform/me")
	if resp.StatusCode != http.StatusTeapot || body != "from upstream" {
		t.Errorf("/api/platform/me: status = %d, body = %q, want the upstream's answer", resp.StatusCode, body)
	}
}

func TestConsoleServesPagesOnGetAndHeadOnly(t *testing.T) {
	_, console, _ := newListeners(t, HealthHandler(), writeDist(t, distFiles))
	resp, body := getResp(t, http.MethodHead, console.URL+"/tenants")
	if resp.StatusCode != http.StatusOK || body != "" || resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("HEAD /tenants: status = %d, body = %q, Cache-Control = %q", resp.StatusCode, body, resp.Header.Get("Cache-Control"))
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		resp, _ := getResp(t, method, console.URL+"/tenants")
		if resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != "GET, HEAD" {
			t.Errorf("%s /tenants: status = %d, Allow = %q, want 405 with Allow: GET, HEAD", method, resp.StatusCode, resp.Header.Get("Allow"))
		}
	}
}

func TestConsoleReturns404WithoutABuild(t *testing.T) {
	for name, dist := range map[string]string{
		"directory missing": filepath.Join(t.TempDir(), "absent"),
		"no index":          t.TempDir(),
		"not configured":    "",
	} {
		_, console, _ := newListeners(t, HealthHandler(), dist)
		if code, _ := get(t, console.URL, "/"); code != http.StatusNotFound {
			t.Errorf("%s: / status = %d, want 404", name, code)
		}
	}
}

func TestConsoleReportsAnUnreachableAPIAs502(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	target, _ := url.Parse(dead.URL)
	dead.Close()
	console := httptest.NewServer(NewConsoleHandler(HealthHandler(), target, ""))
	t.Cleanup(console.Close)
	if code, _ := get(t, console.URL, "/api/platform/x"); code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", code)
	}
}

func TestServeStopsBothListenersWhenTheContextEnds(t *testing.T) {
	listen := func() net.Listener {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		return ln
	}
	a, c := listen(), listen()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, 5*time.Second,
			Listener{Name: "api", Listener: a, Handler: NewAPIHandler(trust.New(trust.DefaultCIDRs()), HealthHandler(), http.NotFoundHandler())},
			Listener{Name: "console", Listener: c, Handler: http.NotFoundHandler()},
		)
	}()

	for _, ln := range []net.Listener{a, c} {
		url := "http://" + ln.Addr().String() + "/healthz"
		deadline := time.Now().Add(2 * time.Second)
		for {
			resp, err := http.Get(url)
			if err == nil {
				resp.Body.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s never answered: %v", url, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after the context ended")
	}
	if _, err := http.Get("http://" + a.Addr().String() + "/healthz"); err == nil {
		t.Fatal("API listener still answers after shutdown")
	}
}

func TestServeReturnsWhenAListenerFails(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close() // Serve on a closed listener fails at once.
	other, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- Serve(context.Background(), time.Second,
			Listener{Name: "api", Listener: ln, Handler: http.NotFoundHandler()},
			Listener{Name: "console", Listener: other, Handler: http.NotFoundHandler()},
		)
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "api") {
			t.Fatalf("Serve err = %v, want one naming the api listener", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after a listener failed")
	}
}

func TestListenerTimeoutsAreSet(t *testing.T) {
	srv := newHTTPServer(http.NotFoundHandler())
	if srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.WriteTimeout <= 0 || srv.IdleTimeout <= 0 {
		t.Fatalf("timeouts not all set: %+v", srv)
	}
}
