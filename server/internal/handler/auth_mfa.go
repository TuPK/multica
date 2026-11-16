package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/mfa"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// MFAChallengeResponse answers a first factor for a user who has a second one.
// It carries no session: no token, no cookies. MFAToken is good for
// POST /auth/mfa/verify only.
type MFAChallengeResponse struct {
	MFARequired bool   `json:"mfa_required"`
	MFAToken    string `json:"mfa_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type VerifyMFARequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

// challengeMFA is called once the first factor (amr) has passed. When the user
// has a confirmed second factor it writes the pending-token response and
// reports true, and the caller must not issue a session. An enrolment that was
// never confirmed is not a second factor.
func (h *Handler) challengeMFA(w http.ResponseWriter, r *http.Request, user db.User, amr []string) bool {
	row, err := h.Queries.GetUserMFA(r.Context(), user.ID)
	if isNotFound(err) || (err == nil && !row.ConfirmedAt.Valid) {
		return false
	}
	if err != nil {
		// Failing open here would let a database error skip the second factor.
		slog.Warn("mfa lookup failed at sign-in", append(logger.RequestAttrs(r), "error", err, "user_id", uuidToString(user.ID))...)
		writeError(w, http.StatusInternalServerError, "failed to check multi-factor state")
		return true
	}
	token, _, err := auth.IssueMFAPendingToken(uuidToString(user.ID), amr, time.Now())
	if err != nil {
		slog.Warn("mfa pending token failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return true
	}
	writeJSON(w, http.StatusOK, MFAChallengeResponse{
		MFARequired: true,
		MFAToken:    token,
		ExpiresIn:   int(auth.MFAPendingTTL.Seconds()),
	})
	return true
}

// VerifyMFA is POST /auth/mfa/verify: the pending token plus a TOTP or recovery
// code buys the session, with "mfa" added to the methods the first factor set.
func (h *Handler) VerifyMFA(w http.ResponseWriter, r *http.Request) {
	var req VerifyMFARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	now := time.Now()
	pending, err := auth.ParseMFAPendingToken(req.MFAToken, now)
	if err != nil {
		writeErrorCode(w, http.StatusUnauthorized, mfaCodeTokenInvalid, "sign in again")
		return
	}
	crypto, err := mfa.LoadCrypto()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, mfaCodeUnavailable, mfaMessageUnavailable)
		return
	}
	if !mfa.Attempts.Reserve(pending.TokenID, pending.ExpiresAt, now) {
		writeErrorCode(w, http.StatusUnauthorized, mfaCodeTokenFailed, "too many attempts; sign in again")
		return
	}

	userID, err := util.ParseUUID(pending.UserID)
	if err != nil {
		writeErrorCode(w, http.StatusUnauthorized, mfaCodeTokenInvalid, "sign in again")
		return
	}
	row, err := h.Queries.GetUserMFA(r.Context(), userID)
	if err != nil && !isNotFound(err) {
		writeError(w, http.StatusInternalServerError, "failed to load multi-factor state")
		return
	}
	if isNotFound(err) || !row.ConfirmedAt.Valid {
		// MFA was turned off after the first factor: this token proves a
		// challenge that no longer exists, so it opens nothing.
		writeErrorCode(w, http.StatusUnauthorized, mfaCodeTokenInvalid, "sign in again")
		return
	}
	valid, err := h.acceptSecondFactor(r.Context(), crypto, row, req.Code, now)
	if err != nil {
		slog.Warn("mfa verify: check failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to check the code")
		return
	}
	if !valid {
		writeErrorCode(w, http.StatusBadRequest, mfaCodeInvalidCode, "that code is not valid")
		return
	}
	mfa.Attempts.Spend(pending.TokenID)

	user, err := h.Queries.GetUser(r.Context(), userID)
	if err != nil {
		writeErrorCode(w, http.StatusUnauthorized, mfaCodeTokenInvalid, "sign in again")
		return
	}
	amr := append(append([]string(nil), pending.AMR...), auth.AMRMFA)
	tokenString, err := h.issueJWT(user, amr)
	if err != nil {
		if errors.Is(err, auth.ErrTemporarilyDisabledUser) {
			writeError(w, http.StatusForbidden, auth.TemporarilyDisabledUserError)
			return
		}
		slog.Warn("mfa login failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	if !h.recordLogin(w, r, user, "mfa") {
		return
	}

	if err := auth.SetAuthCookies(w, tokenString); err != nil {
		slog.Warn("failed to set auth cookies", "error", err)
	}
	if h.CFSigner != nil {
		for _, cookie := range h.CFSigner.SignedCookies(time.Now().Add(auth.AuthTokenTTL())) {
			http.SetCookie(w, cookie)
		}
	}

	slog.Info("user logged in with mfa", append(logger.RequestAttrs(r), "user_id", uuidToString(user.ID))...)
	writeJSON(w, http.StatusOK, LoginResponse{
		Token: tokenString,
		User:  h.userToResponse(user),
	})
}
