// Package server builds the platform service's two HTTP listeners: the API
// and the console. Handlers take their dependencies as values so tests can
// supply fakes.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/console"
	"github.com/multica-ai/multica/platform-management/internal/subscription"
	"github.com/multica-ai/multica/platform-management/internal/trust"
)

const healthTimeout = 3 * time.Second

// Check is one dependency probe behind /healthz.
type Check struct {
	Name string
	Ping func(ctx context.Context) error
}

type healthHandler struct {
	checks  []Check
	timeout time.Duration
}

// HealthHandler answers 200 {"status":"ok"} when every check passes, and 503
// naming each failed dependency otherwise. Ping errors go to the log, not to
// the caller.
func HealthHandler(checks ...Check) http.Handler {
	return newHealthHandler(healthTimeout, checks...)
}

func newHealthHandler(timeout time.Duration, checks ...Check) http.Handler {
	return &healthHandler{checks: checks, timeout: timeout}
}

func (h *healthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var failed []string
	for _, c := range h.checks {
		ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
		err := c.Ping(ctx)
		cancel()
		if err != nil {
			slog.Warn("health check failed", "dependency", c.Name, "error", err)
			failed = append(failed, c.Name)
		}
	}
	if len(failed) > 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unavailable", "failed": failed})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// NewPlatformHandler returns the handler for /api/platform/: the console's
// routes go to consoleAPI, the plan and subscription routes to billingAPI, and
// every other path to auth, which serves sign-in and answers an unknown path
// with a 404. Each staff route applies the staff middleware itself, outside its
// method switch.
func NewPlatformHandler(auth, consoleAPI, billingAPI http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/platform/", auth)
	for _, pattern := range console.Patterns() {
		mux.Handle(pattern, consoleAPI)
	}
	for _, pattern := range subscription.ConsolePatterns() {
		mux.Handle(pattern, billingAPI)
	}
	return mux
}

// Route is one internal route on the API listener, with its ServeMux pattern.
type Route struct {
	Pattern string
	Handler http.Handler
}

// NewAPIHandler returns the API listener's handler: /healthz, every path under
// /api/platform/, which belongs to platform, and the internal routes Multica's
// clients call. Any other path is a 404.
//
// The whole handler sits behind peers.Guard, so a request that carries
// X-User-ID from outside the trusted networks is a 403 on every path.
func NewAPIHandler(peers *trust.Peers, health, platform http.Handler, routes ...Route) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", health)
	mux.Handle("/api/platform/", platform)
	for _, r := range routes {
		mux.Handle(r.Pattern, r.Handler)
	}
	return peers.Guard(mux)
}

// NewConsoleHandler returns the console listener's handler. Only
// /api/platform/ reaches the API at apiURL; every other /api/ path, the
// internal routes included, is a 404 here. The remaining paths serve the
// console build in distDir.
func NewConsoleHandler(health http.Handler, apiURL *url.URL, distDir string) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(apiURL)
			pr.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("console proxy to the API failed", "path", r.URL.Path, "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": "api unavailable"})
		},
	}

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", health)
	mux.Handle("/api/platform/", rejectDotSegments(proxy))
	mux.Handle("/api/", http.NotFoundHandler())
	mux.Handle("/", staticHandler(distDir))
	return mux
}

// rejectDotSegments answers 404 to a path with a "." or ".." segment after
// decoding. ServeMux cleans such paths only when they are not percent-encoded,
// and an upstream that decodes them would resolve /api/platform/../v1/internal/
// to a route the console must not expose.
func rejectDotSegments(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, seg := range strings.Split(r.URL.Path, "/") {
			if seg == "." || seg == ".." {
				http.NotFound(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

const (
	// assetsPrefix is where the build writes content-hashed files. A file there
	// never changes under its name.
	assetsPrefix        = "/assets"
	assetsCacheControl  = "public, max-age=31536000, immutable"
	indexCacheControl   = "no-cache"
	indexFileName       = "index.html"
	staticAllowedMethod = "GET, HEAD"
)

func isAssetPath(clean string) bool {
	return clean == assetsPrefix || strings.HasPrefix(clean, assetsPrefix+"/")
}

// staticHandler serves the console build in dir, a single-page app. A path
// that names a file serves it. Any other path serves index.html, so a reload
// on a client-side route works, except under /assets/, where a missing file
// is a 404: answering a script request with HTML would hide a stale deploy.
// index.html is never cached without revalidation and files under /assets/ are
// cached for good. It never lists a directory and never serves a dot file such
// as .gitkeep.
func staticHandler(dir string) http.Handler {
	if dir == "" {
		return http.NotFoundHandler()
	}
	root := http.Dir(dir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", staticAllowedMethod)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		clean := path.Clean("/" + r.URL.Path)
		for _, part := range strings.Split(clean, "/") {
			if strings.HasPrefix(part, ".") {
				http.NotFound(w, r)
				return
			}
		}
		asset := isAssetPath(clean)
		fileCache := ""
		if asset {
			fileCache = assetsCacheControl
		}
		if clean != "/"+indexFileName && serveFile(w, r, root, clean, fileCache) {
			return
		}
		if asset || !serveFile(w, r, root, "/"+indexFileName, indexCacheControl) {
			http.NotFound(w, r)
		}
	})
}

// serveFile serves the regular file at name, with cacheControl when it is not
// empty, and reports whether there was such a file.
func serveFile(w http.ResponseWriter, r *http.Request, root http.FileSystem, name, cacheControl string) bool {
	f, err := root.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		return false
	}
	if cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	return true
}

// Listener is one bound socket with the handler that serves it.
type Listener struct {
	Name     string
	Listener net.Listener
	Handler  http.Handler
}

func newHTTPServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// Serve runs every listener until ctx ends or one of them fails, then shuts
// them all down, waiting up to grace for in-flight requests. It returns nil
// after a shutdown that ctx requested.
func Serve(ctx context.Context, grace time.Duration, listeners ...Listener) error {
	type result struct {
		name string
		err  error
	}
	servers := make([]*http.Server, len(listeners))
	results := make(chan result, len(listeners))
	for i, l := range listeners {
		servers[i] = newHTTPServer(l.Handler)
		go func(srv *http.Server, l Listener) {
			slog.Info("listening", "listener", l.Name, "addr", l.Listener.Addr().String())
			results <- result{l.Name, srv.Serve(l.Listener)}
		}(servers[i], l)
	}

	var failure error
	select {
	case <-ctx.Done():
	case r := <-results:
		if !errors.Is(r.err, http.ErrServerClosed) {
			failure = fmt.Errorf("%s listener: %w", r.name, r.err)
		}
		// This result is consumed; the others are drained below.
		results <- result{r.name, http.ErrServerClosed}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	for i, srv := range servers {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown did not finish; closing", "listener", listeners[i].Name, "error", err)
			_ = srv.Close()
		}
	}
	for range listeners {
		r := <-results
		if failure == nil && !errors.Is(r.err, http.ErrServerClosed) {
			failure = fmt.Errorf("%s listener: %w", r.name, r.err)
		}
	}
	return failure
}
