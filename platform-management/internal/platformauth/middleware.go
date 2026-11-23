package platformauth

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/internal/session"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Identity is the signed-in staff member the middleware puts in the context.
type Identity struct {
	UserID   string
	Role     staff.Role
	TenantID string
	AuthTime time.Time
}

type identityKey struct{}

// IdentityFrom returns the identity RequirePlatformStaff stored.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// RequirePlatformStaff admits a request only when every check passes: the
// client IP is inside AdminCIDRs when they are set; the mpa_session cookie
// holds a valid token no older than 12 hours whose amr includes mfa; the user has an active staff row
// whose role is one of roles (any role when roles is empty); and the Delegora
// account is active with no session revocation after the sign-in. The checks
// read the staff row and the account on every request, so a revocation applies
// to the next request.
//
// Every failure, including a store error, is the same 404 an unknown route
// gives. Only the mpa_session cookie is read: Authorization headers and the
// multica_auth cookie are ignored.
func (h *Handler) RequirePlatformStaff(roles ...staff.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, reason := h.authenticate(r, roles)
			if reason != "" {
				h.Log.Debug("platform staff check failed", "reason", reason, "path", r.URL.Path)
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, id)))
		})
	}
}

// authenticate returns the identity, or the reason the request is refused.
// The reason goes to the debug log only.
func (h *Handler) authenticate(r *http.Request, roles []staff.Role) (Identity, string) {
	if len(h.AdminCIDRs) > 0 {
		ip, ok := clientIP(r)
		if !ok || !inAny(h.AdminCIDRs, ip) {
			return Identity{}, "ip outside admin cidrs"
		}
	}

	cookie, err := r.Cookie(session.CookieName)
	if err != nil || cookie.Value == "" {
		return Identity{}, "no session cookie"
	}
	claims, err := session.Verify(h.SessionSecret, cookie.Value, h.Now())
	if err != nil {
		return Identity{}, "invalid token"
	}

	// The console accepts only a session that passed the TOTP step.
	if !slices.Contains(claims.AMR, methodMFA) {
		return Identity{}, "no second factor in the session"
	}

	ctx := r.Context()
	role, tenantID, found, err := h.Directory.ActiveStaff(ctx, claims.UserID)
	if err != nil {
		h.Log.Error("platform staff check: staff lookup failed", "error", err)
		return Identity{}, "staff lookup failed"
	}
	if !found {
		return Identity{}, "no active staff row"
	}
	if len(roles) > 0 && !slices.Contains(roles, role) {
		return Identity{}, "role not allowed"
	}

	rec, err := h.Guard.State(ctx, claims.UserID)
	if err != nil {
		h.Log.Error("platform staff check: account state failed", "error", err)
		return Identity{}, "account state failed"
	}
	if rec.Status != accountguard.Active {
		return Identity{}, "account not active"
	}
	// Same rule as the Delegora server: whole seconds, strict.
	if !rec.SessionsValidAfter.IsZero() && claims.AuthUnix < rec.SessionsValidAfter.Unix() {
		return Identity{}, "sessions revoked"
	}
	return Identity{UserID: claims.UserID, Role: role, TenantID: tenantID, AuthTime: claims.AuthTime()}, ""
}

func inAny(prefixes []netip.Prefix, ip netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP returns the address of the client. The console proxy connects from
// loopback and appends the browser's address to X-Forwarded-For, so for a
// loopback peer the last entry of that header is the client. For any other
// peer the header is ignored, because anyone can write it.
func clientIP(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	peer = peer.Unmap()
	if peer.IsLoopback() {
		if values := r.Header.Values("X-Forwarded-For"); len(values) > 0 {
			last := values[len(values)-1]
			if i := strings.LastIndexByte(last, ','); i >= 0 {
				last = last[i+1:]
			}
			if ip, err := netip.ParseAddr(strings.TrimSpace(last)); err == nil {
				return ip.Unmap(), true
			}
		}
	}
	return peer, true
}

func ipKey(ip netip.Addr) string {
	if !ip.IsValid() {
		return "unknown"
	}
	return ip.String()
}
