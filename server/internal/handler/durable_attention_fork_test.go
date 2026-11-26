package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// TestAskBriefInquiryAgentReason: with durable_principal on, the Secretary's question must
// say why it is asked (spec 7.4) and, for missing_info, what it searched. With the flag off
// the same body is accepted as it was before.
func TestAskBriefInquiryAgentReason(t *testing.T) {
	e := newInquiryEnv(t)
	flags := func(on bool) *featureflag.Service {
		p := featureflag.NewStaticProvider()
		p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: on})
		return featureflag.NewService(p)
	}
	previous := testHandler.FeatureFlags
	t.Cleanup(func() { testHandler.FeatureFlags = previous })
	testHandler.FeatureFlags = flags(true)

	withFields := func(fields map[string]any) map[string]any {
		body := e.askBody("Who owns the risk?")
		for k, v := range fields {
			body[k] = v
		}
		return body
	}
	wantOutcomeCode(t, e.ask(t, e.Secretary, withFields(nil)), http.StatusUnprocessableEntity, "attention_reason_required")
	wantOutcomeCode(t, e.ask(t, e.Secretary, withFields(map[string]any{"reason": "judgment"})), http.StatusUnprocessableEntity, "attention_reason_required")
	wantOutcomeCode(t, e.ask(t, e.Secretary, withFields(map[string]any{"reason": "missing_info", "why_not_decide": "no spec"})),
		http.StatusUnprocessableEntity, "attention_searched_required")
	e.ask(t, e.Secretary, withFields(map[string]any{"reason": "judgment", "why_not_decide": "the boss owns this"})).Want(http.StatusCreated)

	testHandler.FeatureFlags = flags(false)
	e.ask(t, e.Secretary, withFields(nil)).Want(http.StatusCreated)
}

// TestProposalAgentReasonRequired: a proposal an agent files needs its reason while
// durable_principal is on. The refusal happens before anything is written.
func TestProposalAgentReasonRequired(t *testing.T) {
	fx := newProposalTestFixture(t)
	p := featureflag.NewStaticProvider()
	p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: true})
	previous := testHandler.FeatureFlags
	t.Cleanup(func() { testHandler.FeatureFlags = previous })
	testHandler.FeatureFlags = featureflag.NewService(p)

	file := func(name string, over map[string]any) *testutil.Response {
		req := asAgent(newRequest("POST", "/api/proposals", proposalBody(fx.SquadID, fx.title(name), over)), fx.LeaderID, fx.LeaderTaskID)
		return testutil.Call(t, testHandler.CreateProposal, req)
	}
	wantOutcomeCode(t, file("no reason", nil), http.StatusUnprocessableEntity, "attention_reason_required")
	file("with reason", map[string]any{"reason": "judgment", "why_not_decide": "the boss owns this call"}).Want(http.StatusCreated)
}
