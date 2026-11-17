package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Spec 7.1 rule 9: a guest comes only from a guest invitation. A share link
// never creates one.
func TestShareLinkNeverCreatesGuest(t *testing.T) {
	clearShareLinksForTestWorkspace(t)

	// Creating a link takes no guest flag and the link has no such column.
	req := newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/share-links",
		map[string]any{"role": "member", "is_guest": true})
	req = withURLParam(req, "id", testWorkspaceID)
	w := httptest.NewRecorder()
	testHandler.CreateShareLink(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create share link: status %d (body %q)", w.Code, w.Body.String())
	}

	for _, role := range []string{"member", "viewer"} {
		// One active link per workspace: drop the previous one first.
		clearShareLinksForTestWorkspace(t)
		link := createTestShareLink(t, testWorkspaceID, role, 5, 0)
		userID := createTestUserAndMember(t, "")
		req := newRequest("POST", "/api/share-links/join", JoinByShareLinkRequest{Code: link.Code})
		req.Header.Set("X-User-ID", userID)
		w := httptest.NewRecorder()
		testHandler.JoinByShareLink(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("join as %s: status %d (body %q)", role, w.Code, w.Body.String())
		}
		gotRole, guest := memberRoleAndGuest(t, userID)
		if gotRole != role || guest {
			t.Errorf("share-link joiner: role=%q is_guest=%v, want %s and false", gotRole, guest, role)
		}
	}

	var guests int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM member m JOIN "user" u ON u.id = m.user_id
		 WHERE m.workspace_id = $1 AND m.is_guest AND u.name LIKE 'sharelink-test-%'`,
		parseUUID(testWorkspaceID),
	).Scan(&guests); err != nil {
		t.Fatal(err)
	}
	if guests != 0 {
		t.Errorf("%d share-link joiners are guests, want 0", guests)
	}
}
