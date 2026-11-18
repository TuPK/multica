package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The member lists carry is_guest so the UI can mark guests.

func listMembersRaw(t *testing.T, withUser bool) map[string]map[string]any {
	t.Helper()
	req := newRequest("GET", "/api/workspaces/"+testWorkspaceID+"/members", nil)
	req = withURLParam(req, "id", testWorkspaceID)
	w := httptest.NewRecorder()
	if withUser {
		testHandler.ListMembersWithUser(w, req)
	} else {
		testHandler.ListMembers(w, req)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("list members: %d %s", w.Code, w.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byUser := map[string]map[string]any{}
	for _, r := range rows {
		byUser[r["user_id"].(string)] = r
	}
	return byUser
}

func TestMemberListShowsIsGuestTrueForGuest(t *testing.T) {
	guestID, _ := guestTestMember(t, "viewer", true)
	for name, withUser := range map[string]bool{"ListMembers": false, "ListMembersWithUser": true} {
		row := listMembersRaw(t, withUser)[guestID]
		if row == nil {
			t.Fatalf("%s: guest missing from list", name)
		}
		if row["is_guest"] != true {
			t.Errorf("%s: is_guest = %v, want true", name, row["is_guest"])
		}
	}
}

func TestMemberListShowsIsGuestFalseForMember(t *testing.T) {
	memberID, _ := guestTestMember(t, "member", false)
	for name, withUser := range map[string]bool{"ListMembers": false, "ListMembersWithUser": true} {
		row := listMembersRaw(t, withUser)[memberID]
		if row == nil {
			t.Fatalf("%s: member missing from list", name)
		}
		v, present := row["is_guest"]
		if !present || v != false {
			t.Errorf("%s: is_guest = %v (present %v), want false", name, v, present)
		}
	}
}
