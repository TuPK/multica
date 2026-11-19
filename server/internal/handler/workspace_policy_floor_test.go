package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/workspaceguard"
)

// requirePolicyDB skips the test when the package has no database.
func requirePolicyDB(t *testing.T) { requireTenantDB(t) }

// floorTenant puts ws in a tenant whose floor requires one of the sign-in
// rules (column is require_sso or require_mfa), removed with the test.
func floorTenant(t *testing.T, ws, column string) string {
	t.Helper()
	tenantID := tenantFixture(t, ws)
	dbfx.Exec(t, `INSERT INTO tenant_policy (tenant_id, `+column+`) VALUES ($1::uuid, true)`, tenantID)
	dbfx.Cleanup(t, `DELETE FROM tenant_policy WHERE tenant_id = $1::uuid`, tenantID)
	return tenantID
}

// The floor holds for a workspace with no policy row and for a plan without the
// feature: the tenant's floor is not a plan feature (spec R6 D101).
func TestEffectivePolicyAppliesTenantFloorToEveryWorkspaceRegardlessOfPlan(t *testing.T) {
	requirePolicyDB(t)
	ws := policyWorkspace(t)
	floorTenant(t, ws, "require_sso")
	for _, available := range []bool{true, false} {
		setPolicyFeature(t, ws, available)
		got, err := workspaceguard.EffectivePolicy(context.Background(), testHandler.Queries, testHandler.Entitlements, parseUUID(ws))
		if err != nil {
			t.Fatalf("EffectivePolicy: %v", err)
		}
		if !got.RequireSSO {
			t.Errorf("plan feature %v: RequireSSO = false, want the tenant floor to apply", available)
		}
	}
}

// A workspace below the floor is refused with the fields that would loosen it.
func TestWorkspacePolicyWriteBelowFloorIs400WithFields(t *testing.T) {
	requirePolicyDB(t)
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	floorTenant(t, ws, "require_mfa")

	resp := doPolicy(t, http.MethodPut, ws, testUserID, map[string]any{"require_mfa": false})
	wantPolicyCode(t, resp, http.StatusBadRequest, "policy_below_tenant_floor")
	var body struct {
		Fields []string `json:"fields"`
	}
	resp.JSON(&body)
	if len(body.Fields) != 1 || body.Fields[0] != "require_mfa" {
		t.Errorf("fields = %v, want [require_mfa]", body.Fields)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM workspace_policy WHERE workspace_id = $1::uuid`, ws); n != 0 {
		t.Errorf("a refused policy was stored: %d rows", n)
	}
}

// A policy stored before the floor was set is tightened when it is enforced.
func TestStoredLooseWorkspacePolicyIsTightenedAtEnforcement(t *testing.T) {
	requirePolicyDB(t)
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	putPolicy(t, ws, map[string]any{"require_sso": false})
	floorTenant(t, ws, "require_sso")

	got, err := workspaceguard.EffectivePolicy(context.Background(), testHandler.Queries, testHandler.Entitlements, parseUUID(ws))
	if err != nil {
		t.Fatalf("EffectivePolicy: %v", err)
	}
	if !got.RequireSSO {
		t.Error("the stored loose policy was enforced without the tenant floor")
	}
}

// The read returns the floor so a console can show what the workspace cannot loosen.
func TestGetWorkspacePolicyReturnsFloor(t *testing.T) {
	requirePolicyDB(t)
	ws := policyWorkspace(t)
	setPolicyFeature(t, ws, true)
	floorTenant(t, ws, "require_mfa")

	var body struct {
		Floor *struct {
			RequireMFA bool `json:"require_mfa"`
		} `json:"floor"`
	}
	doPolicy(t, http.MethodGet, ws, testUserID, nil).Want(http.StatusOK).JSON(&body)
	if body.Floor == nil || !body.Floor.RequireMFA {
		t.Errorf("floor = %+v, want require_mfa true", body.Floor)
	}
}

// Deleting the tenant's last workspace removes the tenant's floor row.
func TestDetachTenantDeletesPolicyRow(t *testing.T) {
	requirePolicyDB(t)
	ws := policyWorkspace(t)
	tenantID := floorTenant(t, ws, "require_mfa")

	teardown(t, ws)
	if n := dbfx.Count(t, `SELECT count(*) FROM tenant_policy WHERE tenant_id = $1::uuid`, tenantID); n != 0 {
		t.Errorf("tenant_policy rows = %d after the tenant went, want 0", n)
	}
}
