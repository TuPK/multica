package handler

import (
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// The add-member requests of a workspace hold the email of people who were
// never members, so they go with the workspace; another workspace's stay.
func TestWorkspaceDeleteRemovesMemberRequests(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	gone := dbfx.Workspace(t, "member request gone", "mr-gone-"+uuid.NewString()[:8])
	kept := dbfx.Workspace(t, "member request kept", "mr-kept-"+uuid.NewString()[:8])
	staff := dbfx.User(t, "Staff", testutil.UniqueEmail("mr-staff"))
	for _, ws := range []string{gone, gone, kept} {
		dbfx.Insert(t, "platform_member_request", testutil.Cols{
			"id": uuid.NewString(), "workspace_id": ws, "email": testutil.UniqueEmail("mr-req"),
			"role": "member", "requested_by": staff,
		})
	}

	teardown(t, gone)

	if n := dbfx.Count(t, `SELECT count(*) FROM platform_member_request WHERE workspace_id = $1::uuid`, gone); n != 0 {
		t.Errorf("%d member requests left for the deleted workspace; want 0", n)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM platform_member_request WHERE workspace_id = $1::uuid`, kept); n != 1 {
		t.Errorf("another workspace has %d member requests; want 1", n)
	}
}
