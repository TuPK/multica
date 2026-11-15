package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// C12: the Trade rehearsal runs the same on a flow a person published from a copy of
// Trade. The copy is the catalog flow forked to a schema 2 draft, published as it is
// and made the active workflow of the squad; the happy path opens as many sub-issues
// as it does on the catalog flow and ends in review.
func TestRehearsal_Trade_ForkPublishedByAPersonRunsTheSameRehearsal(t *testing.T) {
	f := newRehearsalFixture(t)
	withFlowFlags(t, true, true)

	// The catalog flow, as the happy path of TestRehearsal_Trade_HappyPath runs it.
	v1Request := f.newRequest(t)
	gates := f.driveToGate(t, v1Request)
	finishTask(t, f.toHandover(t, v1Request, gates), f.Agents.Handover, "approved")
	advance(t, v1Request)
	want := countTasks(t, v1Request)
	if st := requestStatus(t, v1Request); st != "in_review" {
		t.Fatalf("the catalog request is at %q, want in_review", st)
	}

	rig := &draftRig{t: t, squadID: f.SquadID, agents: f.Agents.all()}
	t.Cleanup(func() {
		ctx := context.Background()
		testPool.Exec(ctx, `UPDATE squad SET active_workflow_id = NULL WHERE id = $1`, f.SquadID)
		testPool.Exec(ctx, `DELETE FROM issue_workflow_draft WHERE squad_id = $1`, f.SquadID)
		testPool.Exec(ctx, `DELETE FROM issue_workflow WHERE squad_id = $1`, f.SquadID)
	})
	var draft FlowDraftResponse
	rig.createDraft(map[string]any{"name": "My Trade copy", "from": map[string]any{"kind": "workflow", "workflow_id": f.Workflow.ID}}).
		Want(http.StatusCreated).JSON(&draft)
	fork := rig.publishOK(draft, map[string]any{"ack": []string{"allow_unattended_release"}})
	if fork.ID == f.Workflow.ID || fork.TemplateRef != nil {
		t.Fatalf("the published copy is %+v", fork)
	}
	dbfx.Exec(t, `UPDATE squad SET active_workflow_id = $2 WHERE id = $1`, f.SquadID, fork.ID)

	id := dbfx.Issue(t, "Trade copy request "+workflowTestSuffix(), testutil.Cols{
		"status": "todo", "assignee_type": "squad", "assignee_id": f.SquadID,
	})
	request, err := testHandler.EnsureFlowStarted(context.Background(), loadIssueRow(t, id))
	if err != nil || len(request.Flow) == 0 {
		t.Fatalf("EnsureFlowStarted: flow %q, err %v", request.Flow, err)
	}
	if _, err := testHandler.CompleteFlowStep(context.Background(), loadIssueRow(t, id), "", FlowCompleteInput{}); err != nil {
		t.Fatalf("CompleteFlowStep: %v", err)
	}
	// Every task that opens is closed by its agent: gates and the handover approve, the rest are done.
	for i := 0; i < 12 && requestStatus(t, request) != "in_review"; i++ {
		rows, err := testPool.Query(context.Background(), `SELECT id::text, flow_task, assignee_id::text FROM issue
			WHERE parent_issue_id = $1 AND status NOT IN ('done', 'cancelled')`, request.ID)
		if err != nil {
			t.Fatal(err)
		}
		type open struct{ id, key, agent string }
		var opened []open
		for rows.Next() {
			var o open
			if err := rows.Scan(&o.id, &o.key, &o.agent); err != nil {
				t.Fatal(err)
			}
			opened = append(opened, o)
		}
		rows.Close()
		for _, o := range opened {
			verdict := ""
			if o.key == "docs_gate" || o.key == "customs_gate" || o.key == "handover" {
				verdict = "approved"
			}
			finishTask(t, o.id, o.agent, verdict)
		}
		advance(t, request)
	}
	if st := requestStatus(t, request); st != "in_review" {
		t.Fatalf("the request on the copy is at %q, want in_review", st)
	}
	if got := countTasks(t, request); got != want {
		t.Fatalf("the copy opened %d sub-issues, the catalog flow %d", got, want)
	}
}
