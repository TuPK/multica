package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/platform-management/accountexport"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/redis/go-redis/v9"
)

const (
	exportLimiterKeyPrefix = "mul:account-export:"
	// exportSecurityEventsCap bounds the security events in one export.
	exportSecurityEventsCap = 10000
)

func accountExportLimit() SlidingWindowRateLimit {
	return SlidingWindowRateLimit{Limit: accountexport.Limit, Window: accountexport.Window}
}

// NewMemoryAccountExportLimiter is the one-per-user-per-hour gate for
// single-process and Redis-less deployments.
func NewMemoryAccountExportLimiter() SlidingWindowRateLimiter {
	return newMemorySlidingWindowRateLimiter(accountExportLimit())
}

// NewRedisAccountExportLimiter is the same gate shared across replicas.
func NewRedisAccountExportLimiter(rdb redis.UniversalClient) SlidingWindowRateLimiter {
	return newRedisSlidingWindowRateLimiter(rdb, accountExportLimit(), exportLimiterKeyPrefix)
}

// AccountExport is the body of GET /api/me/export. The field set is the
// contract: it holds the account's own data and no credential of any kind.
type AccountExport struct {
	ExportedAt           string                    `json:"exported_at"`
	Profile              accountExportProfile      `json:"profile"`
	Memberships          []accountExportMembership `json:"memberships"`
	PersonalAccessTokens []accountExportPAT        `json:"personal_access_tokens"`
	Consents             []accountExportConsent    `json:"consents"`
	SecurityEvents       []auditEventJSON          `json:"security_events"`
	ExternalConnections  []accountExportConnection `json:"external_connections"`
}

type accountExportProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

type accountExportMembership struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	Role          string `json:"role"`
	JoinedAt      string `json:"joined_at"`
}

// accountExportPAT carries the visible prefix only. token_hash is never read.
type accountExportPAT struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	TokenPrefix string  `json:"token_prefix"`
	CreatedAt   string  `json:"created_at"`
	LastUsedAt  *string `json:"last_used_at"`
	ExpiresAt   *string `json:"expires_at"`
}

type accountExportConsent struct {
	Document   string `json:"document"`
	Version    string `json:"version"`
	AcceptedAt string `json:"accepted_at"`
	IP         string `json:"ip"`
}

// accountExportConnection names a connected outside service and the account on
// it. No credential column is read for any connection.
type accountExportConnection struct {
	Provider    string  `json:"provider"`
	Service     string  `json:"service,omitempty"`
	AccountID   string  `json:"account_id"`
	Status      string  `json:"status,omitempty"`
	WorkspaceID *string `json:"workspace_id,omitempty"`
	ConnectedAt string  `json:"connected_at"`
}

// ExportMyAccount serves GET /api/me/export: one JSON download of the caller's
// own account data, at most once an hour per user. Workspace content is not
// part of it.
func (h *Handler) ExportMyAccount(limiter SlidingWindowRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requireUserID(w, r)
		if !ok {
			return
		}
		uid, ok := parseUUIDOrBadRequest(w, userID, "user id")
		if !ok {
			return
		}
		ctx := r.Context()

		allowed, err := slidingWindowLimiterAllow(ctx, limiter, userID)
		if err != nil {
			slog.Error("account export limiter unavailable", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusServiceUnavailable, "export is temporarily unavailable")
			return
		}
		if !allowed {
			retry := accountexport.RetryAfterSeconds(slidingWindowLimiterRetryAfter(ctx, limiter, userID))
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			writeErrorCode(w, http.StatusTooManyRequests, "export_rate_limited", "the export can be downloaded once an hour")
			return
		}

		user, err := h.Queries.GetUser(ctx, uid)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "user not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load user")
			return
		}
		out, err := h.buildAccountExport(r, user)
		if err != nil {
			slog.Warn("account export failed", append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
			writeError(w, http.StatusInternalServerError, "failed to build export")
			return
		}
		body, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to build export")
			return
		}
		// Last line of defense: the queries select no credential column, and
		// this refuses to send a file that holds one anyway.
		var decoded any
		if err := json.Unmarshal(body, &decoded); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to build export")
			return
		}
		if leaks := accountexport.FindSecrets(decoded, nil); len(leaks) > 0 {
			slog.Error("account export blocked: secret-bearing fields", append(logger.RequestAttrs(r), "paths", leaks, "user_id", userID)...)
			writeError(w, http.StatusInternalServerError, "failed to build export")
			return
		}

		if err := h.recordAudit(r, nil, audit.Entry{
			Action:      audit.ActionAccountExportRequested,
			TargetType:  "user",
			TargetID:    userID,
			TargetLabel: user.Email,
		}); err != nil {
			slog.Warn("account export: audit failed", append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
			writeError(w, http.StatusInternalServerError, "failed to record export")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="multica-account-export.json"`)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			slog.Warn("account export: write failed", append(logger.RequestAttrs(r), "error", err)...)
		}
	}
}

func (h *Handler) buildAccountExport(r *http.Request, user db.User) (AccountExport, error) {
	ctx := r.Context()
	out := AccountExport{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Profile: accountExportProfile{
			ID:        uuidToString(user.ID),
			Name:      user.Name,
			Email:     user.Email,
			CreatedAt: timestampToString(user.CreatedAt),
		},
		Memberships:          []accountExportMembership{},
		PersonalAccessTokens: []accountExportPAT{},
		Consents:             []accountExportConsent{},
		SecurityEvents:       []auditEventJSON{},
		ExternalConnections:  []accountExportConnection{},
	}

	members, err := h.Queries.ExportListUserMemberships(ctx, user.ID)
	if err != nil {
		return out, err
	}
	for _, m := range members {
		out.Memberships = append(out.Memberships, accountExportMembership{
			WorkspaceID:   uuidToString(m.WorkspaceID),
			WorkspaceName: m.WorkspaceName,
			Role:          m.Role,
			JoinedAt:      timestampToString(m.JoinedAt),
		})
	}

	pats, err := h.Queries.ExportListUserPersonalAccessTokens(ctx, user.ID)
	if err != nil {
		return out, err
	}
	for _, p := range pats {
		if p.Revoked {
			continue
		}
		out.PersonalAccessTokens = append(out.PersonalAccessTokens, accountExportPAT{
			ID:          uuidToString(p.ID),
			Name:        p.Name,
			TokenPrefix: p.TokenPrefix,
			CreatedAt:   timestampToString(p.CreatedAt),
			LastUsedAt:  timestampToPtr(p.LastUsedAt),
			ExpiresAt:   timestampToPtr(p.ExpiresAt),
		})
	}

	consents, err := h.Queries.ListUserConsents(ctx, user.ID)
	if err != nil {
		return out, err
	}
	for _, c := range consents {
		out.Consents = append(out.Consents, accountExportConsent{
			Document:   c.Document,
			Version:    c.Version,
			AcceptedAt: timestampToString(c.AcceptedAt),
			IP:         c.Ip,
		})
	}

	events, err := h.Queries.ListUserSecurityEvents(ctx, db.ListUserSecurityEventsParams{
		ActorID:  uuidToString(user.ID),
		RowLimit: exportSecurityEventsCap,
	})
	if err != nil {
		return out, err
	}
	for _, e := range events {
		out.SecurityEvents = append(out.SecurityEvents, toAuditEventJSON(e))
	}

	composio, err := h.Queries.ExportListUserComposioConnections(ctx, user.ID)
	if err != nil {
		return out, err
	}
	for _, c := range composio {
		out.ExternalConnections = append(out.ExternalConnections, accountExportConnection{
			Provider:    "composio",
			Service:     c.ToolkitSlug,
			AccountID:   c.ConnectedAccountID,
			Status:      c.Status,
			ConnectedAt: timestampToString(c.ConnectedAt),
		})
	}
	bindings, err := h.Queries.ExportListUserChannelBindings(ctx, user.ID)
	if err != nil {
		return out, err
	}
	for _, b := range bindings {
		out.ExternalConnections = append(out.ExternalConnections, accountExportConnection{
			Provider:    b.ChannelType,
			AccountID:   b.ChannelUserID,
			WorkspaceID: uuidToPtr(b.WorkspaceID),
			ConnectedAt: timestampToString(b.BoundAt),
		})
	}
	weknora, err := h.Queries.ExportListUserWeknoraAccounts(ctx, user.ID)
	if err != nil {
		return out, err
	}
	for _, k := range weknora {
		out.ExternalConnections = append(out.ExternalConnections, accountExportConnection{
			Provider:    "weknora",
			AccountID:   k.WeknoraUserID,
			WorkspaceID: uuidToPtr(k.WorkspaceID),
			ConnectedAt: timestampToString(k.CreatedAt),
		})
	}
	return out, nil
}
