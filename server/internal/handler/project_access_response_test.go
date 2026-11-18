package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The project responses report the stored access mode and whether the caller
// may manage project access (owner or admin), so the UI can show both.

func getProjectAs(t *testing.T, userID, projectID string) map[string]any {
	t.Helper()
	req := requestAs(userID, "GET", "/api/projects/"+projectID+"?workspace_id="+testWorkspaceID, nil)
	req = withURLParam(req, "id", projectID)
	w := httptest.NewRecorder()
	testHandler.GetProject(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetProject as %s: %d %s", userID, w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode GetProject: %v", err)
	}
	return out
}

func listProjectsAs(t *testing.T, userID string) map[string]map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	testHandler.ListProjects(w, requestAs(userID, "GET", "/api/projects?workspace_id="+testWorkspaceID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ListProjects as %s: %d %s", userID, w.Code, w.Body.String())
	}
	var out struct {
		Projects []map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode ListProjects: %v", err)
	}
	byID := map[string]map[string]any{}
	for _, p := range out.Projects {
		byID[p["id"].(string)] = p
	}
	return byID
}

func TestRestrictedProjectResponseReportsAccessRestricted(t *testing.T) {
	f := newRestrictedFixture(t)

	got := getProjectAs(t, testUserID, f.restrictedProjectID)
	if got["access"] != "restricted" {
		t.Errorf("GetProject restricted: access = %v, want restricted", got["access"])
	}
	got = getProjectAs(t, testUserID, f.workspaceProjectID)
	if got["access"] != "workspace" {
		t.Errorf("GetProject workspace: access = %v, want workspace", got["access"])
	}

	list := listProjectsAs(t, testUserID)
	if p, ok := list[f.restrictedProjectID]; !ok || p["access"] != "restricted" {
		t.Errorf("ListProjects restricted: %v", p)
	}
	if p, ok := list[f.workspaceProjectID]; !ok || p["access"] != "workspace" {
		t.Errorf("ListProjects workspace: %v", p)
	}
}

func TestProjectResponseCanManageAccessTrueForOwner(t *testing.T) {
	f := newRestrictedFixture(t)

	if got := getProjectAs(t, testUserID, f.restrictedProjectID); got["can_manage_access"] != true {
		t.Errorf("GetProject owner: can_manage_access = %v, want true", got["can_manage_access"])
	}
	for id, p := range listProjectsAs(t, testUserID) {
		if p["can_manage_access"] != true {
			t.Errorf("ListProjects owner, project %s: can_manage_access = %v, want true", id, p["can_manage_access"])
		}
	}
}

func TestProjectResponseCanManageAccessTrueForAdmin(t *testing.T) {
	f := newRestrictedFixture(t)
	adminID := createTestUserAndMember(t, "admin")

	if got := getProjectAs(t, adminID, f.workspaceProjectID); got["can_manage_access"] != true {
		t.Errorf("GetProject admin: can_manage_access = %v, want true", got["can_manage_access"])
	}
	if p := listProjectsAs(t, adminID)[f.workspaceProjectID]; p == nil || p["can_manage_access"] != true {
		t.Errorf("ListProjects admin: %v", p)
	}
}

func TestProjectResponseCanManageAccessFalseForMember(t *testing.T) {
	f := newRestrictedFixture(t)

	// The grantee sees the restricted project and the workspace project.
	got := getProjectAs(t, f.granteeID, f.restrictedProjectID)
	v, present := got["can_manage_access"]
	if !present || v != false {
		t.Errorf("GetProject member: can_manage_access = %v (present %v), want false", v, present)
	}
	list := listProjectsAs(t, f.granteeID)
	if len(list) == 0 {
		t.Fatal("member sees no projects")
	}
	for id, p := range list {
		v, present := p["can_manage_access"]
		if !present || v != false {
			t.Errorf("ListProjects member, project %s: can_manage_access = %v (present %v), want false", id, v, present)
		}
	}
}

func TestCreateProjectWithOldShapeBodyStillWorks(t *testing.T) {
	w := httptest.NewRecorder()
	testHandler.CreateProject(w, newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{"title": "Old shape project"}))
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProject: %d %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	id, _ := got["id"].(string)
	t.Cleanup(func() {
		r := withURLParam(newRequest("DELETE", "/api/projects/"+id, nil), "id", id)
		testHandler.DeleteProject(httptest.NewRecorder(), r)
	})
	if got["access"] != "workspace" {
		t.Errorf("access = %v, want workspace", got["access"])
	}
}
