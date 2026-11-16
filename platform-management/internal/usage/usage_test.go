package usage

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
)

func pgUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatal(err)
	}
	return u
}

const wsA = "11111111-1111-4111-8111-111111111111"
const wsB = "22222222-2222-4222-8222-222222222222"

func TestAWorkspaceWithNoRowShowsZerosAndANullDay(t *testing.T) {
	rows := []db.ListLatestUsageDailyRow{
		{WorkspaceID: pgUUID(t, wsA), Day: pgtype.Date{Time: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC), Valid: true},
			Members: 3, Agents: 2, Runtimes: 1, IssuesTotal: 40, IssuesCreated: 5, TasksRun: 9, StorageBytes: 2048},
	}
	got := snapshotsByWorkspace(rows)

	withRow := got[wsA]
	if withRow.Day == nil || *withRow.Day != "2026-11-14" || withRow.Members != 3 || withRow.Agents != 2 ||
		withRow.Runtimes != 1 || withRow.IssuesTotal != 40 || withRow.IssuesCreated != 5 || withRow.TasksRun != 9 || withRow.StorageBytes != 2048 {
		t.Fatalf("snapshot with a row = %+v", withRow)
	}

	// A workspace with no row is absent from the map, so its lookup is the zero
	// value, which serialises as zeros and a null day.
	none := got[wsB]
	raw, err := json.Marshal(none)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if v, ok := m["usage_day"]; !ok || v != nil {
		t.Errorf("usage_day = %v (present %v), want null", v, ok)
	}
	for _, key := range []string{"usage_members", "usage_agents", "usage_runtimes", "usage_issues_total",
		"usage_issues_created", "usage_tasks_run", "usage_storage_bytes"} {
		if v, ok := m[key]; !ok || v != float64(0) {
			t.Errorf("%s = %v (present %v), want 0", key, v, ok)
		}
	}
}

func TestTotalsOfNoRowsAreZeroWithANullDay(t *testing.T) {
	raw, err := json.Marshal(Totals{}.Map())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if v, ok := m["usage_day"]; !ok || v != nil {
		t.Errorf("usage_day = %v, want null", v)
	}
	if v := m["usage_workspaces"]; v != float64(0) {
		t.Errorf("usage_workspaces = %v, want 0", v)
	}
	if v := m["usage_storage_bytes"]; v != float64(0) {
		t.Errorf("usage_storage_bytes = %v, want 0", v)
	}
}

// The current counts come from content tables, so the queries that read them
// may only count and sum. Nothing in them selects a column of content.
func TestQueriesOnContentTablesOnlyCountAndSum(t *testing.T) {
	src, err := os.ReadFile("../../queries/usage.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := regexp.MustCompile(`--[^\n]*`).ReplaceAllString(string(src), " ")
	content := regexp.MustCompile(`(?i)\bFROM\s+(issue|agent|agent_runtime|attachment)\b`)
	hits := content.FindAllStringIndex(text, -1)
	if len(hits) != 4 {
		t.Fatalf("usage.sql reads %d content tables, want 4 (agent, agent_runtime, issue, attachment)", len(hits))
	}
	aggregate := regexp.MustCompile(`(?is)SELECT\s+(count\(\*\)|COALESCE\(sum\((\w+\.)?size_bytes\),\s*0\))\s*$`)
	for _, h := range hits {
		before := text[:h[0]]
		if !aggregate.MatchString(before) {
			t.Errorf("content table read is not a bare count or sum: ...%s", before[max(0, len(before)-60):]+text[h[0]:h[1]])
		}
	}
	if strings.Contains(strings.ToLower(text), "select *") || strings.Contains(strings.ToLower(text), ".*") {
		t.Error("usage.sql selects every column of a table")
	}
}

func TestEveryLatestQueryFiltersByWorkspace(t *testing.T) {
	src, err := os.ReadFile("../../queries/usage.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ListLatestUsageDaily", "GetCurrentWorkspaceCounts"} {
		var block string
		for _, b := range strings.Split(string(src), "-- name: ")[1:] {
			if strings.HasPrefix(b, name+" ") {
				block = b
			}
		}
		if block == "" {
			t.Fatalf("query %s not found", name)
		}
		if !regexp.MustCompile(`workspace_id\s*(=|= ANY)\s*\(?\$1`).MatchString(block) {
			t.Errorf("%s has no workspace_id filter:\n%s", name, block)
		}
	}
}
