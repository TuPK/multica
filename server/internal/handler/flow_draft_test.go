package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/featureflag"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Drafts of a flow workflow, the capabilities route, and the routes that check,
// simulate and export a draft (plan P14, P15; spec 7, 8).

// withFlowFlags turns the two flags of flow authoring on or off for one test.
func withFlowFlags(t *testing.T, authoring, graphV2 bool) {
	t.Helper()
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.FlowAuthoring, featureflag.Rule{Default: authoring})
	provider.Set(featureflags.FlowGraphV2, featureflag.Rule{Default: graphV2})
	flags := featureflag.NewService(provider)
	origHandlerFlags := testHandler.FeatureFlags
	testHandler.FeatureFlags = flags
	var origTaskFlags *featureflag.Service
	if testHandler.TaskService != nil {
		origTaskFlags = testHandler.TaskService.FeatureFlags
		testHandler.TaskService.FeatureFlags = flags
	}
	t.Cleanup(func() {
		testHandler.FeatureFlags = origHandlerFlags
		if testHandler.TaskService != nil {
			testHandler.TaskService.FeatureFlags = origTaskFlags
		}
	})
}

// draftRig is a squad of the test workspace that its owner (the test user) manages.
type draftRig struct {
	t       *testing.T
	squadID string
	// agents are the agents of the squad's roster, besides its lead.
	agents []string
}

func newDraftRig(t *testing.T, agentCount int) *draftRig {
	t.Helper()
	cleanupFlowRows(t)
	seedTestCatalog(t)
	withFlowFlags(t, true, true)
	first := seededReadyAgentID(t)
	rig := &draftRig{t: t}
	for i := 0; i < agentCount; i++ {
		rig.agents = append(rig.agents, goldenLeadAgent(t, first))
	}
	if agentCount == 0 {
		rig.agents = []string{first}
	}
	rig.squadID = flowSquad(t, "Draft squad", rig.agents...)
	t.Cleanup(func() {
		ctx := context.Background()
		testPool.Exec(ctx, `UPDATE squad SET active_workflow_id = NULL WHERE id = $1`, rig.squadID)
		testPool.Exec(ctx, `DELETE FROM issue_workflow_draft WHERE squad_id = $1`, rig.squadID)
		testPool.Exec(ctx, `DELETE FROM issue_workflow WHERE squad_id = $1`, rig.squadID)
	})
	return rig
}

// call runs a handler of the authoring family as the workspace owner, or as the
// user the wrapper makes.
func (rig *draftRig) call(h http.HandlerFunc, as func(*http.Request) *http.Request, method, path string, body any, params ...string) *testutil.Response {
	rig.t.Helper()
	req := newRequest(method, "/api/squads/"+rig.squadID+path, body)
	if as != nil {
		req = as(req)
	}
	req = withURLParams(req, append([]string{"workspaceId", testWorkspaceID, "id", rig.squadID}, params...)...)
	return testutil.Call(rig.t, h, req)
}

func (rig *draftRig) createDraft(body map[string]any) *testutil.Response {
	rig.t.Helper()
	return rig.call(testHandler.CreateFlowDraft, nil, "POST", "/flow-drafts", body)
}

func (rig *draftRig) newDraft(name string) FlowDraftResponse {
	rig.t.Helper()
	var d FlowDraftResponse
	rig.createDraft(map[string]any{"name": name}).Want(http.StatusCreated).JSON(&d)
	return d
}

func (rig *draftRig) getDraft(id string) FlowDraftResponse {
	rig.t.Helper()
	var d FlowDraftResponse
	rig.call(testHandler.GetFlowDraft, nil, "GET", "/flow-drafts/"+id, nil, "draftId", id).Want(http.StatusOK).JSON(&d)
	return d
}

func (rig *draftRig) patchDraft(id string, body map[string]any) *testutil.Response {
	rig.t.Helper()
	return rig.call(testHandler.UpdateFlowDraft, nil, "PATCH", "/flow-drafts/"+id, body, "draftId", id)
}

func (rig *draftRig) validateDraft(id string, body map[string]any) *testutil.Response {
	rig.t.Helper()
	return rig.call(testHandler.ValidateFlowDraft, nil, "POST", "/flow-drafts/"+id+"/validate", body, "draftId", id)
}

func wantCode(t *testing.T, resp *testutil.Response, status int, code string) map[string]any {
	t.Helper()
	body := resp.Want(status).Map()
	if body["code"] != code {
		t.Fatalf("code = %v, want %s (body %v)", body["code"], code, body)
	}
	return body
}

func TestFlowDraft_CRUD(t *testing.T) {
	rig := newDraftRig(t, 1)

	created := rig.newDraft("Intake flow")
	if created.Revision != 1 || created.Origin != "manual" || created.SquadID != rig.squadID || created.LineageID != nil {
		t.Fatalf("created draft = %+v", created)
	}
	if _, issues := issueworkflow.ParseFlowDoc(created.Doc); len(issues) > 0 {
		t.Fatalf("a blank draft is not a document: %v", issues)
	}

	var list struct {
		Drafts []FlowDraftSummary `json:"drafts"`
		Total  int                `json:"total"`
	}
	rig.call(testHandler.ListFlowDrafts, nil, "GET", "/flow-drafts", nil).Want(http.StatusOK).JSON(&list)
	if list.Total != 1 || list.Drafts[0].ID != created.ID || list.Drafts[0].Summary.Steps != 1 {
		t.Fatalf("list = %+v", list)
	}

	// A save bumps the revision and keeps what it does not name.
	var saved FlowDraftResponse
	rig.patchDraft(created.ID, map[string]any{"revision": created.Revision, "name": "Intake flow 2"}).Want(http.StatusOK).JSON(&saved)
	if saved.Revision != 2 || saved.Name != "Intake flow 2" || string(saved.Doc) != string(created.Doc) && !sameJSON(saved.Doc, created.Doc) {
		t.Fatalf("saved = %+v", saved)
	}
	doc := loadFlowFixture(t, "research")
	rig.patchDraft(created.ID, map[string]any{"revision": 2, "doc": doc}).Want(http.StatusOK).JSON(&saved)
	if saved.Revision != 3 {
		t.Fatalf("revision = %d, want 3", saved.Revision)
	}
	if got := rig.getDraft(created.ID); got.Revision != 3 || got.Name != "Intake flow 2" || !sameJSON(got.Doc, rawJSON(t, doc)) {
		t.Fatalf("get after save = %+v", got)
	}

	rig.call(testHandler.DeleteFlowDraft, nil, "DELETE", "/flow-drafts/"+created.ID, nil, "draftId", created.ID).Want(http.StatusNoContent)
	wantCode(t, rig.call(testHandler.GetFlowDraft, nil, "GET", "/flow-drafts/"+created.ID, nil, "draftId", created.ID), http.StatusNotFound, "flow_draft_not_found")
}

func rawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ra, _ := json.Marshal(x)
	rb, _ := json.Marshal(y)
	return string(ra) == string(rb)
}

func TestFlowDraft_RevisionConflict409(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.newDraft("Conflict flow")
	rig.patchDraft(d.ID, map[string]any{"revision": 1, "name": "First save"}).Want(http.StatusOK)

	// A second editor still holds revision 1.
	body := wantCode(t, rig.patchDraft(d.ID, map[string]any{"revision": 1, "name": "Late save"}), http.StatusConflict, "flow_draft_conflict")
	if body["revision"] == nil {
		t.Fatalf("a conflict names the revision that is stored: %v", body)
	}
	if got := rig.getDraft(d.ID); got.Name != "First save" || got.Revision != 2 {
		t.Fatalf("the late save changed the draft: %+v", got)
	}
	wantCode(t, rig.patchDraft(d.ID, map[string]any{"name": "No revision"}), http.StatusBadRequest, "flow_draft_revision_required")
}

func TestFlowDraft_AgentActor403(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.newDraft("Agent flow")
	agent := rig.agents[0]
	taskID := createHandlerTestTaskForAgent(t, agent)
	asAgent := func(r *http.Request) *http.Request {
		r.Header.Set("X-Actor-Source", "task_token")
		r.Header.Set("X-Agent-ID", agent)
		r.Header.Set("X-Task-ID", taskID)
		return r
	}
	wantCode(t, rig.call(testHandler.ListFlowDrafts, asAgent, "GET", "/flow-drafts", nil), http.StatusForbidden, "flow_authoring_member_only")
	wantCode(t, rig.call(testHandler.CreateFlowDraft, asAgent, "POST", "/flow-drafts", map[string]any{"name": "By agent"}), http.StatusForbidden, "flow_authoring_member_only")
	wantCode(t, rig.call(testHandler.PublishFlowDraft, asAgent, "POST", "/flow-drafts/"+d.ID+"/publish", map[string]any{"revision": 1}, "draftId", d.ID), http.StatusForbidden, "flow_authoring_member_only")
}

func TestFlowDraft_NotManager403(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.newDraft("Manager only")
	_, asOther := addWorkflowTestMember(t, "draftother")
	wantCode(t, rig.call(testHandler.ListFlowDrafts, asOther, "GET", "/flow-drafts", nil), http.StatusForbidden, "flow_forbidden")
	wantCode(t, rig.call(testHandler.CreateFlowDraft, asOther, "POST", "/flow-drafts", map[string]any{"name": "Nope"}), http.StatusForbidden, "flow_forbidden")
	wantCode(t, rig.call(testHandler.GetFlowDraft, asOther, "GET", "/flow-drafts/"+d.ID, nil, "draftId", d.ID), http.StatusForbidden, "flow_forbidden")
	wantCode(t, rig.call(testHandler.PublishFlowDraft, asOther, "POST", "/flow-drafts/"+d.ID+"/publish", map[string]any{"revision": 1}, "draftId", d.ID), http.StatusForbidden, "flow_publish_forbidden")

	// The member who made the squad manages it.
	creatorID, asCreator := addWorkflowTestMember(t, "draftcreator")
	dbfx.Exec(t, `UPDATE squad SET creator_id = $2 WHERE id = $1`, rig.squadID, creatorID)
	rig.call(testHandler.ListFlowDrafts, asCreator, "GET", "/flow-drafts", nil).Want(http.StatusOK)
}

func TestFlowDraft_FlagOff404(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.newDraft("Flag flow")
	withFlowFlags(t, false, false)
	for name, resp := range map[string]*testutil.Response{
		"list":   rig.call(testHandler.ListFlowDrafts, nil, "GET", "/flow-drafts", nil),
		"create": rig.createDraft(map[string]any{"name": "x"}),
		"get":    rig.call(testHandler.GetFlowDraft, nil, "GET", "/flow-drafts/"+d.ID, nil, "draftId", d.ID),
		"publish": rig.call(testHandler.PublishFlowDraft, nil, "POST", "/flow-drafts/"+d.ID+"/publish",
			map[string]any{"revision": 1}, "draftId", d.ID),
	} {
		t.Run(name, func(t *testing.T) { wantCode(t, resp, http.StatusNotFound, "flow_authoring_disabled") })
	}

	// Authoring on and the graph flag off: drafts work, writing schema 2 does not.
	withFlowFlags(t, true, false)
	rig.call(testHandler.GetFlowDraft, nil, "GET", "/flow-drafts/"+d.ID, nil, "draftId", d.ID).Want(http.StatusOK)
	wantCode(t, rig.call(testHandler.PublishFlowDraft, nil, "POST", "/flow-drafts/"+d.ID+"/publish",
		map[string]any{"revision": 1}, "draftId", d.ID), http.StatusNotFound, "flow_graph_v2_disabled")
	wantCode(t, rig.createDraft(map[string]any{"name": "Imported", "from": map[string]any{"kind": "manifest", "manifest": map[string]any{}}}),
		http.StatusNotFound, "flow_graph_v2_disabled")
}

func TestFlowDraft_DocTooLarge(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.newDraft("Large flow")
	doc := loadFlowFixture(t, "research")
	doc["notes"] = strings.Repeat("x", issueworkflow.DocMaxBytes+10)
	wantCode(t, rig.patchDraft(d.ID, map[string]any{"revision": 1, "doc": doc}), http.StatusUnprocessableEntity, "flow_draft_too_large")
	if got := rig.getDraft(d.ID); got.Revision != 1 {
		t.Fatalf("a document that is too large was saved: revision %d", got.Revision)
	}
}

func TestFlowDraft_LimitPerSquad(t *testing.T) {
	rig := newDraftRig(t, 1)
	for i := 0; i < issueworkflow.MaxDraftsPerSquad; i++ {
		rig.newDraft("Draft " + string(rune('a'+i)))
	}
	body := wantCode(t, rig.createDraft(map[string]any{"name": "One too many"}), http.StatusConflict, "flow_limit_reached")
	if body["limit"] != "drafts" || int(body["max"].(float64)) != issueworkflow.MaxDraftsPerSquad {
		t.Fatalf("limit body = %v", body)
	}
}

func TestFlowDraft_ImportManifestStrict(t *testing.T) {
	rig := newDraftRig(t, 1)
	doc := loadFlowFixture(t, "research")
	good := map[string]any{"format": "multica.flow-manifest/v1", "name": "Research", "doc": doc}

	var d FlowDraftResponse
	rig.createDraft(map[string]any{"from": map[string]any{"kind": "manifest", "manifest": good}}).Want(http.StatusCreated).JSON(&d)
	if d.Origin != "import" || d.Name != "Research" {
		t.Fatalf("imported draft = %+v", d)
	}

	for name, manifest := range map[string]map[string]any{
		"unknown format": {"format": "other/v9", "name": "x", "doc": doc},
		"unknown key":    {"format": "multica.flow-manifest/v1", "name": "x", "doc": doc, "extra": true},
		"broken doc":     {"format": "multica.flow-manifest/v1", "name": "x", "doc": map[string]any{"version": 2, "steps": "not a list"}},
		"newer version":  {"format": "multica.flow-manifest/v1", "name": "x", "doc": map[string]any{"version": 3}},
		"doc too large":  {"format": "multica.flow-manifest/v1", "name": "x", "doc": map[string]any{"version": 2, "notes": strings.Repeat("x", issueworkflow.DocMaxBytes+1024)}},
	} {
		t.Run(name, func(t *testing.T) {
			wantCode(t, rig.createDraft(map[string]any{"name": "Bad", "from": map[string]any{"kind": "manifest", "manifest": manifest}}),
				http.StatusBadRequest, "flow_manifest_invalid")
		})
	}
	var n int
	dbfx.QueryRow(t, `SELECT count(*) FROM issue_workflow_draft WHERE squad_id = $1`, rig.squadID).Scan(&n)
	if n != 1 {
		t.Fatalf("%d drafts after the refused imports, want only the good one", n)
	}
	// A draft from a proposal needs FF_FLOW_PACKS, which this rig leaves off (flow_proposal_test.go has it on).
	wantCode(t, rig.createDraft(map[string]any{"name": "P", "from": map[string]any{"kind": "proposal"}}), http.StatusNotFound, "flow_authoring_disabled")
	// A kind that does not exist is still refused.
	wantCode(t, rig.createDraft(map[string]any{"name": "P", "from": map[string]any{"kind": "telepathy"}}), http.StatusBadRequest, "flow_draft_from_unsupported")

	// The export of a draft carries no one's binding, and imports back.
	bound := loadFlowFixture(t, "research")
	for _, role := range asMaps(bound["roles"]) {
		role["bind"] = map[string]any{"agent_ids": rig.agents}
	}
	rig.patchDraft(d.ID, map[string]any{"revision": d.Revision, "doc": bound}).Want(http.StatusOK)
	var manifest FlowManifestResponse
	rig.call(testHandler.ExportFlowDraft, nil, "GET", "/flow-drafts/"+d.ID+"/export", nil, "draftId", d.ID).Want(http.StatusOK).JSON(&manifest)
	if manifest.Format != "multica.flow-manifest/v1" || strings.Contains(string(manifest.Doc), `"bind"`) {
		t.Fatalf("manifest = %s %s", manifest.Format, manifest.Doc)
	}
}

func TestFlowDraft_ValidateAlways200WithIssues(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.newDraft("Validate flow")

	// A document the draft could not hold is still answered with 200 and its issues.
	var bad FlowValidationResponse
	rig.validateDraft(d.ID, map[string]any{"doc": map[string]any{"version": 2, "steps": []any{}}}).Want(http.StatusOK).JSON(&bad)
	if bad.OK || len(bad.Issues) == 0 {
		t.Fatalf("an empty document validated: %+v", bad)
	}
	var blank FlowValidationResponse
	rig.validateDraft(d.ID, nil).Want(http.StatusOK).JSON(&blank)
	if blank.Revision != 1 || blank.Analysis == nil || blank.FactCatalog == nil {
		t.Fatalf("validation of the draft = %+v", blank)
	}
}

func TestFlowDraft_SimulateWritesNothing(t *testing.T) {
	rig := newDraftRig(t, 1)
	d := rig.newDraft("Simulated flow")
	counts := func() string {
		var s string
		dbfx.QueryRow(t, `SELECT (SELECT count(*) FROM issue WHERE workspace_id = $1)::text || '/' ||
			(SELECT count(*) FROM agent_task_queue)::text || '/' ||
			(SELECT count(*) FROM issue_workflow WHERE workspace_id = $1)::text || '/' ||
			(SELECT count(*) FROM issue_workflow_draft WHERE workspace_id = $1)::text || '/' ||
			(SELECT count(*) FROM comment WHERE workspace_id = $1)::text`, testWorkspaceID).Scan(&s)
		return s
	}
	before := counts()
	var trace FlowTraceResponse
	rig.call(testHandler.SimulateFlowDraft, nil, "POST", "/flow-drafts/"+d.ID+"/simulate", map[string]any{}, "draftId", d.ID).Want(http.StatusOK).JSON(&trace)
	if len(trace.Hops) == 0 || trace.End.Kind == "" {
		t.Fatalf("trace = %+v", trace)
	}
	if after := counts(); after != before {
		t.Fatalf("simulate wrote rows: %s -> %s", before, after)
	}
	// A loop is cut at the hops asked for, and never past the cap.
	var capped FlowTraceResponse
	loop := loadFlowFixture(t, "research")
	rig.call(testHandler.SimulateFlowDraft, nil, "POST", "/flow-drafts/"+d.ID+"/simulate", map[string]any{"doc": loop, "max_hops": 1000}, "draftId", d.ID).Want(http.StatusOK).JSON(&capped)
	if len(capped.Hops) > issueworkflow.MaxSimulateHops {
		t.Fatalf("%d hops, the cap is %d", len(capped.Hops), issueworkflow.MaxSimulateHops)
	}
}

func TestCapabilities_ReportsLimitsKindsAndFeatures(t *testing.T) {
	withFlowFlags(t, true, false)
	var caps FlowCapabilitiesResponse
	testutil.Call(t, testHandler.GetFlowCapabilities, newRequest("GET", "/api/flow/capabilities", nil)).Want(http.StatusOK).JSON(&caps)
	if len(caps.Schemas) == 0 || caps.Limits.Steps == 0 || caps.Limits.DocBytes != issueworkflow.DocMaxBytes {
		t.Fatalf("schemas/limits = %v %+v", caps.Schemas, caps.Limits)
	}
	if caps.Limits.DraftsPerSquad != issueworkflow.MaxDraftsPerSquad || caps.Limits.VersionsPerLineage != issueworkflow.MaxVersionsPerLineage {
		t.Fatalf("limits = %+v", caps.Limits)
	}
	if !hasString(caps.StepKinds, "tasks") || !hasString(caps.TaskKinds, "gate") || !hasString(caps.RoleKinds, "member") || !hasString(caps.StopClasses, "loop_exhausted") {
		t.Fatalf("kinds = %v %v %v %v", caps.StepKinds, caps.TaskKinds, caps.RoleKinds, caps.StopClasses)
	}
	if len(caps.OpsByType) == 0 || len(caps.BuiltinFacts) == 0 {
		t.Fatalf("ops/facts empty: %v %d", caps.OpsByType, len(caps.BuiltinFacts))
	}
	if !caps.Features.Authoring || caps.Features.GraphV2 || caps.Features.Subflow || caps.Features.ForkJoin {
		t.Fatalf("features = %+v", caps.Features)
	}

	// Someone outside the workspace gets nothing.
	req := newRequest("GET", "/api/flow/capabilities", nil)
	req.Header.Set("X-User-ID", "00000000-0000-4000-8000-000000000000")
	testutil.Call(t, testHandler.GetFlowCapabilities, req).Want(http.StatusNotFound)
}

func hasString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestFlowDraft_WorkspaceTeardownSweepsDrafts(t *testing.T) {
	suffix := workflowTestSuffix()
	ws := dbfx.Workspace(t, "Draft teardown "+suffix, "dt-"+suffix)
	insert := func(workspaceID string) {
		dbfx.Exec(t, `INSERT INTO issue_workflow_draft (workspace_id, squad_id, name, doc, created_by, updated_by)
			VALUES ($1, gen_random_uuid(), 'Teardown', '{}'::jsonb, $2, $2)`, workspaceID, testUserID)
	}
	insert(ws)
	insert(testWorkspaceID)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue_workflow_draft WHERE workspace_id = $1 AND name = 'Teardown'`, testWorkspaceID)
	})
	if err := testHandler.Queries.DeleteFlowDraftsForWorkspace(context.Background(), parseUUID(ws)); err != nil {
		t.Fatal(err)
	}
	count := func(workspaceID string) (n int) {
		dbfx.QueryRow(t, `SELECT count(*) FROM issue_workflow_draft WHERE workspace_id = $1 AND name = 'Teardown'`, workspaceID).Scan(&n)
		return n
	}
	if count(ws) != 0 || count(testWorkspaceID) != 1 {
		t.Fatalf("drafts left: workspace %d, other workspace %d", count(ws), count(testWorkspaceID))
	}
}

// Every write of a draft tells the other tabs (spec 11): flow_draft:updated with the squad and the draft.
func TestFlowDraft_EmitsUpdatedEventOnEveryWrite(t *testing.T) {
	rig := newDraftRig(t, 1)
	var mu sync.Mutex
	var seen []map[string]any
	testHandler.Bus.Subscribe(protocol.EventFlowDraftUpdated, func(e events.Event) {
		payload, ok := e.Payload.(map[string]any)
		if !ok || payload["squad_id"] != rig.squadID || e.WorkspaceID != testWorkspaceID {
			return
		}
		mu.Lock()
		seen = append(seen, payload)
		mu.Unlock()
	})
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(seen)
	}

	d := rig.newDraft("Evented")
	if count() != 1 {
		t.Fatalf("a create published %d events, want 1", count())
	}
	rig.patchDraft(d.ID, map[string]any{"revision": 1, "name": "Evented 2"}).Want(http.StatusOK)
	if count() != 2 {
		t.Fatalf("a save published %d events in all, want 2", count())
	}
	// A save that is refused tells no one.
	rig.patchDraft(d.ID, map[string]any{"revision": 1, "name": "Late"}).Want(http.StatusConflict)
	if count() != 2 {
		t.Fatalf("a refused save published an event: %d in all", count())
	}
	rig.call(testHandler.DeleteFlowDraft, nil, "DELETE", "/flow-drafts/"+d.ID, nil, "draftId", d.ID).Want(http.StatusNoContent)
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 {
		t.Fatalf("%d events after the delete, want 3", len(seen))
	}
	for _, payload := range seen {
		if payload["draft_id"] != d.ID {
			t.Fatalf("payload = %v, want draft_id %s", payload, d.ID)
		}
	}
}
