package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/research"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
)

// setResearchService points the handler at a research service for one test.
func setResearchService(t *testing.T, box *secretbox.Box) {
	t.Helper()
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	prev := testHandler.Research
	testHandler.Research = &research.Service{Queries: testHandler.Queries, Box: box}
	clean := func() {
		testPool.Exec(context.Background(), `DELETE FROM research_settings WHERE workspace_id = $1`, testWorkspaceID)
	}
	clean()
	t.Cleanup(func() { clean(); testHandler.Research = prev })
}

func researchPath(suffix string) string { return "/api/research/" + suffix }

func putResearch(t *testing.T, body map[string]any) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.UpdateResearchSettings, newRequest("PUT", researchPath("settings"), body))
}

func TestResearchSettingsStoreKeysSealedAndNeverEchoThem(t *testing.T) {
	setResearchService(t, newPublishBox(t))

	var empty research.Settings
	testutil.Call(t, testHandler.GetResearchSettings, newRequest("GET", researchPath("settings"), nil)).Want(http.StatusOK).JSON(&empty)
	if empty.OpenAlexConfigured || empty.SemanticScholarConfigured || !empty.KeysCanBeStored {
		t.Fatalf("empty settings = %+v", empty)
	}

	var saved research.Settings
	resp := putResearch(t, map[string]any{"openalex_api_key": "OA-SECRET-123", "contact_email": "lab@example.org"}).Want(http.StatusOK)
	resp.JSON(&saved)
	if !saved.OpenAlexConfigured || saved.SemanticScholarConfigured || saved.ContactEmail != "lab@example.org" {
		t.Fatalf("saved = %+v", saved)
	}
	if strings.Contains(resp.Text(), "OA-SECRET-123") {
		t.Fatal("the settings response contains the API key")
	}

	// At rest the key is sealed.
	var sealed []byte
	if err := testPool.QueryRow(context.Background(), `SELECT openalex_key_sealed FROM research_settings WHERE workspace_id = $1`, testWorkspaceID).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if len(sealed) == 0 || strings.Contains(string(sealed), "OA-SECRET-123") {
		t.Fatal("the stored key is not sealed")
	}

	var creds research.Credentials
	testutil.Call(t, testHandler.GetResearchCredentials, newRequest("GET", researchPath("credentials"), nil)).Want(http.StatusOK).JSON(&creds)
	if creds.OpenAlexKey != "OA-SECRET-123" || creds.S2Key != "" || creds.ContactEmail != "lab@example.org" {
		t.Fatalf("credentials = %+v", creds)
	}

	// An absent field is left alone; an empty string clears.
	putResearch(t, map[string]any{"semantic_scholar_api_key": "S2-KEY"}).Want(http.StatusOK)
	testutil.Call(t, testHandler.GetResearchCredentials, newRequest("GET", researchPath("credentials"), nil)).Want(http.StatusOK).JSON(&creds)
	if creds.OpenAlexKey != "OA-SECRET-123" || creds.S2Key != "S2-KEY" {
		t.Fatalf("after second update = %+v", creds)
	}
	putResearch(t, map[string]any{"openalex_api_key": ""}).Want(http.StatusOK).JSON(&saved)
	if saved.OpenAlexConfigured || !saved.SemanticScholarConfigured {
		t.Fatalf("after clearing = %+v", saved)
	}
}

func TestResearchSettingsValidation(t *testing.T) {
	setResearchService(t, newPublishBox(t))
	for name, body := range map[string]map[string]any{
		"key with a space":   {"openalex_api_key": "has space"},
		"key with a newline": {"openalex_api_key": "a\nb"},
		"key too long":       {"openalex_api_key": strings.Repeat("k", 300)},
		"bad email":          {"contact_email": "not-an-email"},
		"display-name email": {"contact_email": "Lab <lab@example.org>"},
	} {
		putResearch(t, body).Want(http.StatusBadRequest)
		_ = name
	}
	testutil.Call(t, testHandler.UpdateResearchSettings, newRequest("PUT", researchPath("settings"), "not an object")).Want(http.StatusBadRequest)
}

func TestResearchSettingsWithoutASealingKeyStillTakesTheEmail(t *testing.T) {
	setResearchService(t, nil)
	putResearch(t, map[string]any{"openalex_api_key": "OA-KEY"}).Want(http.StatusServiceUnavailable)
	var saved research.Settings
	putResearch(t, map[string]any{"contact_email": "lab@example.org"}).Want(http.StatusOK).JSON(&saved)
	if saved.KeysCanBeStored || saved.ContactEmail != "lab@example.org" {
		t.Fatalf("%+v", saved)
	}
}

func TestResearchSettingsAreClosedToAgentsForWrites(t *testing.T) {
	setResearchService(t, newPublishBox(t))
	agentReq := newRequest("PUT", researchPath("settings"), map[string]any{"contact_email": "a@example.org"})
	agentReq.Header.Set("X-Actor-Source", "task_token")
	testutil.Call(t, testHandler.UpdateResearchSettings, agentReq).Want(http.StatusForbidden)
}

func TestResearchCredentialsAreForOwnersAdminsAndAgentTasksOnly(t *testing.T) {
	setResearchService(t, newPublishBox(t))
	putResearch(t, map[string]any{"openalex_api_key": "OA-KEY", "contact_email": "lab@example.org"}).Want(http.StatusOK)

	read := func(req *http.Request) *testutil.Response {
		return testutil.Call(t, testHandler.GetResearchCredentials, req)
	}
	get := func() *http.Request { return newRequest("GET", researchPath("credentials"), nil) }

	// The workspace owner (the test user).
	var creds research.Credentials
	resp := read(get()).Want(http.StatusOK)
	resp.JSON(&creds)
	if creds.OpenAlexKey != "OA-KEY" || creds.ContactEmail != "lab@example.org" {
		t.Fatalf("owner = %+v", creds)
	}

	// An admin.
	adminID := createPermissionTestAdmin(t, "research-cred-admin@multica.test")
	creds = research.Credentials{}
	read(newRequestAs(adminID, "GET", researchPath("credentials"), nil)).Want(http.StatusOK).JSON(&creds)
	if creds.OpenAlexKey != "OA-KEY" {
		t.Fatalf("admin = %+v", creds)
	}

	// A plain member is refused, and the body holds no key.
	memberID := createPermissionTestMember(t, "research-cred-member@multica.test")
	denied := read(newRequestAs(memberID, "GET", researchPath("credentials"), nil)).Want(http.StatusForbidden)
	if strings.Contains(denied.Text(), "OA-KEY") {
		t.Fatal("the 403 body leaks the key")
	}

	// A plain member that carries a Cloud node credential is still a plain member.
	cloud := newRequestAs(memberID, "GET", researchPath("credentials"), nil)
	cloud.Header.Set("X-Actor-Source", "cloud_pat")
	read(cloud).Want(http.StatusForbidden)

	// An agent task token (stamped by the auth middleware with the agent id) may read, even when the
	// human behind the agent is a plain member.
	agent := newRequestAs(memberID, "GET", researchPath("credentials"), nil)
	agent.Header.Set("X-Actor-Source", "task_token")
	agent.Header.Set("X-Agent-ID", "00000000-0000-0000-0000-0000000000a1")
	creds = research.Credentials{}
	read(agent).Want(http.StatusOK).JSON(&creds)
	if creds.OpenAlexKey != "OA-KEY" {
		t.Fatalf("agent task = %+v", creds)
	}

	// A task_token without an agent id is not an agent task.
	noAgent := newRequestAs(memberID, "GET", researchPath("credentials"), nil)
	noAgent.Header.Set("X-Actor-Source", "task_token")
	read(noAgent).Want(http.StatusForbidden)

	// A user that is not in the workspace gets nothing.
	var strangerID string
	if err := testPool.QueryRow(context.Background(), `INSERT INTO "user" (name, email) VALUES ('s','research-cred-stranger@multica.test') RETURNING id`).Scan(&strangerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, strangerID) })
	read(newRequestAs(strangerID, "GET", researchPath("credentials"), nil)).WantOneOf(http.StatusForbidden, http.StatusNotFound)
}

func TestResearchSettingsKeyUnreadableAfterTheSealingSecretChanges(t *testing.T) {
	setResearchService(t, newPublishBox(t))
	putResearch(t, map[string]any{"openalex_api_key": "OA-KEY", "contact_email": "lab@example.org"}).Want(http.StatusOK)
	testHandler.Research.Box = newPublishBox(t) // another secret
	var creds research.Credentials
	testutil.Call(t, testHandler.GetResearchCredentials, newRequest("GET", researchPath("credentials"), nil)).Want(http.StatusOK).JSON(&creds)
	if creds.OpenAlexKey != "" || creds.ContactEmail != "lab@example.org" {
		t.Fatalf("an unreadable key must come back empty, got %+v", creds)
	}
}

func TestResearchSettingsPlainMemberMayReadStatusButNotChangeIt(t *testing.T) {
	setResearchService(t, newPublishBox(t))
	putResearch(t, map[string]any{"openalex_api_key": "OA-KEY"}).Want(http.StatusOK)
	memberID := createPermissionTestMember(t, "research-settings-member@multica.test")

	var got research.Settings
	resp := testutil.Call(t, testHandler.GetResearchSettings, newRequestAs(memberID, "GET", researchPath("settings"), nil)).Want(http.StatusOK)
	resp.JSON(&got)
	if !got.OpenAlexConfigured || strings.Contains(resp.Text(), "OA-KEY") {
		t.Fatalf("member view = %+v / %s", got, resp.Text())
	}
	testutil.Call(t, testHandler.UpdateResearchSettings,
		newRequestAs(memberID, "PUT", researchPath("settings"), map[string]any{"contact_email": "m@example.org"})).Want(http.StatusForbidden)
}
