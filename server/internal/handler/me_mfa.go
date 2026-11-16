package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/mfa"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// sendMFADisabledNotice delivers the disable notice through the mailer. Tests
// replace it with a fake.
var sendMFADisabledNotice = func(svc *service.EmailService, to string) error {
	return svc.SendMFADisabledEmail(to)
}

// Stable error codes of the MFA routes. Clients branch on them.
const (
	mfaCodeUnavailable    = "mfa_unavailable"
	mfaCodeInvalidCode    = "invalid_code"
	mfaCodeAlreadyOn      = "mfa_already_enabled"
	mfaCodeNotEnrolling   = "mfa_not_enrolling"
	mfaCodeNotEnabled     = "mfa_not_enabled"
	mfaCodeTokenInvalid   = "mfa_token_invalid"
	mfaCodeTokenFailed    = "mfa_token_failed"
	mfaMessageUnavailable = "multi-factor sign-in is not configured on this server"
)

type MFACodeRequest struct {
	Code string `json:"code"`
}

type EnrollMFAResponse struct {
	Secret string `json:"secret"`
	URI    string `json:"otpauth_uri"`
}

type ConfirmMFAResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

// mfaCaller resolves the human caller of an /api/me/mfa route and the key that
// seals secrets. It has written the response when ok is false. Without the key
// nothing is read or stored: the feature is off.
func (h *Handler) mfaCaller(w http.ResponseWriter, r *http.Request) (userID string, crypto *mfa.Crypto, ok bool) {
	if isMachineCredentialActor(r) {
		writeError(w, http.StatusForbidden, "this endpoint is only available to human actors")
		return "", nil, false
	}
	userID, ok = requireUserID(w, r)
	if !ok {
		return "", nil, false
	}
	crypto, err := mfa.LoadCrypto()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, mfaCodeUnavailable, mfaMessageUnavailable)
		return "", nil, false
	}
	return userID, crypto, true
}

func decodeMFACode(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req MFACodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return "", false
	}
	return req.Code, true
}

// EnrollMFA starts enrolment: it makes a secret, stores it sealed and
// unconfirmed, and returns it with the otpauth:// URI. Enrolling again before
// confirming replaces the secret; once confirmed, MFA has to be disabled first.
func (h *Handler) EnrollMFA(w http.ResponseWriter, r *http.Request) {
	userID, crypto, ok := h.mfaCaller(w, r)
	if !ok {
		return
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	secret, uri, err := mfa.NewSecret(mfa.Issuer, user.Email)
	if err != nil {
		slog.Warn("mfa enroll: generate secret failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to start enrolment")
		return
	}
	sealed, err := crypto.Seal(secret)
	if err != nil {
		slog.Warn("mfa enroll: seal secret failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to start enrolment")
		return
	}
	if _, err := h.Queries.UpsertPendingUserMFA(r.Context(), db.UpsertPendingUserMFAParams{
		UserID:    user.ID,
		SecretEnc: sealed,
	}); err != nil {
		if isNotFound(err) {
			writeErrorCode(w, http.StatusConflict, mfaCodeAlreadyOn, "multi-factor sign-in is already on")
			return
		}
		slog.Warn("mfa enroll: store secret failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to start enrolment")
		return
	}
	writeJSON(w, http.StatusOK, EnrollMFAResponse{Secret: secret, URI: uri})
}

// ConfirmMFA turns the enrolled secret on when the caller proves they hold it,
// and returns the recovery codes, which are shown only here.
func (h *Handler) ConfirmMFA(w http.ResponseWriter, r *http.Request) {
	userID, crypto, ok := h.mfaCaller(w, r)
	if !ok {
		return
	}
	code, ok := decodeMFACode(w, r)
	if !ok {
		return
	}
	uid := parseUUID(userID)
	row, err := h.Queries.GetUserMFA(r.Context(), uid)
	if err != nil && !isNotFound(err) {
		writeError(w, http.StatusInternalServerError, "failed to load multi-factor state")
		return
	}
	if isNotFound(err) || row.ConfirmedAt.Valid {
		writeErrorCode(w, http.StatusConflict, mfaCodeNotEnrolling, "no enrolment is waiting to be confirmed")
		return
	}
	secret, err := crypto.Open(row.SecretEnc)
	if err != nil {
		slog.Error("mfa confirm: cannot open the stored secret", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to read the enrolled secret")
		return
	}
	step, matched := mfa.MatchStep(secret, code, time.Now())
	if !matched {
		writeErrorCode(w, http.StatusBadRequest, mfaCodeInvalidCode, "that code is not valid")
		return
	}

	codes, err := mfa.NewRecoveryCodes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate recovery codes")
		return
	}
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = crypto.HashRecoveryCode(c)
	}
	n, err := h.Queries.ConfirmUserMFA(r.Context(), db.ConfirmUserMFAParams{
		UserID:         uid,
		RecoveryHashes: hashes,
		LastUsedStep:   step,
		SecretEnc:      row.SecretEnc,
	})
	if err != nil {
		slog.Warn("mfa confirm: store failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to confirm")
		return
	}
	if n == 0 {
		// Enrolled again, or confirmed, between the read and the write.
		writeErrorCode(w, http.StatusConflict, mfaCodeNotEnrolling, "no enrolment is waiting to be confirmed")
		return
	}
	writeJSON(w, http.StatusOK, ConfirmMFAResponse{RecoveryCodes: codes})
}

// DisableMFA turns MFA off for the caller, given a valid TOTP or recovery code.
func (h *Handler) DisableMFA(w http.ResponseWriter, r *http.Request) {
	userID, crypto, ok := h.mfaCaller(w, r)
	if !ok {
		return
	}
	code, ok := decodeMFACode(w, r)
	if !ok {
		return
	}
	uid := parseUUID(userID)
	row, err := h.Queries.GetUserMFA(r.Context(), uid)
	if err != nil && !isNotFound(err) {
		writeError(w, http.StatusInternalServerError, "failed to load multi-factor state")
		return
	}
	if isNotFound(err) || !row.ConfirmedAt.Valid {
		writeErrorCode(w, http.StatusConflict, mfaCodeNotEnabled, "multi-factor sign-in is not on")
		return
	}
	valid, err := h.acceptSecondFactor(r.Context(), crypto, row, code, time.Now())
	if err != nil {
		slog.Warn("mfa disable: check failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to check the code")
		return
	}
	if !valid {
		writeErrorCode(w, http.StatusBadRequest, mfaCodeInvalidCode, "that code is not valid")
		return
	}
	if _, err := h.Queries.DeleteUserMFA(r.Context(), uid); err != nil {
		slog.Warn("mfa disable: delete failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to turn multi-factor sign-in off")
		return
	}
	h.notifyMFADisabled(r, userID)
	w.WriteHeader(http.StatusNoContent)
}

// acceptSecondFactor checks a TOTP or recovery code against a confirmed row
// and uses it up: a recovery code is removed, and the TOTP step is recorded so
// the same step cannot be accepted again. A wrong or already-used code is
// (false, nil); err is a failure to check.
func (h *Handler) acceptSecondFactor(ctx context.Context, crypto *mfa.Crypto, row db.UserMfa, code string, now time.Time) (bool, error) {
	if mfa.LooksLikeRecoveryCode(code) {
		n, err := h.Queries.ConsumeUserMFARecoveryHash(ctx, db.ConsumeUserMFARecoveryHashParams{
			UserID:       row.UserID,
			RecoveryHash: crypto.HashRecoveryCode(code),
		})
		return n == 1, err
	}
	secret, err := crypto.Open(row.SecretEnc)
	if err != nil {
		return false, errors.New("cannot open the stored secret: " + err.Error())
	}
	step, matched := mfa.MatchStep(secret, code, now)
	if !matched {
		return false, nil
	}
	n, err := h.Queries.ClaimUserMFAStep(ctx, db.ClaimUserMFAStepParams{UserID: row.UserID, LastUsedStep: step})
	return n == 1, err
}

// notifyMFADisabled emails the user that their second factor was turned off.
// The disable is already committed, so a failed notice is logged and does not
// fail the request. Without a configured mailer the event is logged without
// the address.
func (h *Handler) notifyMFADisabled(r *http.Request, userID string) {
	if h.EmailService == nil {
		slog.Info("mfa disabled; user notice not sent: mailer not configured", append(logger.RequestAttrs(r), "user_id", userID)...)
		return
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
	if err != nil {
		slog.Warn("mfa disabled; user notice not sent: user lookup failed", append(logger.RequestAttrs(r), "user_id", userID, "error", err)...)
		return
	}
	svc, to := h.EmailService, user.Email
	go func() {
		err := sendMFADisabledNotice(svc, to)
		switch {
		case errors.Is(err, service.ErrMailerNotConfigured):
			slog.Info("mfa disabled; user notice not sent: mailer not configured", "user_id", userID)
		case err != nil:
			slog.Warn("mfa disabled; user notice failed", "user_id", userID, "error", err)
		}
	}()
}
