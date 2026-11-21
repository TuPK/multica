package service

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// An other_issue wakeup condition must not confirm or watch an issue its
// creator cannot see (spec 7.1 rules 4 and 5): the issue is unknown.

type wakeupVisibility struct {
	f       principalFixture
	s       *IssueWakeupService
	issue   pgtype.UUID // the issue the rule sits on, in no project
	agent   string      // private agent owned by the creator
	creator string      // plain member with no project_member row
	hidden  string      // issue of a restricted project
	open    string      // issue of a workspace project
}

func newWakeupVisibility(t *testing.T) wakeupVisibility {
	t.Helper()
	f, s, issue, _ := conditionFixture(t)
	creator := f.member(t, "wake-creator")
	restricted := f.Project(t, "Restricted wake project", testutil.Cols{"visibility": "private"})
	workspaceProject := f.Project(t, "Open wake project", testutil.Cols{"visibility": "workspace"})
	hidden := f.Issue(t, "Hidden watched", testutil.Cols{"status": "in_review", "project_id": restricted})
	open := f.Issue(t, "Open watched", testutil.Cols{"status": "in_review", "project_id": workspaceProject})
	return wakeupVisibility{f: f, s: s, issue: issue, agent: f.privateAgentOwnedBy(t, creator, "wake-vis"),
		creator: creator, hidden: hidden, open: open}
}

func (v wakeupVisibility) create(t *testing.T, other string) (pgtype.UUID, error) {
	t.Helper()
	w, err := v.s.Create(context.Background(), v.issue, parseTestUUID(t, v.creator), pgtype.UUID{}, WakeupInput{
		AgentID: v.agent, Kind: "event", Instruction: "Continue after review",
		Condition: condition(t, map[string]any{"type": "other_issue", "issue_id": other, "state": "in_review"}),
	})
	return w.ID, err
}

func TestWakeupVisibility_ValidateConditionHidesRestrictedIssue(t *testing.T) {
	v := newWakeupVisibility(t)
	if _, err := v.create(t, v.hidden); !containsUnknown(err) {
		t.Fatalf("hidden other issue: err = %v, want unknown issue", err)
	}
	if n := v.f.Count(t, "SELECT count(*) FROM issue_wakeup WHERE issue_id=$1", v.issue); n != 0 {
		t.Fatalf("a rule was stored for a hidden issue: %d rows", n)
	}
	// A missing id answers the same way.
	_, missingErr := v.create(t, "00000000-0000-4000-8000-000000000001")
	if missingErr == nil || !containsUnknown(missingErr) {
		t.Fatalf("missing issue: err = %v", missingErr)
	}
	// An issue the creator sees is accepted.
	if _, err := v.create(t, v.open); err != nil {
		t.Fatalf("visible other issue: %v", err)
	}
}

func TestWakeupVisibility_EvaluateConditionTreatsHiddenIssueAsUnknown(t *testing.T) {
	v := newWakeupVisibility(t)
	id, err := v.create(t, v.open)
	if err != nil {
		t.Fatal(err)
	}
	// The watched issue moves into a restricted project after the rule exists.
	restricted := v.f.Project(t, "Late restricted project", testutil.Cols{"visibility": "private"})
	v.f.Exec(t, "UPDATE issue SET project_id=$1 WHERE id=$2", restricted, v.open)
	w, err := v.f.q.LocklessWakeup(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := v.f.svc.TaskSvc.TxStarter.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	met, _, observed, err := v.s.evaluateCondition(context.Background(), tx, v.f.q.WithTx(tx), w)
	if err != nil {
		t.Fatal(err)
	}
	if met {
		t.Errorf("a hidden issue satisfied the condition: observed=%v", observed)
	}
	if _, leaked := observed["status"]; leaked {
		t.Errorf("observed value carries the hidden issue's status: %v", observed)
	}
}

func containsUnknown(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unknown issue")
}
