package service

import (
	"context"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/util"
)

// TestLessonTaskAfterRetro: a retrospective that fires gives the agent that takes it a
// lesson task (durable spec 9.2, the retro hook). With the flag off it gives none.
func TestLessonTaskAfterRetro(t *testing.T) {
	for _, flagOn := range []bool{true, false} {
		df := newDossierFixture(t, flagOn)
		df.cleanLessonIssues(t)
		df.Cleanup(t, `DELETE FROM retro_trigger WHERE workspace_id = $1`, df.WorkspaceID)
		df.Cleanup(t, `DELETE FROM issue WHERE workspace_id = $1 AND title LIKE 'Retrospective:%'`, df.WorkspaceID)

		rs := &RetroService{
			Queries: df.q,
			Issues:  df.svc.Issues,
			Flags:   df.flags,
			Now:     func() time.Time { return time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC) },
		}
		c := RetroCandidate{
			Trigger:       RetroTriggerAgentFailures,
			SubjectType:   RetroSubjectAgent,
			SubjectID:     util.MustParseUUID(df.seat),
			Name:          "dossier seat",
			LeaderAgentID: util.MustParseUUID(df.seat),
			Evidence:      []string{"three failures in a week"},
		}
		fired, err := rs.fire(context.Background(), df.ws, c, rs.now())
		if err != nil {
			t.Fatalf("flag %v: fire: %v", flagOn, err)
		}
		if !fired {
			t.Fatalf("flag %v: retrospective did not fire", flagOn)
		}
		want := 0
		if flagOn {
			want = 1
		}
		if got := df.lessonIssues(t); got != want {
			t.Errorf("flag %v: lesson tasks = %d, want %d", flagOn, got, want)
		}
	}
}
