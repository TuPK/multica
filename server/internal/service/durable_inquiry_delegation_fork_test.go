package service

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// askInquiry puts a question from the Secretary to the squad leader through the inquiry path: the
// brief inquiry is asked, then admitted to attention, which requests the boss's predictions.
func (d decideEnv) askInquiry(t *testing.T) db.BriefInquiry {
	t.Helper()
	ctx := context.Background()
	squad := d.squad("Inquiry "+util.UUIDToString(dbid.NewV7()), AutonomyLevelWork)
	issue := d.fx.Issue(t, "Ship the research note?")
	res, err := AskBriefInquiry(ctx, sharedTestPool(t), d.q, BriefInquiryAskParams{
		WorkspaceID: d.ws, ActorID: d.agent, SquadID: squad, IssueID: mustUUID(t, issue),
		Question: "Ship the research note today?", Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("ask inquiry: %v", err)
	}
	attention := &AttentionService{Queries: d.q, Flags: flagsWith(true)}
	if _, err := attention.Admit(ctx, AskInput{
		WorkspaceID: d.ws, SquadID: squad, SubjectType: "inquiry", SubjectID: res.Inquiry.ID,
		Reason: "judgment", WhyNotDecide: "Only the boss can say whether to ship.", ByAgent: true,
	}); err != nil {
		t.Fatalf("admit inquiry: %v", err)
	}
	return res.Inquiry
}

// predictInquiry has the Secretary record the boss's predicted answer to an inquiry.
func (d decideEnv) predictInquiry(t *testing.T, inquiry pgtype.UUID, answer string, confidence float64) db.DecisionPrediction {
	t.Helper()
	pred, err := d.props.Principal.Record(context.Background(), PrincipalActor{Type: GovernanceActorAgent, ID: d.agent}, PredictionInput{
		WorkspaceID: d.ws, MemberID: d.member, SubjectType: "inquiry", SubjectID: inquiry,
		DecisionKind: "inquiry", Predicted: answer, Confidence: confidence,
	})
	if err != nil {
		t.Fatalf("record inquiry prediction: %v", err)
	}
	return pred
}

// newInquiryEnv is decideEnv where the boss has 25 resolved inquiry predictions that all matched,
// and has delegated inquiries when delegate is set.
func newInquiryEnv(t *testing.T, delegate bool) decideEnv {
	t.Helper()
	d := newDecideEnv(t)
	if !delegate {
		return d
	}
	d.seedResolved(t, "inquiry", 1, trueN(25))
	if err := d.props.Principal.delegation().Enable(context.Background(), PrincipalActor{Type: GovernanceActorMember, ID: d.member}, d.ws, d.member, "inquiry", 0.9); err != nil {
		t.Fatalf("enable inquiry delegation: %v", err)
	}
	return d
}

// inquiryAnswered reports whether the inquiry holds the given answer excerpt.
func (d decideEnv) inquiryAnswered(t *testing.T, inquiry pgtype.UUID, answer string) bool {
	t.Helper()
	return d.fx.Count(t, `SELECT COUNT(*) FROM brief_inquiry WHERE id = $1 AND answered_at IS NOT NULL AND answer_excerpt = $2`,
		util.UUIDToString(inquiry), answer) == 1
}

// A predicted answer that is eligible answers the inquiry as the boss's own decision, is logged with
// the boss as the actor, and can be undone for 72 hours, which reopens the inquiry for the leader.
func TestInquiryDelegatedAnswerAsPrincipal(t *testing.T) {
	d := newInquiryEnv(t, true)
	ctx := context.Background()
	inquiry := d.askInquiry(t)
	const answer = "Yes, ship it today."
	pred := d.predictInquiry(t, inquiry.ID, answer, 0.95)

	if !d.inquiryAnswered(t, inquiry.ID, answer) {
		t.Fatalf("inquiry not answered with the delegated prediction")
	}
	if n := d.fx.Count(t, `SELECT COUNT(*) FROM decision_prediction WHERE id = $1 AND applied_by_delegation = true AND matched = true AND resolved_at IS NOT NULL`, pred.ID); n != 1 {
		t.Fatalf("prediction not marked applied by delegation and resolved")
	}
	if n := d.fx.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'principal_delegated_decision' AND actor_type = 'member' AND actor_id = $2 AND subject_id = $3`,
		d.wsID, d.member, pred.ID); n != 1 {
		t.Fatalf("%d delegated-decision events for the principal, want 1", n)
	}
	if err := d.props.Principal.delegation().OnReverted(ctx, d.ws, PrincipalActor{Type: GovernanceActorMember, ID: d.member}, pred.ID); err != nil {
		t.Fatalf("revert inside 72 hours: %v", err)
	}
	if d.inquiryAnswered(t, inquiry.ID, answer) {
		t.Fatalf("inquiry still answered after the revert; it must reopen for the squad leader")
	}
	if n := d.fx.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'principal_delegation_reverted' AND subject_id = $2`, d.wsID, pred.ID); n != 1 {
		t.Fatalf("%d revert events, want 1", n)
	}
}

// Without an eligible predicted answer the inquiry stays open for its squad leader, and no principal
// actor is recorded: no prediction at all, a prediction under the confidence the boss set, a kind the
// boss never delegated.
func TestInquiryWithoutEligiblePredictionStaysWithLeader(t *testing.T) {
	noEvent := func(t *testing.T, d decideEnv) {
		t.Helper()
		if n := d.fx.Count(t, `SELECT COUNT(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'principal_delegated_decision'`, d.wsID); n != 0 {
			t.Fatalf("%d delegated-decision events, want none", n)
		}
	}

	t.Run("no prediction", func(t *testing.T) {
		d := newInquiryEnv(t, true)
		inquiry := d.askInquiry(t)
		if n := d.fx.Count(t, `SELECT COUNT(*) FROM brief_inquiry WHERE id = $1 AND answered_at IS NULL`, util.UUIDToString(inquiry.ID)); n != 1 {
			t.Fatalf("inquiry answered without a prediction")
		}
		noEvent(t, d)
	})

	t.Run("prediction under the confidence", func(t *testing.T) {
		d := newInquiryEnv(t, true)
		inquiry := d.askInquiry(t)
		d.predictInquiry(t, inquiry.ID, "Yes, ship it today.", 0.5)
		if n := d.fx.Count(t, `SELECT COUNT(*) FROM brief_inquiry WHERE id = $1 AND answered_at IS NULL`, util.UUIDToString(inquiry.ID)); n != 1 {
			t.Fatalf("inquiry answered by a prediction under the boss's confidence")
		}
		noEvent(t, d)
	})

	t.Run("kind not delegated", func(t *testing.T) {
		d := newInquiryEnv(t, false)
		inquiry := d.askInquiry(t)
		d.predictInquiry(t, inquiry.ID, "Yes, ship it today.", 0.95)
		if n := d.fx.Count(t, `SELECT COUNT(*) FROM brief_inquiry WHERE id = $1 AND answered_at IS NULL`, util.UUIDToString(inquiry.ID)); n != 1 {
			t.Fatalf("inquiry answered although the boss did not delegate inquiries")
		}
		noEvent(t, d)
	})
}
