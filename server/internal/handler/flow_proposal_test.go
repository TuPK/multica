package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Proposals: an agent drafts a workflow from a written procedure and a person
// imports the file (plan P25, spec 16.4).

const proposalSecret = "sk-proposal-secret-0123456789abcdef"

// proposalRig is a draft rig with proposals on, an in-memory store for the files
// agents attach, and a drafting agent that is not on the squad.
type proposalRig struct {
	*draftRig
	store   *mockStorage
	drafter string
}

func newProposalRig(t *testing.T, agentCount int) *proposalRig {
	t.Helper()
	rig := newDraftRig(t, agentCount)
	packFlags(t, true, true, true)
	store := &mockStorage{}
	prev := testHandler.Storage
	testHandler.Storage = store
	t.Cleanup(func() { testHandler.Storage = prev })
	p := &proposalRig{draftRig: rig, store: store, drafter: goldenLeadAgent(t, rig.agents[0])}
	// Registered after the rig's cleanup, so it runs first and still finds the drafts.
	t.Cleanup(func() {
		ctx := context.Background()
		rows, err := testPool.Query(ctx, `SELECT source_ref->>'issue_id' FROM issue_workflow_draft
			WHERE squad_id = $1 AND origin = 'proposal' AND source_ref->>'issue_id' IS NOT NULL`, rig.squadID)
		if err != nil {
			return
		}
		var ids []string
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, id := range ids {
			testPool.Exec(ctx, `DELETE FROM agent_task_queue WHERE issue_id = $1`, id)
			testPool.Exec(ctx, `DELETE FROM attachment WHERE issue_id = $1`, id)
			testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, id)
		}
	})
	return p
}

func flowProposalBody(name, agentID string, extra map[string]any) map[string]any {
	from := map[string]any{"kind": "proposal", "agent_id": agentID, "references": []string{}, "note": "Review a draft, then publish it."}
	for k, v := range extra {
		from[k] = v
	}
	return map[string]any{"name": name, "from": from}
}

// propose asks the drafter for a workflow and returns the draft and the issue it made.
func (p *proposalRig) propose(name string, extra map[string]any) (FlowDraftResponse, string) {
	p.t.Helper()
	var d FlowDraftResponse
	p.createDraft(flowProposalBody(name, p.drafter, extra)).Want(http.StatusCreated).JSON(&d)
	return d, flowProposalIssueID(p.t, d)
}

func flowProposalIssueID(t *testing.T, d FlowDraftResponse) string {
	t.Helper()
	var ref struct {
		IssueID string `json:"issue_id"`
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(d.SourceRef, &ref); err != nil || ref.IssueID == "" {
		t.Fatalf("source_ref = %s (%v), want an issue id", d.SourceRef, err)
	}
	return ref.IssueID
}

// attach stores a file and attaches it to the issue as the uploader; age says how long ago.
func (p *proposalRig) attach(issueID, uploaderType, uploaderID, name, content string, age time.Duration) {
	p.t.Helper()
	key := "proposal-test/" + workflowTestSuffix() + "/" + name
	if _, err := p.store.Upload(context.Background(), key, []byte(content), "application/json", name); err != nil {
		p.t.Fatal(err)
	}
	cols := testutil.Cols{
		"workspace_id": testWorkspaceID, "uploader_type": uploaderType, "uploader_id": uploaderID,
		"filename": name, "url": p.store.ObjectURL(key), "content_type": "application/json",
		"size_bytes": len(content), "created_at": time.Now().Add(-age),
	}
	if issueID != "" {
		cols["issue_id"] = issueID
	}
	dbfx.Insert(p.t, "attachment", cols)
	p.t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM attachment WHERE url = $1`, p.store.ObjectURL(key))
	})
}

func (p *proposalRig) manifestFile(doc map[string]any) string {
	p.t.Helper()
	return string(rawJSON(p.t, p.manifestOf(doc)))
}

func (p *proposalRig) importProposal(as func(*http.Request) *http.Request, d FlowDraftResponse) *testutil.Response {
	p.t.Helper()
	return p.call(testHandler.ImportFlowProposal, as, "POST", "/flow-drafts/"+d.ID+"/import-proposal", map[string]any{"revision": d.Revision}, "draftId", d.ID)
}

func (p *proposalRig) agentActor(agent string) func(*http.Request) *http.Request {
	taskID := createHandlerTestTaskForAgent(p.t, agent)
	return func(r *http.Request) *http.Request {
		r.Header.Set("X-Actor-Source", "task_token")
		r.Header.Set("X-Agent-ID", agent)
		r.Header.Set("X-Task-ID", taskID)
		return r
	}
}

func issueText(t *testing.T, issueID, column string) string {
	t.Helper()
	var s *string
	dbfx.QueryRow(t, `SELECT `+column+` FROM issue WHERE id = $1`, issueID).Scan(&s)
	if s == nil {
		return ""
	}
	return *s
}

func TestProposal_CreatesDraftAndPlainIssue(t *testing.T) {
	p := newProposalRig(t, 2)
	d, issueID := p.propose("From a procedure", nil)

	if d.Origin != "proposal" || d.Revision != 1 || d.SquadID != p.squadID {
		t.Fatalf("draft = %+v", d)
	}
	doc, issues := issueworkflow.ParseFlowDoc(d.Doc)
	if len(issues) > 0 || len(doc.Steps) != 1 || doc.Steps[0].Key != "plan" {
		t.Fatalf("a proposal starts as the blank draft: %v, steps %d", issues, len(doc.Steps))
	}
	var ref map[string]any
	if err := json.Unmarshal(d.SourceRef, &ref); err != nil || ref["issue_id"] != issueID {
		t.Fatalf("source_ref = %s", d.SourceRef)
	}

	// The issue is an ordinary one: given to the drafting agent, on no flow, under no parent.
	var assigneeType, assigneeID string
	var flowIsNull, noParent bool
	dbfx.QueryRow(t, `SELECT assignee_type, assignee_id::text, flow IS NULL, parent_issue_id IS NULL FROM issue WHERE id = $1`, issueID).
		Scan(&assigneeType, &assigneeID, &flowIsNull, &noParent)
	if assigneeType != "agent" || assigneeID != p.drafter || !flowIsNull || !noParent {
		t.Fatalf("issue: assignee %s/%s, no flow %v, no parent %v", assigneeType, assigneeID, flowIsNull, noParent)
	}
	if issueText(t, issueID, "flow_task") != "" {
		t.Fatalf("the proposal issue is a task of a flow")
	}
	if !strings.Contains(issueText(t, issueID, "description"), "flow.json") {
		t.Fatalf("the issue has no brief")
	}

	// It is one draft among the squad's, and the capability says the route exists.
	var list struct {
		Total int `json:"total"`
	}
	p.call(testHandler.ListFlowDrafts, nil, "GET", "/flow-drafts", nil).Want(http.StatusOK).JSON(&list)
	if list.Total != 1 {
		t.Fatalf("drafts = %d, want 1", list.Total)
	}
	var caps FlowCapabilitiesResponse
	testutil.Call(t, testHandler.GetFlowCapabilities, newRequest("GET", "/api/flow/capabilities", nil)).Want(http.StatusOK).JSON(&caps)
	if !caps.Features.Proposals || !caps.Features.Packs {
		t.Fatalf("features = %+v, want proposals with packs on", caps.Features)
	}
}

func TestProposal_ReferencesAreOwnUploads(t *testing.T) {
	p := newProposalRig(t, 1)
	upload := func(uploaderType, uploaderID, name string) string {
		return dbfx.Insert(t, "attachment", testutil.Cols{
			"workspace_id": testWorkspaceID, "uploader_type": uploaderType, "uploader_id": uploaderID,
			"filename": name, "url": "https://files.example.test/" + name, "content_type": "text/markdown", "size_bytes": 10,
		})
	}
	mine := upload("member", testUserID, "procedure-"+workflowTestSuffix()+".md")
	var name string
	dbfx.QueryRow(t, `SELECT filename FROM attachment WHERE id = $1`, mine).Scan(&name)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM attachment WHERE id = $1`, mine) })

	_, issueID := p.propose("With a reference", map[string]any{"references": []string{mine}})
	var linkedTo string
	dbfx.QueryRow(t, `SELECT issue_id::text FROM attachment WHERE id = $1`, mine).Scan(&linkedTo)
	if linkedTo != issueID {
		t.Fatalf("the reference is attached to %s, want %s", linkedTo, issueID)
	}
	if !strings.Contains(issueText(t, issueID, "description"), name) {
		t.Fatalf("the brief does not list the reference %s", name)
	}

	// A file somebody else uploaded, one already attached elsewhere and one that is no id are refused.
	otherID, _ := addWorkflowTestMember(t, "refother")
	theirs := upload("member", otherID, "theirs-"+workflowTestSuffix()+".md")
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM attachment WHERE id = $1`, theirs) })
	for _, ref := range []string{theirs, mine, "nope"} {
		wantCode(t, p.createDraft(flowProposalBody("Bad reference", p.drafter, map[string]any{"references": []string{ref}})),
			http.StatusBadRequest, "flow_proposal_reference_invalid")
	}
}

func TestProposal_AgentMustBeAnAgentOfTheWorkspace(t *testing.T) {
	p := newProposalRig(t, 1)
	wantCode(t, p.createDraft(flowProposalBody("No agent", "", nil)), http.StatusBadRequest, "flow_proposal_agent_invalid")
	wantCode(t, p.createDraft(flowProposalBody("Unknown agent", "00000000-0000-4000-8000-000000000001", nil)), http.StatusBadRequest, "flow_proposal_agent_invalid")
	dbfx.Exec(t, `UPDATE agent SET archived_at = now() WHERE id = $1`, p.drafter)
	wantCode(t, p.createDraft(flowProposalBody("Archived agent", p.drafter, nil)), http.StatusBadRequest, "flow_proposal_agent_invalid")
	var n int
	dbfx.QueryRow(t, `SELECT COUNT(*) FROM issue_workflow_draft WHERE squad_id = $1`, p.squadID).Scan(&n)
	if n != 0 {
		t.Fatalf("a refused request left %d drafts", n)
	}
}

func TestProposal_AgentCanValidateButNotWriteDrafts(t *testing.T) {
	p := newProposalRig(t, 1)
	d, _ := p.propose("N7", nil)
	asAgent := p.agentActor(p.drafter)
	manifest := p.manifestOf(p.publishableDoc(p.agents[0]))

	// The one route of the family an agent may call, and it writes nothing.
	callValidateFlow(t, asAgent, map[string]any{"manifest": manifest, "squad_id": p.squadID}).Want(http.StatusOK)

	forbidden := func(name string, resp *testutil.Response) {
		t.Helper()
		body := resp.Want(http.StatusForbidden).Map()
		if body["code"] != "flow_authoring_member_only" {
			t.Errorf("%s: code = %v, want flow_authoring_member_only", name, body["code"])
		}
	}
	id := d.ID
	forbidden("list", p.call(testHandler.ListFlowDrafts, asAgent, "GET", "/flow-drafts", nil))
	forbidden("create blank", p.call(testHandler.CreateFlowDraft, asAgent, "POST", "/flow-drafts", map[string]any{"name": "By agent"}))
	forbidden("create proposal", p.call(testHandler.CreateFlowDraft, asAgent, "POST", "/flow-drafts", flowProposalBody("By agent", p.drafter, nil)))
	forbidden("get", p.call(testHandler.GetFlowDraft, asAgent, "GET", "/flow-drafts/"+id, nil, "draftId", id))
	forbidden("patch", p.call(testHandler.UpdateFlowDraft, asAgent, "PATCH", "/flow-drafts/"+id, map[string]any{"revision": 1, "doc": p.publishableDoc(p.agents[0])}, "draftId", id))
	forbidden("delete", p.call(testHandler.DeleteFlowDraft, asAgent, "DELETE", "/flow-drafts/"+id, nil, "draftId", id))
	forbidden("validate draft", p.call(testHandler.ValidateFlowDraft, asAgent, "POST", "/flow-drafts/"+id+"/validate", map[string]any{}, "draftId", id))
	forbidden("simulate", p.call(testHandler.SimulateFlowDraft, asAgent, "POST", "/flow-drafts/"+id+"/simulate", map[string]any{}, "draftId", id))
	forbidden("export", p.call(testHandler.ExportFlowDraft, asAgent, "GET", "/flow-drafts/"+id+"/export", nil, "draftId", id))
	forbidden("publish", p.publish(asAgent, d, nil))
	forbidden("import-proposal", p.importProposal(asAgent, d))

	// Nothing changed: the draft is as it was, no version exists, the squad has no active one.
	if got := p.getDraft(id); got.Revision != 1 || !sameJSON(got.Doc, d.Doc) || got.Name != d.Name {
		t.Fatalf("the draft changed: %+v", got)
	}
	var drafts, versions int
	dbfx.QueryRow(t, `SELECT COUNT(*) FROM issue_workflow_draft WHERE squad_id = $1`, p.squadID).Scan(&drafts)
	dbfx.QueryRow(t, `SELECT COUNT(*) FROM issue_workflow WHERE squad_id = $1`, p.squadID).Scan(&versions)
	if drafts != 1 || versions != 0 || squadActiveWorkflowID(t, p.squadID) != "" {
		t.Fatalf("drafts %d, versions %d, active %q", drafts, versions, squadActiveWorkflowID(t, p.squadID))
	}
}

func TestProposal_ImportReadsLatestAgentAttachment(t *testing.T) {
	p := newProposalRig(t, 1)
	d, issueID := p.propose("Latest file", nil)
	older := p.publishableDoc(p.agents[0])
	newest := p.publishableDoc(p.agents[0])
	asMaps(newest["steps"])[1]["title"] = "The newest file"

	p.attach(issueID, "agent", p.drafter, "flow.json", p.manifestFile(older), 3*time.Hour)
	p.attach(issueID, "agent", p.drafter, "notes.md", "# notes", time.Minute)
	p.attach(issueID, "agent", p.drafter, "intake.flow.json", p.manifestFile(newest), time.Hour)
	// A file whose name only contains the ending is not one.
	p.attach(issueID, "agent", p.drafter, "flow.json.bak", "not a manifest", time.Second)

	var imported FlowDraftResponse
	p.importProposal(nil, d).Want(http.StatusOK).JSON(&imported)
	if imported.Revision != 2 || imported.Origin != "proposal" || imported.ID != d.ID {
		t.Fatalf("imported = %+v", imported)
	}
	want := p.manifestOf(newest)["doc"]
	if !sameJSON(imported.Doc, rawJSON(t, want)) {
		t.Fatalf("the draft holds %s, want the newest file %s", imported.Doc, rawJSON(t, want))
	}
	if got := p.getDraft(d.ID); got.Revision != 2 || !sameJSON(got.Doc, rawJSON(t, want)) {
		t.Fatalf("stored draft = %+v", got)
	}
	// A stale revision is the usual conflict.
	wantCode(t, p.importProposal(nil, d), http.StatusConflict, "flow_draft_conflict")
	wantCode(t, p.call(testHandler.ImportFlowProposal, nil, "POST", "/flow-drafts/"+d.ID+"/import-proposal", map[string]any{}, "draftId", d.ID),
		http.StatusBadRequest, "flow_draft_revision_required")
}

func TestProposal_ImportIgnoresFilesFromOthers(t *testing.T) {
	p := newProposalRig(t, 1)
	d, issueID := p.propose("Others", nil)
	otherIssue := dbfx.Issue(t, "elsewhere "+workflowTestSuffix(), testutil.Cols{"status": "todo"})
	other := p.agents[0]

	mine := p.publishableDoc(p.agents[0])
	asMaps(mine["steps"])[1]["title"] = "From the drafting agent"
	foreign := p.manifestFile(p.publishableDoc(p.agents[0]))

	// Newer files from a person, from another agent, and from the drafter on another issue.
	p.attach(issueID, "member", testUserID, "flow.json", foreign, time.Minute)
	p.attach(issueID, "agent", other, "flow.json", foreign, 2*time.Minute)
	p.attach(otherIssue, "agent", p.drafter, "flow.json", foreign, 3*time.Minute)
	wantCode(t, p.importProposal(nil, d), http.StatusNotFound, "flow_proposal_missing")

	// The drafter's own file, older than all of them, is the one that counts.
	p.attach(issueID, "agent", p.drafter, "flow.json", p.manifestFile(mine), 5*time.Hour)
	var imported FlowDraftResponse
	p.importProposal(nil, d).Want(http.StatusOK).JSON(&imported)
	if !sameJSON(imported.Doc, rawJSON(t, p.manifestOf(mine)["doc"])) {
		t.Fatalf("the draft holds a file the drafting agent did not attach to the issue: %s", imported.Doc)
	}
}

func TestProposal_ImportStripsBind(t *testing.T) {
	p := newProposalRig(t, 1)
	d, issueID := p.propose("Bind", nil)
	doc := p.publishableDoc(p.agents[0]) // its roles are bound
	if asMaps(doc["roles"])[0]["bind"] == nil {
		t.Fatal("the fixture has no binding to strip")
	}
	file := string(rawJSON(t, map[string]any{"format": "multica.flow-manifest/v1", "name": "Bound", "doc": doc}))
	p.attach(issueID, "agent", p.drafter, "flow.json", file, time.Minute)

	var imported FlowDraftResponse
	p.importProposal(nil, d).Want(http.StatusOK).JSON(&imported)
	var stored struct {
		Roles []map[string]any `json:"roles"`
	}
	if err := json.Unmarshal(imported.Doc, &stored); err != nil || len(stored.Roles) != 2 {
		t.Fatalf("doc = %s (%v)", imported.Doc, err)
	}
	for _, role := range stored.Roles {
		if _, bound := role["bind"]; bound {
			t.Fatalf("role %v kept its binding: an agent does not choose who fills a role", role["key"])
		}
	}
	if got := p.getDraft(d.ID); strings.Contains(string(got.Doc), `"bind"`) {
		t.Fatalf("stored doc has a binding: %s", got.Doc)
	}
}

func TestProposal_InvalidManifestKeepsDraftUntouched(t *testing.T) {
	p := newProposalRig(t, 1)
	good := p.manifestOf(p.publishableDoc(p.agents[0]))
	badDoc := p.publishableDoc(p.agents[0])
	badDoc["version"] = 3
	cases := map[string]string{
		"not json":        "this is not JSON",
		"other format":    string(rawJSON(t, map[string]any{"format": "other/v1", "name": "x", "doc": good["doc"]})),
		"unknown key":     string(rawJSON(t, map[string]any{"format": "multica.flow-manifest/v1", "name": "x", "doc": good["doc"], "extra": 1})),
		"a bare document": string(rawJSON(t, good["doc"])),
		"unknown version": string(rawJSON(t, map[string]any{"format": "multica.flow-manifest/v1", "name": "x", "doc": badDoc})),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			d, issueID := p.propose("Invalid "+name, nil)
			p.attach(issueID, "agent", p.drafter, "flow.json", content, time.Minute)
			body := wantCode(t, p.importProposal(nil, d), http.StatusBadRequest, "flow_manifest_invalid")
			if issues, _ := body["issues"].([]any); len(issues) == 0 {
				t.Fatalf("the answer lists no issues: %v", body)
			}
			if got := p.getDraft(d.ID); got.Revision != d.Revision || !sameJSON(got.Doc, d.Doc) || got.Name != d.Name {
				t.Fatalf("a refused file changed the draft: %+v", got)
			}
		})
	}
}

func TestProposal_MissingFile404(t *testing.T) {
	p := newProposalRig(t, 1)
	d, _ := p.propose("Nothing yet", nil)
	wantCode(t, p.importProposal(nil, d), http.StatusNotFound, "flow_proposal_missing")
	if got := p.getDraft(d.ID); got.Revision != 1 {
		t.Fatalf("revision = %d after a missing file", got.Revision)
	}

	// A draft that is not a proposal has no issue to read.
	manual := p.newDraft("Manual")
	wantCode(t, p.importProposal(nil, manual), http.StatusBadRequest, "flow_draft_not_proposal")
	// No such draft.
	missing := FlowDraftResponse{ID: "00000000-0000-4000-8000-000000000001", Revision: 1}
	wantCode(t, p.importProposal(nil, missing), http.StatusNotFound, "flow_draft_not_found")

	// The issue was deleted: there is no file to read.
	_, gone := p.propose("Issue gone", nil)
	var list struct {
		Drafts []FlowDraftSummary `json:"drafts"`
	}
	p.call(testHandler.ListFlowDrafts, nil, "GET", "/flow-drafts", nil).Want(http.StatusOK).JSON(&list)
	for _, s := range list.Drafts {
		if s.Origin == "proposal" && s.Name == "Issue gone" {
			dbfx.Exec(t, `DELETE FROM issue WHERE id = $1`, gone)
			wantCode(t, p.importProposal(nil, p.getDraft(s.ID)), http.StatusNotFound, "flow_proposal_missing")
		}
	}
}

// proposalDoc is a document that publishes without a new status, so a member who is not an admin can publish it.
func (p *proposalRig) proposalDoc() map[string]any {
	doc := p.publishableDoc(p.agents[0])
	steps := asMaps(doc["steps"])
	steps[1]["status"] = map[string]any{"key": "in_progress"}
	steps[2]["status"] = map[string]any{"key": "in_review"}
	return doc
}

func TestPublish_ProposalNeedsReviewedAck(t *testing.T) {
	p := newProposalRig(t, 1)
	d, _ := p.propose("Needs a reading", nil)
	var saved FlowDraftResponse
	p.patchDraft(d.ID, map[string]any{"revision": d.Revision, "doc": p.proposalDoc()}).Want(http.StatusOK).JSON(&saved)

	// Without the acknowledgement the publish is refused, like any other that needs one.
	body := wantCode(t, p.publish(nil, saved, nil), http.StatusUnprocessableEntity, "flow_ack_required")
	if required, _ := body["required"].([]any); len(required) != 1 || required[0] != "proposal_reviewed" {
		t.Fatalf("required = %v, want [proposal_reviewed]", body["required"])
	}
	var versions int
	dbfx.QueryRow(t, `SELECT COUNT(*) FROM issue_workflow WHERE squad_id = $1`, p.squadID).Scan(&versions)
	if versions != 0 {
		t.Fatalf("a refused publish left %d versions", versions)
	}

	// A member who manages the squad without being an admin gives it, and it is kept in the version.
	manager := p.managerMember("proposalmgr")
	var wf IssueWorkflowResponse
	p.publish(manager, saved, map[string]any{"ack": []string{"proposal_reviewed"}}).Want(http.StatusCreated).JSON(&wf)
	var cfg struct {
		Policy struct {
			Acks []issueworkflow.Ack `json:"acks"`
		} `json:"policy"`
	}
	if err := json.Unmarshal([]byte(workflowFlowConfig(t, wf.ID)), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Policy.Acks) != 1 || cfg.Policy.Acks[0].Kind != "proposal_reviewed" || cfg.Policy.Acks[0].By == "" || cfg.Policy.Acks[0].By == testUserID {
		t.Fatalf("policy.acks = %+v, want proposal_reviewed given by the manager", cfg.Policy.Acks)
	}

	// Publishing a proposal turns nothing on: the version is not the squad's active one and its runner is off.
	if wf.FlowAuto || squadActiveWorkflowID(t, p.squadID) != "" {
		t.Fatalf("a published proposal is flow_auto %v, active %q; the person turns both on", wf.FlowAuto, squadActiveWorkflowID(t, p.squadID))
	}

	// The ack is asked of a proposal only: a draft of any other origin publishes without it.
	plain := p.draftWith("Plain", p.proposalDoc())
	p.publishOK(plain, nil)
	// An ack nobody asked for is not stored as a requirement on a manual draft, and an unknown one is refused.
	wantCode(t, p.publish(nil, plain, map[string]any{"ack": []string{"nonsense"}}), http.StatusBadRequest, "flow_ack_unknown")
}

func TestProposalBrief_ListsRosterSkillsAndOutputContract(t *testing.T) {
	brief := buildProposalBrief(proposalBrief{
		SquadID: "5f1c2b7e-0000-4000-8000-00000000aaaa", SquadName: "Customs desk", DraftName: "Intake",
		Members: []proposalBriefMember{
			{Key: "lead_agent", Name: "Lead Agent", Lead: true},
			{Key: "doc_checker", Name: "Doc Checker", Skills: []string{"pdf-reading", "customs-codes"}},
			{Key: "writer", Name: "Writer"},
		},
		References: []string{"procedure.md"},
		Note:       "Two checks before release.",
	})
	for _, want := range []string{
		"Customs desk",
		"lead_agent", "Lead Agent", "doc_checker", "Doc Checker", "writer",
		"pdf-reading", "customs-codes",
		"procedure.md", "Two checks before release.",
		"flow.json", "multica.flow-manifest/v1", "flow-authoring",
		"multica flow validate --file flow.json --squad 5f1c2b7e-0000-4000-8000-00000000aaaa",
	} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief lacks %q:\n%s", want, brief)
		}
	}
	// The lead is named as the lead and nobody else is.
	for _, line := range strings.Split(brief, "\n") {
		if strings.Contains(line, "doc_checker") && strings.Contains(strings.ToLower(line), "(lead") {
			t.Errorf("a worker is marked as the lead: %q", line)
		}
	}
	if !regexp.MustCompile(`(?i)lead_agent.*\(lead`).MatchString(brief) {
		t.Errorf("the brief does not say who leads:\n%s", brief)
	}
	if !strings.Contains(strings.ToLower(brief), "no error") {
		t.Errorf("the brief does not ask for a file without errors:\n%s", brief)
	}
}

var uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func TestProposal_DescriptionHasNoForeignAgentIDOrSecret(t *testing.T) {
	p := newProposalRig(t, 2)
	// Every agent carries something that must stay with it.
	for _, id := range append([]string{p.drafter, squadLeadID(t, p.squadID)}, p.agents...) {
		dbfx.Exec(t, `UPDATE agent SET instructions = $2, custom_env = $3::jsonb WHERE id = $1`,
			id, "Use the key "+proposalSecret, fmt.Sprintf(`{"API_KEY":"%s"}`, proposalSecret))
	}
	// An agent of the workspace that is not on the squad, with a name that would show.
	foreignName := "Foreign agent " + workflowTestSuffix()
	foreign := dbfx.Agent(t, foreignName, runtimeOf(t, p.agents[0]))
	dbfx.Exec(t, `UPDATE agent SET instructions = $2 WHERE id = $1`, foreign, "Secret "+proposalSecret)

	for _, drafter := range []string{p.drafter, foreign} {
		var d FlowDraftResponse
		p.createDraft(flowProposalBody("Brief for "+drafter[:8], drafter, nil)).Want(http.StatusCreated).JSON(&d)
		description := issueText(t, flowProposalIssueID(t, d), "description")
		if description == "" {
			t.Fatal("the proposal issue has no description")
		}
		for _, banned := range []string{foreign, foreignName, proposalSecret, "API_KEY"} {
			if strings.Contains(description, banned) {
				t.Errorf("the description contains %q:\n%s", banned, description)
			}
		}
		// The only identifier a brief carries is the squad's, for the check command.
		for _, id := range uuidPattern.FindAllString(description, -1) {
			if id != p.squadID {
				t.Errorf("the description names the id %s, which is not the squad's", id)
			}
		}
		if !strings.Contains(description, p.squadID) {
			t.Errorf("the check command lacks the squad id:\n%s", description)
		}
	}
}

func runtimeOf(t *testing.T, agentID string) string {
	t.Helper()
	var runtimeID string
	dbfx.QueryRow(t, `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID)
	return runtimeID
}

func TestProposal_FlagOff404(t *testing.T) {
	p := newProposalRig(t, 1)
	d, _ := p.propose("Before the flag goes", nil)

	packFlags(t, false, true, true)
	wantCode(t, p.createDraft(flowProposalBody("Off", p.drafter, nil)), http.StatusNotFound, "flow_authoring_disabled")
	wantCode(t, p.importProposal(nil, d), http.StatusNotFound, "flow_authoring_disabled")
	var caps FlowCapabilitiesResponse
	testutil.Call(t, testHandler.GetFlowCapabilities, newRequest("GET", "/api/flow/capabilities", nil)).Want(http.StatusOK).JSON(&caps)
	if caps.Features.Proposals || caps.Features.Packs {
		t.Fatalf("features = %+v, want proposals and packs off", caps.Features)
	}
	// Turning packs off leaves the rest of authoring alone.
	p.createDraft(map[string]any{"name": "Still manual"}).Want(http.StatusCreated)
	p.getDraft(d.ID)

	// Authoring off closes the new routes too, and with the same answer.
	packFlags(t, true, false, true)
	wantCode(t, p.createDraft(flowProposalBody("Off", p.drafter, nil)), http.StatusNotFound, "flow_authoring_disabled")
	wantCode(t, p.importProposal(nil, d), http.StatusNotFound, "flow_authoring_disabled")

	var n int
	dbfx.QueryRow(t, `SELECT COUNT(*) FROM issue_workflow_draft WHERE squad_id = $1 AND origin = 'proposal'`, p.squadID).Scan(&n)
	if n != 1 {
		t.Fatalf("proposal drafts = %d, want only the one made before", n)
	}
}
