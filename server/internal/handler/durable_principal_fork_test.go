package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// TestPrincipalProfileRoutes covers the principal profile over HTTP (durable spec 6.6, 14.4): the
// routes answer 404 while durable_principal is off, except the person's own delete; consent is
// required; and a trait that cites nothing the person wrote is refused with 422.
func TestPrincipalProfileRoutes(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	previous := testHandler.FeatureFlags
	t.Cleanup(func() {
		testHandler.FeatureFlags = previous
		testPool.Exec(context.Background(), `DELETE FROM principal_trait WHERE workspace_id = $1`, testWorkspaceID)
		testPool.Exec(context.Background(), `DELETE FROM principal_profile WHERE workspace_id = $1`, testWorkspaceID)
		testPool.Exec(context.Background(), `DELETE FROM governance_event WHERE workspace_id = $1 AND kind LIKE 'principal_%'`, testWorkspaceID)
	})
	flags := func(on bool) *featureflag.Service {
		p := featureflag.NewStaticProvider()
		p.Set(featureflags.DurablePrincipal, featureflag.Rule{Default: on})
		return featureflag.NewService(p)
	}
	profilePath := "/api/principal/profile"

	t.Run("off: reads and writes answer 404", func(t *testing.T) {
		testHandler.FeatureFlags = flags(false)
		testutil.Call(t, testHandler.GetPrincipalProfile, newRequest("GET", profilePath, nil)).Want(http.StatusNotFound)
		testutil.Call(t, testHandler.EnablePrincipalProfile, newRequest("PUT", profilePath,
			map[string]any{"consent_text": "Tôi đồng ý"})).Want(http.StatusNotFound)
		testutil.Call(t, testHandler.AddPrincipalTrait, newRequest("POST", profilePath+"/traits", map[string]any{})).Want(http.StatusNotFound)
	})

	t.Run("on: consent is required to turn the profile on", func(t *testing.T) {
		testHandler.FeatureFlags = flags(true)
		wantOutcomeCode(t, testutil.Call(t, testHandler.EnablePrincipalProfile,
			newRequest("PUT", profilePath, map[string]any{"consent_text": "  "})),
			http.StatusUnprocessableEntity, "profile_consent_required")
		testutil.Call(t, testHandler.EnablePrincipalProfile,
			newRequest("PUT", profilePath, map[string]any{"consent_text": "Tôi đồng ý", "share_with_owners": false})).
			Want(http.StatusOK)
	})

	t.Run("on: a trait citing nothing of the person's is refused with 422", func(t *testing.T) {
		body := map[string]any{
			"decision_kind": "general",
			"statement":     "Chọn nhanh khi việc nhỏ",
			"origin":        "edited",
			"evidence":      []map[string]any{{"type": "comment", "ref": "00000000-0000-0000-0000-000000000001"}},
		}
		wantOutcomeCode(t, testutil.Call(t, testHandler.AddPrincipalTrait, newRequest("POST", profilePath+"/traits", body)),
			http.StatusUnprocessableEntity, "principal_evidence_invalid")
	})

	t.Run("on: the profile reads back with no traits yet", func(t *testing.T) {
		var got struct {
			Status string           `json:"status"`
			Traits []map[string]any `json:"traits"`
		}
		testutil.Call(t, testHandler.GetPrincipalProfile, newRequest("GET", profilePath, nil)).Want(http.StatusOK).JSON(&got)
		if got.Status != "active" || len(got.Traits) != 0 {
			t.Fatalf("profile = %q with %d traits, want active with none", got.Status, len(got.Traits))
		}
	})

	t.Run("delete works with the flag off", func(t *testing.T) {
		testHandler.FeatureFlags = flags(false)
		testutil.Call(t, testHandler.DeletePrincipalProfile, newRequest("DELETE", profilePath, nil)).Want(http.StatusNoContent)
		testHandler.FeatureFlags = flags(true)
		testutil.Call(t, testHandler.GetPrincipalProfile, newRequest("GET", profilePath, nil)).Want(http.StatusNotFound)
	})
}
