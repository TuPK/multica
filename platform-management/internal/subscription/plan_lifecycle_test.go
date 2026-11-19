package subscription

import (
	"testing"
	"time"

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
