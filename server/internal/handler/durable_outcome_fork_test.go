package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// outcomeFixture is a workspace with a doer agent, a verifier agent, and a done issue
// assigned to the doer that has its outcome. Every row is registered for cleanup.
type outcomeFixture struct {
	fx       companyWorkspace
	wsfx     *testutil.Fixture
	runtime  string
	doer     string
	verifier string
	issue    string
	outcome  string
}

func newOutcomeFixture(t *testing.T) outcomeFixture {
	t.Helper()
	withFeatureFlag(t, testHandler, featureflags.DurableOutcomes, true)
	fx := newCompanyWorkspace(t)
	wsfx := testutil.New(testPool, fx.ID, fx.Owner)
	runtime := wsfx.Runtime(t, "outcome runtime")
	out := outcomeFixture{fx: fx, wsfx: wsfx, runtime: runtime, doer: wsfx.Agent(t, "outcome doer", runtime), verifier: wsfx.Agent(t, "outcome verifier", runtime)}

	out.issue = wsfx.Issue(t, "outcome issue", testutil.Cols{"status": "todo", "assignee_type": "agent", "assignee_id": out.doer})
	wsfx.Cleanup(t, `DELETE FROM outcome_verification WHERE outcome_id IN (SELECT id FROM work_outcome WHERE issue_id = $1)`, out.issue)
	wsfx.Cleanup(t, `DELETE FROM outcome_check_task WHERE outcome_id IN (SELECT id FROM work_outcome WHERE issue_id = $1)`, out.issue)
	wsfx.Cleanup(t, `DELETE FROM acceptance_criterion WHERE issue_id = $1`, out.issue)
	wsfx.Cleanup(t, `DELETE FROM agent_task_queue WHERE issue_id = $1`, out.issue)
	wsfx.Cleanup(t, `DELETE FROM work_outcome WHERE issue_id = $1`, out.issue)
	wsfx.Cleanup(t, `DELETE FROM issue_status_transition WHERE issue_id = $1`, out.issue)

	wsfx.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, out.issue)
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: true})
	svc := &service.OutcomeService{Queries: testHandler.Queries, TxStarter: testPool, Flags: featureflag.NewService(provider)}
	if _, err := svc.ProcessTransitions(context.Background()); err != nil {
		t.Fatalf("process transitions: %v", err)
	}
	wsfx.QueryRow(t, `SELECT id::text FROM work_outcome WHERE issue_id = $1 AND superseded_by IS NULL`, out.issue).Scan(&out.outcome)
	return out
}

// criterion writes a criterion on the issue as a member.
func (o outcomeFixture) criterion(t *testing.T, text string) string {
	t.Helper()
	req := companyReq(o.fx.Member, o.fx.ID, http.MethodPost, "/api/issues/"+o.issue+"/criteria",
		map[string]any{"text": text, "check_kind": "manual", "check_spec": map[string]any{}})
	req = testutil.WithURLParams(req, "id", o.issue)
	var out CriterionResponse
	testutil.Call(t, testHandler.CreateIssueCriterion, req).Want(http.StatusCreated).JSON(&out)
	return out.ID
}

// verifierTask gives the verifier a verification task for criterion, as the job would.
func (o outcomeFixture) verifierTask(t *testing.T, criterion string) string {
	t.Helper()
	task := o.wsfx.Task(t, o.verifier, testutil.Cols{"issue_id": o.issue, "status": "running", "runtime_id": o.runtime,
		"context": fmt.Sprintf(`{"durable_kind":"verification","outcome_id":"%s"}`, o.outcome)})
	o.wsfx.Exec(t, `INSERT INTO outcome_check_task (workspace_id, outcome_id, criterion_id, task_id) VALUES ($1, $2, $3, $4)`,
		o.fx.ID, o.outcome, criterion, task)
	return task
}

// agentReq is a request made by an agent through its task. Every request names a user,
// so the workspace owner stands in for the one the task runs for.
func agentReq(t *testing.T, ws, owner, agent, task, method, path string, body any) *http.Request {
	t.Helper()
	req := testutil.JSONRequest(method, path, body)
	return testutil.WithHeaders(req, "X-Workspace-ID", ws, "X-User-ID", owner, "X-Agent-ID", agent, "X-Task-ID", task)
}

func wantOutcomeCode(t *testing.T, resp *testutil.Response, status int, code string) {
	t.Helper()
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	resp.Want(status).JSON(&body)
	if body.Code != code {
		t.Fatalf("code = %q (%q), want %q", body.Code, body.Error, code)
	}
}

// TestOutcomeVerifyAgentOnlyThroughItsTask: an agent records a check only through the
// verification task it was given for that criterion, and the doer is refused.
func TestOutcomeVerifyAgentOnlyThroughItsTask(t *testing.T) {
	o := newOutcomeFixture(t)
	c := o.criterion(t, "The report has a header.")
	own := o.verifierTask(t, c)
	otherTask := o.wsfx.Task(t, o.verifier, testutil.Cols{"issue_id": o.issue, "status": "completed", "runtime_id": o.runtime})
	path := "/api/outcomes/" + o.outcome + "/verifications"
	body := map[string]any{"criterion_id": c, "result": "pass", "note": "header present"}

	req := testutil.WithURLParams(agentReq(t, o.fx.ID, o.fx.Owner, o.verifier, otherTask, http.MethodPost, path, body), "id", o.outcome)
	wantOutcomeCode(t, testutil.Call(t, testHandler.RecordOutcomeVerification, req), http.StatusForbidden, "not_verification_task")

	req = testutil.WithURLParams(agentReq(t, o.fx.ID, o.fx.Owner, o.verifier, own, http.MethodPost, path, body), "id", o.outcome)
	var v VerificationResponse
	testutil.Call(t, testHandler.RecordOutcomeVerification, req).Want(http.StatusCreated).JSON(&v)
	if v.VerifierType != "agent" || v.Result != "pass" || v.TaskID == nil || *v.TaskID != own {
		t.Fatalf("recorded check = %+v, want an agent pass through its task", v)
	}

	doerTask := o.wsfx.Task(t, o.doer, testutil.Cols{"issue_id": o.issue, "status": "completed", "runtime_id": o.runtime})
	req = testutil.WithURLParams(agentReq(t, o.fx.ID, o.fx.Owner, o.doer, doerTask, http.MethodPost, path, body), "id", o.outcome)
	wantOutcomeCode(t, testutil.Call(t, testHandler.RecordOutcomeVerification, req), http.StatusForbidden, "verifier_is_doer")
}

// TestOutcomeSettledRefusesCriterionEdit: once the outcome settles, a criterion can no
// longer be edited, and the refusal names outcome_settled.
func TestOutcomeSettledRefusesCriterionEdit(t *testing.T) {
	o := newOutcomeFixture(t)
	c := o.criterion(t, "The report has a header.")
	o.wsfx.Exec(t, `UPDATE work_outcome SET settle_at = now() - interval '1 minute' WHERE id = $1`, o.outcome)
	svc := &service.OutcomeService{Queries: testHandler.Queries, TxStarter: testPool, Flags: featureflag.NewService(onFlag())}
	if _, err := svc.Settle(context.Background(), time.Now()); err != nil {
		t.Fatalf("settle: %v", err)
	}
	text := "edited after settle"
	req := companyReq(o.fx.Member, o.fx.ID, http.MethodPatch, "/api/issues/"+o.issue+"/criteria/"+c, map[string]any{"text": text})
	req = testutil.WithURLParams(req, "id", o.issue, "cid", c)
	wantOutcomeCode(t, testutil.Call(t, testHandler.UpdateIssueCriterion, req), http.StatusConflict, "outcome_settled")
}

// onFlag is a flag provider with durable_outcomes on.
func onFlag() *featureflag.StaticProvider {
	p := featureflag.NewStaticProvider()
	p.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: true})
	return p
}

// TestCriterionEditPermissions: the creator may edit; another member may not; an admin may.
func TestCriterionEditPermissions(t *testing.T) {
	o := newOutcomeFixture(t)
	c := o.criterion(t, "The report has a header.")
	patch := func(user, text string) *testutil.Response {
		req := companyReq(user, o.fx.ID, http.MethodPatch, "/api/issues/"+o.issue+"/criteria/"+c, map[string]any{"text": text})
		return testutil.Call(t, testHandler.UpdateIssueCriterion, testutil.WithURLParams(req, "id", o.issue, "cid", c))
	}
	patch(o.fx.Owner, "x").Want(http.StatusOK)
	other := o.wsfx.User(t, "Other Member", "criterion-other-"+fmt.Sprint(time.Now().UnixNano())+"@example.com")
	o.wsfx.Member(t, o.fx.ID, other, "member")
	patch(other, "by a member who did not write it").Want(http.StatusForbidden)
	patch(o.fx.Admin, "by an admin").Want(http.StatusOK)
}

// TestOverrideNeedsAnOwnerOrAdminAndWritesAGovernanceEvent: a member is refused; an admin
// changes a settled conclusion, and the change is in the governance log.
func TestOverrideNeedsAnOwnerOrAdminAndWritesAGovernanceEvent(t *testing.T) {
	o := newOutcomeFixture(t)
	o.wsfx.Exec(t, `UPDATE work_outcome SET settle_at = now() - interval '1 minute' WHERE id = $1`, o.outcome)
	svc := &service.OutcomeService{Queries: testHandler.Queries, TxStarter: testPool, Flags: featureflag.NewService(onFlag())}
	if _, err := svc.Settle(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	path := "/api/outcomes/" + o.outcome + "/override"
	body := map[string]any{"result": "fail", "note": "the header was missing"}
	req := testutil.WithURLParams(companyReq(o.fx.Member, o.fx.ID, http.MethodPost, path, body), "id", o.outcome)
	testutil.Call(t, testHandler.OverrideOutcome, req).Want(http.StatusForbidden)

	req = testutil.WithURLParams(companyReq(o.fx.Admin, o.fx.ID, http.MethodPost, path, body), "id", o.outcome)
	var out OutcomeResponse
	testutil.Call(t, testHandler.OverrideOutcome, req).Want(http.StatusOK).JSON(&out)
	if out.Verdict != "fail" || out.FailCause != "member" {
		t.Fatalf("overridden outcome = %s/%s, want fail/member", out.Verdict, out.FailCause)
	}
	if n := o.wsfx.Count(t, `SELECT count(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'outcome_overridden' AND subject_id = $2`, o.fx.ID, o.outcome); n != 1 {
		t.Fatalf("governance events = %d, want one outcome_overridden", n)
	}
}

// TestOutcomeRoutesRefuseBadInput: a malformed body or a bad value is a 4xx that names
// what is wrong, as JSON, and never a 500.
func TestOutcomeRoutesRefuseBadInput(t *testing.T) {
	o := newOutcomeFixture(t)
	path := "/api/outcomes/" + o.outcome + "/verifications"

	const broken = `{"result":`
	bad := testutil.WithURLParams(companyReq(o.fx.Member, o.fx.ID, http.MethodPost, path, nil), "id", o.outcome)
	bad.Body = io.NopCloser(strings.NewReader(broken))
	bad.ContentLength = int64(len(broken))
	resp := testutil.Call(t, testHandler.RecordOutcomeVerification, bad).Want(http.StatusBadRequest)
	var errBody map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &errBody); err != nil || errBody["error"] == nil {
		t.Fatalf("malformed body reply = %q, want JSON with an error", resp.Text())
	}

	req := testutil.WithURLParams(companyReq(o.fx.Member, o.fx.ID, http.MethodPost, path, map[string]any{"result": "maybe"}), "id", o.outcome)
	wantOutcomeCode(t, testutil.Call(t, testHandler.RecordOutcomeVerification, req), http.StatusBadRequest, "verification_invalid")

	req = testutil.WithURLParams(companyReq(o.fx.Member, o.fx.ID, http.MethodPost, "/api/issues/"+o.issue+"/criteria",
		map[string]any{"text": "x", "check_kind": "agent_judgment", "check_spec": map[string]any{}}), "id", o.issue)
	wantOutcomeCode(t, testutil.Call(t, testHandler.CreateIssueCriterion, req), http.StatusBadRequest, "criterion_invalid")

	req = testutil.WithURLParams(companyReq(o.fx.Member, o.fx.ID, http.MethodGet, "/api/outcomes?cursor=bad!", nil), "id", o.outcome)
	testutil.Call(t, testHandler.ListWorkOutcomes, req).Want(http.StatusBadRequest)
}

// TestOutcomeReadsAreForMembers: a stranger to the workspace reads nothing, and a member
// reads the list, the issue's outcome and its criteria.
func TestOutcomeReadsAreForMembers(t *testing.T) {
	o := newOutcomeFixture(t)
	o.criterion(t, "The report has a header.")
	stranger := o.wsfx.User(t, "Stranger", "outcome-stranger-"+fmt.Sprint(time.Now().UnixNano())+"@example.com")
	testutil.Call(t, testHandler.ListWorkOutcomes, companyReq(stranger, o.fx.ID, http.MethodGet, "/api/outcomes", nil)).WantOneOf(http.StatusForbidden, http.StatusNotFound)

	var list OutcomeListResponse
	testutil.Call(t, testHandler.ListWorkOutcomes, companyReq(o.fx.Member, o.fx.ID, http.MethodGet, "/api/outcomes?agent_id="+o.doer, nil)).Want(http.StatusOK).JSON(&list)
	if len(list.Outcomes) != 1 || list.Outcomes[0].ID != o.outcome {
		t.Fatalf("list = %+v, want the one outcome of the doer", list)
	}
	var issueOutcome OutcomeResponse
	req := testutil.WithURLParams(companyReq(o.fx.Member, o.fx.ID, http.MethodGet, "/api/issues/"+o.issue+"/outcome", nil), "id", o.issue)
	testutil.Call(t, testHandler.GetIssueOutcome, req).Want(http.StatusOK).JSON(&issueOutcome)
	if issueOutcome.ID != o.outcome {
		t.Fatalf("issue outcome = %s, want %s", issueOutcome.ID, o.outcome)
	}
	var criteria struct {
		Criteria []CriterionResponse `json:"criteria"`
	}
	req = testutil.WithURLParams(companyReq(o.fx.Member, o.fx.ID, http.MethodGet, "/api/issues/"+o.issue+"/criteria", nil), "id", o.issue)
	testutil.Call(t, testHandler.ListIssueCriteria, req).Want(http.StatusOK).JSON(&criteria)
	if len(criteria.Criteria) != 1 || !strings.Contains(criteria.Criteria[0].Text, "header") {
		t.Fatalf("criteria = %+v, want the one criterion", criteria)
	}
}

// TestClaimBriefCarriesTheChecksNotTheVerdicts: a verifier's claim gets the checks to make
// and the command that records them, without another verifier's note. A doer's claim on the
// same issue gets the criteria it will be checked against, and not the verification brief.
func TestClaimBriefCarriesTheChecksNotTheVerdicts(t *testing.T) {
	o := newOutcomeFixture(t)
	// An agent_judgment criterion: the verifier checks it, so the briefing names it.
	c := o.wsfx.Insert(t, "acceptance_criterion", testutil.Cols{
		"workspace_id": o.fx.ID, "issue_id": o.issue, "text": "The report has a header.", "check_kind": "agent_judgment",
		"check_spec": `{"rubric":"Check the header."}`, "source": "member", "created_by_type": "member", "created_by_id": o.fx.Member,
	})
	o.wsfx.Exec(t, `INSERT INTO outcome_verification (workspace_id, outcome_id, criterion_id, verifier_type, verifier_id, result, note)
		VALUES ($1, $2, $3, 'agent', $4, 'fail', 'OTHER-VERDICT-TEXT')`, o.fx.ID, o.outcome, c, o.doer)
	ctx := context.Background()
	previousFlags := testHandler.FeatureFlags
	testHandler.FeatureFlags = featureflag.NewService(onFlag())
	t.Cleanup(func() { testHandler.FeatureFlags = previousFlags })

	verifierTask := db.AgentTaskQueue{ID: util.MustParseUUID(o.verifierTask(t, c)), IssueID: util.MustParseUUID(o.issue),
		Context: []byte(fmt.Sprintf(`{"durable_kind":"verification","outcome_id":"%s"}`, o.outcome))}
	resp := AgentTaskResponse{Agent: &TaskAgentData{Instructions: "persona"}}
	testHandler.applyDurableClaimBrief(ctx, verifierTask, &resp)
	brief := resp.Agent.Instructions
	if strings.Contains(brief, "OTHER-VERDICT-TEXT") {
		t.Fatalf("the verifier's briefing shows another verdict:\n%s", brief)
	}
	for _, want := range []string{"The report has a header.", "multica outcome verify", o.outcome, "persona"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("verifier briefing lacks %q:\n%s", want, brief)
		}
	}

	doerTask := db.AgentTaskQueue{ID: util.MustParseUUID(o.wsfx.Task(t, o.doer, testutil.Cols{"issue_id": o.issue, "status": "queued", "runtime_id": o.runtime})), IssueID: util.MustParseUUID(o.issue)}
	resp = AgentTaskResponse{Agent: &TaskAgentData{}}
	testHandler.applyDurableClaimBrief(ctx, doerTask, &resp)
	if !strings.Contains(resp.Agent.Instructions, "Tiêu chí nghiệm thu của issue này") || strings.Contains(resp.Agent.Instructions, "multica outcome verify") {
		t.Fatalf("doer briefing = %q, want the criteria and no verification brief", resp.Agent.Instructions)
	}
}

func TestOutcomePrivateProjectVisibility(t *testing.T) {
	o := newOutcomeFixture(t)
	o.criterion(t, "Private criterion")
	project := o.wsfx.Project(t, "Private outcome project", testutil.Cols{"visibility": "private"})
	o.wsfx.Exec(t, `UPDATE issue SET project_id=$1 WHERE id=$2 AND workspace_id=$3`, project, o.issue, o.fx.ID)
	o.wsfx.Exec(t, `INSERT INTO project_member (workspace_id, project_id, user_id) VALUES ($1,$2,$3)`, o.fx.ID, project, o.fx.Owner)
	o.wsfx.Cleanup(t, `DELETE FROM project_member WHERE workspace_id=$1 AND project_id=$2`, o.fx.ID, project)
	row := loadIssueRow(t, o.issue)
	identifier := fmt.Sprintf("%s-%d", testHandler.getIssuePrefix(context.Background(), row.WorkspaceID), row.Number)
	for _, ref := range []string{o.issue, identifier} {
		for _, route := range []struct {
			name    string
			handler http.HandlerFunc
		}{{"outcome", testHandler.GetIssueOutcome}, {"criteria", testHandler.ListIssueCriteria}} {
			t.Run(ref+"/"+route.name, func(t *testing.T) {
				path := "/api/issues/" + ref + "/" + route.name
				req := testutil.WithURLParams(companyReq(o.fx.Member, o.fx.ID, http.MethodGet, path, nil), "id", ref)
				testutil.Call(t, route.handler, req).WantOneOf(http.StatusForbidden, http.StatusNotFound)
				req = testutil.WithURLParams(companyReq(o.fx.Owner, o.fx.ID, http.MethodGet, path, nil), "id", ref)
				testutil.Call(t, route.handler, req).Want(http.StatusOK)
			})
		}
	}
	var hidden OutcomeListResponse
	testutil.Call(t, testHandler.ListWorkOutcomes, companyReq(o.fx.Member, o.fx.ID, http.MethodGet, "/api/outcomes?agent_id="+o.doer, nil)).Want(http.StatusOK).JSON(&hidden)
	for _, outcome := range hidden.Outcomes {
		if outcome.ID == o.outcome {
			t.Errorf("private outcome %s appeared in non-member list", o.outcome)
		}
	}
	var visible OutcomeListResponse
	testutil.Call(t, testHandler.ListWorkOutcomes, companyReq(o.fx.Owner, o.fx.ID, http.MethodGet, "/api/outcomes?agent_id="+o.doer, nil)).Want(http.StatusOK).JSON(&visible)
	if len(visible.Outcomes) != 1 || visible.Outcomes[0].ID != o.outcome {
		t.Errorf("project member outcomes = %+v, want %s", visible.Outcomes, o.outcome)
	}
}
