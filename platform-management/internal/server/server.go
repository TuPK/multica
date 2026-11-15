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
// routes go to consoleAPI and every other path to auth, which serves sign-in
// and answers an unknown path with a 404. Each console route applies the staff
// middleware itself, outside its method switch.
func NewPlatformHandler(auth, consoleAPI http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/platform/", auth)
	for _, pattern := range console.Patterns() {
		mux.Handle(pattern, consoleAPI)
	}
	return mux
}

// NewAPIHandler returns the API listener's handler: /healthz, and every path
// under /api/platform/, which belongs to platform. Any other path is a 404.
func NewAPIHandler(health, platform http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", health)
	mux.Handle("/api/platform/", platform)
	return mux
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

// staticHandler serves regular files under dir and a directory's index.html.
// It never lists a directory and never serves a dot file such as .gitkeep.
func staticHandler(dir string) http.Handler {
	if dir == "" {
		return http.NotFoundHandler()
	}
	root := http.Dir(dir)
	files := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + r.URL.Path)
		for _, part := range strings.Split(clean, "/") {
			if strings.HasPrefix(part, ".") {
				http.NotFound(w, r)
				return
			}
		}
		target := clean
		f, err := root.Open(target)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		info, err := f.Stat()
		f.Close()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if info.IsDir() {
			index, err := root.Open(path.Join(target, "index.html"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			index.Close()
		}
		files.ServeHTTP(w, r)
	})
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
