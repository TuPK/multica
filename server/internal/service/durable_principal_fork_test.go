package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// principalTest is a company workspace with an owner (the fixture's user), a second member,
// the Secretary agent, and the principal service. durable_principal is set as the test needs it.
type principalTest struct {
	principalFixture
	ws        pgtype.UUID
	owner     PrincipalActor
	other     PrincipalActor
	secretary PrincipalActor
	svc       *PrincipalService
}

func newPrincipalTest(t *testing.T, flagOn bool) principalTest {
	t.Helper()
	f, _ := companyWorkspace(t)
	otherUser := f.User(t, "principal other", testutil.UniqueEmail("principal-other"))
	f.Member(t, f.WorkspaceID, otherUser, "member")
	runtime := f.Runtime(t, "principal runtime", testutil.Cols{"provider": "anthropic"})
	secretary := f.Agent(t, "principal secretary", runtime)
	f.Exec(t, `UPDATE workspace SET secretary_agent_id = $1 WHERE id = $2`, secretary, f.WorkspaceID)

	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: flagOn})
	ws := util.MustParseUUID(f.WorkspaceID)
	pt := principalTest{
		principalFixture: f,
		ws:               ws,
		owner:            PrincipalActor{Type: GovernanceActorMember, ID: util.MustParseUUID(f.UserID)},
		other:            PrincipalActor{Type: GovernanceActorMember, ID: util.MustParseUUID(otherUser)},
		secretary:        PrincipalActor{Type: GovernanceActorAgent, ID: util.MustParseUUID(secretary)},
		svc:              &PrincipalService{Queries: f.q, TxStarter: f.Pool, Flags: featureflag.NewService(provider)},
	}
	f.Cleanup(t, `DELETE FROM principal_delegation WHERE workspace_id = $1`, f.WorkspaceID)
	f.Cleanup(t, `DELETE FROM decision_prediction WHERE workspace_id = $1`, f.WorkspaceID)
	f.Cleanup(t, `DELETE FROM chat_session WHERE workspace_id = $1`, f.WorkspaceID)
	f.Cleanup(t, `DELETE FROM principal_trait WHERE workspace_id = $1`, f.WorkspaceID)
	f.Cleanup(t, `DELETE FROM principal_profile WHERE workspace_id = $1`, f.WorkspaceID)
	f.Cleanup(t, `DELETE FROM governance_event WHERE workspace_id = $1`, f.WorkspaceID)
	return pt
}

// enable turns the owner's profile on with consent, the way the person does.
func (pt principalTest) enable(t *testing.T) {
	t.Helper()
	if _, err := pt.svc.Enable(context.Background(), pt.ws, pt.owner.ID, "Tôi đồng ý", false); err != nil {
		t.Fatalf("enable: %v", err)
	}
}

// comment is a comment the given member wrote on a fresh issue of the workspace.
func (pt principalTest) comment(t *testing.T, author PrincipalActor) string {
	t.Helper()
	issue := pt.Issue(t, "principal evidence issue")
	return pt.Comment(t, issue, "a decision in words", testutil.Cols{
		"author_type": "member", "author_id": util.UUIDToString(author.ID),
	})
}

// evidenceOf cites one ref, the way a trait's evidence is written.
func evidenceOf(refType, ref string) []PrincipalEvidence {
	return []PrincipalEvidence{{Type: refType, Ref: ref}}
}

// chatMessage is a message a member sent in a chat session the member created.
func (pt principalTest) chatMessage(t *testing.T, creator PrincipalActor) string {
	t.Helper()
	session := pt.Insert(t, "chat_session", testutil.Cols{
		"workspace_id": pt.WorkspaceID, "agent_id": util.UUIDToString(pt.secretary.ID),
		"creator_id": util.UUIDToString(creator.ID),
	})
	return pt.Insert(t, "chat_message", testutil.Cols{
		"chat_session_id": session, "role": "user", "content": "an answer",
	})
}

func TestPrincipalFlagOffWritesNothing(t *testing.T) {
	pt := newPrincipalTest(t, false)
	ctx := context.Background()
	if _, err := pt.svc.Enable(ctx, pt.ws, pt.owner.ID, "Tôi đồng ý", false); !errors.Is(err, ErrPrincipalDisabled) {
		t.Fatalf("enable with the flag off: err = %v, want ErrPrincipalDisabled", err)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM principal_profile WHERE workspace_id = $1`, pt.WorkspaceID); n != 0 {
		t.Fatalf("%d profile rows written with the flag off", n)
	}
}

func TestEnableRequiresConsentAndLogsIt(t *testing.T) {
	pt := newPrincipalTest(t, true)
	ctx := context.Background()
	if _, err := pt.svc.Enable(ctx, pt.ws, pt.owner.ID, "   ", false); !errors.Is(err, ErrPrincipalConsentRequired) {
		t.Fatalf("blank consent: err = %v, want ErrPrincipalConsentRequired", err)
	}
	pt.enable(t)
	profile, _, err := pt.svc.Profile(ctx, pt.ws, pt.owner.ID)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	if profile.Status != "active" || profile.Version != 1 {
		t.Fatalf("profile = %s v%d, want active v1", profile.Status, profile.Version)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'principal_profile_enabled'`, pt.WorkspaceID); n != 1 {
		t.Fatalf("%d enable events logged, want 1", n)
	}
}

func TestTraitRequiresOwnEvidence(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	in := func(ev []PrincipalEvidence) TraitInput {
		return TraitInput{
			WorkspaceID: pt.ws, MemberID: pt.owner.ID, DecisionKind: "general",
			Statement: "Chọn nhanh khi việc nhỏ", Origin: "edited", Evidence: ev,
		}
	}

	cases := []struct {
		name string
		ev   []PrincipalEvidence
	}{
		{"no evidence", nil},
		{"malformed ref", evidenceOf("comment", "not-a-uuid")},
		{"unknown evidence type", evidenceOf("email", "00000000-0000-0000-0000-000000000001")},
		{"comment of another person", evidenceOf("comment", pt.comment(t, pt.other))},
		{"chat message of another person", evidenceOf("interview", pt.chatMessage(t, pt.other))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := pt.svc.AddTrait(ctx, pt.owner, in(c.ev))
			if !errors.Is(err, ErrPrincipalEvidenceInvalid) {
				t.Fatalf("err = %v, want ErrPrincipalEvidenceInvalid", err)
			}
		})
	}

	t.Run("the person's own comment is accepted", func(t *testing.T) {
		trait, err := pt.svc.AddTrait(ctx, pt.owner, in(evidenceOf("comment", pt.comment(t, pt.owner))))
		if err != nil {
			t.Fatalf("add trait: %v", err)
		}
		if trait.Origin != "edited" || trait.Status != "active" {
			t.Fatalf("trait = %s/%s, want edited/active", trait.Origin, trait.Status)
		}
	})

	t.Run("the Secretary cannot cite another person's decision", func(t *testing.T) {
		input := in(evidenceOf("comment", pt.comment(t, pt.other)))
		input.Origin = "learned"
		if _, err := pt.svc.AddTrait(ctx, pt.secretary, input); !errors.Is(err, ErrPrincipalEvidenceInvalid) {
			t.Fatalf("err = %v, want ErrPrincipalEvidenceInvalid", err)
		}
	})
}

func TestTraitOriginAndAuthorFollowTheActor(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	ev := evidenceOf("comment", pt.comment(t, pt.owner))
	base := TraitInput{WorkspaceID: pt.ws, MemberID: pt.owner.ID, DecisionKind: "general", Statement: "một câu", Evidence: ev}

	t.Run("a person cannot write a learned trait", func(t *testing.T) {
		in := base
		in.Origin = "learned"
		if _, err := pt.svc.AddTrait(ctx, pt.owner, in); !errors.Is(err, ErrPrincipalTraitInvalid) {
			t.Fatalf("err = %v, want ErrPrincipalTraitInvalid", err)
		}
	})
	t.Run("the Secretary writes only learned traits", func(t *testing.T) {
		in := base
		in.Origin = "interview"
		if _, err := pt.svc.AddTrait(ctx, pt.secretary, in); !errors.Is(err, ErrPrincipalTraitInvalid) {
			t.Fatalf("err = %v, want ErrPrincipalTraitInvalid", err)
		}
	})
	t.Run("another member cannot add to a profile", func(t *testing.T) {
		in := base
		in.Origin = "edited"
		if _, err := pt.svc.AddTrait(ctx, pt.other, in); !errors.Is(err, ErrPrincipalForbidden) {
			t.Fatalf("err = %v, want ErrPrincipalForbidden", err)
		}
	})
	t.Run("a statement is 1 to 500 characters and the kind is known", func(t *testing.T) {
		for _, bad := range []TraitInput{
			func() TraitInput { in := base; in.Origin = "edited"; in.Statement = "   "; return in }(),
			func() TraitInput { in := base; in.Origin = "edited"; in.DecisionKind = "bogus"; return in }(),
			func() TraitInput { in := base; in.Origin = "edited"; in.DecisionKind = "proposal:"; return in }(),
		} {
			if _, err := pt.svc.AddTrait(ctx, pt.owner, bad); !errors.Is(err, ErrPrincipalTraitInvalid) {
				t.Fatalf("input %+v: err = %v, want ErrPrincipalTraitInvalid", bad, err)
			}
		}
		in := base
		in.Origin = "edited"
		in.DecisionKind = "proposal:budget"
		if _, err := pt.svc.AddTrait(ctx, pt.owner, in); err != nil {
			t.Fatalf("proposal:budget: %v", err)
		}
	})
}

func TestTraitLimitIs200Active(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	ev := evidenceOf("comment", pt.comment(t, pt.owner))
	evJSON, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}
	pt.Exec(t, `INSERT INTO principal_trait (workspace_id, member_id, decision_kind, statement, evidence, origin)
		SELECT $1, $2, 'general', 'a trait', $3::jsonb, 'learned' FROM generate_series(1, 200)`,
		pt.WorkspaceID, pt.owner.ID, string(evJSON))
	_, err = pt.svc.AddTrait(ctx, pt.secretary, TraitInput{
		WorkspaceID: pt.ws, MemberID: pt.owner.ID, DecisionKind: "general", Statement: "one more",
		Origin: "learned", Evidence: ev,
	})
	if !errors.Is(err, ErrPrincipalTraitLimit) {
		t.Fatalf("201st trait: err = %v, want ErrPrincipalTraitLimit", err)
	}
}

func TestEditedTraitNotRetiredByJob(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	ev := evidenceOf("comment", pt.comment(t, pt.owner))
	edited, err := pt.svc.AddTrait(ctx, pt.owner, TraitInput{
		WorkspaceID: pt.ws, MemberID: pt.owner.ID, DecisionKind: "general", Statement: "tự viết",
		Origin: "edited", Evidence: ev,
	})
	if err != nil {
		t.Fatalf("add edited trait: %v", err)
	}
	learned, err := pt.svc.AddTrait(ctx, pt.secretary, TraitInput{
		WorkspaceID: pt.ws, MemberID: pt.owner.ID, DecisionKind: "general", Statement: "học được",
		Origin: "learned", Evidence: ev,
	})
	if err != nil {
		t.Fatalf("add learned trait: %v", err)
	}
	editedID := util.UUIDToString(edited.ID)
	learnedID := util.UUIDToString(learned.ID)

	if err := pt.svc.RetireTrait(ctx, pt.secretary, pt.ws, pt.owner.ID, edited.ID); !errors.Is(err, ErrPrincipalTraitEdited) {
		t.Fatalf("job retiring an edited trait: err = %v, want ErrPrincipalTraitEdited", err)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM principal_trait WHERE id = $1 AND status = 'active'`, editedID); n != 1 {
		t.Fatalf("the edited trait was retired by the job")
	}
	if err := pt.svc.RetireTrait(ctx, pt.secretary, pt.ws, pt.owner.ID, learned.ID); err != nil {
		t.Fatalf("job retiring a learned trait: %v", err)
	}
	if err := pt.svc.RetireTrait(ctx, pt.other, pt.ws, pt.owner.ID, edited.ID); !errors.Is(err, ErrPrincipalForbidden) {
		t.Fatalf("another member retiring: err = %v, want ErrPrincipalForbidden", err)
	}
	if err := pt.svc.RetireTrait(ctx, pt.owner, pt.ws, pt.owner.ID, edited.ID); err != nil {
		t.Fatalf("the person retiring their own edited trait: %v", err)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM principal_trait WHERE id = $1 AND status = 'retired' AND retired_at IS NOT NULL`, editedID); n != 1 {
		t.Fatalf("retired trait not marked retired")
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM principal_trait WHERE id = $1 AND status = 'retired'`, learnedID); n != 1 {
		t.Fatalf("learned trait not retired by the job")
	}
}

func TestPersonEditsOwnTraitOnly(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	learned, err := pt.svc.AddTrait(ctx, pt.secretary, TraitInput{
		WorkspaceID: pt.ws, MemberID: pt.owner.ID, DecisionKind: "general", Statement: "học được",
		Origin: "learned", Evidence: evidenceOf("comment", pt.comment(t, pt.owner)),
	})
	if err != nil {
		t.Fatalf("add learned trait: %v", err)
	}
	if _, err := pt.svc.EditTrait(ctx, pt.secretary, pt.ws, learned.ID, "agent edit"); !errors.Is(err, ErrPrincipalForbidden) {
		t.Fatalf("agent editing: err = %v, want ErrPrincipalForbidden", err)
	}
	edited, err := pt.svc.EditTrait(ctx, pt.owner, pt.ws, learned.ID, "câu của tôi")
	if err != nil {
		t.Fatalf("person editing: %v", err)
	}
	if edited.Origin != "edited" || edited.Statement != "câu của tôi" {
		t.Fatalf("after edit: origin %q statement %q", edited.Origin, edited.Statement)
	}
}

func TestProfileReadAccess(t *testing.T) {
	owner := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	other := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	secretary := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	member := PrincipalActor{Type: GovernanceActorMember, ID: owner}
	cases := []struct {
		name  string
		actor PrincipalActor
		share bool
		owner bool
		want  bool
	}{
		{"the person reads their own profile", member, false, false, true},
		{"the Secretary reads any profile", PrincipalActor{Type: GovernanceActorAgent, ID: secretary}, false, false, true},
		{"another agent is refused", PrincipalActor{Type: GovernanceActorAgent, ID: other}, true, true, false},
		{"another member is refused", PrincipalActor{Type: GovernanceActorMember, ID: other}, false, false, false},
		{"an owner reads a shared profile", PrincipalActor{Type: GovernanceActorMember, ID: other}, true, true, true},
		{"an owner is refused an unshared profile", PrincipalActor{Type: GovernanceActorMember, ID: other}, false, true, false},
		{"a non-owner is refused a shared profile", PrincipalActor{Type: GovernanceActorMember, ID: other}, true, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PrincipalCanRead(c.actor, owner, secretary, c.owner, c.share); got != c.want {
				t.Fatalf("PrincipalCanRead = %v, want %v", got, c.want)
			}
		})
	}
}

func TestDeleteProfileRemovesItsRows(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := pt.svc.AddTrait(ctx, pt.owner, TraitInput{
			WorkspaceID: pt.ws, MemberID: pt.owner.ID, DecisionKind: "general", Statement: "một câu",
			Origin: "edited", Evidence: evidenceOf("comment", pt.comment(t, pt.owner)),
		}); err != nil {
			t.Fatalf("add trait: %v", err)
		}
	}
	if err := pt.svc.Delete(ctx, pt.other, pt.ws, pt.owner.ID); !errors.Is(err, ErrPrincipalForbidden) {
		t.Fatalf("another member deleting: err = %v, want ErrPrincipalForbidden", err)
	}
	if err := pt.svc.Delete(ctx, pt.owner, pt.ws, pt.owner.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM principal_profile WHERE workspace_id = $1`, pt.WorkspaceID); n != 0 {
		t.Errorf("%d profile rows left", n)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM principal_trait WHERE workspace_id = $1`, pt.WorkspaceID); n != 0 {
		t.Errorf("%d trait rows left", n)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'principal_profile_deleted' AND details = '{}'::jsonb`, pt.WorkspaceID); n != 1 {
		t.Errorf("%d content-free delete events, want 1", n)
	}
}

// A person can always remove their profile, even after the flag is turned off (spec 6.6).
func TestDeleteWorksWithTheFlagOff(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: false})
	pt.svc.Flags = featureflag.NewService(provider)
	if err := pt.svc.Delete(context.Background(), pt.owner, pt.ws, pt.owner.ID); err != nil {
		t.Fatalf("delete with the flag off: %v", err)
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM principal_profile WHERE workspace_id = $1`, pt.WorkspaceID); n != 0 {
		t.Fatalf("profile not removed with the flag off")
	}
}

func TestReviewJobsNeverReadProfile(t *testing.T) {
	// The jobs that score people, review work and change the org must not touch the profile
	// tables (spec 6.6). Their queries and services are scanned for the table names.
	root := filepath.Join("..", "..")
	patterns := []string{
		filepath.Join(root, "pkg", "db", "queries", "scorecard*.sql"),
		filepath.Join(root, "pkg", "db", "queries", "review*.sql"),
		filepath.Join(root, "pkg", "db", "queries", "org_change*.sql"),
		filepath.Join(root, "pkg", "db", "queries", "org*.sql"),
		filepath.Join(root, "internal", "service", "scorecard*.go"),
		filepath.Join(root, "internal", "service", "review*.go"),
		filepath.Join(root, "internal", "service", "org_change*.go"),
		filepath.Join(root, "internal", "service", "durable_scorecard*.go"),
		filepath.Join(root, "internal", "service", "scorecard_durable_fork.go"),
	}
	banned := []string{"principal_profile", "principal_trait", "principal_delegation", "decision_prediction"}
	scanned := 0
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			scanned++
			for _, table := range banned {
				if strings.Contains(string(raw), table) {
					t.Errorf("%s reads %s; scorecard, review and org change must not", file, table)
				}
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no scorecard, review or org change files found to scan")
	}
}

func TestInterviewAnswersBecomeTraitsWithTheirMessage(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	if n := len(InterviewQuestions()); n != 8 {
		t.Fatalf("interview has %d questions, want 8", n)
	}
	msg := pt.chatMessage(t, pt.owner)
	trait, err := pt.svc.AnswerInterview(ctx, pt.ws, pt.owner.ID, 0, "Chậm mà chắc", util.MustParseUUID(msg))
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if trait.Origin != "interview" || trait.DecisionKind != "general" {
		t.Fatalf("trait = %s/%s, want interview/general", trait.Origin, trait.DecisionKind)
	}
	var evidence []PrincipalEvidence
	if err := json.Unmarshal(trait.Evidence, &evidence); err != nil {
		t.Fatalf("evidence: %v", err)
	}
	if len(evidence) != 1 || evidence[0].Type != "interview" || evidence[0].Ref != util.UUIDToString(util.MustParseUUID(msg)) {
		t.Fatalf("evidence = %+v, want the answer message", evidence)
	}

	if _, err := pt.svc.AnswerInterview(ctx, pt.ws, pt.owner.ID, 1, "x", util.MustParseUUID(pt.chatMessage(t, pt.other))); !errors.Is(err, ErrPrincipalEvidenceInvalid) {
		t.Fatalf("another person's message: err = %v, want ErrPrincipalEvidenceInvalid", err)
	}
	if _, err := pt.svc.AnswerInterview(ctx, pt.ws, pt.owner.ID, 8, "x", util.MustParseUUID(msg)); !errors.Is(err, ErrPrincipalTraitInvalid) {
		t.Fatalf("question 8 of 8: err = %v, want ErrPrincipalTraitInvalid", err)
	}
}

// The interview runs in the Secretary's chat: the first consent asks question 1, and each answer
// asks the next one, eight in all (durable spec 6.3).
func TestInterviewKickoffInChat(t *testing.T) {
	pt := newPrincipalTest(t, true)
	pt.enable(t)
	ctx := context.Background()
	session := pt.interviewSession(t)
	if n := pt.Count(t, `SELECT COUNT(*) FROM chat_message WHERE chat_session_id = $1 AND role = 'assistant'`, session); n != 1 {
		t.Fatalf("after consent: %d questions asked, want 1", n)
	}
	questions := InterviewQuestions()
	if c := pt.Count(t, `SELECT COUNT(*) FROM chat_message WHERE chat_session_id = $1 AND role = 'assistant' AND content = $2`, session, questions[0]); c != 1 {
		t.Fatalf("the first question is not the first of the eight")
	}
	for q := 0; q < len(questions); q++ {
		msg := pt.Insert(t, "chat_message", testutil.Cols{"chat_session_id": session, "role": "user", "content": "câu trả lời"})
		if _, err := pt.svc.AnswerInterview(ctx, pt.ws, pt.owner.ID, q, "Một câu trả lời", util.MustParseUUID(msg)); err != nil {
			t.Fatalf("answer %d: %v", q, err)
		}
	}
	if n := pt.Count(t, `SELECT COUNT(*) FROM chat_message WHERE chat_session_id = $1 AND role = 'assistant'`, session); n != len(questions) {
		t.Fatalf("%d questions asked in all, want %d", n, len(questions))
	}
}

// interviewSession is the person's interview chat with the Secretary.
func (pt principalTest) interviewSession(t *testing.T) string {
	t.Helper()
	var id string
	pt.QueryRow(t, `SELECT id FROM chat_session WHERE workspace_id = $1 AND creator_id = $2 AND title = $3`,
		pt.WorkspaceID, util.UUIDToString(pt.owner.ID), principalInterviewTitle).Scan(&id)
	return id
}
