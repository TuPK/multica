package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func searchIssuesAs(t *testing.T, userID, token, extra string) string {
	t.Helper()
	w := serveAs(testHandler.SearchIssues, requestAs(userID, "GET",
		"/api/issues/search?workspace_id="+testWorkspaceID+"&include_closed=true"+extra+"&q="+url.QueryEscape(token), nil))
	if w.Code != http.StatusOK {
		t.Fatalf("SearchIssues as %s: %d %s", userID, w.Code, w.Body.String())
	}
	return w.Body.String()
}

// TestRestrictedSearchIssuesOmitsHidden: the outsider's issue search neither
// returns the restricted issue nor echoes its title; the grantee and the owner
// find it. A match that only the title of the restricted project's issue
// carries must not leak through a comment or description either.
func TestRestrictedSearchIssuesOmitsHidden(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	token := "needlesch" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	hidden := createIssueInProject(t, "Hidden "+token, f.restrictedProjectID)
	open := createIssueInProject(t, "Open "+token, f.workspaceProjectID)
	dbfx.Exec(t, `UPDATE issue SET description = $2 WHERE id = $1`, hidden, "Body "+token)

	body := searchIssuesAs(t, f.outsiderID, token, "")
	if strings.Contains(body, hidden) || strings.Contains(body, "Hidden "+token) || strings.Contains(body, "Body "+token) {
		t.Errorf("outsider search leaks the hidden issue: %s", body)
	}
	if !strings.Contains(body, open) {
		t.Errorf("outsider search misses the open issue: %s", body)
	}
	for name, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
		if body := searchIssuesAs(t, userID, token, ""); !strings.Contains(body, hidden) {
			t.Errorf("%s search misses the restricted issue: %s", name, body)
		}
	}
}

// TestRestrictedSearchCountsOmitHidden: the search returns no total, so the
// observable counts are the page contents. Hidden rows must neither appear nor
// take a page slot: with limit=1 the outsider still gets the visible issue
// even when hidden issues rank ahead, and the result has exactly one row.
func TestRestrictedSearchCountsOmitHidden(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	token := "needlecnt" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	// Titles starting with the token rank at least as high as the visible one.
	h1 := createIssueInProject(t, token+" one", f.restrictedProjectID)
	h2 := createIssueInProject(t, token+" two", f.restrictedProjectID)
	open := createIssueInProject(t, "Open "+token+" tail", f.workspaceProjectID)

	count := func(userID, extra string) (int, string) {
		body := searchIssuesAs(t, userID, token, extra)
		var out map[string]json.RawMessage
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for key := range out {
			if key != "issues" {
				if strings.Contains(strings.ToLower(key), "total") || strings.Contains(strings.ToLower(key), "count") {
					t.Errorf("unexpected count field %q: %s", key, body)
				}
			}
		}
		var issues []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(out["issues"], &issues); err != nil {
			t.Fatalf("decode issues: %v", err)
		}
		return len(issues), body
	}

	if n, body := count(f.outsiderID, "&limit=1"); n != 1 || !strings.Contains(body, open) || strings.Contains(body, h1) || strings.Contains(body, h2) {
		t.Errorf("outsider limit=1: want only the open issue, got %d rows: %s", n, body)
	}
	if n, body := count(f.outsiderID, ""); n != 1 {
		t.Errorf("outsider: want 1 row, got %d: %s", n, body)
	}
	if n, body := count(f.outsiderID, "&limit=1&offset=1"); n != 0 {
		t.Errorf("outsider offset=1: want 0 rows, got %d: %s", n, body)
	}
	if n, body := count(f.granteeID, ""); n != 3 {
		t.Errorf("grantee: want 3 rows, got %d: %s", n, body)
	}
	if n, body := count(testUserID, ""); n != 3 {
		t.Errorf("owner: want 3 rows, got %d: %s", n, body)
	}
}
