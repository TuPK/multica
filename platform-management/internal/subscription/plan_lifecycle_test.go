package subscription

import (
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// Spec R6 section 3b.1, rule 7: history lists who, when, the changed fields with
// old and new values, and the reason when there is one.
func TestPlanHistoryListsWhoWhenFieldsOldAndNew(t *testing.T) {
	at := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	rows := []plan.HistoryRow{{
		At: at, Action: "plan.changed", ActorLabel: "root@example.test",
		Detail: map[string]any{
			"changes": map[string]any{
				"name":        map[string]any{"from": "Pro", "to": "Pro Plus"},
				"description": map[string]any{"from": "", "to": "For teams"},
			},
			"reason": "rename",
		},
	}}
	got := planHistoryEntries(rows)
	if len(got) != 1 || got[0].Actor != "root@example.test" || got[0].Action != "plan.changed" || !got[0].At.Equal(at) {
		t.Fatalf("entry = %+v", got)
	}
	if got[0].Reason != "rename" {
		t.Fatalf("reason = %q", got[0].Reason)
	}
	want := []historyChange{
		{Field: "description", Old: "", New: "For teams"},
		{Field: "name", Old: "Pro", New: "Pro Plus"},
	}
	if len(got[0].Changes) != len(want) {
		t.Fatalf("changes = %+v", got[0].Changes)
	}
	for i := range want {
		if got[0].Changes[i] != want[i] {
			t.Fatalf("changes[%d] = %+v, want %+v", i, got[0].Changes[i], want[i])
		}
	}
}

func TestPlanHistoryRowWithoutChangesIsEmptyList(t *testing.T) {
	got := planHistoryEntries([]plan.HistoryRow{{Action: "plan.activated", Detail: map[string]any{}}})
	if got[0].Changes == nil || len(got[0].Changes) != 0 {
		t.Fatalf("changes must be an empty list, got %#v", got[0].Changes)
	}
}

// Spec R6 section 3b.1 rule 5: activating an active plan and deactivating an
// archived one change nothing and write no audit row.
func TestDeactivateAndActivateAreIdempotent(t *testing.T) {
	e := newConsoleEnv(t)
	wantStatus(t, e.do(adminUser, "PUT", "/api/platform/plans/team", teamBody), 200)
	steps := []struct {
		route   string
		changed bool
	}{
		{"deactivate", true}, {"deactivate", false}, {"activate", true}, {"activate", false},
	}
	for _, s := range steps {
		rec := e.do(adminUser, "POST", "/api/platform/plans/team/"+s.route, "")
		wantStatus(t, rec, 200)
		if got := decodeMap(t, rec)["changed"]; got != s.changed {
			t.Fatalf("%s: changed = %v, want %v", s.route, got, s.changed)
		}
	}
	var deactivated, activated int
	for _, a := range e.plans.audits {
		switch a.Action {
		case audit.ActionPlanDeactivated:
			deactivated++
		case audit.ActionPlanActivated:
			activated++
		}
	}
	if deactivated != 1 || activated != 1 {
		t.Fatalf("audit rows: deactivated=%d activated=%d, want 1 each", deactivated, activated)
	}
}
