package console

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// Database test: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. The tenant detail carries the number of
// account groups and never their names (spec 7b.2, I6).
func TestDBTenantDetailCountsAccountGroupsWithoutTheirNames(t *testing.T) {
	f := newDBFixture(t)
	supportID, _ := f.staffUser(staff.RoleSupport)
	ownerID, _ := f.user("owner")
	memberID, _ := f.user("member")
	ws := f.workspace(ownerID, memberID)

	ctx := context.Background()
	for _, name := range []string{"Accounting-group-name", "Operations-group-name"} {
		if _, err := f.pool.Exec(ctx,
			`INSERT INTO member_group (workspace_id, name, created_by) VALUES ($1, $2, $3)`,
			ws, name, ownerID); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { f.pool.Exec(ctx, `DELETE FROM member_group WHERE workspace_id = $1`, ws) })

	rec := f.do(supportID, "GET", "/api/platform/tenants/"+ws, "")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["group_count"] != 2.0 {
		t.Errorf("group_count = %v, want 2", body["group_count"])
	}
	if strings.Contains(rec.Body.String(), "group-name") {
		t.Errorf("tenant detail names an account group: %s", rec.Body.String())
	}
}

// A workspace with no groups reads as zero, not as a missing field.
func TestDBTenantDetailReadsZeroGroupsForAWorkspaceWithNone(t *testing.T) {
	f := newDBFixture(t)
	supportID, _ := f.staffUser(staff.RoleSupport)
	ownerID, _ := f.user("owner")
	memberID, _ := f.user("member")
	ws := f.workspace(ownerID, memberID)

	rec := f.do(supportID, "GET", "/api/platform/tenants/"+ws, "")
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["group_count"] != 0.0 {
		t.Errorf("group_count = %v, want 0", body["group_count"])
	}
}
