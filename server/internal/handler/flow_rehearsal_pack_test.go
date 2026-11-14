package handler

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// Rehearsals of a pack all the way to a running workflow (plan P28; spec 12.3 N1 to N10, 16). Each test imports a
// bundle, makes a workflow of it and runs a request to its end. The agents are fake: a test closes a task the way the
// agent's CLI call would, as the rehearsals of schema 2 do, and no agent CLI or runtime is started.
//
// The bundles are the ban-hang-online folder of the Kit Viet upstream (read as it is) and the fixtures of
// server/internal/pack/testdata. The flow files an agent would write are test data in testdata/pack_rehearsal.

const upstreamBanHangDir = "../../../catalog/_sources/kitviet/upstream/ban-hang-online"

// rehearsalFlags turns on packs, flow authoring and the documents of schema 2, for the handler and the task service.
func rehearsalFlags(t *testing.T) {
	t.Helper()
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.FlowPacks, featureflag.Rule{Default: true})
	provider.Set(featureflags.FlowAuthoring, featureflag.Rule{Default: true})
	provider.Set(featureflags.FlowGraphV2, featureflag.Rule{Default: true})
	flags := featureflag.NewService(provider)
	prevHandler := testHandler.FeatureFlags
	testHandler.FeatureFlags = flags
	var prevTask *featureflag.Service
	if testHandler.TaskService != nil {
		prevTask = testHandler.TaskService.FeatureFlags
		testHandler.TaskService.FeatureFlags = flags
	}
	t.Cleanup(func() {
		testHandler.FeatureFlags = prevHandler
		if testHandler.TaskService != nil {
			testHandler.TaskService.FeatureFlags = prevTask
		}
	})
}

// readTree reads a folder as the files of a bundle.
func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = string(content)
		return nil
	})
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}
	return files
}

// rehearsalFile is a file of testdata/pack_rehearsal: a flow.json the fake agent writes.
func rehearsalFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "pack_rehearsal", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// packRehearsal is a pack installed in the test workspace, with what a rehearsal needs to run it.
type packRehearsal struct {
	t       *testing.T
	ws      packWS
	suffix  string
	squadID string
	// agents are the agents the install made, by the key of the bundle.
	agents map[string]string
	result PackCommitResponse
}

func newPackRehearsal(t *testing.T) *packRehearsal {
	t.Helper()
	cleanupFlowRows(t)
	seedTestCatalog(t)
	setupHubImportTest(t)
	rehearsalFlags(t)
	runtime := runtimeOf(t, seededReadyAgentID(t))
	return &packRehearsal{
		t:      t,
		ws:     packWS{ID: testWorkspaceID, AdminID: testUserID, RuntimeID: runtime},
		suffix: workflowTestSuffix(),
		agents: map[string]string{},
	}
}

// preview reads a bundle as the person who imports it.
func (p *packRehearsal) preview(files map[string]string) PackPreviewResponse {
	p.t.Helper()
	return p.ws.previewOK(p.t, files)
}

// commit installs the previewed bundle. The install is a new one each time, so what an earlier test left is not in the way.
func (p *packRehearsal) commit(preview PackPreviewResponse, body map[string]any) PackCommitResponse {
	p.t.Helper()
	body["digest"] = preview.Digest
	body["new_install"] = true
	var res PackCommitResponse
	p.ws.commit(p.t, p.ws.AdminID, body).Want(http.StatusCreated).JSON(&res)
	p.register(res)
	return res
}

// register notes what an install made, so that the test cleans it up.
func (p *packRehearsal) register(res PackCommitResponse) {
	p.t.Helper()
	p.result = res
	if res.SquadID != nil {
		p.squadID = *res.SquadID
	}
	var agentIDs, skillIDs []string
	for _, a := range res.Agents {
		p.agents[a.Key] = a.AgentID
		agentIDs = append(agentIDs, a.AgentID)
	}
	for _, s := range res.Skills {
		if s.Created {
			skillIDs = append(skillIDs, s.SkillID)
		}
	}
	squad, installID := p.squadID, res.InstallID
	p.t.Cleanup(func() { dropInstalledPack(squad, installID, agentIDs, skillIDs) })
}

// dropInstalledPack removes the squad, agents and skills an install made, and what the rehearsal made on them.
func dropInstalledPack(squadID, installID string, agentIDs, skillIDs []string) {
	ctx := context.Background()
	exec := func(sql string, args ...any) { testPool.Exec(ctx, sql, args...) }
	if squadID != "" {
		exec(`UPDATE squad SET active_workflow_id = NULL WHERE id = $1`, squadID)
		exec(`DELETE FROM issue WHERE workspace_id = $1 AND parent_issue_id IN
			(SELECT id FROM issue WHERE workspace_id = $1 AND assignee_type = 'squad' AND assignee_id = $2)`, testWorkspaceID, squadID)
		exec(`DELETE FROM issue WHERE workspace_id = $1 AND assignee_type = 'squad' AND assignee_id = $2`, testWorkspaceID, squadID)
		exec(`DELETE FROM issue_workflow_draft WHERE squad_id = $1`, squadID)
		exec(`DELETE FROM issue_workflow WHERE squad_id = $1`, squadID)
		exec(`DELETE FROM squad_member WHERE squad_id = $1`, squadID)
	}
	if len(agentIDs) > 0 {
		exec(`DELETE FROM agent_task_queue WHERE agent_id = ANY($1::uuid[])`, agentIDs)
		exec(`DELETE FROM attachment WHERE workspace_id = $1 AND issue_id IN (SELECT id FROM issue WHERE assignee_id = ANY($2::uuid[]))`, testWorkspaceID, agentIDs)
		exec(`DELETE FROM issue WHERE workspace_id = $1 AND assignee_id = ANY($2::uuid[])`, testWorkspaceID, agentIDs)
		exec(`DELETE FROM agent_skill WHERE agent_id = ANY($1::uuid[])`, agentIDs)
	}
	if squadID != "" {
		exec(`DELETE FROM squad WHERE id = $1`, squadID)
	}
	if len(agentIDs) > 0 {
		exec(`DELETE FROM agent WHERE id = ANY($1::uuid[])`, agentIDs)
	}
	if len(skillIDs) > 0 {
		exec(`DELETE FROM agent_skill WHERE skill_id = ANY($1::uuid[])`, skillIDs)
		exec(`DELETE FROM skill_version WHERE skill_id = ANY($1::uuid[])`, skillIDs)
		exec(`DELETE FROM skill_file WHERE skill_id = ANY($1::uuid[])`, skillIDs)
		exec(`DELETE FROM skill WHERE id = ANY($1::uuid[])`, skillIDs)
	}
	if installID != "" {
		exec(`DELETE FROM squad_pack_install WHERE id = $1`, installID)
	}
}

// squad is the caller of the routes of the squad's drafts, with a store for the files an agent attaches.
func (p *packRehearsal) squad(drafter string) *proposalRig {
	p.t.Helper()
	store := &mockStorage{}
	prev := testHandler.Storage
	testHandler.Storage = store
	p.t.Cleanup(func() { testHandler.Storage = prev })
	return &proposalRig{draftRig: &draftRig{t: p.t, squadID: p.squadID}, store: store, drafter: drafter}
}

// bind gives the roles of a draft their agents, by role key, and saves it.
func (p *packRehearsal) bind(rig *draftRig, draft FlowDraftResponse, agentOf map[string]string) FlowDraftResponse {
	p.t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(draft.Doc, &doc); err != nil {
		p.t.Fatal(err)
	}
	for _, role := range asMaps(doc["roles"]) {
		key, _ := role["key"].(string)
		if agent, ok := agentOf[key]; ok {
			role["bind"] = map[string]any{"agent_ids": []string{agent}}
		}
	}
	var saved FlowDraftResponse
	rig.patchDraft(draft.ID, map[string]any{"revision": draft.Revision, "doc": doc}).Want(http.StatusOK).JSON(&saved)
	return saved
}

// draftDoc reads the document of a draft.
func draftDoc(t *testing.T, draft FlowDraftResponse) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(draft.Doc, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// request makes a request for the squad on a version, as the person who manages it, and returns the issue.
func (p *packRehearsal) request(workflowID string, extra ...map[string]any) db.Issue {
	p.t.Helper()
	var created IssueResponse
	createSquadIssue(p.t, p.squadID, workflowID, nil, extra...).Want(http.StatusCreated).JSON(&created)
	p.t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, created.ID)
	})
	return loadIssueRow(p.t, created.ID)
}

func (p *packRehearsal) state(request db.Issue) IssueFlowStateResponse {
	p.t.Helper()
	state, err := testHandler.flowStateResponse(context.Background(), loadIssueRow(p.t, uuidToString(request.ID)), issueworkflow.RoleRequest)
	if err != nil {
		p.t.Fatalf("flowStateResponse: %v", err)
	}
	return state
}

func (p *packRehearsal) closed(request db.Issue) bool {
	return dbfx.Count(p.t, `SELECT count(*) FROM issue WHERE id = $1 AND flow->>'closed_at' IS NOT NULL`, request.ID) == 1
}

// drive runs a request to its end the way the people of the workflow would. At a lead step the lead completes the
// step; at a manual step the owner takes the first exit; at a step of tasks every open task is closed by whoever it is
// given to, a gate with the verdict the function gives. A request that is not closed after forty moves is a failure.
func (p *packRehearsal) drive(request db.Issue, doc map[string]any, verdict func(step, task string, activation int) string) {
	p.t.Helper()
	kinds := map[string]map[string]any{}
	for _, step := range asMaps(doc["steps"]) {
		kinds[step["key"].(string)] = step
	}
	taskKinds := map[string]string{}
	for _, step := range asMaps(doc["steps"]) {
		for _, task := range asMaps(step["tasks"]) {
			taskKinds[step["key"].(string)+"/"+task["key"].(string)] = task["kind"].(string)
		}
	}
	ctx := context.Background()
	for move := 0; move < 40; move++ {
		if p.closed(request) {
			return
		}
		state := p.state(request)
		if state.Stop != nil {
			p.t.Fatalf("the request stopped at %q (%+v) after %d moves", state.Step, state.Stop, move)
		}
		step, ok := kinds[state.Step]
		if !ok {
			p.t.Fatalf("the request is at %q, which the document does not have", state.Step)
		}
		switch step["kind"] {
		case issueworkflow.StepKindLead:
			if _, err := testHandler.CompleteFlowStep(ctx, loadIssueRow(p.t, uuidToString(request.ID)), "", FlowCompleteInput{}); err != nil {
				p.t.Fatalf("complete the lead step %q: %v", state.Step, err)
			}
		case issueworkflow.StepKindManual:
			edge := asMaps(step["edges"])[0]["id"].(string)
			if _, err := testHandler.ExitFlow(ctx, loadIssueRow(p.t, uuidToString(request.ID)), edge, memberActor(p.t, testUserID)); err != nil {
				p.t.Fatalf("take the exit %q of %q: %v", edge, state.Step, err)
			}
		default:
			rows, err := testPool.Query(ctx, `SELECT id::text, flow_task, COALESCE(assignee_type, ''), COALESCE(assignee_id::text, '') FROM issue
				WHERE parent_issue_id = $1 AND flow_step = $2 AND status NOT IN ('done', 'cancelled') ORDER BY flow_task`, request.ID, state.Step)
			if err != nil {
				p.t.Fatal(err)
			}
			type open struct{ id, task, kind, assignee string }
			var opened []open
			for rows.Next() {
				var o open
				if err := rows.Scan(&o.id, &o.task, &o.kind, &o.assignee); err != nil {
					p.t.Fatal(err)
				}
				opened = append(opened, o)
			}
			rows.Close()
			if len(opened) == 0 {
				p.t.Fatalf("no open task at %q, and the request is not closed (state %+v)", state.Step, state)
			}
			for _, o := range opened {
				key, _, _ := strings.Cut(o.task, "~")
				outcome := ""
				if taskKinds[state.Step+"/"+key] == issueworkflow.KindGate {
					outcome = verdict(state.Step, key, state.Activations[state.Step])
				}
				if o.kind == "agent" {
					finishTask(p.t, o.id, o.assignee, outcome)
				} else {
					body := map[string]any{"status": "done"}
					if outcome != "" {
						body["outcome"] = outcome
					}
					updateIssueForTest(p.t, o.id, body).Want(http.StatusOK)
				}
			}
		}
		advance(p.t, request)
	}
	p.t.Fatalf("the request is not closed after 40 moves: %+v", p.state(request))
}

func approvePackGates(string, string, int) string { return "approved" }

// tasksOf counts the task issues of a request at a step.
func tasksOfStep(t *testing.T, request db.Issue, step string) int {
	t.Helper()
	return dbfx.Count(t, `SELECT count(*) FROM issue WHERE parent_issue_id = $1 AND flow_step = $2`, request.ID, step)
}

func TestRehearsalPack_KitVietUpstreamRaw(t *testing.T) {
	p := newPackRehearsal(t)

	// 1. The folder of the trade, as the upstream has it: seven agents and the skills of the trade, no flow file.
	preview := p.preview(readTree(t, upstreamBanHangDir))
	if len(preview.Agents) != 7 || len(preview.Skills) != 18 {
		t.Fatalf("the bundle has %d agents and %d skills, want 7 and 18", len(preview.Agents), len(preview.Skills))
	}
	for _, issue := range preview.Issues {
		if issue.Severity == "error" {
			t.Fatalf("the upstream folder, as it is, has an error: %+v", issue)
		}
	}
	if preview.Flow.Present {
		t.Fatalf("the upstream folder carries no flow: %+v", preview.Flow)
	}

	// 2. A squad with the general lead. The agents are made under names of this run, and the writer gets the three
	// skills of its trade.
	writerSkills := []string{"mo-ta-san-pham-shop", "tieu-de-seo-san-tmdt", "tinh-gia-ban-san"}
	var choices []map[string]any
	for _, a := range preview.Agents {
		choice := map[string]any{"key": a.Key, "action": "create", "runtime_id": p.ws.RuntimeID, "name": a.Name + " " + p.suffix}
		if a.Key == "soan_listing_san_pham" {
			choice["skills"] = writerSkills
		}
		choices = append(choices, choice)
	}
	skillChoices := map[string]string{}
	for _, s := range preview.Skills {
		if s.ExistingSkillID != nil {
			skillChoices[s.Name] = "keep_existing"
		}
	}
	res := p.commit(preview, map[string]any{
		"squad":  map[string]any{"name": "Ban hang online " + p.suffix},
		"agents": choices, "skills": skillChoices,
		"leader": map[string]any{"generic": true, "runtime_id": p.ws.RuntimeID},
	})
	if res.SquadID == nil || len(res.Agents) != 8 {
		t.Fatalf("the install made %d agents (want 7 and the lead), squad %v", len(res.Agents), res.SquadID)
	}
	lead := p.agents["lead"]
	if lead == "" {
		t.Fatalf("no general lead among %v", p.agents)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM agent_skill WHERE agent_id = $1`, p.agents["soan_listing_san_pham"]); n != 3 {
		t.Fatalf("the writer has %d skills, want the 3 it was given", n)
	}

	// 3. The fake agent is asked to draft. It writes the flow of the skill day-chuyen-len-san-pham-moi and attaches it.
	rig := p.squad(lead)
	draft, issueID := rig.propose("Day chuyen len san pham moi", map[string]any{"note": "Hand over the flow of day-chuyen-len-san-pham-moi."})
	if draft.Origin != "proposal" {
		t.Fatalf("draft = %+v", draft)
	}
	rig.attach(issueID, "agent", lead, "flow.json", rehearsalFile(t, "ban_hang_flow.json"), time.Minute)
	var imported FlowDraftResponse
	rig.importProposal(nil, draft).Want(http.StatusOK).JSON(&imported)
	if imported.Revision != 2 {
		t.Fatalf("imported = %+v", imported)
	}

	// 4. The roles are bound by the person; the file carried none.
	importedDoc := draftDoc(t, imported)
	if len(asMaps(importedDoc["steps"])) != 5 {
		t.Fatalf("the flow has %d steps, want 5", len(asMaps(importedDoc["steps"])))
	}
	for _, role := range asMaps(importedDoc["roles"]) {
		if role["bind"] != nil {
			t.Fatalf("role %v came bound", role["key"])
		}
	}
	bound := p.bind(rig.draftRig, imported, map[string]string{
		"collector": p.agents["gom_du_lieu_shop"], "competitor": p.agents["tra_cuu_doi_thu_san"], "writer": p.agents["soan_listing_san_pham"],
		"words_gate": p.agents["soat_tu_cam_quang_cao_san"], "margin_gate": p.agents["soat_bien_loi_nhuan_shop"],
	})

	// 5. The check has no error, and the simulation reaches a terminal.
	var validation FlowValidationResponse
	rig.validateDraft(bound.ID, map[string]any{}).Want(http.StatusOK).JSON(&validation)
	if !validation.OK {
		t.Fatalf("validate: %+v", validation.Issues)
	}
	var trace FlowTraceResponse
	rig.call(testHandler.SimulateFlowDraft, nil, "POST", "/flow-drafts/"+bound.ID+"/simulate", map[string]any{}, "draftId", bound.ID).Want(http.StatusOK).JSON(&trace)
	if trace.End.Kind != "terminal" {
		t.Fatalf("the simulation ends %+v after %d hops", trace.End, len(trace.Hops))
	}

	// 6. The publish needs the reading of a proposal, and then goes through.
	body := wantCode(t, rig.publish(nil, bound, nil), http.StatusUnprocessableEntity, "flow_ack_required")
	if required, _ := body["required"].([]any); len(required) != 1 || required[0] != "proposal_reviewed" {
		t.Fatalf("required = %v", body["required"])
	}
	var wf IssueWorkflowResponse
	rig.publish(nil, bound, map[string]any{"ack": []string{"proposal_reviewed"}}).Want(http.StatusCreated).JSON(&wf)

	// 7. A trial run on that version, with the squad's active workflow left as it was.
	request := p.request(wf.ID, map[string]any{"flow_trial": true})
	if st := p.state(request); st.WorkflowID != wf.ID || !st.Trial || st.Step != "plan" {
		t.Fatalf("trial state %+v", st)
	}
	if got := squadActiveWorkflowID(t, p.squadID); got != "" {
		t.Fatalf("a trial made %s the squad's active workflow", got)
	}
	p.drive(request, importedDoc, approvePackGates)
	p.assertEnded(request, "done", map[string]int{"collect": 1, "draft": 1, "check": 1, "handover": 1})
	if n := tasksOfStep(t, request, "collect"); n != 2 {
		t.Fatalf("collect opened %d tasks, want 2", n)
	}

	// 8. The numbers of the version: one request, and it is a trial, so it shows only when trials are asked for.
	var report issueworkflow.Report
	rig.insights(nil, wf.ID, "?include_trial=true").Want(http.StatusOK).JSON(&report)
	if report.Requests.Total != 1 || report.Requests.ByEnd["terminal:done"] != 1 {
		t.Fatalf("insights = %+v", report.Requests)
	}
	if without := rig.report(wf.ID, ""); without.Requests.Total != 0 {
		t.Fatalf("a trial counts without being asked for: %+v", without.Requests)
	}
}

// assertEnded checks that a request is closed at a status and that it entered the steps as many times as said.
func (p *packRehearsal) assertEnded(request db.Issue, status string, entered map[string]int) {
	p.t.Helper()
	if !p.closed(request) || requestStatus(p.t, request) != status {
		p.t.Fatalf("the request is at %q, closed %v; want it closed at %q", requestStatus(p.t, request), p.closed(request), status)
	}
	state := p.state(request)
	for step, n := range entered {
		if state.Activations[step] != n {
			p.t.Errorf("step %q was entered %d times, want %d (activations %v)", step, state.Activations[step], n, state.Activations)
		}
	}
}

func TestRehearsalPack_SkillsOnly(t *testing.T) {
	p := newPackRehearsal(t)

	// 1. A bundle of skills and no agent; the three agents are declared while importing, each with its skills.
	preview := p.preview(packFixture(t, "skills_only"))
	if len(preview.Agents) != 0 || len(preview.Skills) != 3 {
		t.Fatalf("the bundle has %d agents and %d skills, want 0 and 3", len(preview.Agents), len(preview.Skills))
	}
	skills := map[string]string{}
	for _, s := range preview.Skills {
		skills[s.Name] = "keep_existing"
	}
	extra := func(key, name string, skillNames ...string) map[string]any {
		return map[string]any{"key": key, "name": name + " " + p.suffix, "instructions": "", "runtime_id": p.ws.RuntimeID, "skills": skillNames}
	}
	res := p.commit(preview, map[string]any{
		"squad": map[string]any{"name": "Skills only " + p.suffix},
		"extra_agents": []map[string]any{
			extra("outliner", "Outliner", "outline-first"),
			extra("drafter", "Drafter", "clear-brief"),
			extra("reviewer", "Reviewer", "data-check"),
		},
		"skills": skills,
		"leader": map[string]any{"generic": true, "runtime_id": p.ws.RuntimeID},
	})
	if res.SquadID == nil || len(res.Agents) != 4 {
		t.Fatalf("the install made %d agents, want the three and the lead", len(res.Agents))
	}
	for key, want := range map[string]int{"outliner": 1, "drafter": 1, "reviewer": 1} {
		if n := dbfx.Count(t, `SELECT count(*) FROM agent_skill WHERE agent_id = $1`, p.agents[key]); n != want {
			t.Fatalf("%s has %d skills, want %d", key, n, want)
		}
	}

	// 2. A blank draft, built through the API: two steps of work, a gate that a third agent holds, and a step for a person.
	rig := p.squad(p.agents["lead"])
	doc := baseDoc(
		[]any{
			map[string]any{"key": "outliner", "kind": "agent", "bind": map[string]any{"agent_ids": []string{p.agents["outliner"]}}},
			map[string]any{"key": "drafter", "kind": "agent", "bind": map[string]any{"agent_ids": []string{p.agents["drafter"]}}},
			map[string]any{"key": "reviewer", "kind": "agent", "bind": map[string]any{"agent_ids": []string{p.agents["reviewer"]}}},
		},
		docLead("outline"),
		docStep("outline", []any{docTask("outline_it", "work", "outliner", map[string]any{"skills": []any{"outline-first"}})}, docEdge("o_next", "write", nil)),
		docStep("write", []any{docTask("write_it", "work", "drafter", map[string]any{"skills": []any{"clear-brief"}})}, docEdge("w_next", "review", nil)),
		docStep("review", []any{docTask("cross_check", "gate", "reviewer", map[string]any{"skills": []any{"data-check"}})},
			docEdge("r_pass", "sign_off", joinIs("pass")),
			docStopEdge("r_fail", "loop_exhausted", joinIs("fail")),
			docStopEdge("r_block", "grounds_missing", joinIs("block")),
			docStopEdge("r_none", "undecided", nil)),
		map[string]any{
			"key": "sign_off", "title": "Sign off", "kind": "manual", "status": map[string]any{"key": "in_review"}, "exit_role": "managers",
			"notice": "stop2", "edges": []any{map[string]any{"id": "signed", "label": "Signed", "to": "done", "trigger": "manual"}},
		},
	)
	doc["policy"] = map[string]any{}
	steps := asMaps(doc["steps"])
	steps[1]["status"] = map[string]any{"key": "todo"}
	steps[2]["status"] = map[string]any{"key": "in_progress"}
	steps[3]["status"] = map[string]any{"key": "in_review"}

	// A skill the agent of the role does not have is refused at publish, with the agent and the skill named.
	wrong := rig.draftWith("Skills only wrong skill", copyDoc(t, doc))
	var wrongDoc map[string]any
	if err := json.Unmarshal(wrong.Doc, &wrongDoc); err != nil {
		t.Fatal(err)
	}
	asMaps(asMaps(wrongDoc["steps"])[2]["tasks"])[0]["skills"] = []any{"data-check"}
	rig.patchDraft(wrong.ID, map[string]any{"revision": wrong.Revision, "doc": wrongDoc}).Want(http.StatusOK)
	refused := wantCode(t, rig.publish(nil, rig.getDraft(wrong.ID), nil), http.StatusUnprocessableEntity, "flow_draft_invalid")
	if !strings.Contains(string(rawJSON(t, refused["issues"])), "skill_not_attached") {
		t.Fatalf("the publish was refused without naming the skill: %v", refused["issues"])
	}

	// The right skills publish.
	saved := rig.draftWith("Skills only flow v2", doc)
	var wf IssueWorkflowResponse
	rig.publish(nil, saved, nil).Want(http.StatusCreated).JSON(&wf)
	rig.setActive(wf.ID)

	// 3. A request on the active version, run to the end: the lead plans, the agents write, the gate passes, a person signs.
	request := p.request("")
	if st := p.state(request); st.WorkflowID != wf.ID || st.Trial {
		t.Fatalf("state %+v, want the active version and no trial", st)
	}
	p.drive(request, doc, approvePackGates)
	p.assertEnded(request, "done", map[string]int{"outline": 1, "write": 1, "review": 1, "sign_off": 1})
	var report issueworkflow.Report
	rig.insights(nil, wf.ID, "").Want(http.StatusOK).JSON(&report)
	if report.Requests.Total != 1 || report.Requests.ByEnd["terminal:done"] != 1 {
		t.Fatalf("insights = %+v", report.Requests)
	}
}

// copyDoc makes a copy of a document the test built.
func copyDoc(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rawJSON(t, doc), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// gatedNativeBundle is the native bundle of the fixtures with a third agent, a checker, and a flow whose check can send
// the work back: the files of the bundle are the fixture's, the manifest and the flow are the test's.
func gatedNativeBundle(t *testing.T) map[string]string {
	t.Helper()
	files := packFixture(t, "native_bundle")
	var manifest map[string]any
	if err := json.Unmarshal([]byte(files["squad-bundle.json"]), &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["id"] = "newsletter-desk-gated"
	manifest["agents"] = append(manifest["agents"].([]any), map[string]any{
		"key": "checker", "name": "Checker", "role_label": "Checks the issue", "description": "Approves the issue or sends it back",
		"instructions": "agents/checker/instructions.md", "skills": []any{"tone-check"}, "max_concurrent_tasks": 1,
	})
	files["squad-bundle.json"] = string(rawJSON(t, manifest))
	files["agents/checker/instructions.md"] = "Read the issue against the outline. Approve it, or reject it with the lines to fix.\n"
	files["flow.json"] = rehearsalFile(t, "native_gated_flow.json")
	return files
}

// installGatedNative installs the gated bundle with its flow as a draft, the roles bound by key.
func (p *packRehearsal) installGatedNative() (PackCommitResponse, PackPreviewResponse) {
	p.t.Helper()
	preview := p.preview(gatedNativeBundle(p.t))
	if !preview.Flow.Present || preview.Flow.Validation == nil || !preview.Flow.Validation.OK {
		p.t.Fatalf("the flow of the bundle: %+v", preview.Flow)
	}
	var choices []map[string]any
	for _, key := range []string{"planner", "writer", "checker"} {
		choices = append(choices, map[string]any{"key": key, "action": "create", "runtime_id": p.ws.RuntimeID, "name": strings.ToUpper(key[:1]) + key[1:] + " " + p.suffix})
	}
	skills := map[string]string{}
	for _, s := range preview.Skills {
		skills[s.Name] = "keep_existing"
	}
	res := p.commit(preview, map[string]any{
		"squad":  map[string]any{"name": "Gated desk " + p.suffix},
		"agents": choices, "skills": skills, "leader": map[string]any{"agent": "planner"},
		"create_draft_from_flow": true,
	})
	if res.DraftID == nil || len(res.FlowIssues) != 0 {
		p.t.Fatalf("the install made no draft of the flow: %+v", res)
	}
	return res, preview
}

func TestRehearsalPack_NativeBundleWithFlow(t *testing.T) {
	p := newPackRehearsal(t)
	res, _ := p.installGatedNative()

	// The draft of the flow has its roles bound by the key of the agents of the bundle, and nothing else is guessed.
	rig := p.squad(p.agents["planner"])
	draft := rig.getDraft(*res.DraftID)
	if draft.Origin != "pack" {
		t.Fatalf("draft origin %q", draft.Origin)
	}
	doc := draftDoc(t, draft)
	boundTo := map[string]string{}
	for _, role := range asMaps(doc["roles"]) {
		bind, _ := role["bind"].(map[string]any)
		ids, _ := bind["agent_ids"].([]any)
		if len(ids) != 1 {
			t.Fatalf("role %v is bound to %v", role["key"], bind)
		}
		boundTo[role["key"].(string)] = ids[0].(string)
	}
	if boundTo["writer"] != p.agents["writer"] || boundTo["checker"] != p.agents["checker"] {
		t.Fatalf("roles bound to %v, agents %v", boundTo, p.agents)
	}

	// It publishes as it is, and the squad runs it.
	var validation FlowValidationResponse
	rig.validateDraft(draft.ID, map[string]any{}).Want(http.StatusOK).JSON(&validation)
	if !validation.OK {
		t.Fatalf("validate: %+v", validation.Issues)
	}
	var wf IssueWorkflowResponse
	rig.publish(nil, draft, nil).Want(http.StatusCreated).JSON(&wf)
	rig.setActive(wf.ID)

	// A run in which the gate rejects once and then passes.
	request := p.request("")
	p.drive(request, doc, func(step, task string, activation int) string {
		if task == "check_it" && activation == 1 {
			return "rejected"
		}
		return "approved"
	})
	p.assertEnded(request, "done", map[string]int{"write": 2, "check": 2})
	if st := p.state(request); st.Loops["redo"] != 1 {
		t.Fatalf("loops %v, want one round of redo", st.Loops)
	}
	if n := tasksOfStep(t, request, "write"); n != 2 {
		t.Fatalf("write opened %d task issues, want 2 (one per round)", n)
	}
	var report issueworkflow.Report
	rig.insights(nil, wf.ID, "").Want(http.StatusOK).JSON(&report)
	if report.Requests.Total != 1 || len(report.Loops) != 1 || report.Loops[0].Key != "redo" || report.Loops[0].Max != 2 || report.Loops[0].Histogram[1] != 1 {
		t.Fatalf("insights = %+v loops %+v", report.Requests, report.Loops)
	}
	var taken int
	for _, e := range report.Edges {
		if e.Step == "check" && e.Edge == "check_fix" {
			taken = e.Taken
		}
	}
	if taken != 1 {
		t.Fatalf("the way back was taken %d times, want 1 (edges %+v)", taken, report.Edges)
	}
}

func TestRehearsalPack_TwoWorkflowsOneSquad(t *testing.T) {
	p := newPackRehearsal(t)
	res, _ := p.installGatedNative()
	rig := p.squad(p.agents["planner"])

	// Lineage one: the flow of the bundle, which the squad runs.
	var first IssueWorkflowResponse
	rig.publish(nil, rig.getDraft(*res.DraftID), nil).Want(http.StatusCreated).JSON(&first)
	rig.setActive(first.ID)

	// Lineage two: a draft of its own, with other steps and its own gate.
	doc := baseDoc(
		[]any{
			map[string]any{"key": "writer", "kind": "agent", "bind": map[string]any{"agent_ids": []string{p.agents["writer"]}}},
			map[string]any{"key": "checker", "kind": "agent", "bind": map[string]any{"agent_ids": []string{p.agents["checker"]}}},
		},
		docLead("brief"),
		docStep("brief", []any{docTask("brief_it", "work", "writer", nil)}, docEdge("b_next", "approve", nil)),
		docStep("approve", []any{docTask("approve_it", "gate", "checker", nil)},
			docEdge("a_pass", "done", joinIs("pass")),
			docStopEdge("a_fail", "loop_exhausted", joinIs("fail")),
			docStopEdge("a_block", "grounds_missing", joinIs("block")),
			docStopEdge("a_none", "undecided", nil)),
	)
	doc["policy"] = map[string]any{}
	steps := asMaps(doc["steps"])
	steps[1]["status"] = map[string]any{"key": "todo"}
	steps[2]["status"] = map[string]any{"key": "in_progress"}
	var second IssueWorkflowResponse
	rig.publish(nil, rig.draftWith("Brief flow", doc), nil).Want(http.StatusCreated).JSON(&second)
	if first.LineageID == nil || second.LineageID == nil || *first.LineageID == *second.LineageID {
		t.Fatalf("lineages %v and %v, want two", first.LineageID, second.LineageID)
	}

	// A request that names no version runs the active one; one that names the second runs that, and the squad keeps its own.
	onActive := p.request("")
	if got := pinnedWorkflow(t, uuidToString(onActive.ID)); got != first.ID {
		t.Fatalf("the plain request is on %q, want the active %s", got, first.ID)
	}
	chosen := p.request(second.ID)
	if got := pinnedWorkflow(t, uuidToString(chosen.ID)); got != second.ID {
		t.Fatalf("the chosen request is on %q, want %s", got, second.ID)
	}
	if got := squadActiveWorkflowID(t, p.squadID); got != first.ID {
		t.Fatalf("choosing a version moved the squad to %s", got)
	}

	p.drive(chosen, doc, approvePackGates)
	p.assertEnded(chosen, "done", map[string]int{"brief": 1, "approve": 1})
	if st := p.state(chosen); st.Activations["write"] != 0 || st.Activations["check"] != 0 {
		t.Fatalf("the chosen request went through steps of the other flow: %v", st.Activations)
	}

	// The other request is still where it was, on its own flow.
	if st := p.state(onActive); st.WorkflowID != first.ID || st.Step != "plan" || p.closed(onActive) {
		t.Fatalf("the plain request moved: %+v", st)
	}
	firstDoc := draftDoc(t, rig.getDraft(*res.DraftID))
	p.drive(onActive, firstDoc, approvePackGates)
	p.assertEnded(onActive, "done", map[string]int{"write": 1, "check": 1})

	// Each version counts its own request.
	for id, name := range map[string]string{first.ID: "first", second.ID: "second"} {
		var report issueworkflow.Report
		rig.insights(nil, id, "").Want(http.StatusOK).JSON(&report)
		if report.Requests.Total != 1 {
			t.Errorf("the %s version counts %d requests, want 1", name, report.Requests.Total)
		}
	}
}
