package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	proposalkinds "github.com/multica-ai/multica/server/internal/service/proposal_kinds"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// Seat offers on the Hub (durable spec 13.3, F12). An offer names a seat: the squad it joins,
// the runtime it asks for and the outcome it is hired on. A candidate is hired from an offer by a
// hire proposal, which carries the candidate's attestation. The hire goes through the ordinary
// org change, so the approval and the probation of a hire apply unchanged.

var (
	// ErrSeatOffersDisabled: durable_seats is off; no offer is posted or hired from.
	ErrSeatOffersDisabled = errors.New("seat offers are not available on this server")
	// ErrSeatOfferNotFound: no open offer with that id in the Hub org.
	ErrSeatOfferNotFound = errors.New("seat offer not found")
	// ErrSeatOfferInvalid: the offer names no usable seat or gives no rationale.
	ErrSeatOfferInvalid = errors.New("seat offer needs a name of 1 to 100 characters, a squad, a runtime and a rationale")
	// ErrSeatAttestationMalformed: the attestation is not JSON. A signature that does not check is
	// not refused: it is filed and read as unverified (SeatAttestationVerified).
	ErrSeatAttestationMalformed = errors.New("attestation is not JSON")
)

// SeatOfferInput is what a Hub owner posts as an offer.
type SeatOfferInput struct {
	Name            string
	Role            string
	SquadID         pgtype.UUID
	RuntimeID       pgtype.UUID
	OutcomeCriteria string
	Rationale       string
}

// SeatOfferService posts seat offers and hires from them.
type SeatOfferService struct {
	Queries   *db.Queries
	Flags     *featureflag.Service
	Proposals *ProposalService
}

func (s *SeatOfferService) on(ctx context.Context) bool {
	return featureflags.DurableSeatsEnabled(ctx, s.Flags)
}

// Post saves an offer in the workspace (its Hub org). Only a member posts an offer; the
// handler checks the role.
func (s *SeatOfferService) Post(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, in SeatOfferInput) (db.SeatOffer, error) {
	if !s.on(ctx) {
		return db.SeatOffer{}, ErrSeatOffersDisabled
	}
	name := strings.TrimSpace(in.Name)
	rationale := strings.TrimSpace(in.Rationale)
	if name == "" || utf8.RuneCountInString(name) > 100 || rationale == "" || !in.SquadID.Valid || !in.RuntimeID.Valid {
		return db.SeatOffer{}, ErrSeatOfferInvalid
	}
	if actor.Type != GovernanceActorMember || !actor.ID.Valid {
		return db.SeatOffer{}, ErrSeatOfferInvalid
	}
	if _, err := orgSquad(ctx, s.Queries, ws, util.UUIDToString(in.SquadID), "squad_id"); err != nil {
		return db.SeatOffer{}, err
	}
	if _, err := s.Queries.GetAgentRuntimeForWorkspace(ctx, db.GetAgentRuntimeForWorkspaceParams{ID: in.RuntimeID, WorkspaceID: ws}); err != nil {
		return db.SeatOffer{}, errors.New("runtime not found in this workspace")
	}
	return s.Queries.CreateSeatOffer(ctx, db.CreateSeatOfferParams{
		ID:              dbid.NewV7(),
		WorkspaceID:     ws,
		Name:            name,
		Role:            strings.TrimSpace(in.Role),
		SquadID:         in.SquadID,
		RuntimeID:       in.RuntimeID,
		OutcomeCriteria: strings.TrimSpace(in.OutcomeCriteria),
		Rationale:       rationale,
		CreatedBy:       actor.ID,
	})
}

// Hire files a hire proposal for a candidate from an open offer in the workspace. templateRef is
// the instruction template the candidate brings, as a hire names it. attestation is the
// candidate's signed track record, or nil when it has none. The proposal holds the attestation
// as filed; whether it verifies is read with SeatAttestationVerified.
func (s *SeatOfferService) Hire(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, offerID pgtype.UUID, templateRef string, attestation []byte) (db.Proposal, error) {
	if !s.on(ctx) {
		return db.Proposal{}, ErrSeatOffersDisabled
	}
	if len(attestation) > 0 && !json.Valid(attestation) {
		return db.Proposal{}, ErrSeatAttestationMalformed
	}
	offer, err := s.Queries.GetSeatOfferInWorkspace(ctx, db.GetSeatOfferInWorkspaceParams{ID: offerID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && offer.Status != "open") {
		return db.Proposal{}, ErrSeatOfferNotFound
	}
	if err != nil {
		return db.Proposal{}, err
	}
	payload, err := json.Marshal(hirePayload{
		Type:        OrgChangeHire,
		SquadID:     util.UUIDToString(offer.SquadID),
		Name:        offer.Name,
		Template:    templateRef,
		RuntimeID:   util.UUIDToString(offer.RuntimeID),
		Rationale:   offer.Rationale,
		Attestation: attestation,
	})
	if err != nil {
		return db.Proposal{}, err
	}
	res, err := s.Proposals.Create(ctx, ProposalCreateParams{
		WorkspaceID: ws,
		SquadID:     offer.SquadID,
		Content: ProposalContent{
			Title:          "Hire " + offer.Name,
			Summary:        offer.Rationale,
			Evidence:       []string{"Seat offer " + util.UUIDToString(offer.ID)},
			Recommendation: "Hire the candidate into the offered seat.",
			CostRisk:       "The seat's cost follows the runtime the offer names.",
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
	return Verify(p.Attestation, keys) == nil, nil
}
