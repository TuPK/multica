package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
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

// TestAppRequestDecisionRecordsAttentionSeconds: approving or rejecting an app action request
// records actual_seconds in attention_ledger when X-Attention-Opened-At is sent, and leaves
// actual_seconds NULL when the header is absent (durable spec 7.2, plan C1).
func TestAppRequestDecisionRecordsAttentionSeconds(t *testing.T) {
	p := featureflag.NewStaticProvider()
	p.Set(featureflags.ComposioMCPApps, featureflag.Rule{Default: true})
	p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: true})
	flags := featureflag.NewService(p)
	att := &service.AttentionService{Queries: testHandler.Queries, Flags: flags}

	f := newAppsFixture(t, func(cfg *connectedapps.Config) {
		cfg.Attention = att
	})
	f.h.FeatureFlags = flags
	if w := f.putAgentApps(t, testUserID, []map[string]string{{"toolkit_slug": "gmail", "account_scope": "personal", "level": "confirm_writes"}}); w.Code != http.StatusOK {
		t.Fatalf("attach app: %d %s", w.Code, w.Body.String())
	}

	ctx := context.Background()
	seedPending := func(t *testing.T, summary string) string {
		t.Helper()
		req := f.legacyRequest(t, "personal", "gmail", "GMAIL_SEND_EMAIL", "send", map[string]any{"to": "a@example.com"}, summary)
		id := parseUUID(req.ID)
		row, err := f.h.Queries.GetAppActionRequest(ctx, db.GetAppActionRequestParams{ID: id, WorkspaceID: parseUUID(testWorkspaceID)})
		if err != nil {
			t.Fatalf("load seeded request: %v", err)
		}
		if err := att.AdmitAppRequest(ctx, row); err != nil {
			t.Fatalf("admit app request: %v", err)
		}
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM attention_ledger WHERE subject_type = 'app_action_request' AND subject_id = $1`, req.ID)
		})
		return req.ID
	}

	readLedger := func(t *testing.T, reqID string) (string, pgtype.Int4) {
		t.Helper()
		var state string
		var secs pgtype.Int4
		if err := testPool.QueryRow(ctx,
			`SELECT state, actual_seconds FROM attention_ledger WHERE subject_type = 'app_action_request' AND subject_id = $1`,
			reqID).Scan(&state, &secs); err != nil {
			t.Fatalf("read attention_ledger for %s: %v", reqID, err)
		}
		return state, secs
	}

	cases := []struct {
		name     string
		action   string
		header   string
		wantSecs *int32
	}{
		{"approve with header", "approve", "45", int32Ptr(45)},
		{"approve without header", "approve", "", nil},
		{"reject with header", "reject", "120", int32Ptr(120)},
		{"reject without header", "reject", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reqID := seedPending(t, tc.name)
			var body any
			if tc.action == "reject" {
				body = map[string]any{"reason": "not now"}
			}
			req := appsReqAs(t, testUserID, http.MethodPost, "/api/app-requests/"+reqID+"/"+tc.action, body)
			if tc.header != "" {
				req.Header.Set(attentionOpenedHeader, tc.header)
			}
			req = withURLParam(req, "id", reqID)
			if tc.action == "approve" {
				testutil.Call(t, f.h.ApproveAppRequest, req).Want(http.StatusOK)
			} else {
				testutil.Call(t, f.h.RejectAppRequest, req).Want(http.StatusOK)
			}
			state, secs := readLedger(t, reqID)
			if state != "answered" {
				t.Fatalf("ledger state = %q, want answered", state)
			}
			if tc.wantSecs == nil {
				if secs.Valid {
					t.Fatalf("actual_seconds = %d, want NULL", secs.Int32)
				}
			} else {
				if !secs.Valid || secs.Int32 != *tc.wantSecs {
					t.Fatalf("actual_seconds = %+v, want %d", secs, *tc.wantSecs)
				}
			}
		})
	}
}

func int32Ptr(v int32) *int32 { return &v }
