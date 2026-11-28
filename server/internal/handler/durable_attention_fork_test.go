package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// TestAttentionSecondsFromHeader: the decision handlers measure the time from opening a question
// to deciding from X-Attention-Opened-At (seconds or RFC 3339, capped at 30 minutes). A missing or
// unparseable header leaves the decision unmeasured (durable spec 7.2).
func TestAttentionSecondsFromHeader(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	cases := []struct {
		name  string
		value string
		want  *int
	}{
		{"absent", "", nil},
		{"unparseable", "yesterday", nil},
		{"negative seconds", "-5", nil},
		{"seconds", "90", intPtr(90)},
		{"seconds capped", "9999", intPtr(1800)},
		{"rfc3339", now.Add(-2 * time.Minute).Format(time.RFC3339), intPtr(120)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			if c.value != "" {
				r.Header.Set(attentionOpenedHeader, c.value)
			}
			got := attentionSecondsOf(r)
			switch {
			case c.want == nil && got != nil:
				t.Fatalf("seconds = %d, want unmeasured", *got)
			case c.want != nil && (got == nil || *got != *c.want):
				t.Fatalf("seconds = %v, want %d", got, *c.want)
			}
		})
	}
}
