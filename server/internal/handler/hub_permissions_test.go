package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// The permission matrix for the hub routes. Every route is called as each of five
// callers, and the status must match the design:
//
//   - owner and admin: source writes, export, and everything a member may do;
//   - member: reads, and the import preview and commit (whoever may create
//     skills may import them); source writes and export are 403;
//   - non-member (no membership in any workspace) and cross-workspace (a member
//     of another workspace only): 404, the same answer a missing workspace gets.
//
// Phase 5 routes (sync, update check, apply, fork) and the Phase 4 item routes
// are in the same matrix. Skill edit rules: the creator, an owner or an admin may
// edit a skill, so a plain member is refused with 403. Plugin changes are admin-only.

const (
	hubRoleOwner          = "owner"
	hubRoleAdmin          = "admin"
	hubRoleMember         = "member"
	hubRoleNonMember      = "non-member"
	hubRoleCrossWorkspace = "cross-workspace"

	// hubAnyStatusButDenied is for a route whose success status depends on the
	// request body: the assertion is only that the caller got past the gate.
	hubAnyStatusButDenied = -1
)

var hubPermRoles = []string{hubRoleOwner, hubRoleAdmin, hubRoleMember, hubRoleNonMember, hubRoleCrossWorkspace}

// hubPermWorkspace is a fresh workspace with one caller per role. Every caller is
// a real user; only the membership differs.
type hubPermWorkspace struct {
	ws    string
	users map[string]string
}

func newHubPermWorkspace(t *testing.T) hubPermWorkspace {
	t.Helper()
	ws := dbfx.Workspace(t, hubTestLabel(t, "workspace"), hubTestSlug(t))
	dbfx.Cleanup(t, "DELETE FROM hub_activity WHERE workspace_id = $1", ws)

	f := hubPermWorkspace{ws: ws, users: map[string]string{}}
	for _, role := range []string{hubRoleOwner, hubRoleAdmin, hubRoleMember} {
		user := dbfx.User(t, hubTestLabel(t, role), hubTestEmail(t))
		dbfx.Member(t, ws, user, role)
		f.users[role] = user
	}
	f.users[hubRoleNonMember] = dbfx.User(t, hubTestLabel(t, "non-member"), hubTestEmail(t))

	other := dbfx.Workspace(t, hubTestLabel(t, "other workspace"), hubTestSlug(t))
	outsider := dbfx.User(t, hubTestLabel(t, "cross-workspace"), hubTestEmail(t))
	dbfx.Member(t, other, outsider, hubRoleOwner)
	f.users[hubRoleCrossWorkspace] = outsider
	return f
}

// hubPermCase is one route, with the status each role must get.
type hubPermCase struct {
	name string
	want map[string]int
	// call makes the request as userID in workspace ws and returns the response.
	call func(t *testing.T, ws, userID string) *testutil.Response
}

// hubPermExpect builds the status map for the common shapes: who may pass the
// gate, and what status the allowed roles get. Denied roles get 403 when they are
// members, 404 when they are not.
func hubPermExpect(allowed []string, allowedStatus int) map[string]int {
	want := map[string]int{}
	for _, role := range hubPermRoles {
		switch {
		case roleIn(role, allowed):
			want[role] = allowedStatus
		case role == hubRoleMember:
			want[role] = http.StatusForbidden
		default:
			want[role] = http.StatusNotFound
		}
	}
	return want
}

func roleIn(role string, roles []string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

var (
	hubAnyMember = []string{hubRoleOwner, hubRoleAdmin, hubRoleMember}
	hubAdmins    = []string{hubRoleOwner, hubRoleAdmin}
)

func hubPermCases() []hubPermCase {
	return []hubPermCase{
		{
			name: "list sources",
			want: hubPermExpect(hubAnyMember, http.StatusOK),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				return testutil.Call(t, testHandler.ListHubSources,
					hubTestRequest(http.MethodGet, "/api/hub/sources", nil, ws, uid))
			},
		},
		{
			name: "create source",
			want: hubPermExpect(hubAdmins, http.StatusCreated),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				resp := testutil.Call(t, testHandler.CreateHubSource,
					hubTestRequest(http.MethodPost, "/api/hub/sources", map[string]any{
						"name": hubTestLabel(t, "created"), "type": "github",
						"url": "https://github.com/hub-test/" + hubTestUUID(t),
					}, ws, uid))
				if resp.Code == http.StatusCreated {
					var created HubSourceResponse
					resp.JSON(&created)
					dbfx.Cleanup(t, "DELETE FROM hub_source WHERE id = $1", created.ID)
				}
				return resp
			},
		},
		{
			name: "update source",
			want: hubPermExpect(hubAdmins, http.StatusOK),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				source := insertHubSourceRowForTest(t, ws)
				return testutil.Call(t, testHandler.UpdateHubSource,
					testutil.WithURLParams(hubTestRequest(http.MethodPatch, "/api/hub/sources/"+source, map[string]any{
						"name": hubTestLabel(t, "renamed"),
					}, ws, uid), "id", source))
			},
		},
		{
			name: "delete source",
			want: hubPermExpect(hubAdmins, http.StatusNoContent),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				source := insertHubSourceRowForTest(t, ws)
				return testutil.Call(t, testHandler.DeleteHubSource,
					testutil.WithURLParams(hubTestRequest(http.MethodDelete, "/api/hub/sources/"+source+"?items=keep", nil, ws, uid), "id", source))
			},
		},
		{
			// An empty body is a malformed preview: any member gets past the gate
			// and is refused by validation, so only the gate is asserted.
			name: "import preview",
			want: hubPermExpect(hubAnyMember, hubAnyStatusButDenied),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				return testutil.Call(t, testHandler.PreviewHubImport,
					hubTestRequest(http.MethodPost, "/api/hub/import/preview", map[string]any{}, ws, uid))
			},
		},
		{
			// An empty preview id is refused after the membership check, so a
			// member gets 400 and the outsiders still get 404 before the body.
			name: "import commit",
			want: hubPermExpect(hubAnyMember, http.StatusBadRequest),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				return testutil.Call(t, testHandler.CommitHubImport,
					hubTestRequest(http.MethodPost, "/api/hub/import/commit", map[string]any{}, ws, uid))
			},
		},
		{
			name: "activity",
			want: hubPermExpect(hubAnyMember, http.StatusOK),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				return testutil.Call(t, testHandler.GetHubActivity,
					hubTestRequest(http.MethodGet, "/api/hub/activity", nil, ws, uid))
			},
		},
		{
			name: "quota",
			want: hubPermExpect(hubAnyMember, http.StatusOK),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				return testutil.Call(t, testHandler.GetHubQuota,
					hubTestRequest(http.MethodGet, "/api/hub/quota", nil, ws, uid))
			},
		},
		{
			name: "export",
			want: hubPermExpect(hubAdmins, http.StatusOK),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				return testutil.Call(t, testHandler.ExportHubItems,
					hubTestRequest(http.MethodGet, "/api/hub/export", nil, ws, uid))
			},
		},
		{
			name: "list items",
			want: hubPermExpect(hubAnyMember, http.StatusOK),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				return testutil.Call(t, testHandler.ListHubItems,
					hubTestRequest(http.MethodGet, "/api/hub/items", nil, ws, uid))
			},
		},
		{
			name: "item bundle",
			want: hubPermExpect(hubAnyMember, http.StatusOK),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				skill := hubPermSkill(t, ws, `'{}'::jsonb`)
				return testutil.Call(t, testHandler.GetHubItemBundle,
					testutil.WithURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/skill/"+skill+"/bundle", nil, ws, uid), "kind", "skill", "id", skill))
			},
		},
		{
			// A manual skill has no source to check: the gate is what is asserted.
			name: "check update",
			want: hubPermExpect(hubAnyMember, hubAnyStatusButDenied),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				skill := hubPermSkill(t, ws, `'{}'::jsonb`)
				return testutil.Call(t, testHandler.GetHubItemUpdate,
					testutil.WithURLParams(hubTestRequest(http.MethodGet, "/api/hub/items/skill/"+skill+"/update", nil, ws, uid), "kind", "skill", "id", skill))
			},
		},
		{
			// The skill is manual, so apply is refused after the edit gate: only the
			// gate (admins and owners pass, a plain member is 403) is asserted.
			name: "apply update",
			want: hubPermExpect(hubAdmins, hubAnyStatusButDenied),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				skill := hubPermSkill(t, ws, `'{}'::jsonb`)
				return testutil.Call(t, testHandler.ApplyHubItemUpdate,
					testutil.WithURLParams(hubTestRequest(http.MethodPost, "/api/hub/items/skill/"+skill+"/update/apply", map[string]any{}, ws, uid), "kind", "skill", "id", skill))
			},
		},
		{
			name: "fork",
			want: hubPermExpect(hubAdmins, http.StatusOK),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				skill := hubPermSkill(t, ws, `'{"origin":{"type":"github","source_url":"https://github.com/hub-test/fork-`+hubTestUUID(t)+`"}}'::jsonb`)
				return testutil.Call(t, testHandler.ForkHubItem,
					testutil.WithURLParams(hubTestRequest(http.MethodPost, "/api/hub/items/skill/"+skill+"/fork", nil, ws, uid), "kind", "skill", "id", skill))
			},
		},
		{
			name: "sync source",
			want: hubPermExpect(hubAdmins, http.StatusOK),
			call: func(t *testing.T, ws, uid string) *testutil.Response {
				source := insertHubSourceRowForTest(t, ws)
				return testutil.Call(t, testHandler.SyncHubSource,
					testutil.WithURLParams(hubTestRequest(http.MethodPost, "/api/hub/sources/"+source+"/sync", nil, ws, uid), "id", source))
			},
		},
	}
}

// hubPermSkill inserts a skill in ws with the given config and returns its id,
// with its hub row removed on cleanup.
func hubPermSkill(t *testing.T, ws, config string) string {
	t.Helper()
	skill := insertHubItemsSkill(t, ws, "hubperm-"+hubItemsSuffix(), "# Permission fixture\n", config)
	registerHubItemCleanup(t, ws, skill)
	return skill
}

// TestHubPermissions_Matrix runs every case as every role. A workspace is built
// per case so that rows one case creates cannot change another case's answers.
func TestHubPermissions_Matrix(t *testing.T) {
	setupHubImportTest(t)
	for _, c := range hubPermCases() {
		t.Run(c.name, func(t *testing.T) {
			f := newHubPermWorkspace(t)
			for _, role := range hubPermRoles {
				t.Run(role, func(t *testing.T) {
					resp := c.call(t, f.ws, f.users[role])
					want := c.want[role]
					switch {
					case want == hubAnyStatusButDenied:
						if resp.Code == http.StatusForbidden || resp.Code == http.StatusNotFound {
							t.Fatalf("status = %d, want the request past the gate; body: %s", resp.Code, resp.Body.String())
						}
					case resp.Code != want:
						t.Fatalf("status = %d, want %d; body: %s", resp.Code, want, resp.Body.String())
					}
				})
			}
		})
	}
}

// TestHubPermissions_ExportIsZipForAdmins pins the export body on success:
// the owner gets a zip, not JSON, so a client that expects a file gets one.
func TestHubPermissions_ExportIsZipForAdmins(t *testing.T) {
	f := newHubPermWorkspace(t)
	resp := testutil.Call(t, testHandler.ExportHubItems,
		hubTestRequest(http.MethodGet, "/api/hub/export", nil, f.ws, f.users[hubRoleAdmin]))
	resp.Want(http.StatusOK)
	if ct := resp.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("Content-Type = %q, want application/zip", ct)
	}
}

// ---- helpers shared by the hub Phase 6 tests -------------------------------

// hubTestUUID returns a fresh uuid from the database, for ids and unique values.
func hubTestUUID(t *testing.T) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(context.Background(), `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatalf("setup: new uuid: %v", err)
	}
	return id
}

// hubTestSlug is a unique, valid workspace slug for the test.
func hubTestSlug(t *testing.T) string {
	return "hub-test-" + strings.ReplaceAll(hubTestUUID(t), "-", "")[:16]
}

// hubTestEmail is a unique address for a fixture user.
func hubTestEmail(t *testing.T) string {
	return fmt.Sprintf("hub-%s@example.test", hubTestUUID(t))
}

// insertHubSourceRowForTest inserts a source row directly, for tests that need a
// source to act on and do not exercise its creation.
func insertHubSourceRowForTest(t *testing.T, workspaceID string) string {
	t.Helper()
	return dbfx.Insert(t, "hub_source", testutil.Cols{
		"workspace_id": workspaceID,
		"name":         hubTestLabel(t, "source"),
		"type":         "github",
		"url":          "https://github.com/hub-test/" + hubTestUUID(t),
	})
}
