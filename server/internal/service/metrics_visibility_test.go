package service

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Stored aggregates (metric readings and scorecard inputs) are read by every
// member, so they cannot be filtered per viewer. They leave out issues of
// restricted projects instead (spec 7.1 rule 5, decision 2026-11-19).

// visibilityDoneIssues seeds two squad issues done yesterday, both reopened
// once: one in a workspace project and one in a restricted project.
func (f govFixture) visibilityDoneIssues(t *testing.T) (yesterday time.Time) {
	t.Helper()
	yesterday = f.now.AddDate(0, 0, -1)
	at := yesterday.Add(15 * time.Hour)
	openProj := f.Project(t, "Open metrics project", testutil.Cols{"access": "workspace"})
	hiddenProj := f.Project(t, "Hidden metrics project", testutil.Cols{"access": "restricted"})
	for _, p := range []struct{ title, project string }{
		{"visible done", openProj},
		{"hidden done", hiddenProj},
	} {
		f.Issue(t, p.title, testutil.Cols{
			"assignee_type": "squad", "assignee_id": f.squad, "status": "done", "project_id": p.project,
			"updated_at": at, "done_at": at, "last_reopened_at": at, "reopen_count": 1,
		})
	}
	return yesterday
}

func TestMetricsVisibility_StoredReadingsExcludeRestrictedProject(t *testing.T) {
	f := newGovFixture(t)
	f.visibilityDoneIssues(t)

	if _, err := f.metricsService().RollupBuiltinMetrics(context.Background(), f.ws, f.now); err != nil {
		t.Fatalf("rollup: %v", err)
	}
	got := f.readingsOf(t, ScopeSquad, f.squad)
	if got["delivery.issues_done"] != 1 {
		t.Errorf("delivery.issues_done = %v, want 1 (the restricted-project issue is excluded)", got["delivery.issues_done"])
	}
	// Both issues were reopened; only the visible one counts, so rework is 1 of 1.
	if got["delivery.rework_rate"] != 100 {
		t.Errorf("delivery.rework_rate = %v, want 100", got["delivery.rework_rate"])
	}
}

func TestMetricsVisibility_ReopenedCountExcludesRestrictedProject(t *testing.T) {
	f := newGovFixture(t)
	yesterday := f.visibilityDoneIssues(t)
	reopened, err := f.q.BuiltinIssuesReopened(context.Background(), db.BuiltinIssuesReopenedParams{
		WorkspaceID: f.ws,
		SquadIds:    []pgtype.UUID{f.squadPG(t)},
		Since:       pgtype.Timestamptz{Time: yesterday, Valid: true},
		Until:       pgtype.Timestamptz{Time: f.now, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reopened != 1 {
		t.Errorf("reopened = %d, want 1 (the restricted-project issue is excluded)", reopened)
	}
}

func TestScorecardVisibility_InputsExcludeRestrictedProject(t *testing.T) {
	f := newGovFixture(t)
	yesterday := f.visibilityDoneIssues(t)
	scope := ScorecardScope{SubjectType: ScorecardSubjectSquad, SubjectID: f.squadPG(t), SquadID: f.squadPG(t)}
	in, err := ScorecardInputsFor(context.Background(), f.q, f.ws, scope, yesterday, f.now, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Done) != 1 {
		t.Errorf("scorecard done inputs = %d, want 1 (the restricted-project issue is excluded)", len(in.Done))
	}
}
