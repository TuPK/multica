package hub

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},
		{"127.5.4.3", true},
		{"10.0.0.1", true},
		{"172.16.5.5", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true}, // cloud metadata
		{"100.64.0.1", true},      // carrier-grade NAT
		{"100.100.100.200", true}, // cloud metadata in CGNAT space
		{"0.0.0.0", true},
		{"198.18.0.1", true}, // benchmarking
		{"224.0.0.1", true},  // multicast
		{"240.0.0.1", true},  // reserved
		{"::1", true},
		{"::", true},
		{"fe80::1", true},
		{"fc00::1", true},
		{"fd00:ec2::254", true}, // IPv6 metadata
		{"::ffff:127.0.0.1", true},
		{"::ffff:10.1.2.3", true},
		{"64:ff9b::a00:1", true}, // NAT64 embedding 10.0.0.1
		{"2002:0a00:0001::1", true},
		{"8.8.8.8", false},
		{"93.184.216.34", false},
		{"172.32.0.1", false},
		{"2606:4700:4700::1111", false},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			if got := IsBlockedIP(net.ParseIP(tc.ip)); got != tc.blocked {
				t.Fatalf("IsBlockedIP(%s) = %v, want %v", tc.ip, got, tc.blocked)
			}
		})
	}
}

// stubResolver answers every lookup from a fixed list and counts calls.
type stubResolver struct {
	ips   []string
	calls atomic.Int32
}

func (s *stubResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	s.calls.Add(1)
	out := make([]net.IPAddr, 0, len(s.ips))
	for _, ip := range s.ips {
		out = append(out, net.IPAddr{IP: net.ParseIP(ip)})
	}
	return out, nil
}

// loopbackOnly makes the fetcher treat loopback as reachable while every other
// private range stays blocked. That lets a test server on 127.0.0.1 be the first hop
// while a redirect to 10.x is still refused.
func loopbackOnly(f *SafeFetcher) *SafeFetcher {
	f.exempt = func(ip net.IP) bool { return ip.IsLoopback() }
	return f
}

func TestSafeFetcherBlocksInternalTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secret internal"))
	}))
	defer srv.Close()

	cases := []struct {
		name string
		url  string
	}{
		{"loopback literal", srv.URL},
		{"localhost name", "http://localhost:" + portOf(t, srv.URL) + "/"},
		{"10.x literal", "http://10.0.0.1/"},
		{"172.16 literal", "http://172.16.0.1/"},
		{"192.168 literal", "http://192.168.0.1/"},
		{"cloud metadata", "http://169.254.169.254/latest/meta-data/"},
		{"CGNAT", "http://100.64.0.1/"},
		{"IPv6 loopback", "http://[::1]:8080/"},
		{"IPv6 link local", "http://[fe80::1]/"},
		{"IPv4-mapped loopback", "http://[::ffff:127.0.0.1]:" + portOf(t, srv.URL) + "/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := NewSafeFetcher(FetcherOptions{})
			_, _, err := f.Get(context.Background(), tc.url, 1<<20)
			if !errors.Is(err, ErrBlockedAddress) {
				t.Fatalf("Get(%s) err = %v, want ErrBlockedAddress", tc.url, err)
			}
		})
	}
}

func TestSafeFetcherRejectsBadURLs(t *testing.T) {
	cases := []string{
		"file:///etc/passwd",
		"gopher://example.com/",
		"ftp://example.com/x",
		"javascript:alert(1)",
		"https:///no-host",
		"https://user:pass@example.com/",
		"://broken",
	}
	for _, raw := range cases {
		f := NewSafeFetcher(FetcherOptions{})
		if _, _, err := f.Get(context.Background(), raw, 1<<20); !errors.Is(err, ErrBadURL) {
			t.Errorf("Get(%q) err = %v, want ErrBadURL", raw, err)
		}
	}
}

func TestSafeFetcherRedirectToPrivateIsBlocked(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "http://10.0.0.1:9/internal", http.StatusFound)
	}))
	defer srv.Close()

	f := loopbackOnly(NewSafeFetcher(FetcherOptions{}))
	_, _, err := f.Get(context.Background(), srv.URL+"/start", 1<<20)
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v, want ErrBlockedAddress for redirect to 10.0.0.1", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("first hop served %d times, want 1", hits.Load())
	}
}

func TestSafeFetcherDNSRebindingBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	port := portOf(t, srv.URL)

	// A name whose only answer is a private address must not be dialed, even though
	// the URL itself names a public-looking host.
	rebind := &stubResolver{ips: []string{"10.0.0.7"}}
	f := NewSafeFetcher(FetcherOptions{Resolver: rebind})
	_, _, err := f.Get(context.Background(), "http://rebind.example:"+port+"/", 1<<20)
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("rebind to private err = %v, want ErrBlockedAddress", err)
	}

	// A name with a mix of allowed and blocked answers is dialed only at the allowed
	// one, and the lookup happens once per dial.
	mixed := &stubResolver{ips: []string{"10.0.0.7", "127.0.0.1"}}
	f = loopbackOnly(NewSafeFetcher(FetcherOptions{Resolver: mixed}))
	body, _, err := f.Get(context.Background(), "http://mixed.example:"+port+"/", 1<<20)
	if err != nil {
		t.Fatalf("mixed resolution: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
	if mixed.calls.Load() != 1 {
		t.Fatalf("resolver called %d times, want 1", mixed.calls.Load())
	}
}

func TestSafeFetcherControlHookRefusesBlockedAddress(t *testing.T) {
	f := NewSafeFetcher(FetcherOptions{})
	for _, addr := range []string{"10.0.0.1:80", "127.0.0.1:80", "169.254.169.254:80", "[::1]:80"} {
		if err := f.checkAddress(addr); !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("checkAddress(%s) = %v, want ErrBlockedAddress", addr, err)
		}
	}
	if err := f.checkAddress("8.8.8.8:443"); err != nil {
		t.Errorf("checkAddress(public) = %v", err)
	}
	open := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
	if err := open.checkAddress("10.0.0.1:80"); err != nil {
		t.Errorf("AllowPrivate checkAddress = %v", err)
	}
}

func TestSafeFetcherAllowPrivateReachesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
	body, final, err := f.Get(context.Background(), srv.URL+"/x", 1<<20)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(body) != "hello" || !strings.HasSuffix(final, "/x") {
		t.Fatalf("body=%q final=%q", body, final)
	}
}

func TestSafeFetcherSizeCaps(t *testing.T) {
	big := strings.Repeat("a", 4096)
	chunked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.(http.Flusher).Flush() // no Content-Length: the cap must hold while streaming
		_, _ = w.Write([]byte(big))
	}))
	defer chunked.Close()
	declared := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4096")
		_, _ = w.Write([]byte(big))
	}))
	defer declared.Close()

	f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
	cases := []struct {
		name    string
		url     string
		max     int64
		wantErr error
		wantLen int
	}{
		{"streamed over cap", chunked.URL, 1024, ErrTooLarge, 0},
		{"declared over cap", declared.URL, 1024, ErrTooLarge, 0},
		{"exactly at cap", chunked.URL, 4096, nil, 4096},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _, err := f.Get(context.Background(), tc.url, tc.max)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil || len(body) != tc.wantLen {
				t.Fatalf("len=%d err=%v", len(body), err)
			}
		})
	}
}

func TestSafeFetcherStatusHandling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/missing":
			http.NotFound(w, r)
		case "/broken":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer srv.Close()
	f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})

	if _, _, err := f.Get(context.Background(), srv.URL+"/missing", 1<<10); !errors.Is(err, ErrNotFound) {
		t.Errorf("404 err = %v, want ErrNotFound", err)
	}
	if _, _, err := f.Get(context.Background(), srv.URL+"/broken", 1<<10); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("500 err = %v, want a non-NotFound error", err)
	}
}

func TestSafeFetcherRedirectLoopIsCapped(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, r.URL.String(), http.StatusFound)
	}))
	defer srv.Close()

	f := NewSafeFetcher(FetcherOptions{AllowPrivate: true, MaxRedirects: 3})
	if _, _, err := f.Get(context.Background(), srv.URL+"/loop", 1<<10); err == nil {
		t.Fatal("redirect loop did not fail")
	}
	if hits.Load() > 4 {
		t.Fatalf("server hit %d times, redirects not capped at 3", hits.Load())
	}
}

func TestSafeFetcherDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()

	f := NewSafeFetcher(FetcherOptions{AllowPrivate: true, Timeout: 100 * time.Millisecond})
	start := time.Now()
	if _, _, err := f.Get(context.Background(), srv.URL, 1<<10); err == nil {
		t.Fatal("slow server did not time out")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("deadline not enforced: took %v", elapsed)
	}
}

func portOf(t *testing.T, rawURL string) string {
	t.Helper()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(strings.TrimPrefix(rawURL, "http://"), "https://"))
	if err != nil {
		t.Fatalf("split %s: %v", rawURL, err)
	}
	return port
}
