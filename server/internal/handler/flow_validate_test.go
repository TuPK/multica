package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// The stateless check of a manifest: POST /api/flow/validate (spec 7.1, 10, 16.4).

func (rig *draftRig) manifestOf(doc map[string]any) map[string]any {
	for _, role := range asMaps(doc["roles"]) {
		delete(role, "bind")
	}
	return map[string]any{"format": "multica.flow-manifest/v1", "name": "Checked", "doc": doc}
}

func callValidateFlow(t *testing.T, as func(*http.Request) *http.Request, body any) *testutil.Response {
	t.Helper()
	req := newRequest("POST", "/api/flow/validate", body)
	if as != nil {
		req = as(req)
	}
	return testutil.Call(t, testHandler.ValidateFlowManifest, req)
}

func TestValidateFlowManifest_ChecksAManifestAgainstASquad(t *testing.T) {
	rig := newDraftRig(t, 1)
	manifest := rig.manifestOf(rig.publishableDoc(rig.agents[0]))

	var out FlowValidationResponse
	callValidateFlow(t, nil, map[string]any{"manifest": manifest, "squad_id": rig.squadID}).Want(http.StatusOK).JSON(&out)
	if !out.OK || out.Analysis == nil || out.FactCatalog == nil || out.Revision != 0 {
		t.Fatalf("answer = %+v", out)
	}
	// Without a squad the same manifest is checked for what does not depend on one.
	out = FlowValidationResponse{}
	callValidateFlow(t, nil, map[string]any{"manifest": manifest}).Want(http.StatusOK).JSON(&out)
	if !out.OK {
		t.Fatalf("answer without a squad = %+v", out)
	}

	// A document with errors is still a 200 with every issue.
	broken := rig.publishableDoc(rig.agents[0])
	asMaps(asMaps(broken["steps"])[1]["tasks"])[0]["role"] = "ghost"
	out = FlowValidationResponse{}
	callValidateFlow(t, nil, map[string]any{"manifest": rig.manifestOf(broken), "squad_id": rig.squadID}).Want(http.StatusOK).JSON(&out)
	if out.OK || len(out.Issues) == 0 {
		t.Fatalf("a broken manifest validated: %+v", out)
	}
	// A document that does not even parse is reported as issues too.
	out = FlowValidationResponse{}
	callValidateFlow(t, nil, map[string]any{"manifest": map[string]any{
		"format": "multica.flow-manifest/v1", "name": "x", "doc": map[string]any{"version": 2, "steps": "not a list"},
	}}).Want(http.StatusOK).JSON(&out)
	if out.OK || len(out.Issues) == 0 {
		t.Fatalf("an unreadable document validated: %+v", out)
	}
}

func TestValidateFlowManifest_AgentMayCallItDraftsStayClosed(t *testing.T) {
	rig := newDraftRig(t, 1)
	agent := rig.agents[0]
	taskID := createHandlerTestTaskForAgent(t, agent)
	asAgent := func(r *http.Request) *http.Request {
		r.Header.Set("X-Actor-Source", "task_token")
		r.Header.Set("X-Agent-ID", agent)
		r.Header.Set("X-Task-ID", taskID)
		return r
	}
	manifest := rig.manifestOf(rig.publishableDoc(agent))
	callValidateFlow(t, asAgent, map[string]any{"manifest": manifest, "squad_id": rig.squadID}).Want(http.StatusOK)
	wantCode(t, rig.call(testHandler.ListFlowDrafts, asAgent, "GET", "/flow-drafts", nil), http.StatusForbidden, "flow_authoring_member_only")
}

func TestValidateFlowManifest_PeopleInAManifestAreNotChecked(t *testing.T) {
	rig := newDraftRig(t, 1)
	// A manifest carries no people. One that does is read as an import reads it: without them.
	doc := rig.publishableDoc(rig.agents[0])
	asMaps(doc["roles"])[0]["bind"] = map[string]any{"agent_ids": []string{"00000000-0000-4000-8000-000000000001"}}
	manifest := map[string]any{"format": "multica.flow-manifest/v1", "name": "Bound", "doc": doc}
	var out FlowValidationResponse
	callValidateFlow(t, nil, map[string]any{"manifest": manifest, "squad_id": rig.squadID}).Want(http.StatusOK).JSON(&out)
	for _, issue := range out.Issues {
		if issue.Code == "role_not_member" {
			t.Fatalf("a bound agent was checked: %+v", issue)
		}
	}
}

func TestValidateFlowManifest_RejectsWhatIsNotAManifest(t *testing.T) {
	rig := newDraftRig(t, 1)
	good := rig.manifestOf(rig.publishableDoc(rig.agents[0]))
	for name, body := range map[string]map[string]any{
		"no manifest":    {},
		"other format":   {"manifest": map[string]any{"format": "other/v1", "name": "x", "doc": map[string]any{}}},
		"unknown key":    {"manifest": map[string]any{"format": "multica.flow-manifest/v1", "name": "x", "doc": good["doc"], "extra": 1}},
		"manifest a doc": {"manifest": good["doc"]},
	} {
		t.Run(name, func(t *testing.T) {
			wantCode(t, callValidateFlow(t, nil, body), http.StatusBadRequest, "flow_manifest_invalid")
		})
	}
	// A body that is not an object.
	testutil.Call(t, testHandler.ValidateFlowManifest, newRequest("POST", "/api/flow/validate", []int{1, 2})).Want(http.StatusBadRequest)
}

func TestValidateFlowManifest_UnknownOrInvalidSquad(t *testing.T) {
	rig := newDraftRig(t, 1)
	manifest := rig.manifestOf(rig.publishableDoc(rig.agents[0]))
	callValidateFlow(t, nil, map[string]any{"manifest": manifest, "squad_id": "00000000-0000-4000-8000-000000000042"}).Want(http.StatusNotFound)
	callValidateFlow(t, nil, map[string]any{"manifest": manifest, "squad_id": "not-a-uuid"}).Want(http.StatusBadRequest)
	// A person who is not a member of the workspace gets nothing.
	req := newRequest("POST", "/api/flow/validate", map[string]any{"manifest": manifest})
	req.Header.Set("X-User-ID", "00000000-0000-4000-8000-000000000000")
	testutil.Call(t, testHandler.ValidateFlowManifest, req).Want(http.StatusNotFound)
}

func TestValidateFlowManifest_FlagOff404(t *testing.T) {
	rig := newDraftRig(t, 1)
	manifest := rig.manifestOf(rig.publishableDoc(rig.agents[0]))
	withFlowFlags(t, false, false)
	wantCode(t, callValidateFlow(t, nil, map[string]any{"manifest": manifest}), http.StatusNotFound, "flow_authoring_disabled")
}

func TestValidateFlowManifest_LimitsAnActor(t *testing.T) {
	rig := newDraftRig(t, 1)
	manifest := rig.manifestOf(rig.publishableDoc(rig.agents[0]))
	original := flowValidateLimiter
	flowValidateLimiter = newFlowActorLimiter(2, time.Minute)
	t.Cleanup(func() { flowValidateLimiter = original })

	callValidateFlow(t, nil, map[string]any{"manifest": manifest}).Want(http.StatusOK)
	callValidateFlow(t, nil, map[string]any{"manifest": manifest}).Want(http.StatusOK)
	resp := callValidateFlow(t, nil, map[string]any{"manifest": manifest})
	wantCode(t, resp, http.StatusTooManyRequests, "flow_validate_rate_limited")
	// Another actor has its own budget.
	_, asOther := addWorkflowTestMember(t, "validateother")
	callValidateFlow(t, asOther, map[string]any{"manifest": manifest}).Want(http.StatusOK)
}

func TestFlowActorLimiter_WindowResets(t *testing.T) {
	l := newFlowActorLimiter(2, time.Minute)
	now := time.Now()
	if !l.allow("a", now) || !l.allow("a", now) || l.allow("a", now) {
		t.Fatal("the third call in a window must be refused")
	}
	if !l.allow("a", now.Add(time.Minute)) {
		t.Fatal("a new window must start fresh")
	}
}
