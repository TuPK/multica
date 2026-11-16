package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/multica-ai/multica/server/internal/entitlement"
)

// daemonPathPrefix is the one place that exempts daemon traffic from the
// per-workspace API rate limit.
const daemonPathPrefix = "/api/daemon/"

// LimitSource reports the API rate limit of a workspace: the requests allowed
// per minute and whether that limit is enforced.
type LimitSource interface {
	APIRateLimit(ctx context.Context, workspaceID string) (limit int, enforced bool)
}

type entitlementLimits struct{ provider entitlement.Provider }

// EntitlementLimits adapts the entitlement client to a LimitSource using the
// api_rate_per_minute gate. It returns nil for a nil provider, which makes
// WorkspaceRateLimit a no-op.
func EntitlementLimits(provider entitlement.Provider) LimitSource {
	if provider == nil {
		return nil
	}
	return entitlementLimits{provider: provider}
}

func (e entitlementLimits) APIRateLimit(ctx context.Context, workspaceID string) (int, bool) {
	id, err := uuid.Parse(workspaceID)
	if err != nil {
		return 0, false
	}
	gate := e.provider.Gate(ctx, id, entitlement.GateAPIRatePerMinute).Gate
	if gate.Action != entitlement.ActionEnforce || gate.Limit == nil {
		return 0, false
	}
	return *gate.Limit, true
}

// WorkspaceRateLimit limits API requests per workspace with a fixed one-minute
// window in Redis. It must run after the workspace middleware, which puts the
// workspace in the request context. A nil rdb or limits makes it a no-op, and
// a Redis failure lets the request through.
func WorkspaceRateLimit(rdb redis.UniversalClient, limits LimitSource, now func() time.Time) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if rdb == nil || limits == nil {
			return next
		}
		// redisDown limits the failure warning to one per outage.
		var redisDown atomic.Bool
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			workspaceID := WorkspaceIDFromContext(r.Context())
			if workspaceID == "" || strings.HasPrefix(r.URL.Path, daemonPathPrefix) {
				next.ServeHTTP(w, r)
				return
			}
			limit, enforced := limits.APIRateLimit(r.Context(), workspaceID)
			if !enforced {
				next.ServeHTTP(w, r)
				return
			}

			t := now()
			if limit > 0 {
				minute := t.Unix() / 60
				count, err := countRequest(r.Context(), rdb, fmt.Sprintf("mul:rl:ws:%s:%d", workspaceID, minute))
				if err != nil {
					if redisDown.CompareAndSwap(false, true) {
						slog.Warn("workspace ratelimit: redis error; allowing requests", "error", err)
					}
					next.ServeHTTP(w, r)
					return
				}
				redisDown.Store(false)
				if count <= int64(limit) {
					next.ServeHTTP(w, r)
					return
				}
			}

			retryAfter := 60 - t.Second()
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"error": "rate limit exceeded",
				"code":  "rate_limited",
				"limit": limit,
			})
		})
	}
}

// countRequest increments the window counter and sets its expiry when the
// window is first created. A lost EXPIRE leaves one stale minute-keyed
// counter that no later window reads.
func countRequest(ctx context.Context, rdb redis.UniversalClient, key string) (int64, error) {
	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		if err := rdb.Expire(ctx, key, 2*time.Minute).Err(); err != nil {
			return 0, err
		}
	}
	return count, nil
}
