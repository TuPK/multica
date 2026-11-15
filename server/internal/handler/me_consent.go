package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/consent"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// maxConsentItems bounds one accept request. There are two documents; the cap
// only keeps a hostile body from driving a long transaction.
const maxConsentItems = 16

// legalConsent reads the LEGAL_* variables on each call, so a changed version
// takes effect without a code path that caches it.
func legalConsent() consent.Config { return consent.FromEnv(os.Getenv) }

// pendingConsents lists the required documents the user has not accepted at
// the current version. With the feature off it does not query.
func pendingConsents(ctx context.Context, q *db.Queries, userID pgtype.UUID) ([]consent.Required, error) {
	cfg := legalConsent()
	if len(cfg.Documents()) == 0 {
		return []consent.Required{}, nil
	}
	rows, err := q.ListUserConsents(ctx, userID)
	if err != nil {
		return nil, err
	}
	accepted := make([]consent.Acceptance, len(rows))
	for i, r := range rows {
		accepted[i] = consent.Acceptance{Document: r.Document, Version: r.Version}
	}
	return cfg.Pending(accepted), nil
}

// AcceptConsentsRequest is the body of POST /api/me/consents.
type AcceptConsentsRequest struct {
	Documents []struct {
		Document string `json:"document"`
		Version  string `json:"version"`
	} `json:"documents"`
}

// AcceptConsentsResponse is the body of a successful POST /api/me/consents.
type AcceptConsentsResponse struct {
	ConsentRequired []consent.Required `json:"consent_required"`
}

// AcceptConsents serves POST /api/me/consents. It records each accepted
// (document, version) once, with one audit row per new acceptance, and refuses
// the whole request when any item is unknown or not the current version.
func (h *Handler) AcceptConsents(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	uid, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	var req AcceptConsentsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Documents) > maxConsentItems {
		writeError(w, http.StatusBadRequest, "too many documents")
		return
	}
	items := make([]consent.Item, len(req.Documents))
	for i, d := range req.Documents {
		items[i] = consent.Item{Document: d.Document, Version: d.Version}
	}
	switch err := legalConsent().Validate(items); {
	case errors.Is(err, consent.ErrInvalidDocument):
		writeErrorCode(w, http.StatusBadRequest, "invalid_document", "unknown document")
		return
	case errors.Is(err, consent.ErrVersionMismatch):
		writeErrorCode(w, http.StatusConflict, "consent_version_mismatch", "document version is not the current one")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to record consent")
		return
	}

	ip, _, _ := auditstore.RequestFields(r, h.clientIPForRateLimit)
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record consent")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.Queries.WithTx(tx)
	for _, it := range items {
		n, err := qtx.InsertUserConsent(r.Context(), db.InsertUserConsentParams{
			UserID: uid, Document: it.Document, Version: it.Version, Ip: ip,
		})
		if err != nil {
			slog.Warn("record consent failed", append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
			writeError(w, http.StatusInternalServerError, "failed to record consent")
			return
		}
		if n == 0 {
			continue
		}
		if err := h.recordAudit(r, tx, audit.Entry{
			Action:     audit.ActionConsentAccepted,
			TargetType: "user",
			TargetID:   userID,
			Detail:     map[string]any{"document": it.Document, "version": it.Version},
		}); err != nil {
			slog.Warn("record consent: audit failed", append(logger.RequestAttrs(r), "error", err, "user_id", userID)...)
			writeError(w, http.StatusInternalServerError, "failed to record consent")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record consent")
		return
	}

	pending, err := pendingConsents(r.Context(), h.Queries, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load consent state")
		return
	}
	writeJSON(w, http.StatusOK, AcceptConsentsResponse{ConsentRequired: pending})
}
