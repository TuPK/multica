// Package trust decides when the platform believes the X-User-ID header.
//
// Delegora's proxy sends X-User-ID to the platform for user routes and no other
// credential, so the header proves nothing by itself. The platform believes it
// only when the TCP peer is inside a trusted network. The peer is the socket's
// remote address: X-Forwarded-For and X-Real-IP are caller-written text and are
// never read.
//
// Deployment requirement: the API listener must not be reachable from outside
// the internal network. A reverse proxy or port mapping that makes outside
// callers appear to come from a private address defeats this rule.
package trust

import (
	"context"
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// UserHeader is the header Delegora's proxy sets on user routes.
const UserHeader = "X-User-ID"

// DefaultCIDRs is the trusted set when PLATFORM_TRUSTED_CIDRS is unset: the
// loopback ranges and the IPv4 private ranges of RFC 1918.
func DefaultCIDRs() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

// Peers is a set of trusted networks.
type Peers struct{ cidrs []netip.Prefix }

// New returns the trust rule for the given networks.
func New(cidrs []netip.Prefix) *Peers { return &Peers{cidrs: cidrs} }

// trusted reports whether the request's socket peer is in a trusted network. A
// peer that cannot be parsed is not trusted.
func (p *Peers) trusted(r *http.Request) bool {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	// A dual-stack listener reports an IPv4 peer as ::ffff:a.b.c.d.
	addr := ap.Addr().Unmap()
	for _, c := range p.cidrs {
		if c.Contains(addr) {
			return true
		}
	}
	return false
}

type userKey struct{}

// UserFrom returns the user RequireUser stored.
func UserFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userKey{}).(string)
	return id, ok
}

func refuse(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": code})
}

func untrusted(w http.ResponseWriter) {
	refuse(w, http.StatusForbidden, "untrusted_peer", "X-User-ID is accepted only from a trusted peer")
}

// Guard refuses, with 403, any request that carries X-User-ID from an
// untrusted peer. It wraps the whole listener, so no route can forget the rule.
func (p *Peers) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.Header.Values(UserHeader)) > 0 && !p.trusted(r) {
			untrusted(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireUser admits a request that names a user through X-User-ID from a
// trusted peer, and stores the user, as a lower-case UUID, in the context. No
// header, more than one, or one that is not a UUID is 401. The header from an
// untrusted peer is 403, and the handler does not run.
func (p *Peers) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values(UserHeader)
		if len(values) == 0 {
			refuse(w, http.StatusUnauthorized, "unauthorized", "X-User-ID is required")
			return
		}
		if !p.trusted(r) {
			untrusted(w)
			return
		}
		id, ok := canonicalUserID(values)
		if !ok {
			refuse(w, http.StatusUnauthorized, "unauthorized", "X-User-ID must be one UUID")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, id)))
	})
}

func canonicalUserID(values []string) (string, bool) {
	if len(values) != 1 {
		return "", false
	}
	return CanonicalUUID(strings.TrimSpace(values[0]))
}

// CanonicalUUID returns the lower-case dashed form of a UUID given in the
// dashed form. pgtype also accepts the 32-digit form and braces; Delegora sends
// the dashed form, so any other text is not an id this service issued.
func CanonicalUUID(s string) (string, bool) {
	var u pgtype.UUID
	if len(s) != 36 || u.Scan(s) != nil || !u.Valid {
		return "", false
	}
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for i, c := range u.Bytes {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out = append(out, '-')
		}
		out = append(out, hexDigits[c>>4], hexDigits[c&0x0f])
	}
	return string(out), true
}
