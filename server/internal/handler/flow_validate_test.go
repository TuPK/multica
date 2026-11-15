package handler

import (
	"net/http"
	"strings"
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

// N7: any member of the workspace may check a manifest, a person as well as an
// agent, and neither needs to manage a squad.
func TestStatelessValidate_MemberAndAgentAllowed(t *testing.T) {
	rig := newDraftRig(t, 1)
	agent := rig.agents[0]
	taskID := createHandlerTestTaskForAgent(t, agent)
	asAgent := func(r *http.Request) *http.Request {
		r.Header.Set("X-Actor-Source", "task_token")
		r.Header.Set("X-Agent-ID", agent)
		r.Header.Set("X-Task-ID", taskID)
		return r
	}
	_, asMember := addWorkflowTestMember(t, "stateless")
	body := map[string]any{"manifest": rig.manifestOf(rig.publishableDoc(agent)), "squad_id": rig.squadID}
	for name, as := range map[string]func(*http.Request) *http.Request{"plain member": asMember, "agent": asAgent} {
		t.Run(name, func(t *testing.T) {
			var out FlowValidationResponse
			callValidateFlow(t, as, body).Want(http.StatusOK).JSON(&out)
			// A person who is not an admin cannot create the statuses the sample document
			// names (C7); that is the answer, not a refusal to answer.
			for _, issue := range out.Issues {
				if issue.Code != "status_create_forbidden" {
					t.Fatalf("unexpected issue %s at %s: %s", issue.Code, issue.Path, issue.Message)
				}
			}
			if out.Analysis == nil {
				t.Fatal("no analysis in the answer")
			}
		})
	}
	// The same plain member cannot touch the squad's drafts.
	wantCode(t, rig.call(testHandler.ListFlowDrafts, asMember, "GET", "/flow-drafts", nil), http.StatusForbidden, "flow_forbidden")
}

// The check leaves no row behind: no draft, no workflow, no issue, no activity.
func TestStatelessValidate_WritesNothing(t *testing.T) {
	rig := newDraftRig(t, 1)
	count := func() [4]int {
		var n [4]int
		for i, q := range []string{
			`SELECT count(*) FROM issue_workflow_draft WHERE squad_id = $1`,
			`SELECT count(*) FROM issue_workflow WHERE squad_id = $1`,
			`SELECT count(*) FROM issue WHERE workspace_id = $1`,
			`SELECT count(*) FROM activity_log WHERE workspace_id = $1`,
		} {
			arg := testWorkspaceID
			if i < 2 {
				arg = rig.squadID
			}
			dbfx.QueryRow(t, q, arg).Scan(&n[i])
		}
		return n
	}
	before := count()
	manifest := rig.manifestOf(rig.publishableDoc(rig.agents[0]))
	callValidateFlow(t, nil, map[string]any{"manifest": manifest, "squad_id": rig.squadID}).Want(http.StatusOK)
	broken := rig.publishableDoc(rig.agents[0])
	asMaps(asMaps(broken["steps"])[1]["tasks"])[0]["role"] = "ghost"
	callValidateFlow(t, nil, map[string]any{"manifest": rig.manifestOf(broken), "squad_id": rig.squadID}).Want(http.StatusOK)
	callValidateFlow(t, nil, map[string]any{"manifest": manifest}).Want(http.StatusOK)
	if after := count(); after != before {
		t.Fatalf("rows (draft, workflow, issue, activity) before %v, after %v", before, after)
	}
}

// The roster of a squad is read only for a squad of the caller's workspace: a
// squad of another workspace answers as a missing one and the manifest is not
// checked against it.
func TestStatelessValidate_SquadRosterOnlyWhenVisible(t *testing.T) {
	rig := newDraftRig(t, 1)
	other := newIniWorkspace(t)
	body := func(squad string) map[string]any {
		return map[string]any{"manifest": rig.manifestOf(rig.publishableDoc(rig.agents[0])), "squad_id": squad}
	}
	resp := callValidateFlow(t, nil, body(other.Product))
	resp.Want(http.StatusNotFound)
	if strings.Contains(resp.Body.String(), other.Product) || strings.Contains(resp.Body.String(), other.Leader) {
		t.Fatalf("the answer names the other workspace's squad or its lead: %s", resp.Body.String())
	}
	callValidateFlow(t, nil, body(rig.squadID)).Want(http.StatusOK)
	// Without a squad nothing is read from one.
	callValidateFlow(t, nil, map[string]any{"manifest": rig.manifestOf(rig.publishableDoc(rig.agents[0]))}).Want(http.StatusOK)
}

// The limit is per actor and per workspace, and a refused call says when to come back.
func TestStatelessValidate_RateLimited(t *testing.T) {
	rig := newDraftRig(t, 1)
	manifest := rig.manifestOf(rig.publishableDoc(rig.agents[0]))
	original := flowValidateLimiter
	flowValidateLimiter = newFlowActorLimiter(3, time.Minute)
	t.Cleanup(func() { flowValidateLimiter = original })

	for i := 0; i < 3; i++ {
		callValidateFlow(t, nil, map[string]any{"manifest": manifest}).Want(http.StatusOK)
	}
	resp := callValidateFlow(t, nil, map[string]any{"manifest": manifest})
	wantCode(t, resp, http.StatusTooManyRequests, "flow_validate_rate_limited")
	if got := resp.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want 60", got)
	}
}
