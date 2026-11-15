package auditstore

import (
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// RequestFields returns the client IP, user agent and request id of r for an
// audit entry. clientIP is the handler's trusted-proxy-aware resolver, so the
// recorded address matches the one used for rate limiting. No other header is
// read.
func RequestFields(r *http.Request, clientIP func(*http.Request) string) (ip, userAgent, requestID string) {
	return clientIP(r), r.UserAgent(), chimw.GetReqID(r.Context())
}
