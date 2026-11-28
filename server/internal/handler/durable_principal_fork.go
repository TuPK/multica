package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The principal profile over HTTP (durable spec 6.6, F5). A person reads and changes their own
// profile; the workspace Secretary reads it and writes learned traits; an owner reads it only when
// the person shared it. Every route but the person's own delete answers 404 while durable_principal
// is off.

// PrincipalTraitResponse is one trait of a profile.
type PrincipalTraitResponse struct {
	ID           string          `json:"id"`
	DecisionKind string          `json:"decision_kind"`
	Statement    string          `json:"statement"`
	Origin       string          `json:"origin"`
	Evidence     json.RawMessage `json:"evidence"`
	CreatedAt    string          `json:"created_at"`
}

// PrincipalProfileResponse is a profile with its active traits.
type PrincipalProfileResponse struct {
	MemberID        string                   `json:"member_id"`
	Status          string                   `json:"status"`
	ConsentAt       string                   `json:"consent_at"`
	ShareWithOwners bool                     `json:"share_with_owners"`
	Version         int32                    `json:"version"`
	Traits          []PrincipalTraitResponse `json:"traits"`
}

// principalCaller is the caller of a principal route and the workspace it acts in.
type principalCaller struct {
	actor service.PrincipalActor
	ws    pgtype.UUID
	owner bool
}

// requirePrincipalPerson refuses an agent: consenting, editing, deleting and the interview are the
// person's own (durable spec 6.6). Person-only routes also carry RequireHumanActor on the router.
func requirePrincipalPerson(w http.ResponseWriter, c principalCaller) bool {
	if c.actor.Type != service.GovernanceActorMember {
		writeError(w, http.StatusForbidden, "this endpoint is only available to the person")
		return false
	}
	return true
}

func rejectPrincipalMachineCredential(w http.ResponseWriter, r *http.Request) bool {
	if isMachineCredentialActor(r) {
		writeError(w, http.StatusForbidden, "this endpoint is only available to human actors")
		return true
	}
	return false
}

// principalCallerFor resolves the caller. With requireFlag, a server with durable_principal off
// answers 404 before anything else is read.
func (h *Handler) principalCallerFor(w http.ResponseWriter, r *http.Request, requireFlag bool) (principalCaller, bool) {
	if requireFlag && (h.FeatureFlags == nil || !featureflags.DurablePrincipalEnabled(r.Context(), h.FeatureFlags)) {
		writeError(w, http.StatusNotFound, "not found")
		return principalCaller{}, false
	}
	workspaceID := h.resolveWorkspaceID(r)
	c, ok := h.initiativeCallerFor(w, r, workspaceID)
	if !ok {
		return principalCaller{}, false
	}
	ws, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return principalCaller{}, false
	}
	return principalCaller{
		actor: service.PrincipalActor{Type: c.actor.Type, ID: c.actor.ID},
		ws:    ws,
		owner: c.admin,
	}, true
}

// principalMember is the person a request is about: member_id when the request names one, else the
// caller when the caller is a person. The Secretary must name the person.
func principalMember(w http.ResponseWriter, c principalCaller, raw string) (pgtype.UUID, bool) {
	if raw == "" {
		if c.actor.Type == service.GovernanceActorMember {
			return c.actor.ID, true
		}
		writeError(w, http.StatusBadRequest, "member_id is required")
		return pgtype.UUID{}, false
	}
	return parseUUIDOrBadRequest(w, raw, "member_id")
}

func principalTraitResponse(t db.PrincipalTrait) PrincipalTraitResponse {
	return PrincipalTraitResponse{
		ID:           util.UUIDToString(t.ID),
		DecisionKind: t.DecisionKind,
		Statement:    t.Statement,
		Origin:       t.Origin,
		Evidence:     json.RawMessage(t.Evidence),
		CreatedAt:    timestampToString(t.CreatedAt),
	}
}

func principalProfileResponse(p db.PrincipalProfile, traits []db.PrincipalTrait) PrincipalProfileResponse {
	out := PrincipalProfileResponse{
		MemberID:        util.UUIDToString(p.MemberID),
		Status:          p.Status,
		ConsentAt:       timestampToString(p.ConsentAt),
		ShareWithOwners: p.ShareWithOwners,
		Version:         p.Version,
		Traits:          make([]PrincipalTraitResponse, 0, len(traits)),
	}
	for _, t := range traits {
		out.Traits = append(out.Traits, principalTraitResponse(t))
	}
	return out
}

func (h *Handler) principalService() *service.PrincipalService {
	return &service.PrincipalService{Queries: h.Queries, TxStarter: h.TxStarter, Flags: h.FeatureFlags}
}

// writePrincipalError maps the profile's rules to status codes and machine codes.
func writePrincipalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrPrincipalDisabled):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, service.ErrPrincipalConsentRequired):
		writeErrorCode(w, http.StatusUnprocessableEntity, "profile_consent_required", err.Error())
	case errors.Is(err, service.ErrPrincipalNoProfile), errors.Is(err, service.ErrPrincipalTraitNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrPrincipalForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrPrincipalTraitEdited):
		writeErrorCode(w, http.StatusForbidden, "principal_trait_edited", err.Error())
	case errors.Is(err, service.ErrPrincipalEvidenceInvalid):
		writeErrorCode(w, http.StatusUnprocessableEntity, "principal_evidence_invalid", err.Error())
	case errors.Is(err, service.ErrPrincipalTraitInvalid):
		writeErrorCode(w, http.StatusUnprocessableEntity, "principal_trait_invalid", err.Error())
	case errors.Is(err, service.ErrPrincipalTraitLimit):
		writeErrorCode(w, http.StatusConflict, "principal_trait_limit", err.Error())
	case errors.Is(err, service.ErrPrincipalNoSecretary):
		writeErrorCode(w, http.StatusConflict, "principal_no_secretary", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "failed to update the principal profile")
	}
}

// GetPrincipalProfile reads a profile (GET /api/principal/profile?member_id=). The person reads
// their own; the Secretary reads any; an owner reads one the person shared.
func (h *Handler) GetPrincipalProfile(w http.ResponseWriter, r *http.Request) {
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	member, ok := principalMember(w, c, r.URL.Query().Get("member_id"))
	if !ok {
		return
	}
	svc := h.principalService()
	profile, traits, err := svc.Profile(r.Context(), c.ws, member)
	if err != nil {
		writePrincipalError(w, err)
		return
	}
	secretary, err := h.Queries.GetWorkspaceSecretaryAgentID(r.Context(), c.ws)
	if err != nil {
		secretary = pgtype.UUID{}
	}
	if !service.PrincipalCanRead(c.actor, member, secretary, c.owner, profile.ShareWithOwners) {
		writeError(w, http.StatusForbidden, "this actor may not read this principal profile")
		return
	}
	writeJSON(w, http.StatusOK, principalProfileResponse(profile, traits))
}

// EnablePrincipalProfile turns the caller's profile on after consent (PUT /api/principal/profile).
// Only a person can (RequireHumanActor).
//
//	{consent_text, share_with_owners}
func (h *Handler) EnablePrincipalProfile(w http.ResponseWriter, r *http.Request) {
	if rejectPrincipalMachineCredential(w, r) {
		return
	}
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	if !requirePrincipalPerson(w, c) {
		return
	}
	var req struct {
		ConsentText     string `json:"consent_text"`
		ShareWithOwners bool   `json:"share_with_owners"`
	}
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	profile, err := h.principalService().Enable(r.Context(), c.ws, c.actor.ID, req.ConsentText, req.ShareWithOwners)
	if err != nil {
		writePrincipalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, principalProfileResponse(profile, nil))
}

// DeletePrincipalProfile removes the caller's profile and its traits (DELETE /api/principal/profile).
// It answers 204 with the flag off too: a person can always remove what was kept.
func (h *Handler) DeletePrincipalProfile(w http.ResponseWriter, r *http.Request) {
	if rejectPrincipalMachineCredential(w, r) {
		return
	}
	c, ok := h.principalCallerFor(w, r, false)
	if !ok {
		return
	}
	if !requirePrincipalPerson(w, c) {
		return
	}
	if err := h.principalService().Delete(r.Context(), c.actor, c.ws, c.actor.ID); err != nil {
		writePrincipalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AddPrincipalTrait adds a trait (POST /api/principal/profile/traits). The person adds interview
// and edited traits about themselves; the Secretary adds learned traits for the member it names.
//
//	{member_id?, decision_kind, statement, origin, evidence: [{type, ref}]}
func (h *Handler) AddPrincipalTrait(w http.ResponseWriter, r *http.Request) {
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	var req struct {
		MemberID     string `json:"member_id"`
		DecisionKind string `json:"decision_kind"`
		Statement    string `json:"statement"`
		Origin       string `json:"origin"`
		Evidence     []struct {
			Type string `json:"type"`
			Ref  string `json:"ref"`
		} `json:"evidence"`
	}
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	member, ok := principalMember(w, c, req.MemberID)
	if !ok {
		return
	}
	evidence := make([]service.PrincipalEvidence, 0, len(req.Evidence))
	for _, e := range req.Evidence {
		evidence = append(evidence, service.PrincipalEvidence{Type: e.Type, Ref: e.Ref})
	}
	trait, err := h.principalService().AddTrait(r.Context(), c.actor, service.TraitInput{
		WorkspaceID: c.ws, MemberID: member, DecisionKind: req.DecisionKind,
		Statement: req.Statement, Origin: req.Origin, Evidence: evidence,
	})
	if err != nil {
		writePrincipalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, principalTraitResponse(trait))
}

// EditPrincipalTrait is the person's own edit of a trait's sentence (PATCH
// /api/principal/profile/traits/{id}). Only a person can (RequireHumanActor).
//
//	{statement}
func (h *Handler) EditPrincipalTrait(w http.ResponseWriter, r *http.Request) {
	if rejectPrincipalMachineCredential(w, r) {
		return
	}
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	if !requirePrincipalPerson(w, c) {
		return
	}
	traitID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "trait id")
	if !ok {
		return
	}
	var req struct {
		Statement string `json:"statement"`
	}
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	trait, err := h.principalService().EditTrait(r.Context(), c.actor, c.ws, traitID, req.Statement)
	if err != nil {
		writePrincipalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, principalTraitResponse(trait))
}

// RetirePrincipalTrait takes a trait out of a profile (DELETE /api/principal/profile/traits/{id}).
// The person retires their own traits; the Secretary retires learned ones (?member_id=).
func (h *Handler) RetirePrincipalTrait(w http.ResponseWriter, r *http.Request) {
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	traitID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "trait id")
	if !ok {
		return
	}
	member, ok := principalMember(w, c, r.URL.Query().Get("member_id"))
	if !ok {
		return
	}
	if err := h.principalService().RetireTrait(r.Context(), c.actor, c.ws, member, traitID); err != nil {
		writePrincipalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetPrincipalInterview lists the eight questions of the start-up interview
// (GET /api/principal/interview). Only a person is interviewed.
func (h *Handler) GetPrincipalInterview(w http.ResponseWriter, r *http.Request) {
	if rejectPrincipalMachineCredential(w, r) {
		return
	}
	c, ok := h.principalCallerFor(w, r, true)
	if !ok || !requirePrincipalPerson(w, c) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"questions": service.InterviewQuestions()})
}

// AnswerPrincipalInterview records one answer as an interview trait (POST
// /api/principal/interview/answers). The server writes the answer into the person's interview chat
// with the Secretary, and that message is the trait's evidence.
//
//	{question, answer}
func (h *Handler) AnswerPrincipalInterview(w http.ResponseWriter, r *http.Request) {
	if rejectPrincipalMachineCredential(w, r) {
		return
	}
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	if !requirePrincipalPerson(w, c) {
		return
	}
	var req struct {
		Question int    `json:"question"`
		Answer   string `json:"answer"`
	}
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	trait, err := h.principalService().AnswerInterview(r.Context(), c.ws, c.actor.ID, req.Question, req.Answer)
	if err != nil {
		writePrincipalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, principalTraitResponse(trait))
}

// PrincipalEvidenceCandidate is a proposal decision the person may cite as evidence of a trait they
// add by hand.
type PrincipalEvidenceCandidate struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Decision  string `json:"decision"`
	DecidedAt string `json:"decided_at"`
}

// ListPrincipalEvidenceCandidates lists the proposals the person decided most recently (GET
// /api/principal/evidence-candidates). Only the person sees their own decisions.
func (h *Handler) ListPrincipalEvidenceCandidates(w http.ResponseWriter, r *http.Request) {
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	if !requirePrincipalPerson(w, c) {
		return
	}
	rows, err := h.Queries.ListOwnRecentProposalDecisions(r.Context(), db.ListOwnRecentProposalDecisionsParams{
		WorkspaceID: c.ws, MemberID: c.actor.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list the decisions")
		return
	}
	decisions := make([]PrincipalEvidenceCandidate, 0, len(rows))
	for _, row := range rows {
		decisions = append(decisions, PrincipalEvidenceCandidate{
			ID: util.UUIDToString(row.ID), Title: row.IssueTitle, Decision: row.State,
			DecidedAt: timestampToString(row.DecidedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"decisions": decisions})
}
