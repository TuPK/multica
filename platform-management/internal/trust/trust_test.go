package trust

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

const userID = "11111111-2222-3333-4444-555555555555"

func defaults(t *testing.T) *Peers {
	t.Helper()
	return New(DefaultCIDRs())
}

// reached records whether the wrapped handler ran, and with which user.
type reached struct {
	called bool
	user   string
}

func (r *reached) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.called = true
		r.user, _ = UserFrom(req.Context())
		w.WriteHeader(http.StatusOK)
	})
}

func serve(h http.Handler, remote, header string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/x/summary", nil)
	req.RemoteAddr = remote
	if header != "" {
		req.Header.Set(UserHeader, header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRequireUserPassesATrustedPeer(t *testing.T) {
	for _, remote := range []string{
		"127.0.0.1:5000", "[::1]:5000", "10.1.2.3:5000", "172.16.0.9:5000", "172.31.255.255:5000", "192.168.1.1:5000",
		"[::ffff:10.0.0.1]:5000", // an IPv4 peer on a dual-stack listener
	} {
		var got reached
		rec := serve(defaults(t).RequireUser(got.handler()), remote, userID)
		if rec.Code != http.StatusOK || !got.called || got.user != userID {
			t.Errorf("%s: status %d, called %v, user %q; want 200 and the user", remote, rec.Code, got.called, got.user)
		}
	}
}

// The check is on the peer, before the handler runs.
func TestRequireUserRefusesAnUntrustedPeerBeforeTheHandler(t *testing.T) {
	for _, remote := range []string{
		"203.0.113.9:5000", "8.8.8.8:1", "172.32.0.1:5000", "172.15.255.255:5000", "[2001:db8::1]:5000",
		"not-an-address", "",
	} {
		var got reached
		rec := serve(defaults(t).RequireUser(got.handler()), remote, userID)
		if rec.Code != http.StatusForbidden || got.called {
			t.Errorf("%q: status %d, called %v; want 403 and no handler", remote, rec.Code, got.called)
		}
	}
}

func TestRequireUserWithoutTheHeaderIs401(t *testing.T) {
	var got reached
	rec := serve(defaults(t).RequireUser(got.handler()), "127.0.0.1:5000", "")
	if rec.Code != http.StatusUnauthorized || got.called {
		t.Fatalf("status %d, called %v; want 401 and no handler", rec.Code, got.called)
	}
}

func TestRequireUserRefusesAMalformedOrRepeatedHeader(t *testing.T) {
	for _, value := range []string{"not-a-uuid", "11111111-2222-3333-4444-55555555555", " ", "'; drop table member;--"} {
		var got reached
		rec := serve(defaults(t).RequireUser(got.handler()), "127.0.0.1:5000", value)
		if rec.Code != http.StatusUnauthorized || got.called {
			t.Errorf("%q: status %d, called %v; want 401", value, rec.Code, got.called)
		}
	}

	var got reached
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Add(UserHeader, userID)
	req.Header.Add(UserHeader, "99999999-2222-3333-4444-555555555555")
	rec := httptest.NewRecorder()
	defaults(t).RequireUser(got.handler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || got.called {
		t.Errorf("two headers: status %d, called %v; want 401", rec.Code, got.called)
	}
}

// The user is the canonical lower-case form, whatever case the caller used.
func TestRequireUserCanonicalisesTheUser(t *testing.T) {
	var got reached
	rec := serve(defaults(t).RequireUser(got.handler()), "127.0.0.1:1", "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE")
	if rec.Code != 200 || got.user != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Fatalf("status %d, user %q", rec.Code, got.user)
	}
}

// X-Forwarded-For is whatever the caller wrote; the peer is the socket.
func TestForwardedHeadersDoNotMakeAPeerTrusted(t *testing.T) {
	var got reached
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.9:1"
	req.Header.Set(UserHeader, userID)
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	req.Header.Set("X-Real-IP", "127.0.0.1")
	rec := httptest.NewRecorder()
	defaults(t).RequireUser(got.handler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || got.called {
		t.Fatalf("status %d, called %v; want 403", rec.Code, got.called)
	}
}

// Guard covers every route: a request that carries the header from an untrusted
// peer is refused even where no user is needed, and one without it passes.
func TestGuardRefusesTheHeaderFromAnUntrustedPeerOnly(t *testing.T) {
	cases := []struct {
		name, remote, header string
		want                 int
	}{
		{"untrusted with header", "203.0.113.9:1", userID, http.StatusForbidden},
		{"untrusted without header", "203.0.113.9:1", "", http.StatusOK},
		{"trusted with header", "10.0.0.5:1", userID, http.StatusOK},
		{"trusted without header", "10.0.0.5:1", "", http.StatusOK},
	}
	for _, tc := range cases {
		var got reached
		rec := serve(defaults(t).Guard(got.handler()), tc.remote, tc.header)
		if rec.Code != tc.want || got.called != (tc.want == http.StatusOK) {
			t.Errorf("%s: status %d, called %v; want %d", tc.name, rec.Code, got.called, tc.want)
		}
	}
}

func TestConfiguredCIDRsReplaceTheDefaults(t *testing.T) {
	only := New([]netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")})
	var got reached
	if rec := serve(only.RequireUser(got.handler()), "203.0.113.9:1", userID); rec.Code != 200 {
		t.Errorf("configured range: status %d, want 200", rec.Code)
	}
	if rec := serve(only.RequireUser(got.handler()), "127.0.0.1:1", userID); rec.Code != http.StatusForbidden {
		t.Errorf("loopback with its own range configured: status %d, want 403", rec.Code)
	}
}

func TestDefaultCIDRsAreLoopbackAndRFC1918(t *testing.T) {
	want := map[string]bool{
		"127.0.0.0/8": true, "::1/128": true, "10.0.0.0/8": true, "172.16.0.0/12": true, "192.168.0.0/16": true,
	}
	got := DefaultCIDRs()
	if len(got) != len(want) {
		t.Fatalf("defaults = %v", got)
	}
	for _, p := range got {
		if !want[p.String()] {
			t.Errorf("unexpected default %s", p)
		}
	}
}

func TestRefusalBodiesAreJSONWithACode(t *testing.T) {
	rec := serve(defaults(t).RequireUser(http.NotFoundHandler()), "203.0.113.9:1", userID)
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type = %q", ct)
	}
	if body := rec.Body.String(); body != `{"code":"untrusted_peer","error":"X-User-ID is accepted only from a trusted peer"}`+"\n" {
		t.Errorf("body = %q", body)
	}
}

func TestCanonicalUUID(t *testing.T) {
	good := map[string]string{
		userID:                                 userID,
		"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
	}
	for in, want := range good {
		if got, ok := CanonicalUUID(in); !ok || got != want {
			t.Errorf("CanonicalUUID(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{
		"", "x", "11111111222233334444555555555555", "{11111111-2222-3333-4444-555555555555}",
		"11111111-2222-3333-4444-55555555555", "11111111-2222-3333-4444-5555555555555", "gggggggg-2222-3333-4444-555555555555",
	} {
		if got, ok := CanonicalUUID(in); ok {
			t.Errorf("CanonicalUUID(%q) = %q, want a refusal", in, got)
		}
	}
}
