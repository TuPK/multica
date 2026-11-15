package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// RevokeAllSessionsResponse is the body of POST /api/me/sessions/revoke-all.
// Token has the shape of LoginResponse.Token: the caller's replacement session,
// minted after the mark so it survives the revocation it just caused.
type RevokeAllSessionsResponse struct {
	RevokedAt string `json:"revoked_at"`
	Token     string `json:"token"`
}

// RevokeAllSessions signs the caller out of every device: it moves the user's
// sessions_valid_after mark to now, so each JWT whose login time (`auth_time`)
// is earlier is refused with 401 session_revoked, and closes the user's open
// WebSocket connections. The caller keeps working because the response carries
// a new session whose auth_time equals the mark (the check is strict `<`).
//
// Personal access tokens are not sessions and are not touched.
//
// Resolution is whole seconds, so a token issued in the same second before the
// revocation survives it.
//
// guard is the account guard whose cached record for this user is dropped once
// the mark is written. It is a parameter rather than a Handler field because
// the router builds it; a nil guard skips the cache drop.
func (h *Handler) RevokeAllSessions(guard *accountguard.Guard) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requireUserID(w, r)
		if !ok {
			return
		}
		uid, ok := parseUUIDOrBadRequest(w, userID, "user id")
		if !ok {
			return
		}
		user, err := h.Queries.GetUser(r.Context(), uid)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "user not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load user")
			return
		}

		revokedAt := time.Unix(time.Now().Unix(), 0)
		if err := h.Queries.RevokeSessionsForUser(r.Context(), db.RevokeSessionsForUserParams{
			UserID:             uid,
			SessionsValidAfter: pgtype.Timestamptz{Time: revokedAt, Valid: true},
		}); err != nil {
			slog.Warn("revoke all sessions: write failed", append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
			writeError(w, http.StatusInternalServerError, "failed to revoke sessions")
			return
		}

		// The mark is already written. A failed invalidation only leaves the
		// old record readable for at most accountguard.TTL.
		if guard != nil {
			if err := guard.Invalidate(r.Context(), userID); err != nil {
				slog.Warn("revoke all sessions: account cache invalidation failed; stale for up to the cache TTL",
					append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
			}
		}
		h.Hub.DisconnectUser(userID)

		token, err := h.issueJWT(user)
		if err != nil {
			slog.Warn("revoke all sessions: failed to issue replacement token", append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
			writeError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}
		if err := auth.SetAuthCookies(w, token); err != nil {
			slog.Warn("revoke all sessions: failed to set auth cookies", append(logger.RequestAttrs(r), "error", err)...)
		}
		if h.CFSigner != nil {
			for _, cookie := range h.CFSigner.SignedCookies(time.Now().Add(auth.AuthTokenTTL())) {
				http.SetCookie(w, cookie)
			}
		}

		slog.Info("user signed out of all devices", append(logger.RequestAttrs(r), "user_id", userID)...)
		writeJSON(w, http.StatusOK, RevokeAllSessionsResponse{
			RevokedAt: revokedAt.UTC().Format(time.RFC3339),
			Token:     token,
		})
	}
}
