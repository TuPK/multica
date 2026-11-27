package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Seat offers on the Hub over HTTP (durable spec 13.3, F12). Reads are for workspace members.
// Posting and closing an offer are for owners and admins, as a person (refused to agents and
// ordinary members both in router_fork.go and here). Applying files a hire proposal into the
// offer's squad. All routes answer 404 while durable_open is off (spec 14.1).

const (
	seatOfferCreateBodyLimit = 16 << 10
	seatOfferApplyBodyLimit  = 256 << 10
)

// SeatOfferBudgetBody is the proposed budget of a seat offer.
type SeatOfferBudgetBody struct {
	AmountMicros *int64 `json:"amount_micros"`
	Period       string `json:"period"`
}

// SeatOfferEnvelopeBody is the proposed autonomy envelope of a seat offer.
type SeatOfferEnvelopeBody struct {
	FloorLevel   *int `json:"floor_level"`
	CeilingLevel *int `json:"ceiling_level"`
}

// SeatOfferResponse is one seat offer in a Hub org.
type SeatOfferResponse struct {
	ID               string                 `json:"id"`
	WorkspaceID      string                 `json:"workspace_id"`
	Name             string                 `json:"name"`
	Role             string                 `json:"role"`
	WorkClass        string                 `json:"work_class"`
	SquadID          string                 `json:"squad_id"`
	RuntimeID        *string                `json:"runtime_id"`
	OutcomeCriteria  string                 `json:"outcome_criteria"`
	ProposedBudget   *SeatOfferBudgetBody   `json:"proposed_budget"`
	ProposedEnvelope *SeatOfferEnvelopeBody `json:"proposed_envelope"`
	Rationale        string                 `json:"rationale"`
	Status           string                 `json:"status"`
	CreatedBy        string                 `json:"created_by"`
	CreatedAt        string                 `json:"created_at"`
}

// SeatOfferListResponse wraps the seat offers of a Hub org.
type SeatOfferListResponse struct {
	Offers []SeatOfferResponse `json:"offers"`
}

// CreateSeatOfferRequest posts a new seat offer into the workspace's Hub org.
type CreateSeatOfferRequest struct {
	Name             string                 `json:"name"`
	Role             string                 `json:"role"`
	WorkClass        string                 `json:"work_class"`
	SquadID          string                 `json:"squad_id"`
	RuntimeID        string                 `json:"runtime_id"`
	OutcomeCriteria  string                 `json:"outcome_criteria"`
	ProposedBudget   *SeatOfferBudgetBody   `json:"proposed_budget"`
	ProposedEnvelope *SeatOfferEnvelopeBody `json:"proposed_envelope"`
	Rationale        string                 `json:"rationale"`
}

// ApplySeatOfferRequest files a hire proposal for a candidate applying to an open seat offer.
type ApplySeatOfferRequest struct {
	TemplateRef string          `json:"template_ref"`
	RuntimeID   string          `json:"runtime_id"`
	Attestation json.RawMessage `json:"attestation,omitempty"`
}

// ApplySeatOfferResponse reports the filed hire proposal and whether the candidate's attestation
// verified against its issuer host's published keys.
type ApplySeatOfferResponse struct {
	ProposalID          string `json:"proposal_id"`
	OfferID             string `json:"offer_id"`
	Status              string `json:"status"`
	AttestationVerified bool   `json:"attestation_verified"`
}

// seatOfferKeyFetcherFor builds the remote well-known key resolver for verifying candidate
// attestations. Tests may override it to point at a local httptest.Server.
var seatOfferKeyFetcherFor = func(h *Handler) service.AttestationKeyResolver {
	own := h.attestationService().KeySet()
	return &service.WellKnownKeyFetcher{Own: &own}
}

func (h *Handler) seatOfferOff(w http.ResponseWriter, r *http.Request) bool {
	if featureflags.DurableOpenEnabled(r.Context(), h.FeatureFlags) {
		return false
	}
	writeError(w, http.StatusNotFound, "not found")
	return true
}

func (h *Handler) seatOfferService() *service.SeatOfferService {
	return &service.SeatOfferService{
		Queries:   h.Queries,
		Flags:     h.FeatureFlags,
		Proposals: h.proposalService(),
	}
}

func seatOfferToResponse(o db.SeatOffer) SeatOfferResponse {
	resp := SeatOfferResponse{
		ID:              util.UUIDToString(o.ID),
		WorkspaceID:     util.UUIDToString(o.WorkspaceID),
		Name:            o.Name,
		Role:            o.Role,
		WorkClass:       o.WorkClass,
		SquadID:         util.UUIDToString(o.SquadID),
		OutcomeCriteria: o.OutcomeCriteria,
		Rationale:       o.Rationale,
		Status:          o.Status,
		CreatedBy:       util.UUIDToString(o.CreatedBy),
		CreatedAt:       timestampToString(o.CreatedAt),
	}
	if o.RuntimeID.Valid {
		id := util.UUIDToString(o.RuntimeID)
		resp.RuntimeID = &id
	}
	if o.ProposedBudgetMicros.Valid {
		amount := o.ProposedBudgetMicros.Int64
		resp.ProposedBudget = &SeatOfferBudgetBody{
			AmountMicros: &amount,
			Period:       o.ProposedBudgetPeriod,
		}
	}
	if o.ProposedEnvelopeFloor.Valid && o.ProposedEnvelopeCeiling.Valid {
		floor := int(o.ProposedEnvelopeFloor.Int32)
		ceiling := int(o.ProposedEnvelopeCeiling.Int32)
		resp.ProposedEnvelope = &SeatOfferEnvelopeBody{
			FloorLevel:   &floor,
			CeilingLevel: &ceiling,
		}
	}
	return resp
}

// ListSeatOffers handles GET /api/hub/seat-offers.
func (h *Handler) ListSeatOffers(w http.ResponseWriter, r *http.Request) {
	if h.seatOfferOff(w, r) {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	var limit int32
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = int32(n)
	}
	offers, err := h.seatOfferService().List(r.Context(), wsUUID, status, limit)
	switch {
	case err == nil:
		out := make([]SeatOfferResponse, 0, len(offers))
		for _, o := range offers {
			out = append(out, seatOfferToResponse(o))
		}
		writeJSON(w, http.StatusOK, SeatOfferListResponse{Offers: out})
	case errors.Is(err, service.ErrSeatOffersDisabled):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, service.ErrSeatOfferInvalid):
		writeErrorCode(w, http.StatusBadRequest, "seat_offer_invalid", err.Error())
	default:
		slog.Warn("list seat offers failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to list seat offers")
	}
}

// GetSeatOffer handles GET /api/hub/seat-offers/{id}.
func (h *Handler) GetSeatOffer(w http.ResponseWriter, r *http.Request) {
	if h.seatOfferOff(w, r) {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	offerID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "offer id")
	if !ok {
		return
	}
	offer, err := h.seatOfferService().Get(r.Context(), wsUUID, offerID)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, seatOfferToResponse(offer))
	case errors.Is(err, service.ErrSeatOffersDisabled), errors.Is(err, service.ErrSeatOfferNotFound):
		writeError(w, http.StatusNotFound, "seat offer not found")
	default:
		slog.Warn("get seat offer failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to load seat offer")
	}
}

// CreateSeatOffer handles POST /api/hub/seat-offers. Owners and admins only, as a person.
func (h *Handler) CreateSeatOffer(w http.ResponseWriter, r *http.Request) {
	if h.seatOfferOff(w, r) {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	actor, ok := h.governanceBossFor(w, r, workspaceID)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, seatOfferCreateBodyLimit)
	var req CreateSeatOfferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	squadID, ok := parseUUIDOrBadRequest(w, req.SquadID, "squad_id")
	if !ok {
		return
	}
	var runtimeID pgtype.UUID
	if strings.TrimSpace(req.RuntimeID) != "" {
		runtimeID, ok = parseUUIDOrBadRequest(w, req.RuntimeID, "runtime_id")
		if !ok {
			return
		}
	}
	in := service.SeatOfferInput{
		Name:            req.Name,
		Role:            req.Role,
		WorkClass:       req.WorkClass,
		SquadID:         squadID,
		RuntimeID:       runtimeID,
		OutcomeCriteria: req.OutcomeCriteria,
		Rationale:       req.Rationale,
	}
	if req.ProposedBudget != nil {
		in.ProposedBudgetMicros = req.ProposedBudget.AmountMicros
		in.ProposedBudgetPeriod = req.ProposedBudget.Period
	}
	if req.ProposedEnvelope != nil {
		if req.ProposedEnvelope.FloorLevel == nil || req.ProposedEnvelope.CeilingLevel == nil {
			writeErrorCode(w, http.StatusBadRequest, "seat_offer_envelope_invalid", service.ErrSeatOfferEnvelopeInvalid.Error())
			return
		}
		in.ProposedEnvelope = &service.Envelope{
			Floor:   *req.ProposedEnvelope.FloorLevel,
			Ceiling: *req.ProposedEnvelope.CeilingLevel,
		}
	}
	offer, err := h.seatOfferService().Post(r.Context(), wsUUID, actor, in)
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, seatOfferToResponse(offer))
	case errors.Is(err, service.ErrSeatOffersDisabled):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, service.ErrGovernanceActorNotBoss):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrSeatOfferBudgetInvalid):
		writeErrorCode(w, http.StatusBadRequest, "seat_offer_budget_invalid", err.Error())
	case errors.Is(err, service.ErrSeatOfferEnvelopeInvalid):
		writeErrorCode(w, http.StatusBadRequest, "seat_offer_envelope_invalid", err.Error())
	case errors.Is(err, service.ErrSeatOfferInvalid), errors.Is(err, service.ErrSeatOfferRuntimeNotFound):
		writeErrorCode(w, http.StatusBadRequest, "seat_offer_invalid", err.Error())
	default:
		if strings.Contains(err.Error(), "squad") {
			writeErrorCode(w, http.StatusBadRequest, "seat_offer_invalid", err.Error())
			return
		}
		slog.Warn("create seat offer failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to post seat offer")
	}
}

// CloseSeatOffer handles POST /api/hub/seat-offers/{id}/close. Owners and admins only, as a person.
func (h *Handler) CloseSeatOffer(w http.ResponseWriter, r *http.Request) {
	if h.seatOfferOff(w, r) {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	actor, ok := h.governanceBossFor(w, r, workspaceID)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	offerID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "offer id")
	if !ok {
		return
	}
	offer, err := h.seatOfferService().Close(r.Context(), wsUUID, actor, offerID)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, seatOfferToResponse(offer))
	case errors.Is(err, service.ErrSeatOffersDisabled), errors.Is(err, service.ErrSeatOfferNotFound):
		writeError(w, http.StatusNotFound, "seat offer not found")
	case errors.Is(err, service.ErrGovernanceActorNotBoss):
		writeError(w, http.StatusForbidden, err.Error())
	default:
		slog.Warn("close seat offer failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to close seat offer")
	}
}

// ApplySeatOffer handles POST /api/hub/seat-offers/{id}/apply. Files a hire proposal into the
// offer's squad carrying the candidate's attestation, and verifies the attestation against its
// issuer host's well-known public keys.
func (h *Handler) ApplySeatOffer(w http.ResponseWriter, r *http.Request) {
	if h.seatOfferOff(w, r) {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	caller, ok := h.outcomeCallerFor(w, r, workspaceID)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	offerID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "offer id")
	if !ok {
		return
	}
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, seatOfferApplyBodyLimit+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read request body")
		return
	}
	if len(rawBody) > seatOfferApplyBodyLimit {
		writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
		return
	}
	var req ApplySeatOfferRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "attestation_malformed", "invalid request body")
		return
	}
	var runtimeID pgtype.UUID
	if strings.TrimSpace(req.RuntimeID) != "" {
		runtimeID, ok = parseUUIDOrBadRequest(w, req.RuntimeID, "runtime_id")
		if !ok {
			return
		}
	}
	proposal, err := h.seatOfferService().Apply(r.Context(), wsUUID, caller.actor, offerID, service.SeatOfferApplyInput{
		TemplateRef: strings.TrimSpace(req.TemplateRef),
		RuntimeID:   runtimeID,
		Attestation: req.Attestation,
	})
	var perr *service.ProposalPayloadError
	switch {
	case err == nil:
		verified, _ := service.VerifySeatAttestation(r.Context(), proposal.Payload, seatOfferKeyFetcherFor(h))
		writeJSON(w, http.StatusCreated, ApplySeatOfferResponse{
			ProposalID:          util.UUIDToString(proposal.ID),
			OfferID:             util.UUIDToString(offerID),
			Status:              proposal.State,
			AttestationVerified: verified,
		})
	case errors.Is(err, service.ErrSeatOffersDisabled), errors.Is(err, service.ErrSeatOfferNotFound):
		writeError(w, http.StatusNotFound, "seat offer not found")
	case errors.Is(err, service.ErrSeatAttestationMalformed):
		writeErrorCode(w, http.StatusBadRequest, "attestation_malformed", err.Error())
	case errors.Is(err, service.ErrSeatOfferRuntimeRequired),
		errors.Is(err, service.ErrSeatOfferRuntimeNotFound),
		errors.Is(err, service.ErrSeatOfferInvalid):
		writeErrorCode(w, http.StatusBadRequest, "seat_offer_invalid", err.Error())
	case errors.As(err, &perr):
		writeErrorCode(w, http.StatusBadRequest, "seat_offer_invalid", perr.Error())
	default:
		slog.Warn("apply seat offer failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to file hire proposal")
	}
}
