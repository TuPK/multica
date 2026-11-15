package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Database-backed: they run only against the dedicated *_test database named by
// PLATFORM_TEST_DATABASE_URL (see requirePlatformTestDatabase). The pure rules
// are tested in platform-management/consent.

func resetUserConsent(t *testing.T) {
	t.Helper()
	clean := func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM user_consent WHERE user_id = $1::uuid`, testUserID)
		_, _ = testPool.Exec(context.Background(),
			`DELETE FROM audit_event WHERE action = 'consent.accepted' AND actor_id = $1`, testUserID)
	}
	clean()
	t.Cleanup(clean)
}

func setLegal(t *testing.T, termsVersion, privacyVersion string) {
	t.Helper()
	t.Setenv("LEGAL_TERMS_VERSION", termsVersion)
	t.Setenv("LEGAL_TERMS_URL", "https://legal.example/terms")
	t.Setenv("LEGAL_PRIVACY_VERSION", privacyVersion)
	t.Setenv("LEGAL_PRIVACY_URL", "https://legal.example/privacy")
}

func getMeMap(t *testing.T) map[string]any {
	t.Helper()
	return testutil.Call(t, testHandler.GetMe, newRequest(http.MethodGet, "/api/me", nil)).Want(http.StatusOK).Map()
}

func acceptConsents(t *testing.T, docs ...map[string]string) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.AcceptConsents, newRequest(http.MethodPost, "/api/me/consents",
		map[string]any{"documents": docs}))
}

func countConsentRows(t *testing.T) (rows, auditRows int) {
	t.Helper()
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM user_consent WHERE user_id = $1::uuid`, testUserID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_event WHERE action = 'consent.accepted' AND actor_id = $1`, testUserID).Scan(&auditRows); err != nil {
		t.Fatal(err)
	}
	return rows, auditRows
}

func TestGetMeOmitsConsentRequiredWhenLegalEmpty(t *testing.T) {
	requirePlatformTestDatabase(t)
	resetUserConsent(t)
	setLegal(t, "", "")
	if _, present := getMeMap(t)["consent_required"]; present {
		t.Fatal("consent_required must be absent when the feature is off")
	}
}

func TestGetMeListsPendingConsentAndVersionChangeRequiresAgain(t *testing.T) {
	requirePlatformTestDatabase(t)
	resetUserConsent(t)
	setLegal(t, "1", "")

	req, _ := getMeMap(t)["consent_required"].([]any)
	if len(req) != 1 {
		t.Fatalf("want terms pending, got %v", req)
	}
	acceptConsents(t, map[string]string{"document": "terms", "version": "1"}).Want(http.StatusOK)
	if _, present := getMeMap(t)["consent_required"]; present {
		t.Fatal("consent_required must be absent once everything is accepted")
	}

	setLegal(t, "2", "")
	req, _ = getMeMap(t)["consent_required"].([]any)
	if len(req) != 1 {
		t.Fatalf("a new version must make terms required again, got %v", req)
	}
	entry := req[0].(map[string]any)
	if entry["document"] != "terms" || entry["version"] != "2" || entry["url"] != "https://legal.example/terms" {
		t.Fatalf("got %v", entry)
	}
}

func TestAcceptConsentsTwiceRecordsOneRow(t *testing.T) {
	requirePlatformTestDatabase(t)
	resetUserConsent(t)
	setLegal(t, "1", "1")
	item := map[string]string{"document": "terms", "version": "1"}

	var out AcceptConsentsResponse
	acceptConsents(t, item).Want(http.StatusOK)
	acceptConsents(t, item, map[string]string{"document": "privacy", "version": "1"}).Want(http.StatusOK).JSON(&out)

	if out.ConsentRequired == nil || len(out.ConsentRequired) != 0 {
		t.Fatalf("want an empty array, got %#v", out.ConsentRequired)
	}
	rows, auditRows := countConsentRows(t)
	if rows != 2 || auditRows != 2 {
		t.Fatalf("want 2 consent rows and 2 audit rows, got %d and %d", rows, auditRows)
	}
}

func TestAcceptConsentsMismatchReturns409AndRecordsNothing(t *testing.T) {
	requirePlatformTestDatabase(t)
	resetUserConsent(t)
	setLegal(t, "2", "1")

	resp := acceptConsents(t,
		map[string]string{"document": "privacy", "version": "1"},
		map[string]string{"document": "terms", "version": "1"},
	).Want(http.StatusConflict)
	if got := resp.Map()["code"]; got != "consent_version_mismatch" {
		t.Fatalf("code = %v", got)
	}
	if rows, auditRows := countConsentRows(t); rows != 0 || auditRows != 0 {
		t.Fatalf("nothing may be recorded, got %d rows and %d audit rows", rows, auditRows)
	}
}

func TestAcceptConsentsUnknownDocumentReturns400(t *testing.T) {
	requirePlatformTestDatabase(t)
	resetUserConsent(t)
	setLegal(t, "1", "1")
	resp := acceptConsents(t, map[string]string{"document": "cookies", "version": "1"}).Want(http.StatusBadRequest)
	if got := resp.Map()["code"]; got != "invalid_document" {
		t.Fatalf("code = %v", got)
	}
}

func TestAcceptConsentsRefusesMachineToken(t *testing.T) {
	requirePlatformTestDatabase(t)
	setLegal(t, "1", "")
	req := newRequest(http.MethodPost, "/api/me/consents", map[string]any{"documents": []any{}})
	req.Header.Set("X-Actor-Source", "task_token")
	testutil.Call(t, RequireHumanActor(http.HandlerFunc(testHandler.AcceptConsents)).ServeHTTP, req).Want(http.StatusForbidden)
}
