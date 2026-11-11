package hub

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// authRecorder is an httptest server that records the Authorization header of
// every request it receives.
type authRecorder struct {
	mu     sync.Mutex
	seen   []string
	server *httptest.Server
}

func newAuthRecorder(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *authRecorder {
	t.Helper()
	rec := &authRecorder{}
	rec.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.seen = append(rec.seen, r.Header.Get("Authorization"))
		rec.mu.Unlock()
		if handler != nil {
			handler(w, r)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(rec.server.Close)
	return rec
}

func (rec *authRecorder) headers() []string {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return append([]string(nil), rec.seen...)
}

func hostOf(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u.Host
}

func TestGetAuthorizedSendsTokenOnlyToPinnedHost(t *testing.T) {
	srv := newAuthRecorder(t, nil)
	f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})

	if _, _, err := f.GetAuthorized(context.Background(), srv.server.URL+"/a", 1<<20,
		Credential{Host: hostOf(t, srv.server.URL), Token: "tok-pinned"}); err != nil {
		t.Fatalf("GetAuthorized: %v", err)
	}
	// Host comparison is case-insensitive.
	if _, _, err := f.GetAuthorized(context.Background(), srv.server.URL+"/b", 1<<20,
		Credential{Host: strings.ToUpper(hostOf(t, srv.server.URL)), Token: "tok-pinned"}); err != nil {
		t.Fatalf("GetAuthorized upper host: %v", err)
	}
	// A credential pinned to another host is never sent here.
	if _, _, err := f.GetAuthorized(context.Background(), srv.server.URL+"/c", 1<<20,
		Credential{Host: "other.example.test", Token: "tok-elsewhere"}); err != nil {
		t.Fatalf("GetAuthorized other host: %v", err)
	}
	if _, _, err := f.Get(context.Background(), srv.server.URL+"/d", 1<<20); err != nil {
		t.Fatalf("Get: %v", err)
	}

	want := []string{"Bearer tok-pinned", "Bearer tok-pinned", "", ""}
	got := srv.headers()
	if len(got) != len(want) {
		t.Fatalf("requests = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request %d Authorization = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGetAuthorizedDropsTokenOnRedirectToAnotherHost(t *testing.T) {
	target := newAuthRecorder(t, nil)
	origin := newAuthRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.server.URL+"/archive.zip", http.StatusFound)
	})
	f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})

	if _, _, err := f.GetAuthorized(context.Background(), origin.server.URL+"/start", 1<<20,
		Credential{Host: hostOf(t, origin.server.URL), Token: "tok-secret"}); err != nil {
		t.Fatalf("GetAuthorized: %v", err)
	}
	if got := origin.headers(); len(got) != 1 || got[0] != "Bearer tok-secret" {
		t.Fatalf("origin saw %q, want the bearer token", got)
	}
	for _, h := range target.headers() {
		if h != "" {
			t.Fatalf("redirect target received Authorization %q", h)
		}
	}
}

// Go's own redirect rule forwards Authorization to subdomains of the original
// host. The token is pinned to the exact host, so a subdomain redirect must drop it.
func TestGetAuthorizedDropsTokenOnRedirectToSubdomain(t *testing.T) {
	var target *authRecorder
	origin := newAuthRecorder(t, func(w http.ResponseWriter, r *http.Request) {
		_, port, _ := net.SplitHostPort(target.server.Listener.Addr().String())
		http.Redirect(w, r, "http://cdn.origin.test:"+port+"/archive.zip", http.StatusFound)
	})
	target = newAuthRecorder(t, nil)
	_, originPort, _ := net.SplitHostPort(origin.server.Listener.Addr().String())

	// Names under origin.test resolve to the loopback test servers.
	resolver := loopbackResolver{}
	f := NewSafeFetcher(FetcherOptions{AllowPrivate: true, Resolver: resolver})

	start := "http://origin.test:" + originPort + "/start"
	if _, _, err := f.GetAuthorized(context.Background(), start, 1<<20,
		Credential{Host: "origin.test:" + originPort, Token: "tok-subdomain"}); err != nil {
		t.Fatalf("GetAuthorized: %v", err)
	}
	if got := origin.headers(); len(got) != 1 || got[0] != "Bearer tok-subdomain" {
		t.Fatalf("origin saw %q, want the bearer token", got)
	}
	for _, h := range target.headers() {
		if h != "" {
			t.Fatalf("subdomain redirect target received Authorization %q", h)
		}
	}
}

type loopbackResolver struct{}

func (loopbackResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if strings.HasSuffix(host, "origin.test") {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	return nil, fmt.Errorf("no such host %q", host)
}
