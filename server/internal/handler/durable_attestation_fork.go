package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/service"
)

// Track-record attestations over HTTP (durable spec 13.2, F12). Issuing is for owners and admins,
// as a person. The public keys and verification need no session; verification is rate limited in
// router_fork.go. All three answer 404 while durable_open is off (spec 14.1).

// attestationBodyLimit caps a document sent for verification.
const attestationBodyLimit = 256 << 10

// attestationKeysTimeout bounds the fetch of another host's published keys.
const attestationKeysTimeout = 10 * time.Second

// IssueAttestationRequest asks for an agent's attestation over a period, from and to inclusive,
// as YYYY-MM-DD.
type IssueAttestationRequest struct {
	AgentID       string `json:"agent_id"`
	From          string `json:"from"`
	To            string `json:"to"`
	TemplateRef   string `json:"template_ref"`
	ShowWorkspace bool   `json:"show_workspace"`
}

// VerifyAttestationResponse says whether a document verifies, and why not when it does not.
type VerifyAttestationResponse struct {
	Valid  bool   `json:"valid"`
	Issuer string `json:"issuer,omitempty"`
	KeyID  string `json:"key_id,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// attestationOff answers 404 while durable_open is off and reports whether it did.
func (h *Handler) attestationOff(w http.ResponseWriter, r *http.Request) bool {
	if featureflags.DurableOpenEnabled(r.Context(), h.FeatureFlags) {
		return false
	}
	writeError(w, http.StatusNotFound, "not found")
	return true
}

// attestationService wires the attestation service to the server's key and host, set once when the
// routes are mounted.
func (h *Handler) attestationService() *service.AttestationService {
	if h.durable.Attestations == nil {
		return &service.AttestationService{Queries: h.Queries}
	}
	return h.durable.Attestations
}

// IssueAttestation signs an agent's track record for the period and returns the document.
func (h *Handler) IssueAttestation(w http.ResponseWriter, r *http.Request) {
	if h.attestationOff(w, r) {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return
	}
	var req IssueAttestationRequest
	if !readCappedJSON(w, r, durableRequestBodyLimit, &req, "invalid request body") {
		return
	}
	agent, ok := parseUUIDOrBadRequest(w, req.AgentID, "agent_id")
	if !ok {
		return
	}
	from, errFrom := time.Parse(time.DateOnly, req.From)
	to, errTo := time.Parse(time.DateOnly, req.To)
	if errFrom != nil || errTo != nil {
		writeErrorCode(w, http.StatusBadRequest, "attestation_period_invalid", "from and to must be dates as YYYY-MM-DD")
		return
	}
	doc, err := h.attestationService().Issue(r.Context(), wsUUID, service.AttestationRequest{
		Agent: agent, From: from, To: to, TemplateRef: req.TemplateRef, ShowWorkspace: req.ShowWorkspace,
	})
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(doc)
	case errors.Is(err, service.ErrAttestationDisabled), errors.Is(err, service.ErrAttestationNoHost):
		writeErrorCode(w, http.StatusServiceUnavailable, "attestation_disabled", "attestation is not configured on this server")
	case errors.Is(err, service.ErrAttestationSampleSmall):
		writeErrorCode(w, http.StatusUnprocessableEntity, "attestation_sample_too_small", "the agent needs 30 scored outcomes in the period")
	case errors.Is(err, service.ErrAttestationAgentNotFound):
		writeErrorCode(w, http.StatusNotFound, "agent_not_found", "agent not found")
	case errors.Is(err, service.ErrAttestationPeriod), errors.Is(err, service.ErrAttestationTemplate):
		writeErrorCode(w, http.StatusBadRequest, "attestation_request_invalid", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "failed to issue the attestation")
	}
}

// GetAttestationKeys publishes this server's public keys at the well-known path. It works even
// when no signing key is set: verification of older documents still needs the retired keys.
func (h *Handler) GetAttestationKeys(w http.ResponseWriter, r *http.Request) {
	if h.attestationOff(w, r) {
		return
	}
	doc, err := service.PublicKeysDocument(h.attestationService().KeySet())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to publish the keys")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(doc)
}

// VerifyAttestation checks a document's signature against the keys of the host it names. The host
// is this server, or another host whose keys are fetched over HTTPS. A document that does not
// verify is answered with valid false and a reason, not an error.
func (h *Handler) VerifyAttestation(w http.ResponseWriter, r *http.Request) {
	if h.attestationOff(w, r) {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, attestationBodyLimit+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read the body")
		return
	}
	if len(raw) > attestationBodyLimit {
		writeError(w, http.StatusRequestEntityTooLarge, "attestation is too large")
		return
	}
	var probe struct {
		Issuer struct {
			Host  string `json:"host"`
			KeyID string `json:"key_id"`
		} `json:"issuer"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		writeJSON(w, http.StatusOK, VerifyAttestationResponse{Reason: "malformed"})
		return
	}
	keys, err := h.attestationKeysFor(r.Context(), probe.Issuer.Host)
	if err != nil {
		writeJSON(w, http.StatusOK, VerifyAttestationResponse{Issuer: probe.Issuer.Host, Reason: "keys_unavailable"})
		return
	}
	if err := service.Verify(raw, keys); err != nil {
		writeJSON(w, http.StatusOK, VerifyAttestationResponse{Issuer: probe.Issuer.Host, KeyID: probe.Issuer.KeyID, Reason: attestationReason(err)})
		return
	}
	writeJSON(w, http.StatusOK, VerifyAttestationResponse{Valid: true, Issuer: probe.Issuer.Host, KeyID: probe.Issuer.KeyID})
}

// attestationKeysFor returns the keys to verify a document from host: this server's own set, or the
// set host publishes at its well-known path.
func (h *Handler) attestationKeysFor(ctx context.Context, host string) (service.KeySet, error) {
	if !validAttestationHost(host) {
		return service.KeySet{}, errors.New("invalid issuer host")
	}
	if own := h.attestationService().KeySet(); own.Host == host {
		return own, nil
	}
	fetcher := hub.NewSafeFetcher(hub.FetcherOptions{Timeout: attestationKeysTimeout})
	body, _, err := fetcher.Get(ctx, "https://"+host+service.AttestationKeysPath, attestationBodyLimit)
	if err != nil {
		return service.KeySet{}, err
	}
	return service.ParsePublicKeys(host, body)
}

func validAttestationHost(host string) bool {
	if host == "" || strings.ContainsAny(host, "/?#@\\ \t\r\n") {
		return false
	}
	u, err := url.Parse("https://" + host)
	if err != nil || u.Host != host || u.Hostname() == "" || u.User != nil {
		return false
	}
	return true
}

func attestationReason(err error) string {
	switch {
	case errors.Is(err, service.ErrBadSignature):
		return "bad_signature"
	case errors.Is(err, service.ErrUnknownKey):
		return "unknown_key"
	case errors.Is(err, service.ErrForeignKey):
		return "foreign_issuer"
	default:
		return "malformed"
	}
}
