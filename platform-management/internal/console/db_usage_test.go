package console

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Database test: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it.
func TestDBTenantListShowsWorkspacesWithAndWithoutAUsageRow(t *testing.T) {
	f := newDBFixture(t)
	supportID, _ := f.staffUser(staff.RoleSupport)
	ownerID, _ := f.user("owner")
	memberID, _ := f.user("member")
	withRow := f.workspace(ownerID, memberID)
	withoutRow := f.workspace(ownerID, memberID)

	ctx := context.Background()
	for _, day := range []string{"2026-11-13", "2026-11-14"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO platform_usage_daily
			(workspace_id, day, members, storage_bytes) VALUES ($1, $2::date, $3, $4)`,
			withRow, day, 2, 2048); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { f.pool.Exec(ctx, `DELETE FROM platform_usage_daily WHERE workspace_id = $1`, withRow) })

	rec := f.do(supportID, "GET", "/api/platform/tenants?limit=200", "")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Tenants []map[string]any `json:"tenants"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	byID := map[string]map[string]any{}
	for _, tn := range body.Tenants {
		byID[tn["id"].(string)] = tn
	}
	with, without := byID[withRow], byID[withoutRow]
	if with == nil || without == nil {
		t.Fatalf("list lacks a workspace: with=%v without=%v", with != nil, without != nil)
	}
	if with["usage_day"] != "2026-11-14" || with["usage_members"] != 2.0 || with["usage_storage_bytes"] != 2048.0 {
		t.Errorf("workspace with rows shows %v, want the newest row", with)
	}
	if v, ok := without["usage_day"]; !ok || v != nil || without["usage_members"] != 0.0 {
		t.Errorf("workspace with no row shows %v, want a null day and zeros", without)
	}

	detail := decodeJSON(t, f.do(supportID, "GET", "/api/platform/tenants/"+withoutRow, ""))
	cur, _ := detail["usage_current"].(map[string]any)
	if cur["members"] != 2.0 {
		t.Errorf("usage_current = %v, want the 2 members counted now", detail["usage_current"])
	}
	if detail["usage_day"] != nil {
		t.Errorf("detail usage_day = %v, want null", detail["usage_day"])
	}
}
