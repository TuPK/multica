package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// The principal profile (durable spec 6.2 to 6.3, 6.6, F5): what a person's decisions say about
// how they decide, kept with the person. Only the person, and the Secretary while it learns, write
// it. Reads go through PrincipalCanRead. The flag durable_principal gates the writes that start or
// change a profile; Delete works with the flag off, so a person can always remove what was kept.

const (
	// GovernanceKindPrincipalEnabled is written when a person turns the profile on (spec 6.6).
	GovernanceKindPrincipalEnabled = "principal_profile_enabled"
	// GovernanceKindPrincipalDeleted is written when a person deletes the profile. It carries no content.
	GovernanceKindPrincipalDeleted = "principal_profile_deleted"

	principalStatementMax    = 500
	principalMaxActiveTraits = 200
	principalEvidenceMax     = 5
	principalKindMax         = 100

	principalOriginLearned   = "learned"
	principalOriginInterview = "interview"
	principalOriginEdited    = "edited"
)

func init() {
	governanceKinds[GovernanceKindPrincipalEnabled] = true
	governanceKinds[GovernanceKindPrincipalDeleted] = true
}

var (
	// ErrPrincipalDisabled: durable_principal is off, so the write is not made.
	ErrPrincipalDisabled = errors.New("the principal profile is not enabled on this server")
	// ErrPrincipalConsentRequired: a profile is kept only after the person consents to the text shown.
	ErrPrincipalConsentRequired = errors.New("the person must consent before a principal profile is kept")
	// ErrPrincipalNoProfile: the person has no profile.
	ErrPrincipalNoProfile = errors.New("the person has no principal profile")
	// ErrPrincipalForbidden: the actor may not change this profile.
	ErrPrincipalForbidden = errors.New("this actor may not change this principal profile")
	// ErrPrincipalEvidenceInvalid: a trait cites something the person did not write or decide.
	ErrPrincipalEvidenceInvalid = errors.New("a trait must cite the person's own decisions, comments or interview answers")
	// ErrPrincipalTraitInvalid: a trait breaks a rule of its statement, kind or origin.
	ErrPrincipalTraitInvalid = errors.New("invalid principal trait")
	// ErrPrincipalTraitLimit: the person already holds the most active traits.
	ErrPrincipalTraitLimit = errors.New("a person holds at most 200 active traits; retire one first")
	// ErrPrincipalTraitEdited: a job tried to retire a trait the person wrote or edited.
	ErrPrincipalTraitEdited = errors.New("an edited trait is retired only by its person")
	// ErrPrincipalTraitNotFound: no active trait with that id on this profile.
	ErrPrincipalTraitNotFound = errors.New("principal trait not found")
)

// principalInterviewQuestions are the eight fixed questions of the start-up interview (spec 6.3).
var principalInterviewQuestions = [...]string{
	"Khi phải chọn giữa nhanh và kỹ, bạn thường chọn bên nào, và với loại việc nào?",
	"Chi phí (tiền, giờ, rủi ro) tới mức nào thì bạn muốn tự quyết, không cần hỏi?",
	"Có điều gì agent không bao giờ được làm, dù nghe có vẻ hợp lý?",
	"Khi viết cho khách hàng, bạn muốn giọng văn thế nào?",
	"Khi nào bạn muốn được hỏi trước, dù việc đó nhỏ?",
	"Loại quyết định nào bạn muốn giao hẳn cho agent khi nó đã làm đúng nhiều lần?",
	"Loại việc nào bạn luôn muốn xem lại sau khi đã duyệt?",
	"Khi bạn từ chối một đề xuất, lý do thường là gì?",
}

// InterviewQuestions returns the eight questions of the interview, in order.
func InterviewQuestions() []string {
	out := make([]string, len(principalInterviewQuestions))
	copy(out, principalInterviewQuestions[:])
	return out
}

// PrincipalActor is who makes a change: a member (a person) or an agent.
type PrincipalActor struct {
	Type string // GovernanceActorMember or GovernanceActorAgent
	ID   pgtype.UUID
}

// PrincipalEvidence is one ref a trait rests on. Type is decision (a proposal the person decided),
// comment (a comment the person wrote) or interview (a chat message the person sent).
type PrincipalEvidence struct {
	Type string `json:"type"`
	Ref  string `json:"ref"`
}

// TraitInput is one trait to add to a person's profile.
type TraitInput struct {
	WorkspaceID  pgtype.UUID
	MemberID     pgtype.UUID
	DecisionKind string
	Statement    string
	Origin       string
	Evidence     []PrincipalEvidence
}

// PrincipalService keeps the profiles. Flags is read on each write, so the flag can change while the
// service runs.
type PrincipalService struct {
	Queries   *db.Queries
	TxStarter TxStarter
	Flags     *featureflag.Service
}

func (s *PrincipalService) on(ctx context.Context) bool {
	return featureflags.DurablePrincipalEnabled(ctx, s.Flags)
}

// Enable turns the person's profile on after consent. consentText is the text the person agreed
// to; its hash is kept in the governance log, so the consent can be shown later without the text.
// Enabling again keeps the profile and moves its version on.
func (s *PrincipalService) Enable(ctx context.Context, ws, member pgtype.UUID, consentText string, share bool) (db.PrincipalProfile, error) {
	if !s.on(ctx) {
		return db.PrincipalProfile{}, ErrPrincipalDisabled
	}
	if strings.TrimSpace(consentText) == "" {
		return db.PrincipalProfile{}, ErrPrincipalConsentRequired
	}
	profile, err := s.Queries.UpsertPrincipalProfile(ctx, db.UpsertPrincipalProfileParams{
		WorkspaceID: ws, MemberID: member, ShareWithOwners: share,
	})
	if err != nil {
		return db.PrincipalProfile{}, fmt.Errorf("enable principal profile: %w", err)
	}
	sum := sha256.Sum256([]byte(consentText))
	if err := NewGovernanceLog().Record(ctx, s.Queries, GovernanceEvent{
		WorkspaceID: ws, Kind: GovernanceKindPrincipalEnabled,
		ActorType: GovernanceActorMember, ActorID: member,
		SubjectType: "principal_profile", SubjectID: member,
		Summary: "A person turned on their principal profile",
		Details: map[string]any{"consent_sha256": hex.EncodeToString(sum[:]), "share_with_owners": share},
	}); err != nil {
		return db.PrincipalProfile{}, fmt.Errorf("log principal consent: %w", err)
	}
	return profile, nil
}

// Profile reads the person's profile and its active traits. It does not decide who may read it;
// the caller checks PrincipalCanRead first.
func (s *PrincipalService) Profile(ctx context.Context, ws, member pgtype.UUID) (db.PrincipalProfile, []db.PrincipalTrait, error) {
	profile, err := s.Queries.GetPrincipalProfile(ctx, db.GetPrincipalProfileParams{WorkspaceID: ws, MemberID: member})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PrincipalProfile{}, nil, ErrPrincipalNoProfile
	}
	if err != nil {
		return db.PrincipalProfile{}, nil, fmt.Errorf("load principal profile: %w", err)
	}
	traits, err := s.Queries.ListActivePrincipalTraits(ctx, db.ListActivePrincipalTraitsParams{WorkspaceID: ws, MemberID: member})
	if err != nil {
		return db.PrincipalProfile{}, nil, fmt.Errorf("load principal traits: %w", err)
	}
	return profile, traits, nil
}

// AddTrait adds a trait to a profile. The person writes interview and edited traits about
// themselves; the Secretary, while it learns, writes learned traits. Each trait must cite the
// person's own decisions, comments or chat messages (spec 6.3), and a profile holds at most 200.
func (s *PrincipalService) AddTrait(ctx context.Context, actor PrincipalActor, in TraitInput) (db.PrincipalTrait, error) {
	if !s.on(ctx) {
		return db.PrincipalTrait{}, ErrPrincipalDisabled
	}
	if err := s.authorTrait(ctx, actor, in); err != nil {
		return db.PrincipalTrait{}, err
	}
	statement := strings.TrimSpace(in.Statement)
	if n := utf8.RuneCountInString(statement); n == 0 || n > principalStatementMax || !validPrincipalKind(in.DecisionKind) {
		return db.PrincipalTrait{}, ErrPrincipalTraitInvalid
	}
	if _, err := s.Queries.GetPrincipalProfile(ctx, db.GetPrincipalProfileParams{WorkspaceID: in.WorkspaceID, MemberID: in.MemberID}); errors.Is(err, pgx.ErrNoRows) {
		return db.PrincipalTrait{}, ErrPrincipalNoProfile
	} else if err != nil {
		return db.PrincipalTrait{}, fmt.Errorf("load principal profile: %w", err)
	}
	evidence, err := s.checkEvidence(ctx, in.WorkspaceID, in.MemberID, in.Evidence)
	if err != nil {
		return db.PrincipalTrait{}, err
	}
	active, err := s.Queries.CountActivePrincipalTraits(ctx, db.CountActivePrincipalTraitsParams{WorkspaceID: in.WorkspaceID, MemberID: in.MemberID})
	if err != nil {
		return db.PrincipalTrait{}, fmt.Errorf("count principal traits: %w", err)
	}
	if active >= principalMaxActiveTraits {
		return db.PrincipalTrait{}, ErrPrincipalTraitLimit
	}
	trait, err := s.Queries.InsertPrincipalTrait(ctx, db.InsertPrincipalTraitParams{
		ID: dbid.NewV7(), WorkspaceID: in.WorkspaceID, MemberID: in.MemberID,
		DecisionKind: in.DecisionKind, Statement: statement, Evidence: evidence, Origin: in.Origin,
	})
	if err != nil {
		return db.PrincipalTrait{}, fmt.Errorf("insert principal trait: %w", err)
	}
	if _, err := s.Queries.BumpPrincipalProfileVersion(ctx, db.BumpPrincipalProfileVersionParams{WorkspaceID: in.WorkspaceID, MemberID: in.MemberID}); err != nil {
		return db.PrincipalTrait{}, fmt.Errorf("bump principal profile: %w", err)
	}
	return trait, nil
}

// EditTrait is the person's own edit of a trait's sentence. The trait becomes edited, so no job
// rewrites or retires it afterwards (spec 6.3).
func (s *PrincipalService) EditTrait(ctx context.Context, actor PrincipalActor, ws, traitID pgtype.UUID, statement string) (db.PrincipalTrait, error) {
	if !s.on(ctx) {
		return db.PrincipalTrait{}, ErrPrincipalDisabled
	}
	if actor.Type != GovernanceActorMember {
		return db.PrincipalTrait{}, ErrPrincipalForbidden
	}
	statement = strings.TrimSpace(statement)
	if n := utf8.RuneCountInString(statement); n == 0 || n > principalStatementMax {
		return db.PrincipalTrait{}, ErrPrincipalTraitInvalid
	}
	trait, err := s.Queries.EditPrincipalTraitStatement(ctx, db.EditPrincipalTraitStatementParams{
		ID: traitID, WorkspaceID: ws, MemberID: actor.ID, Statement: statement,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PrincipalTrait{}, ErrPrincipalTraitNotFound
	}
	if err != nil {
		return db.PrincipalTrait{}, fmt.Errorf("edit principal trait: %w", err)
	}
	if _, err := s.Queries.BumpPrincipalProfileVersion(ctx, db.BumpPrincipalProfileVersionParams{WorkspaceID: ws, MemberID: actor.ID}); err != nil {
		return db.PrincipalTrait{}, fmt.Errorf("bump principal profile: %w", err)
	}
	return trait, nil
}

// RetireTrait takes an active trait out of the profile; the row stays as history (spec 6.3). The
// person may retire any of their traits. The Secretary may retire learned traits only: an edited
// trait belongs to the person.
func (s *PrincipalService) RetireTrait(ctx context.Context, actor PrincipalActor, ws, member, traitID pgtype.UUID) error {
	if !s.on(ctx) {
		return ErrPrincipalDisabled
	}
	trait, err := s.Queries.GetPrincipalTrait(ctx, db.GetPrincipalTraitParams{ID: traitID, WorkspaceID: ws, MemberID: member})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPrincipalTraitNotFound
	}
	if err != nil {
		return fmt.Errorf("load principal trait: %w", err)
	}
	switch {
	case actor.Type == GovernanceActorMember && principalSameUUID(actor.ID, member):
	case actor.Type == GovernanceActorAgent && s.isSecretary(ctx, ws, actor.ID):
		if trait.Origin == principalOriginEdited {
			return ErrPrincipalTraitEdited
		}
	default:
		return ErrPrincipalForbidden
	}
	if trait.Status != "active" {
		return ErrPrincipalTraitNotFound
	}
	n, err := s.Queries.RetirePrincipalTrait(ctx, db.RetirePrincipalTraitParams{ID: traitID, WorkspaceID: ws, MemberID: member})
	if err != nil {
		return fmt.Errorf("retire principal trait: %w", err)
	}
	if n == 0 {
		return ErrPrincipalTraitNotFound
	}
	if _, err := s.Queries.BumpPrincipalProfileVersion(ctx, db.BumpPrincipalProfileVersionParams{WorkspaceID: ws, MemberID: member}); err != nil {
		return fmt.Errorf("bump principal profile: %w", err)
	}
	return nil
}

// AnswerInterview records one answer of the start-up interview as an interview trait, whose
// evidence is the chat message the person sent with the answer (spec 6.3). question is the index
// of the question, 0 to 7.
func (s *PrincipalService) AnswerInterview(ctx context.Context, ws, member pgtype.UUID, question int, answer string, messageID pgtype.UUID) (db.PrincipalTrait, error) {
	if question < 0 || question >= len(principalInterviewQuestions) {
		return db.PrincipalTrait{}, ErrPrincipalTraitInvalid
	}
	return s.AddTrait(ctx, PrincipalActor{Type: GovernanceActorMember, ID: member}, TraitInput{
		WorkspaceID:  ws,
		MemberID:     member,
		DecisionKind: "general",
		Statement:    answer,
		Origin:       principalOriginInterview,
		Evidence:     []PrincipalEvidence{{Type: "interview", Ref: util.UUIDToString(messageID)}},
	})
}

// Delete removes the person's profile and its traits in one transaction (spec 6.6). Only the person
// may delete it, and it works with the flag off. The governance entry carries no content. A profile
// that does not exist removes nothing and is not an error.
func (s *PrincipalService) Delete(ctx context.Context, actor PrincipalActor, ws, member pgtype.UUID) error {
	if actor.Type != GovernanceActorMember || !principalSameUUID(actor.ID, member) {
		return ErrPrincipalForbidden
	}
	return s.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.DeletePrincipalTraits(ctx, db.DeletePrincipalTraitsParams{WorkspaceID: ws, MemberID: member}); err != nil {
			return fmt.Errorf("delete principal traits: %w", err)
		}
		// The predictions keep their match counts; what they said goes (spec 6.6).
		if _, err := q.ForgetPredictionReasons(ctx, db.ForgetPredictionReasonsParams{WorkspaceID: ws, MemberID: member}); err != nil {
			return fmt.Errorf("forget prediction reasons: %w", err)
		}
		n, err := q.DeletePrincipalProfile(ctx, db.DeletePrincipalProfileParams{WorkspaceID: ws, MemberID: member})
		if err != nil {
			return fmt.Errorf("delete principal profile: %w", err)
		}
		if n == 0 {
			return nil
		}
		return NewGovernanceLog().Record(ctx, q, GovernanceEvent{
			WorkspaceID: ws, Kind: GovernanceKindPrincipalDeleted,
			ActorType: GovernanceActorMember, ActorID: member,
			SubjectType: "principal_profile", SubjectID: member,
			Summary: "A person deleted their principal profile",
		})
	})
}

// PrincipalCanRead is who may read a profile (spec 6.6): the person; the workspace Secretary; and an
// owner, only when the person shared the profile with owners. Every other agent is refused.
func PrincipalCanRead(actor PrincipalActor, member, secretary pgtype.UUID, isOwner, shareWithOwners bool) bool {
	switch actor.Type {
	case GovernanceActorMember:
		if principalSameUUID(actor.ID, member) {
			return true
		}
		return isOwner && shareWithOwners
	case GovernanceActorAgent:
		return principalSameUUID(actor.ID, secretary)
	}
	return false
}

// authorTrait checks who may add the trait and with which origin.
func (s *PrincipalService) authorTrait(ctx context.Context, actor PrincipalActor, in TraitInput) error {
	switch {
	case actor.Type == GovernanceActorMember && principalSameUUID(actor.ID, in.MemberID):
		if in.Origin != principalOriginInterview && in.Origin != principalOriginEdited {
			return ErrPrincipalTraitInvalid
		}
		return nil
	case actor.Type == GovernanceActorAgent && s.isSecretary(ctx, in.WorkspaceID, actor.ID):
		if in.Origin != principalOriginLearned {
			return ErrPrincipalTraitInvalid
		}
		return nil
	}
	return ErrPrincipalForbidden
}

// isSecretary reports whether the agent is the workspace's Secretary. A lookup error counts as no.
func (s *PrincipalService) isSecretary(ctx context.Context, ws, agent pgtype.UUID) bool {
	secretary, err := s.Queries.GetWorkspaceSecretaryAgentID(ctx, ws)
	return err == nil && principalSameUUID(secretary, agent)
}

// checkEvidence proves every ref is one the person wrote or decided in this workspace, and returns
// the evidence as stored. A ref that is not a UUID, or of another person, is refused.
func (s *PrincipalService) checkEvidence(ctx context.Context, ws, member pgtype.UUID, ev []PrincipalEvidence) ([]byte, error) {
	if len(ev) == 0 || len(ev) > principalEvidenceMax {
		return nil, ErrPrincipalEvidenceInvalid
	}
	for _, e := range ev {
		ref, err := util.ParseUUID(e.Ref)
		if err != nil {
			return nil, ErrPrincipalEvidenceInvalid
		}
		var owned bool
		switch e.Type {
		case "decision":
			owned, err = s.Queries.IsOwnProposalDecision(ctx, db.IsOwnProposalDecisionParams{ID: ref, WorkspaceID: ws, MemberID: member})
		case "comment":
			owned, err = s.Queries.IsOwnComment(ctx, db.IsOwnCommentParams{ID: ref, WorkspaceID: ws, MemberID: member})
		case "interview":
			owned, err = s.Queries.IsOwnChatMessage(ctx, db.IsOwnChatMessageParams{ID: ref, WorkspaceID: ws, MemberID: member})
		default:
			return nil, ErrPrincipalEvidenceInvalid
		}
		if err != nil {
			return nil, fmt.Errorf("check principal evidence: %w", err)
		}
		if !owned {
			return nil, ErrPrincipalEvidenceInvalid
		}
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("encode principal evidence: %w", err)
	}
	return raw, nil
}

func (s *PrincipalService) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin principal tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(s.Queries.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// validPrincipalKind accepts the decision kinds of spec 6.2: general, outcome, inquiry, and
// proposal:<kind> or effect:<class> with a name.
func validPrincipalKind(kind string) bool {
	switch kind {
	case "general", "outcome", "inquiry":
		return true
	}
	for _, prefix := range []string{"proposal:", "effect:"} {
		if strings.HasPrefix(kind, prefix) {
			return len(kind) > len(prefix) && len(kind) <= principalKindMax
		}
	}
	return false
}

// principalSameUUID compares two ids, and is false when either is missing.
func principalSameUUID(a, b pgtype.UUID) bool {
	return a.Valid && b.Valid && a.Bytes == b.Bytes
}
