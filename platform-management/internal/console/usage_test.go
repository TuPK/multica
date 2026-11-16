package console

import (
	"encoding/json"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/usage"
)

func dayPtr(s string) *string { return &s }

func TestTenantListShowsUsageAndZerosForAWorkspaceWithNone(t *testing.T) {
	e := newEnv(t)
	e.reader.tenants = []TenantRow{
		{ID: wsID, Name: "With usage", Slug: "a", Status: "active", CreatedAt: t0, LastActivityAt: t0,
			Usage: usage.Snapshot{Day: dayPtr("2026-11-14"), Members: 3, Agents: 2, Runtimes: 1, IssuesTotal: 40, IssuesCreated: 5, TasksRun: 9, StorageBytes: 2048}},
		{ID: "33333333-3333-4333-8333-333333333333", Name: "No usage", Slug: "b", Status: "active", CreatedAt: t0, LastActivityAt: t0},
	}
	for _, who := range []string{suppID, billID, adminID} {
		rec := e.do(who, "GET", "/api/platform/tenants", "")
		wantStatus(t, rec, 200)
		var body struct {
			Tenants []map[string]any `json:"tenants"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Tenants) != 2 {
			t.Fatalf("%s: tenants = %d, want 2", who, len(body.Tenants))
		}
		with, none := body.Tenants[0], body.Tenants[1]
		want := map[string]any{
			"usage_day": "2026-11-14", "usage_members": 3.0, "usage_agents": 2.0, "usage_runtimes": 1.0,
			"usage_issues_total": 40.0, "usage_issues_created": 5.0, "usage_tasks_run": 9.0, "usage_storage_bytes": 2048.0,
		}
		for k, v := range want {
			if with[k] != v {
				t.Errorf("%s: %s = %v, want %v", who, k, with[k], v)
			}
		}
		if v, ok := none["usage_day"]; !ok || v != nil {
			t.Errorf("%s: usage_day = %v (present %v), want null", who, v, ok)
		}
		for k := range want {
			if k != "usage_day" && none[k] != 0.0 {
				t.Errorf("%s: no-usage %s = %v, want 0", who, k, none[k])
			}
		}
	}
}

func TestTenantDetailShowsLatestUsageAndCurrentCounts(t *testing.T) {
	e := newEnv(t)
	d := e.reader.details[wsID]
	d.Usage = usage.Snapshot{Day: dayPtr("2026-11-14"), Members: 3, TasksRun: 9}
	d.Current = usage.Current{Members: 4, Agents: 2, Runtimes: 1, IssuesTotal: 41, StorageBytes: 4096}
	e.reader.details[wsID] = d

	rec := e.do(suppID, "GET", "/api/platform/tenants/"+wsID, "")
	wantStatus(t, rec, 200)
	body := decodeJSON(t, rec)
	if body["usage_day"] != "2026-11-14" || body["usage_tasks_run"] != 9.0 {
		t.Errorf("latest usage = %v / %v", body["usage_day"], body["usage_tasks_run"])
	}
	// The block carries the numbers of usage.Current, under its own JSON names.
	want, _ := json.Marshal(d.Current)
	got, _ := json.Marshal(body["usage_current"])
	var wantMap, gotMap map[string]float64
	if err := json.Unmarshal(want, &wantMap); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &gotMap); err != nil {
		t.Fatal(err)
	}
	if len(gotMap) != 5 {
		t.Errorf("usage_current has %d numbers, want 5: %v", len(gotMap), gotMap)
	}
	for k, v := range wantMap {
		if gotMap[k] != v {
			t.Errorf("usage_current.%s = %v, want %v", k, gotMap[k], v)
		}
	}
	if gotMap["members"] != 4 || gotMap["storage_bytes"] != 4096 {
		t.Errorf("usage_current = %v", gotMap)
	}
}

func TestTenantDetailWithNoUsageRowIsNotAnError(t *testing.T) {
	e := newEnv(t)
	rec := e.do(suppID, "GET", "/api/platform/tenants/"+wsID, "")
	wantStatus(t, rec, 200)
	body := decodeJSON(t, rec)
	if v, ok := body["usage_day"]; !ok || v != nil {
		t.Errorf("usage_day = %v (present %v), want null", v, ok)
	}
	if cur, ok := body["usage_current"].(map[string]any); !ok || cur["members"] != 0.0 {
		t.Errorf("usage_current = %v, want zeros", body["usage_current"])
	}
}

func TestOverviewAddsUsageTotalsAndZerosWhenThereAreNone(t *testing.T) {
	e := newEnv(t)
	body := decodeJSON(t, e.do(suppID, "GET", "/api/platform/overview", ""))
	if v, ok := body["usage_day"]; !ok || v != nil {
		t.Errorf("usage_day = %v (present %v), want null", v, ok)
	}
	if body["usage_workspaces"] != 0.0 || body["usage_storage_bytes"] != 0.0 {
		t.Errorf("empty usage totals = %v / %v, want 0", body["usage_workspaces"], body["usage_storage_bytes"])
	}

	e.reader.overview = Overview{WorkspacesTotal: 2, Usage: usage.Totals{
		Workspaces: 2, Snapshot: usage.Snapshot{Day: dayPtr("2026-11-14"), Members: 7, TasksRun: 11, StorageBytes: 9000},
	}}
	body = decodeJSON(t, e.do(suppID, "GET", "/api/platform/overview", ""))
	if body["usage_day"] != "2026-11-14" || body["usage_workspaces"] != 2.0 || body["usage_members"] != 7.0 ||
		body["usage_tasks_run"] != 11.0 || body["usage_storage_bytes"] != 9000.0 {
		t.Errorf("usage totals = %v", body)
	}
}
