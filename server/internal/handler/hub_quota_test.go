package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// newHubQuotaFixture is a workspace with an owner and n hub items of the given sizes.
func newHubQuotaFixture(t *testing.T, sizes ...int64) (string, string) {
	t.Helper()
	ws := dbfx.Workspace(t, hubTestLabel(t, "workspace"), hubTestSlug(t))
	owner := dbfx.User(t, hubTestLabel(t, "owner"), hubTestEmail(t))
	dbfx.Member(t, ws, owner, hubRoleOwner)
	for _, size := range sizes {
		dbfx.Insert(t, "hub_item", testutil.Cols{
			"workspace_id": ws,
			"kind":         "skill",
			"ref_id":       hubTestUUID(t),
			"tracking":     "manual",
			"size_bytes":   size,
		})
	}
	return ws, owner
}

func TestHubQuota_ReportsUsageAndLimits(t *testing.T) {
	t.Setenv(hubMaxItemsEnv, "2")
	t.Setenv(hubMaxStorageBytesEnv, "100")
	ws, owner := newHubQuotaFixture(t, 30, 40)

	var resp HubQuotaResponse
	testutil.Call(t, testHandler.GetHubQuota,
		hubTestRequest(http.MethodGet, "/api/hub/quota", nil, ws, owner)).Want(http.StatusOK).JSON(&resp)
	if resp.Items != (HubQuotaMeter{Used: 2, Limit: 2}) {
		t.Fatalf("items = %+v, want used 2 limit 2", resp.Items)
	}
	if resp.StorageBytes != (HubQuotaMeter{Used: 70, Limit: 100}) {
		t.Fatalf("storage_bytes = %+v, want used 70 limit 100", resp.StorageBytes)
	}
}

func TestHubQuota_CheckRefusesWhatOverflows(t *testing.T) {
	t.Setenv(hubMaxItemsEnv, "2")
	t.Setenv(hubMaxStorageBytesEnv, "100")
	ws, _ := newHubQuotaFixture(t, 30, 40)
	ctx := context.Background()
	wsID := parseUUID(ws)

	// One more item would make three of two.
	err := testHandler.checkHubQuota(ctx, wsID, 1, 0)
	var quotaErr *HubQuotaError
	if !errors.As(err, &quotaErr) || quotaErr.Resource != hubQuotaResourceItems {
		t.Fatalf("items check = %v, want an items HubQuotaError", err)
	}
	rec := httptest.NewRecorder()
	if !writeHubQuotaError(rec, err) {
		t.Fatalf("writeHubQuotaError did not recognize its own error")
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode 409 body: %v", err)
	}
	if body["code"] != "quota_exceeded" || body["error"] == "" {
		t.Fatalf("409 body = %v, want code quota_exceeded with a message", body)
	}

	// Storage: 70 used, 31 more is 101 against a limit of 100.
	err = testHandler.checkHubQuota(ctx, wsID, 0, 31)
	if !errors.As(err, &quotaErr) || quotaErr.Resource != hubQuotaResourceStorage {
		t.Fatalf("storage check = %v, want a storage HubQuotaError", err)
	}

	// Exactly at the limit is allowed: the test is used+add > limit.
	if err := testHandler.checkHubQuota(ctx, wsID, 0, 30); err != nil {
		t.Fatalf("storage at the limit = %v, want nil", err)
	}
	if err := testHandler.checkHubQuota(ctx, wsID, 0, 0); err != nil {
		t.Fatalf("no addition = %v, want nil", err)
	}
}

func TestHubQuota_ZeroOrNegativeMeansUnlimited(t *testing.T) {
	t.Setenv(hubMaxItemsEnv, "0")
	t.Setenv(hubMaxStorageBytesEnv, "-5")
	ws, owner := newHubQuotaFixture(t, 10)

	var resp HubQuotaResponse
	testutil.Call(t, testHandler.GetHubQuota,
		hubTestRequest(http.MethodGet, "/api/hub/quota", nil, ws, owner)).Want(http.StatusOK).JSON(&resp)
	if resp.Items.Limit != 0 || resp.StorageBytes.Limit != 0 {
		t.Fatalf("limits = %+v / %+v, want 0 (unlimited)", resp.Items, resp.StorageBytes)
	}
	if err := testHandler.checkHubQuota(context.Background(), parseUUID(ws), 100000, 1<<40); err != nil {
		t.Fatalf("unlimited check = %v, want nil", err)
	}
}

func TestHubQuota_InvalidEnvUsesDefaults(t *testing.T) {
	t.Setenv(hubMaxItemsEnv, "lots")
	t.Setenv(hubMaxStorageBytesEnv, "")
	limits := hubQuotaLimitsFromEnv()
	if limits.Items != hubDefaultMaxItems || limits.StorageBytes != hubDefaultMaxStorageBytes {
		t.Fatalf("limits = %+v, want the defaults", limits)
	}
}

func TestHubQuota_WriteErrorIgnoresOtherErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	if writeHubQuotaError(rec, errors.New("database unavailable")) {
		t.Fatalf("a non-quota error was reported as a quota refusal")
	}
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("a non-quota error wrote a response: %d %q", rec.Code, rec.Body.String())
	}
}
