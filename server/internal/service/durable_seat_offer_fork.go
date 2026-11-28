package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/hub"
	proposalkinds "github.com/multica-ai/multica/server/internal/service/proposal_kinds"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// Seat offers on the Hub (durable spec 13.3, F12). An offer names a seat: the role, work class,
// squad it joins, optional runtime it requires, default outcome criteria, proposed budget and
// proposed autonomy envelope. A candidate is hired from an offer by a hire proposal, which carries
// the candidate's attestation. The hire goes through the ordinary org change, so the approval,
// probation, and full starting verification mode apply unchanged.

const (
	// SeatOfferListLimitMax bounds one listing of a Hub org's seat offers.
	SeatOfferListLimitMax int32 = 100

	seatOfferNameMax      = 100
	seatOfferRoleMax      = 300
	seatOfferWorkClassMax = 200
	seatOfferCriteriaMax  = 2000
	seatOfferRationaleMax = 2000

	wellKnownKeysMaxBytes       = 256 << 10
	wellKnownKeysDefaultTimeout = 10 * time.Second
)

var (
	// ErrSeatOffersDisabled: durable_open is off; no offer is posted, read, closed or hired from.
	ErrSeatOffersDisabled = errors.New("seat offers are not available on this server")
	// ErrSeatOfferNotFound: no matching offer with that id in the Hub org.
	ErrSeatOfferNotFound = errors.New("seat offer not found")
	// ErrSeatOfferInvalid: the offer names no usable seat or fails field validation.
	ErrSeatOfferInvalid = errors.New("seat offer needs a name of 1 to 100 characters, a squad and a rationale")
	// ErrSeatOfferBudgetInvalid: proposed budget amount and period must both be set validly or both omitted.
	ErrSeatOfferBudgetInvalid = errors.New("proposed budget needs a non-negative amount_micros and a period of week, month or quarter")
	// ErrSeatOfferEnvelopeInvalid: proposed envelope bounds must be 0 to 3 with floor <= ceiling.
	ErrSeatOfferEnvelopeInvalid = errors.New("proposed envelope floor and ceiling must be 0 to 3 with floor <= ceiling")
	// ErrSeatOfferRuntimeRequired: the offer does not fix a runtime, so the application must name one.
	ErrSeatOfferRuntimeRequired = errors.New("runtime_id is required when the offer does not specify one")
	// ErrSeatOfferRuntimeNotFound: the named runtime does not belong to this workspace.
	ErrSeatOfferRuntimeNotFound = errors.New("runtime not found in this workspace")
	// ErrSeatAttestationMalformed: the attestation is not a JSON object. A structurally valid JSON
	// object whose signature does not verify is not refused: it is filed and read as unverified.
	ErrSeatAttestationMalformed = errors.New("attestation must be a JSON object")
)

// SeatOfferInput is what a Hub owner or admin posts as an offer (spec 13.3).
type SeatOfferInput struct {
	Name                 string
	Role                 string
	WorkClass            string
	SquadID              pgtype.UUID
	RuntimeID            pgtype.UUID // optional: "runtime yêu cầu (nếu có)"
	OutcomeCriteria      string
	ProposedBudgetMicros *int64
	ProposedBudgetPeriod string
	ProposedEnvelope     *Envelope
	Rationale            string
}

// SeatOfferApplyInput is what an applicant provides to file a hire proposal from an open offer.
type SeatOfferApplyInput struct {
	TemplateRef string
	RuntimeID   pgtype.UUID // required only when the offer has no fixed runtime
	Attestation []byte
}

// SeatOfferService posts, lists, closes and hires from seat offers inside a workspace (its Hub org).
type SeatOfferService struct {
	Queries   *db.Queries
	Flags     *featureflag.Service
	Proposals *ProposalService
}

func (s *SeatOfferService) on(ctx context.Context) bool {
	return featureflags.DurableOpenEnabled(ctx, s.Flags)
}

// Post saves an offer in the workspace (its Hub org). Only an owner or admin member may post.
func (s *SeatOfferService) Post(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, in SeatOfferInput) (db.SeatOffer, error) {
	if !s.on(ctx) {
		return db.SeatOffer{}, ErrSeatOffersDisabled
	}
	if err := requireGovernanceBoss(ctx, s.Queries, ws, actor); err != nil {
		return db.SeatOffer{}, err
	}
	name := strings.TrimSpace(in.Name)
	role := strings.TrimSpace(in.Role)
	workClass := strings.TrimSpace(in.WorkClass)
	criteria := strings.TrimSpace(in.OutcomeCriteria)
	rationale := strings.TrimSpace(in.Rationale)
	if name == "" || utf8.RuneCountInString(name) > seatOfferNameMax ||
		utf8.RuneCountInString(role) > seatOfferRoleMax ||
		utf8.RuneCountInString(workClass) > seatOfferWorkClassMax ||
		utf8.RuneCountInString(criteria) > seatOfferCriteriaMax ||
		rationale == "" || utf8.RuneCountInString(rationale) > seatOfferRationaleMax ||
		!in.SquadID.Valid {
		return db.SeatOffer{}, ErrSeatOfferInvalid
	}

	budgetMicros, budgetPeriod, err := validateOfferBudget(in.ProposedBudgetMicros, in.ProposedBudgetPeriod)
	if err != nil {
		return db.SeatOffer{}, err
	}
	envFloor, envCeiling, err := validateOfferEnvelope(in.ProposedEnvelope)
	if err != nil {
		return db.SeatOffer{}, err
	}

	if _, err := orgSquad(ctx, s.Queries, ws, util.UUIDToString(in.SquadID), "squad_id"); err != nil {
		return db.SeatOffer{}, err
	}
	if in.RuntimeID.Valid {
		if _, err := s.Queries.GetAgentRuntimeForWorkspace(ctx, db.GetAgentRuntimeForWorkspaceParams{ID: in.RuntimeID, WorkspaceID: ws}); err != nil {
			return db.SeatOffer{}, ErrSeatOfferRuntimeNotFound
		}
	}
	return s.Queries.CreateSeatOffer(ctx, db.CreateSeatOfferParams{
		ID:                      dbid.NewV7(),
		WorkspaceID:             ws,
		Name:                    name,
		Role:                    role,
		WorkClass:               workClass,
		SquadID:                 in.SquadID,
		RuntimeID:               in.RuntimeID,
		OutcomeCriteria:         criteria,
		ProposedBudgetMicros:    budgetMicros,
		ProposedBudgetPeriod:    budgetPeriod,
		ProposedEnvelopeFloor:   envFloor,
		ProposedEnvelopeCeiling: envCeiling,
		Rationale:               rationale,
		CreatedBy:               actor.ID,
	})
}

// List returns the seat offers scoped to the workspace (its Hub org), optionally filtered by status
// ("open", "closed", or "" for all).
func (s *SeatOfferService) List(ctx context.Context, ws pgtype.UUID, status string, limit int32) ([]db.SeatOffer, error) {
	if !s.on(ctx) {
		return nil, ErrSeatOffersDisabled
	}
	status = strings.TrimSpace(status)
	switch status {
	case "", "open", "closed":
	default:
		return nil, ErrSeatOfferInvalid
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > SeatOfferListLimitMax {
		limit = SeatOfferListLimitMax
	}
	return s.Queries.ListSeatOffersByWorkspace(ctx, db.ListSeatOffersByWorkspaceParams{
		WorkspaceID: ws, Status: status, RowLimit: limit,
	})
}

// Get loads one seat offer scoped to the workspace.
func (s *SeatOfferService) Get(ctx context.Context, ws, offerID pgtype.UUID) (db.SeatOffer, error) {
	if !s.on(ctx) {
		return db.SeatOffer{}, ErrSeatOffersDisabled
	}
	offer, err := s.Queries.GetSeatOfferInWorkspace(ctx, db.GetSeatOfferInWorkspaceParams{ID: offerID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SeatOffer{}, ErrSeatOfferNotFound
	}
	return offer, err
}

// Close marks an open offer closed. Only an owner or admin member may close an offer.
func (s *SeatOfferService) Close(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, offerID pgtype.UUID) (db.SeatOffer, error) {
	if !s.on(ctx) {
		return db.SeatOffer{}, ErrSeatOffersDisabled
	}
	if err := requireGovernanceBoss(ctx, s.Queries, ws, actor); err != nil {
		return db.SeatOffer{}, err
	}
	offer, err := s.Queries.CloseSeatOfferInWorkspace(ctx, db.CloseSeatOfferInWorkspaceParams{ID: offerID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SeatOffer{}, ErrSeatOfferNotFound
	}
	return offer, err
}

// Hire files a hire proposal for a candidate from an open offer in the workspace. templateRef is
// the instruction template the candidate brings, as a hire names it. attestation is the
// candidate's signed track record, or nil when it has none. The proposal holds the attestation
// as filed; whether it verifies is read with SeatAttestationVerified or VerifySeatAttestation.
func (s *SeatOfferService) Hire(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, offerID pgtype.UUID, templateRef string, attestation []byte) (db.Proposal, error) {
	return s.Apply(ctx, ws, actor, offerID, SeatOfferApplyInput{
		TemplateRef: templateRef,
		Attestation: attestation,
	})
}

// Apply files a hire proposal for a candidate from an open offer in the workspace. When the offer
// fixes a runtime, that runtime is used (and a mismatched runtime_id is refused). When the offer
// leaves runtime optional, the application must name a runtime in the workspace.
func (s *SeatOfferService) Apply(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, offerID pgtype.UUID, in SeatOfferApplyInput) (db.Proposal, error) {
	if !s.on(ctx) {
		return db.Proposal{}, ErrSeatOffersDisabled
	}
	attestation, err := validateAttestationJSON(in.Attestation)
	if err != nil {
		return db.Proposal{}, err
	}
	offer, err := s.Queries.GetSeatOfferInWorkspace(ctx, db.GetSeatOfferInWorkspaceParams{ID: offerID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && offer.Status != "open") {
		return db.Proposal{}, ErrSeatOfferNotFound
	}
	if err != nil {
		return db.Proposal{}, err
	}

	runtimeID := offer.RuntimeID
	if runtimeID.Valid {
		if in.RuntimeID.Valid && in.RuntimeID != runtimeID {
			return db.Proposal{}, ErrSeatOfferInvalid
		}
	} else {
		if !in.RuntimeID.Valid {
			return db.Proposal{}, ErrSeatOfferRuntimeRequired
		}
		if _, err := s.Queries.GetAgentRuntimeForWorkspace(ctx, db.GetAgentRuntimeForWorkspaceParams{ID: in.RuntimeID, WorkspaceID: ws}); err != nil {
			return db.Proposal{}, ErrSeatOfferRuntimeNotFound
		}
		runtimeID = in.RuntimeID
	}

	payload, err := json.Marshal(hirePayload{
		Type:        OrgChangeHire,
		SquadID:     util.UUIDToString(offer.SquadID),
		Name:        offer.Name,
		Template:    in.TemplateRef,
		RuntimeID:   util.UUIDToString(runtimeID),
		Rationale:   offer.Rationale,
		Attestation: attestation,
	})
	if err != nil {
		return db.Proposal{}, err
	}
	evidence := []string{"Seat offer " + util.UUIDToString(offer.ID)}
	if offer.WorkClass != "" {
		evidence = append(evidence, "Work class: "+offer.WorkClass)
	}
	if offer.ProposedEnvelopeFloor.Valid && offer.ProposedEnvelopeCeiling.Valid {
		evidence = append(evidence, fmt.Sprintf("Proposed envelope: L%d..L%d", offer.ProposedEnvelopeFloor.Int32, offer.ProposedEnvelopeCeiling.Int32))
	}
	costRisk := "The seat's cost follows the runtime the offer names."
	if offer.ProposedBudgetMicros.Valid {
		costRisk = fmt.Sprintf("Proposed budget: %d micros per %s.", offer.ProposedBudgetMicros.Int64, offer.ProposedBudgetPeriod)
	}
	res, err := s.Proposals.Create(ctx, ProposalCreateParams{
		WorkspaceID: ws,
		SquadID:     offer.SquadID,
		Content: ProposalContent{
			Title:          "Hire " + offer.Name,
			Summary:        offer.Rationale,
			Evidence:       evidence,
			Recommendation: "Hire the candidate into the offered seat.",
			CostRisk:       costRisk,
			Ask:            "Approve the hire?",
		},
		CreatorType: actor.Type,
		CreatorID:   actor.ID,
		Kind:        proposalkinds.KeyOrgChange,
		Payload:     payload,
	})
	if err != nil {
		return db.Proposal{}, err
	}
	return res.Proposal, nil
}

// SeatAttestationVerified reads whether a hire proposal's attestation verifies against keys, the
// key set of the host that issued it. A proposal with no attestation is not verified. Reading it
// here, rather than storing a flag, means a payload cannot claim a verification it does not have.
func SeatAttestationVerified(payload []byte, keys KeySet) (bool, error) {
	var p hirePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return false, err
	}
	if len(p.Attestation) == 0 {
		return false, nil
	}
	// The attestation must bind to this payload's template, as VerifySeatAttestation requires.
	var probe struct {
		Subject struct {
			TemplateRef string `json:"template_ref"`
		} `json:"subject"`
	}
	if err := json.Unmarshal(p.Attestation, &probe); err != nil || p.Template == "" || probe.Subject.TemplateRef != p.Template {
		return false, nil
	}
	return Verify(p.Attestation, keys) == nil, nil
}

// AttestationKeyResolver resolves the public KeySet published by an attestation's issuer host.
type AttestationKeyResolver interface {
	KeysForHost(ctx context.Context, host string) (KeySet, error)
}

// WellKnownKeyFetcher resolves a remote host's attestation keys from
// https://<host>/.well-known/multica-attestation-keys.json (spec 13.2).
// When Own is set and matches host, it is returned directly without making a network request.
type WellKnownKeyFetcher struct {
	Own     *KeySet
	Timeout time.Duration
	// Scheme defaults to "https". Tests may override it together with Get.
	Scheme string
	// Get overrides the HTTP fetch (for tests). When nil, hub.NewSafeFetcher is used.
	Get func(ctx context.Context, targetURL string, maxBytes int64) ([]byte, error)
}

// KeysForHost loads and parses the public keys for host.
func (f *WellKnownKeyFetcher) KeysForHost(ctx context.Context, host string) (KeySet, error) {
	host = strings.TrimSpace(host)
	if !validAttestationHost(host) {
		return KeySet{}, fmt.Errorf("attestation: invalid issuer host %q", host)
	}
	if f != nil && f.Own != nil && f.Own.Host != "" && f.Own.Host == host {
		return *f.Own, nil
	}
	scheme := "https"
	timeout := wellKnownKeysDefaultTimeout
	var getter func(ctx context.Context, targetURL string, maxBytes int64) ([]byte, error)
	if f != nil {
		if f.Scheme != "" {
			scheme = f.Scheme
		}
		if f.Timeout > 0 {
			timeout = f.Timeout
		}
		getter = f.Get
	}
	targetURL := scheme + "://" + host + AttestationKeysPath
	var body []byte
	var err error
	if getter != nil {
		body, err = getter(ctx, targetURL, wellKnownKeysMaxBytes)
	} else {
		fetcher := hub.NewSafeFetcher(hub.FetcherOptions{Timeout: timeout})
		body, _, err = fetcher.Get(ctx, targetURL, wellKnownKeysMaxBytes)
	}
	if err != nil {
		return KeySet{}, err
	}
	return ParsePublicKeys(host, body)
}

// VerifySeatAttestation checks a hire proposal's attestation by fetching the issuer host's public
// keys through resolver. Any absent, malformed, unreachable or failing attestation reads as
// unverified (false, nil); only a corrupt proposal payload returns an error.
func VerifySeatAttestation(ctx context.Context, payload []byte, resolver AttestationKeyResolver) (bool, error) {
	var p hirePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return false, err
	}
	if len(p.Attestation) == 0 || resolver == nil {
		return false, nil
	}
	var probe struct {
		Subject struct {
			TemplateRef string `json:"template_ref"`
		} `json:"subject"`
		Issuer struct {
			Host string `json:"host"`
		} `json:"issuer"`
	}
	if err := json.Unmarshal(p.Attestation, &probe); err != nil || probe.Issuer.Host == "" || p.Template == "" || probe.Subject.TemplateRef != p.Template {
		return false, nil
	}
	keys, err := resolver.KeysForHost(ctx, probe.Issuer.Host)
	if err != nil {
		return false, nil
	}
	return Verify(p.Attestation, keys) == nil, nil
}

func validateOfferBudget(micros *int64, period string) (pgtype.Int8, string, error) {
	period = strings.TrimSpace(period)
	if micros == nil && period == "" {
		return pgtype.Int8{}, "", nil
	}
	if micros == nil || *micros < 0 {
		return pgtype.Int8{}, "", ErrSeatOfferBudgetInvalid
	}
	switch period {
	case "week", "month", "quarter":
		return pgtype.Int8{Int64: *micros, Valid: true}, period, nil
	default:
		return pgtype.Int8{}, "", ErrSeatOfferBudgetInvalid
	}
}

func validateOfferEnvelope(env *Envelope) (pgtype.Int4, pgtype.Int4, error) {
	if env == nil {
		return pgtype.Int4{}, pgtype.Int4{}, nil
	}
	if env.Floor < 0 || env.Ceiling > 3 || env.Floor > env.Ceiling {
		return pgtype.Int4{}, pgtype.Int4{}, ErrSeatOfferEnvelopeInvalid
	}
	return pgtype.Int4{Int32: int32(env.Floor), Valid: true}, pgtype.Int4{Int32: int32(env.Ceiling), Valid: true}, nil
}

// validateAttestationJSON accepts nil/empty (no attestation) or a valid JSON object. Non-JSON
// bytes, null, arrays, primitives, and whitespace-only non-nil inputs are rejected.
func validateAttestationJSON(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, ErrSeatAttestationMalformed
	}
	var obj map[string]any
	if err := json.Unmarshal(trimmed, &obj); err != nil || obj == nil {
		return nil, ErrSeatAttestationMalformed
	}
	return trimmed, nil
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
