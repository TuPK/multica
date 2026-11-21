package handler

// Viewer-triggered writes that reach UpdateIssueStatus (flow engine, proposal
// decision, initiative transitions) and restricted projects (spec 7.1 rules
// 3, 5). The outsider is a plain member not granted the restricted project; a
// refusal must come from visibility, so the outsider is also the squad member
// the role checks let in.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// TestRestrictedFlowAndProposalWritesAnswerMissing: the flow routes, the
// proposal decision and the initiative transitions answer the outsider like
// the same call on an issue or initiative that does not exist, and leave the
// issue status as it was.
func TestRestrictedFlowAndProposalWritesAnswerMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	// A flow request in a restricted project: loadIssueForUser hides it.
	t.Run("flow request in a restricted project", func(t *testing.T) {
		f := newRestrictedFixture(t)
		fx := newFlowFixture(t)
		dbfx.Exec(t, `UPDATE issue SET project_id = $1 WHERE id = $2`, f.restrictedProjectID, fx.Request)
		missingID := uuid.NewString()
		before := issueStatusOf(t, fx.Request)
		routes := []struct {
			name    string
			handler http.HandlerFunc
			method  string
			path    string
			body    any
		}{
			{"PlanIssueFlow", testHandler.PlanIssueFlow, "POST", "/flow/plan", map[string]any{"verdict": "ok"}},
			{"AdvanceIssueFlow", testHandler.AdvanceIssueFlow, "POST", "/flow/advance", nil},
			{"SetIssueFlowVar", testHandler.SetIssueFlowVar, "PUT", "/flow/vars/export", map[string]any{"value": true}},
			{"CompleteIssueFlowStep", testHandler.CompleteIssueFlowStep, "POST", "/flow/steps/plan/complete", map[string]any{"verdict": "ok"}},
			{"ExitIssueFlow", testHandler.ExitIssueFlow, "POST", "/flow/exits/default", nil},
			{"CloseIssueFlowTask", testHandler.CloseIssueFlowTask, "POST", "/flow/close", map[string]any{"verdict": "ok"}},
			{"ResumeIssueFlow", testHandler.ResumeIssueFlow, "POST", "/flow/resume", map[string]any{}},
			{"DetachIssueFlow", testHandler.DetachIssueFlow, "POST", "/flow/detach", map[string]any{"cancel_tasks": true}},
			{"StartIssueFlow", testHandler.StartIssueFlow, "POST", "/flow/start", map[string]any{}},
		}
		for _, route := range routes {
			call := func(id string) *httptest.ResponseRecorder {
				req := requestAs(f.outsiderID, route.method, "/api/issues/"+id+route.path, route.body)
				return serveAs(route.handler, withURLParams(req, "id", id, "key", "export", "step", "plan", "edgeId", "default"))
			}
			missing := call(missingID)
			if missing.Code != http.StatusNotFound {
				t.Fatalf("%s on a missing issue: want 404, got %d %s", route.name, missing.Code, missing.Body.String())
			}
			sameAnswer(t, route.name+" as outsider", call(fx.Request), missing, fx.Request, missingID)
		}
		if got := issueStatusOf(t, fx.Request); got != before {
			t.Errorf("a flow write of the outsider moved the request from %s to %s", before, got)
		}
	})

	// A flow task outside any project whose request is in a restricted one: the
	// task is visible, the request it names is not.
	t.Run("flow task of a hidden request", func(t *testing.T) {
		f := newRestrictedFixture(t)
		fx := newFlowFixture(t)
		dbfx.Exec(t, `UPDATE issue SET project_id = $1 WHERE id = $2`, f.restrictedProjectID, fx.Request)
		missingID := uuid.NewString()
		before := issueRowSnapshot(t, fx.Request)
		taskBefore := issueRowSnapshot(t, fx.Work)
		routes := []struct {
			name    string
			handler http.HandlerFunc
			method  string
			path    string
			body    any
		}{
			{"GetIssueFlow", testHandler.GetIssueFlow, "GET", "/flow", nil},
			{"SetIssueFlowVar", testHandler.SetIssueFlowVar, "PUT", "/flow/vars/export", map[string]any{"value": true}},
			{"CloseIssueFlowTask", testHandler.CloseIssueFlowTask, "POST", "/flow/close", map[string]any{"verdict": "ok"}},
		}
		for _, route := range routes {
			call := func(id string) *httptest.ResponseRecorder {
				req := requestAs(f.outsiderID, route.method, "/api/issues/"+id+route.path, route.body)
				return serveAs(route.handler, withURLParams(req, "id", id, "key", "export"))
			}
			missing := call(missingID)
			if missing.Code != http.StatusNotFound {
				t.Fatalf("%s on a missing issue: want 404, got %d %s", route.name, missing.Code, missing.Body.String())
			}
			sameAnswer(t, route.name+" as outsider", call(fx.Work), missing, fx.Work, missingID)
		}
		if after := issueRowSnapshot(t, fx.Request); after != before {
			t.Errorf("a flow write of the outsider changed the hidden request:\nbefore: %s\nafter:  %s", before, after)
		}
		if after := issueRowSnapshot(t, fx.Work); after != taskBefore {
			t.Errorf("a flow write of the outsider changed the task:\nbefore: %s\nafter:  %s", taskBefore, after)
		}
	})

	// A proposal decision is for an owner or admin, who see every project, so a
	// member who cites a restricted blocked issue cannot reach it.
	t.Run("proposal decision is admin only", func(t *testing.T) {
		f := newRestrictedFixture(t)
		fx := newProposalTestFixture(t)
		blocked := dbfx.Issue(t, fx.title("restricted waiting"), testutil.Cols{
			"status": "blocked", "project_id": f.restrictedProjectID,
			"assignee_type": "agent", "assignee_id": fx.LeaderID,
		})
		p := fx.fileProposal(t, "cites restricted", map[string]any{"evidence": []string{"mention://issue/" + blocked}})
		for _, decision := range []string{"approve", "reject"} {
			req := asUser(decideRequest(p.ID, map[string]any{"decision": decision, "note": "go"}), f.outsiderID)
			testutil.Call(t, testHandler.DecideProposal, req).Want(http.StatusForbidden)
		}
		if got := issueStatusOf(t, blocked); got != "blocked" {
			t.Errorf("a refused decision released the restricted issue to %s", got)
		}
	})

	// A plain member of the owning squad takes initiative transitions; the
	// backing issue sits in a restricted project the member does not see.
	t.Run("initiative transitions", func(t *testing.T) {
		fx := newIniWorkspace(t)
		restricted := dbfx.Project(t, "Restricted initiative "+uuid.NewString()[:8], testutil.Cols{"workspace_id": fx.ID, "visibility": "private"})
		dbfx.SquadMember(t, fx.Product, "member", fx.MemberID, testutil.Cols{"role": "member"})
		created := fx.createInitiative(t, "Hidden initiative")
		var issueID string
		dbfx.QueryRow(t, `SELECT issue_id FROM initiative WHERE id = $1`, created.ID).Scan(&issueID)
		dbfx.Exec(t, `UPDATE issue SET project_id = $1 WHERE id = $2`, restricted, issueID)
		fx.linkStage(t, created.ID, "validation_report", "competitor_profile")
		fx.makeAutoExit(t, 0)
		missingID := uuid.NewString()
		before := issueRowSnapshot(t, issueID)

		routes := []struct {
			name    string
			handler http.HandlerFunc
		}{
			{"AdvanceInitiative", testHandler.AdvanceInitiative},
			{"AttestInitiative", testHandler.AttestInitiative},
			{"SendBackInitiative", testHandler.SendBackInitiative},
			{"ShelveInitiative", testHandler.ShelveInitiative},
			{"ResumeInitiative", testHandler.ResumeInitiative},
			{"CloseInitiative", testHandler.CloseInitiative},
			{"ForceInitiative", testHandler.ForceInitiative},
			{"WaiveInitiativeArtifact", testHandler.WaiveInitiativeArtifact},
		}
		for _, route := range routes {
			call := func(id string) *httptest.ResponseRecorder {
				req := fx.as(newRequest("POST", "/api/initiatives/"+id,
					map[string]any{"note": "go", "reason": "go", "to_stage": "build", "gate_days": []int{30}}), fx.MemberID)
				return serveAs(route.handler, withURLParams(req, "id", id, "kind", "validation_report"))
			}
			missing := call(missingID)
			if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "initiative not found") {
				t.Fatalf("%s on a missing initiative: want 404 initiative not found, got %d %s", route.name, missing.Code, missing.Body.String())
			}
			sameAnswer(t, route.name+" as outsider", call(created.ID), missing, created.ID, missingID)
		}
		if after := issueRowSnapshot(t, issueID); after != before {
			t.Errorf("an initiative write of the outsider changed the backing issue:\nbefore: %s\nafter:  %s", before, after)
		}
		var stage string
		dbfx.QueryRow(t, `SELECT stage_key FROM initiative WHERE id = $1`, created.ID).Scan(&stage)
		if stage != "validate" {
			t.Errorf("the initiative moved to stage %s", stage)
		}
	})
}
