package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/sso"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// dnsLookupTimeout bounds one TXT lookup, so a slow resolver cannot hold a
// request open.
const dnsLookupTimeout = 10 * time.Second

// SSODomainHandler serves the domain verification routes of the platform spec
// 6.3. The TXT resolver is injected: production passes net.DefaultResolver and
// tests pass a fake.
type SSODomainHandler struct {
	h      *Handler
	lookup sso.TXTLookup
}

func NewSSODomainHandler(h *Handler, lookup sso.TXTLookup) *SSODomainHandler {
	return &SSODomainHandler{h: h, lookup: lookup}
}

// Routes mounts the domain routes under /api/workspaces/{id}. The caller wraps
// them in the owner-only role check.
func (d *SSODomainHandler) Routes(r chi.Router) {
	r.Get("/sso/domains", d.List)
	r.Post("/sso/domains", d.Add)
	r.Delete("/sso/domains/{domain}", d.Remove)
	r.Post("/sso/domains/{domain}/verify", d.Verify)
}

// SSODomainResponse is a domain claim and the TXT record that proves it.
type SSODomainResponse struct {
	Domain     string  `json:"domain"`
	TXTName    string  `json:"txt_name"`
	TXTValue   string  `json:"txt_value"`
	VerifiedAt *string `json:"verified_at"`
}

func ssoDomainToResponse(row db.SsoDomain) SSODomainResponse {
	resp := SSODomainResponse{
		Domain:   row.Domain,
		TXTName:  sso.ChallengeName(row.Domain),
		TXTValue: sso.ChallengeValue(row.Token),
	}
	if row.VerifiedAt.Valid {
		s := row.VerifiedAt.Time.UTC().Format(time.RFC3339)
		resp.VerifiedAt = &s
	}
	return resp
}

// ssoFeatureAvailable reports whether the plan of a workspace includes sso.
// Cloud sends `enforce` for a plan without it; a stale or missing answer counts
// as available, as for every other gate.
func ssoFeatureAvailable(ctx context.Context, provider entitlement.Provider, workspaceID pgtype.UUID) bool {
	if provider == nil || !workspaceID.Valid {
		return true
	}
	return provider.Gate(ctx, uuid.UUID(workspaceID.Bytes), entitlement.GateSSO).Gate.Action != entitlement.ActionEnforce
}

func (d *SSODomainHandler) requireSSOFeature(w http.ResponseWriter, r *http.Request, workspaceID pgtype.UUID) bool {
	if ssoFeatureAvailable(r.Context(), d.h.Entitlements, workspaceID) {
		return true
	}
	writeJSON(w, http.StatusPaymentRequired, map[string]any{
		"error": "single sign-on is not included in the plan of this workspace",
		"code":  codePlanFeatureUnavailable,
		"gate":  string(entitlement.GateSSO),
	})
	return false
}

// refuseDomainVerifiedElsewhere answers 409 and returns false when another
// workspace has verified domain.
func (d *SSODomainHandler) refuseDomainVerifiedElsewhere(w http.ResponseWriter, r *http.Request, workspaceID pgtype.UUID, domain string) bool {
	owner, err := d.h.Queries.GetVerifiedSSODomainOwner(r.Context(), domain)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return true
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to check the domain")
		return false
	case owner == workspaceID:
		return true
	}
	writeErrorCode(w, http.StatusConflict, "domain_already_verified", "this domain is already verified by another workspace")
	return false
}

// List returns the domain claims of the workspace. Owner only.
// GET /api/workspaces/{id}/sso/domains
func (d *SSODomainHandler) List(w http.ResponseWriter, r *http.Request) {
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	rows, err := d.h.Queries.ListSSODomains(r.Context(), workspaceUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list domains")
		return
	}
	domains := make([]SSODomainResponse, 0, len(rows))
	for _, row := range rows {
		domains = append(domains, ssoDomainToResponse(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": domains})
}

// Add claims a domain for the workspace and returns the TXT record to publish.
// Adding a claim again returns it with the token it already has. A public email
// domain and a domain another workspace has verified are refused.
// POST /api/workspaces/{id}/sso/domains
func (d *SSODomainHandler) Add(w http.ResponseWriter, r *http.Request) {
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	if !d.requireSSOFeature(w, r, workspaceUUID) {
		return
	}
	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	domain, err := sso.NormalizeDomain(req.Domain)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_domain", "enter a domain such as example.com")
		return
	}
	if sso.IsPublicEmailDomain(domain) {
		writeErrorCode(w, http.StatusBadRequest, "public_email_domain", "a public email domain cannot be verified")
		return
	}
	if !d.refuseDomainVerifiedElsewhere(w, r, workspaceUUID, domain) {
		return
	}
	token, err := sso.NewToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue a token")
		return
	}
	row, err := d.h.Queries.AddSSODomain(r.Context(), db.AddSSODomainParams{WorkspaceID: workspaceUUID, Domain: domain, Token: token})
	if err != nil {
		slog.Warn("add sso domain failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to add the domain")
		return
	}
	writeJSON(w, http.StatusCreated, ssoDomainToResponse(row))
}

// Verify looks for the TXT record of a claim and marks the domain verified when
// it holds the claim's token.
// POST /api/workspaces/{id}/sso/domains/{domain}/verify
func (d *SSODomainHandler) Verify(w http.ResponseWriter, r *http.Request) {
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	if !d.requireSSOFeature(w, r, workspaceUUID) {
		return
	}
	domain, err := sso.NormalizeDomain(chi.URLParam(r, "domain"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_domain", "enter a domain such as example.com")
		return
	}
	row, err := d.h.Queries.GetSSODomain(r.Context(), db.GetSSODomainParams{WorkspaceID: workspaceUUID, Domain: domain})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "domain_not_found", "add the domain first")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the domain")
		return
	}
	if row.VerifiedAt.Valid {
		writeJSON(w, http.StatusOK, ssoDomainToResponse(row))
		return
	}
	if !d.refuseDomainVerifiedElsewhere(w, r, workspaceUUID, domain) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dnsLookupTimeout)
	defer cancel()
	found, err := sso.CheckTXT(ctx, d.lookup, domain, row.Token)
	if err != nil {
		slog.Warn("sso domain dns lookup failed", append(logger.RequestAttrs(r), "domain", domain, "error", err)...)
		writeErrorCode(w, http.StatusBadGateway, "dns_lookup_failed", "the DNS lookup failed; try again shortly")
		return
	}
	if !found {
		writeErrorCode(w, http.StatusUnprocessableEntity, "txt_record_not_found", "no TXT record at "+sso.ChallengeName(domain)+" holds the expected value")
		return
	}

	verified, err := d.h.Queries.MarkSSODomainVerified(r.Context(), db.MarkSSODomainVerifiedParams{WorkspaceID: workspaceUUID, Domain: domain})
	switch {
	case isUniqueViolation(err):
		writeErrorCode(w, http.StatusConflict, "domain_already_verified", "this domain is already verified by another workspace")
		return
	case errors.Is(err, pgx.ErrNoRows):
		// A concurrent verify of the same claim finished first.
		verified, err = d.h.Queries.GetSSODomain(r.Context(), db.GetSSODomainParams{WorkspaceID: workspaceUUID, Domain: domain})
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify the domain")
		return
	}
	writeJSON(w, http.StatusOK, ssoDomainToResponse(verified))
}

// Remove drops a domain claim, verified or not.
// DELETE /api/workspaces/{id}/sso/domains/{domain}
func (d *SSODomainHandler) Remove(w http.ResponseWriter, r *http.Request) {
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	domain, err := sso.NormalizeDomain(chi.URLParam(r, "domain"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_domain", "enter a domain such as example.com")
		return
	}
	n, err := d.h.Queries.DeleteSSODomain(r.Context(), db.DeleteSSODomainParams{WorkspaceID: workspaceUUID, Domain: domain})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove the domain")
		return
	}
	if n == 0 {
		writeErrorCode(w, http.StatusNotFound, "domain_not_found", "domain not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
