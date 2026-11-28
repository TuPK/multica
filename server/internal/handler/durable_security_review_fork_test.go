package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// TestDurableEffectsAndIntentsRejectMachineActors covers review finding 9:
// task_token and cloud_pat callers cannot approve, reject or batch-approve intents,
// undo effects, or change the workspace effect policy; mayDecideIntent rejects a
// task_token caller immediately; DeclareEffect and GET endpoints remain available
// to agents, and an authorized human succeeds.
func TestDurableEffectsAndIntentsRejectMachineActors(t *testing.T) {
	withGmailDraftTools(t)
	f, _ := intentFixture(t)
	f.h.durable.Effects = &service.EffectService{Queries: testHandler.Queries, Flags: effectFlagsOn()}
	f.h.durable.EffectPolicy = &service.EffectPolicyStore{Queries: testHandler.Queries, TxStarter: testPool, Flags: effectFlagsOn()}
	t.Cleanup(func() {
		ctx := context.Background()
		testPool.Exec(ctx, `DELETE FROM effect_policy_override WHERE workspace_id = $1`, testWorkspaceID)
		testPool.Exec(ctx, `DELETE FROM effect_event WHERE workspace_id = $1 AND target LIKE 'gmail/%'`, testWorkspaceID)
	})

	intentID := rehearsedSend(t, f, "Hold for human approval.")
	batchIntentID := rehearsedSend(t, f, "Hold for batch approval.")
	rejectIntentID := rehearsedSend(t, f, "Hold for human rejection.")
	effectID := seedGmailDraftEffect(t, f, time.Now().Add(time.Hour), true)

	// Direct check on mayDecideIntent: an owner's task_token must return false immediately.
	{
		rec := httptest.NewRecorder()
		probeReq := effectAPIRequest(t, http.MethodPost, "/api/intents/"+intentID+"/approve", testUserID, f.taskID, nil)
		probeReq.Header.Set("X-Actor-Source", "task_token")
		probeReq.Header.Set("X-Agent-ID", f.agentID)
		m, ok := f.h.workspaceMember(rec, probeReq, testWorkspaceID)
		if !ok {
			t.Fatalf("workspaceMember failed: %d %s", rec.Code, rec.Body.String())
		}
		if f.h.mayDecideIntent(probeReq, m, parseIntentID(t, intentID)) {
			t.Fatal("mayDecideIntent returned true for an agent task_token acting under the owner")
		}
	}

	stampMachine := func(req *http.Request, source string) *http.Request {
		req.Header.Set("X-Actor-Source", source)
		if source == "task_token" {
			req.Header.Set("X-Agent-ID", f.agentID)
			req.Header.Set("X-Task-ID", f.taskID)
		}
		return req
	}

	policyBody := map[string]any{
		"autonomy_level": 1,
		"effect_class":   connectedapps.EffectWrite,
		"reversibility":  connectedapps.ReversibilityCompensable,
		"decision":       connectedapps.PolicyBlock,
	}

	for _, source := range []string{"task_token", "cloud_pat"} {
		t.Run(source+" refused on human-only writes", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := testutil.WithURLParams(stampMachine(intentRequest(t, "/api/intents/"+intentID+"/approve", testUserID, `{}`), source), "id", intentID)
			f.h.ApproveIntent(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s ApproveIntent = %d (%s), want 403", source, w.Code, w.Body.String())
			}

			w = httptest.NewRecorder()
			req = testutil.WithURLParams(stampMachine(intentRequest(t, "/api/intents/"+rejectIntentID+"/reject", testUserID, `{"note":"nope"}`), source), "id", rejectIntentID)
			f.h.RejectIntent(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s RejectIntent = %d (%s), want 403", source, w.Code, w.Body.String())
			}

			w = httptest.NewRecorder()
			batchBody, _ := json.Marshal(map[string]any{"ids": []string{batchIntentID}})
			req = stampMachine(intentRequest(t, "/api/intents/approve-batch", testUserID, string(batchBody)), source)
			f.h.ApproveIntentBatch(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s ApproveIntentBatch = %d (%s), want 403", source, w.Code, w.Body.String())
			}

			w = httptest.NewRecorder()
			req = stampMachine(effectUndoRequest(t, effectID, testUserID), source)
			f.h.UndoEffect(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s UndoEffect = %d (%s), want 403", source, w.Code, w.Body.String())
			}

			w = httptest.NewRecorder()
			req = stampMachine(effectAPIRequest(t, http.MethodPut, "/api/effects/policy", testUserID, "", policyBody), source)
			f.h.SaveEffectPolicy(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s SaveEffectPolicy = %d (%s), want 403", source, w.Code, w.Body.String())
			}
		})
	}

	if n := len(f.sdk.executions()); n != 0 {
		t.Fatalf("provider executions after refused machine calls = %d, want 0", n)
	}
	if state, _ := intentRow(t, intentID); state != "pending" {
		t.Fatalf("intent state after refused machine approve = %q, want pending", state)
	}

	// Agent task_token keeps access to DeclareEffect and GET endpoints.
	t.Run("agent keeps declare and reads", func(t *testing.T) {
		w := httptest.NewRecorder()
		declReq := stampMachine(effectAPIRequest(t, http.MethodPost, "/api/effects/declare", testUserID, f.taskID, map[string]any{
			"class": connectedapps.EffectRead, "target": "smtp:customer@example.com", "summary": "agent check",
		}), "task_token")
		f.h.DeclareEffect(w, declReq)
		if w.Code != http.StatusOK {
			t.Fatalf("agent DeclareEffect = %d (%s), want 200", w.Code, w.Body.String())
		}
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM effect_event WHERE task_id = $1`, f.taskID)
		})

		for name, call := range map[string]func(*httptest.ResponseRecorder){
			"ListEffects": func(rec *httptest.ResponseRecorder) {
				f.h.ListEffects(rec, stampMachine(effectAPIRequest(t, http.MethodGet, "/api/effects", testUserID, "", nil), "task_token"))
			},
			"GetEffectPolicy": func(rec *httptest.ResponseRecorder) {
				f.h.GetEffectPolicy(rec, stampMachine(effectAPIRequest(t, http.MethodGet, "/api/effects/policy", testUserID, "", nil), "task_token"))
			},
			"ListIntents": func(rec *httptest.ResponseRecorder) {
				f.h.ListIntents(rec, stampMachine(effectAPIRequest(t, http.MethodGet, "/api/intents", testUserID, "", nil), "task_token"))
			},
		} {
			rec := httptest.NewRecorder()
			call(rec)
			if rec.Code != http.StatusOK {
				t.Fatalf("agent %s = %d (%s), want 200", name, rec.Code, rec.Body.String())
			}
		}
	})

	// Authorized human succeeds on approve, reject, batch-approve, undo, and policy save.
	t.Run("human succeeds", func(t *testing.T) {
		w := httptest.NewRecorder()
		f.h.ApproveIntent(w, testutil.WithURLParams(intentRequest(t, "/api/intents/"+intentID+"/approve", testUserID, `{}`), "id", intentID))
		if w.Code != http.StatusOK {
			t.Fatalf("human ApproveIntent = %d (%s), want 200", w.Code, w.Body.String())
		}

		w = httptest.NewRecorder()
		f.h.RejectIntent(w, testutil.WithURLParams(intentRequest(t, "/api/intents/"+rejectIntentID+"/reject", testUserID, `{"note":"human says no"}`), "id", rejectIntentID))
		if w.Code != http.StatusOK {
			t.Fatalf("human RejectIntent = %d (%s), want 200", w.Code, w.Body.String())
		}

		w = httptest.NewRecorder()
		batchBody, _ := json.Marshal(map[string]any{"ids": []string{batchIntentID}})
		f.h.ApproveIntentBatch(w, intentRequest(t, "/api/intents/approve-batch", testUserID, string(batchBody)))
		if w.Code != http.StatusOK {
			t.Fatalf("human ApproveIntentBatch = %d (%s), want 200", w.Code, w.Body.String())
		}

		w = httptest.NewRecorder()
		// The agent subtest's cleanup removes every effect of this task, so undo a fresh one.
		undoID := seedGmailDraftEffect(t, f, time.Now().Add(time.Hour), true)
		f.h.UndoEffect(w, effectUndoRequest(t, undoID, testUserID))
		if w.Code != http.StatusOK {
			t.Fatalf("human UndoEffect = %d (%s), want 200", w.Code, w.Body.String())
		}

		w = httptest.NewRecorder()
		f.h.SaveEffectPolicy(w, effectAPIRequest(t, http.MethodPut, "/api/effects/policy", testUserID, "", policyBody))
		if w.Code != http.StatusOK {
			t.Fatalf("human SaveEffectPolicy = %d (%s), want 200", w.Code, w.Body.String())
		}
	})
}

// TestDurablePrincipalAndEnvelopeRejectMachineActors covers review finding 10:
// person-only principal routes and squad autonomy envelope writes reject both
// cloud_pat and task_token with 403.
func TestDurablePrincipalAndEnvelopeRejectMachineActors(t *testing.T) {
	o := newOutcomeFixture(t)
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: true})
	provider.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: true})
	prevFlags := testHandler.FeatureFlags
	testHandler.FeatureFlags = featureflag.NewService(provider)
	t.Cleanup(func() { testHandler.FeatureFlags = prevFlags })

	task := o.wsfx.Task(t, o.doer, testutil.Cols{"issue_id": o.issue, "status": "running", "runtime_id": o.runtime})
	squad := o.wsfx.Squad(t, "envelope actor guard squad", o.doer)
	randomID := uuid.NewString()

	cases := []struct {
		name    string
		method  string
		path    string
		body    any
		params  []string
		handler http.HandlerFunc
	}{
		{"enable profile", http.MethodPut, "/api/principal/profile", map[string]any{"consent_text": "Tôi đồng ý"}, nil, testHandler.EnablePrincipalProfile},
		{"delete profile", http.MethodDelete, "/api/principal/profile", nil, nil, testHandler.DeletePrincipalProfile},
		{"edit trait", http.MethodPatch, "/api/principal/profile/traits/" + randomID, map[string]any{"statement": "edited"}, []string{"id", randomID}, testHandler.EditPrincipalTrait},
		{"enable delegation", http.MethodPut, "/api/principal/delegations", map[string]any{"decision_kind": "general", "min_confidence": 0.9}, nil, testHandler.EnablePrincipalDelegation},
		{"disable delegation", http.MethodDelete, "/api/principal/delegations?decision_kind=general", nil, nil, testHandler.DisablePrincipalDelegation},
		{"revert decision", http.MethodPost, "/api/principal/decisions/" + randomID + "/revert", nil, []string{"id", randomID}, testHandler.RevertDelegatedDecision},
		{"get interview", http.MethodGet, "/api/principal/interview", nil, nil, testHandler.GetPrincipalInterview},
		{"answer interview", http.MethodPost, "/api/principal/interview/answers", map[string]any{"question": 0, "answer": "yes"}, nil, testHandler.AnswerPrincipalInterview},
		{"set squad envelope", http.MethodPut, "/api/squads/" + squad + "/autonomy/envelope", map[string]any{"floor_level": 0, "ceiling_level": 2, "reason": "test"}, []string{"id", squad}, testHandler.SetSquadEnvelope},
		{"clear squad envelope", http.MethodDelete, "/api/squads/" + squad + "/autonomy/envelope", nil, []string{"id", squad}, testHandler.ClearSquadEnvelope},
	}

	for _, source := range []string{"cloud_pat", "task_token"} {
		for _, tc := range cases {
			t.Run(source+"/"+tc.name, func(t *testing.T) {
				req := companyReq(o.fx.Owner, o.fx.ID, tc.method, tc.path, tc.body)
				req.Header.Set("X-Actor-Source", source)
				if source == "task_token" {
					req.Header.Set("X-Agent-ID", o.doer)
					req.Header.Set("X-Task-ID", task)
				}
				if tc.params != nil {
					req = testutil.WithURLParams(req, tc.params...)
				}
				testutil.Call(t, tc.handler, req).Want(http.StatusForbidden)
			})
		}
	}
}

// TestHubAgentBundleHonorsDurableOpenFlag covers review finding 7:
// GET /api/hub/items/agent/{id}/bundle, /bundle/download, and hubItemForRef
// return 404 when durable_open is off, matching GetHubItem.
func TestHubAgentBundleHonorsDurableOpenFlag(t *testing.T) {
	f := newHubAttestationFixture(t)
	refID, _ := f.insertAgentItem(t, map[string][]byte{
		"AGENT.md":            []byte("# Reviewer\n"),
		"attestations/a.json": f.signAttestation(t, f.remoteHost, "acme/code-reviewer@1.0.0"),
	})

	// Flag off: both bundle endpoints and hubItemForRef answer not found.
	withFeatureFlag(t, testHandler, featureflags.DurableOpen, false)

	bundleReq := withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/agent/"+refID+"/bundle", nil, f.ws, f.owner), "kind", string(hub.KindAgent), "id", refID)
	testutil.Call(t, testHandler.GetHubItemBundle, bundleReq).Want(http.StatusNotFound)

	dlReq := withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/agent/"+refID+"/bundle/download", nil, f.ws, f.owner), "kind", string(hub.KindAgent), "id", refID)
	testutil.Call(t, testHandler.DownloadHubItemBundle, dlReq).Want(http.StatusNotFound)

	if _, err := testHandler.hubItemForRef(context.Background(), parseUUID(f.ws), string(hub.KindAgent), parseUUID(refID)); !errors.Is(err, errHubItemNotFound) {
		t.Fatalf("hubItemForRef with durable_open off = %v, want errHubItemNotFound", err)
	}

	// Flag on: both bundle endpoints succeed.
	withFeatureFlag(t, testHandler, featureflags.DurableOpen, true)

	bundleReqOn := withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/agent/"+refID+"/bundle", nil, f.ws, f.owner), "kind", string(hub.KindAgent), "id", refID)
	testutil.Call(t, testHandler.GetHubItemBundle, bundleReqOn).Want(http.StatusOK)

	dlReqOn := withURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/agent/"+refID+"/bundle/download", nil, f.ws, f.owner), "kind", string(hub.KindAgent), "id", refID)
	testutil.Call(t, testHandler.DownloadHubItemBundle, dlReqOn).Want(http.StatusOK)
}

// TestVerifyAttestationRejectsInvalidIssuerHost covers review finding 11:
// attestationKeysFor validates issuer.host via validAttestationHost before building
// a URL or contacting any server.
func TestVerifyAttestationRejectsInvalidIssuerHost(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	srvHost := strings.TrimPrefix(srv.URL, "https://")

	h := &Handler{}
	withFeatureFlag(t, h, featureflags.DurableOpen, true)

	badHosts := []string{
		srvHost + "/any/path?x=1#",
		"attacker.example/any/path?x=1#",
		"user:pass@" + srvHost,
		"attacker.example\\evil",
		" " + srvHost,
		":443",
		"attacker.example:badport",
	}

	for _, host := range badHosts {
		if _, err := h.attestationKeysFor(context.Background(), host); err == nil || !strings.Contains(err.Error(), "invalid issuer host") {
			t.Fatalf("attestationKeysFor(%q) err = %v, want invalid issuer host error", host, err)
		}
		raw, _ := json.Marshal(map[string]any{"issuer": map[string]any{"host": host, "key_id": "k1"}})
		req := httptest.NewRequest(http.MethodPost, "/api/attestations/verify", bytes.NewReader(raw))
		rec := httptest.NewRecorder()
		h.VerifyAttestation(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("VerifyAttestation(%q) status = %d, want 200", host, rec.Code)
		}
		var got VerifyAttestationResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode VerifyAttestation(%q): %v", host, err)
		}
		if got.Valid || got.Reason != "keys_unavailable" {
			t.Fatalf("VerifyAttestation(%q) = %+v, want valid=false reason=keys_unavailable", host, got)
		}
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("TLS server received %d requests for invalid issuer hosts, want 0", got)
	}
}

// TestDurableWritesEnforceBodyLimits covers review finding 12 (durable spec 14.4):
// decodeInitiativeBody and the durable JSON write endpoints cap request bodies
// (16 KiB standard, 32 KiB for intent preview edits, 256 KiB for plugin storage)
// and answer 413 when exceeded.
func TestDurableWritesEnforceBodyLimits(t *testing.T) {
	fx := newCompanyWorkspace(t)
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableOpen, featureflag.Rule{Default: true})
	provider.Set(featureflags.DurableEffects, featureflag.Rule{Default: true})
	provider.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: true})
	provider.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: true})
	provider.Set(featureflags.PluginsV1, featureflag.Rule{Default: true})
	flags := featureflag.NewService(provider)

	h := *testHandler
	h.FeatureFlags = flags
	h.durable.Effects = &service.EffectService{Queries: testHandler.Queries, Flags: flags}
	h.durable.EffectPolicy = &service.EffectPolicyStore{Queries: testHandler.Queries, TxStarter: testPool, Flags: flags}
	h.durable.Intents = &service.IntentService{Queries: testHandler.Queries, TxStarter: testPool, Flags: flags}

	over16K := map[string]any{"note": strings.Repeat("a", (16<<10)+1024)}
	over32K := map[string]any{"note": strings.Repeat("a", (32<<10)+1024)}
	unknownID := uuid.NewString()

	cases := []struct {
		name    string
		method  string
		path    string
		body    any
		params  []string
		handler http.HandlerFunc
	}{
		{"decodeInitiativeBody (EnablePrincipalProfile)", http.MethodPut, "/api/principal/profile", over16K, nil, h.EnablePrincipalProfile},
		{"CloseReachGap", http.MethodPost, "/api/reach/gaps", over16K, nil, h.CloseReachGap},
		{"ApproveIntent over 32KiB", http.MethodPost, "/api/intents/" + unknownID + "/approve", over32K, []string{"id", unknownID}, h.ApproveIntent},
		{"RejectIntent", http.MethodPost, "/api/intents/" + unknownID + "/reject", over16K, []string{"id", unknownID}, h.RejectIntent},
		{"ApproveIntentBatch", http.MethodPost, "/api/intents/approve-batch", over16K, nil, h.ApproveIntentBatch},
		{"DeclareEffect", http.MethodPost, "/api/effects/declare", over16K, nil, h.DeclareEffect},
		{"SaveEffectPolicy", http.MethodPut, "/api/effects/policy", over16K, nil, h.SaveEffectPolicy},
		{"IssueAttestation", http.MethodPost, "/api/attestations/issue", over16K, nil, h.IssueAttestation},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := companyReq(fx.Owner, fx.ID, tc.method, tc.path, tc.body)
			if tc.params != nil {
				req = testutil.WithURLParams(req, tc.params...)
			}
			testutil.Call(t, tc.handler, req).Want(http.StatusRequestEntityTooLarge)
		})
	}

	// ApproveIntent allows bodies between 16 KiB and 32 KiB (spec 14.4: 32 KiB for intent preview).
	t.Run("ApproveIntent under 32KiB passes body cap", func(t *testing.T) {
		under32K := map[string]any{"note": strings.Repeat("b", 20<<10)}
		req := testutil.WithURLParams(companyReq(fx.Owner, fx.ID, http.MethodPost, "/api/intents/"+unknownID+"/approve", under32K), "id", unknownID)
		rec := testutil.Call(t, h.ApproveIntent, req)
		if rec.Code == http.StatusRequestEntityTooLarge {
			t.Fatalf("ApproveIntent rejected a 20 KiB body with 413; spec 14.4 allows up to 32 KiB")
		}
		rec.Want(http.StatusNotFound)
	})

	// PutPluginStorage rejects oversized bodies with 413 Problem response.
	t.Run("PutPluginStorage", func(t *testing.T) {
		installationID := installPluginForAction(t, []string{"storage:user"})
		overPluginCap := map[string]any{"value": strings.Repeat("x", (256<<10)+1024)}
		rec := httptest.NewRecorder()
		h.PutPluginStorage(rec, pluginActionRequest(http.MethodPut, "/v1/storage/user/big", installationID, overPluginCap, map[string]string{"scope": "user", "key": "big"}))
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("PutPluginStorage status = %d (%s), want 413", rec.Code, rec.Body.String())
		}
	})
}

// TestListIntentsHidesPrivateProjectIssues covers review finding 14:
// GET /api/intents filters out intents tied to an issue in a private project
// the caller cannot see, matching ApproveIntent and RejectIntent.
func TestListIntentsHidesPrivateProjectIssues(t *testing.T) {
	f, _ := intentFixture(t)
	pf := newPrivateProjectFixture(t)

	publicIntentID := rehearsedSend(t, f, "Public quote.")
	secretIntentID := rehearsedSend(t, f, "Secret quote.")
	if _, err := testPool.Exec(context.Background(), `UPDATE effect_intent SET issue_id = $2 WHERE id = $1`, secretIntentID, pf.secretIssue); err != nil {
		t.Fatalf("bind intent to private issue: %v", err)
	}
	if _, err := testPool.Exec(context.Background(), `UPDATE effect_intent SET issue_id = $2 WHERE id = $1`, publicIntentID, pf.publicIssue); err != nil {
		t.Fatalf("bind intent to public issue: %v", err)
	}

	listFor := func(userID string) map[string]bool {
		t.Helper()
		rec := httptest.NewRecorder()
		req := effectAPIRequest(t, http.MethodGet, "/api/intents?limit=200", userID, "", nil)
		f.h.ListIntents(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("ListIntents(%s): %d %s", userID, rec.Code, rec.Body.String())
		}
		var out struct {
			Intents []IntentResponse `json:"intents"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode ListIntents: %v", err)
		}
		seen := make(map[string]bool, len(out.Intents))
		for _, it := range out.Intents {
			seen[it.ID] = true
		}
		return seen
	}

	for label, userID := range map[string]string{"outsideAdmin": pf.outsideAdmin, "outsider": pf.outsider} {
		seen := listFor(userID)
		if seen[secretIntentID] {
			t.Fatalf("%s saw private-project intent %s in ListIntents", label, secretIntentID)
		}
		if !seen[publicIntentID] {
			t.Fatalf("%s did not see public intent %s in ListIntents", label, publicIntentID)
		}
	}

	seenInsider := listFor(pf.insider)
	if !seenInsider[secretIntentID] || !seenInsider[publicIntentID] {
		t.Fatalf("insider ListIntents missing intents: secret=%v public=%v", seenInsider[secretIntentID], seenInsider[publicIntentID])
	}
}
