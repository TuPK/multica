package handler

// Visibility of restricted projects on the remaining issue read paths (spec 7.1
// rules 3 and 5): single-issue reads answer like a missing issue, lists,
// counts, facets, search and wakeup lists omit hidden issues, and a parent
// lookup does not reveal a hidden parent. The fixture is the one of
// project_restricted_visibility_test.go.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// issueNumber returns the per-workspace number of an issue.
func issueNumber(t *testing.T, issueID string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), `SELECT number FROM issue WHERE id = $1`, issueID).Scan(&n); err != nil {
		t.Fatalf("read issue number: %v", err)
	}
	return n
}

// markDuplicate makes dupID a cancelled duplicate of originalID.
func markRestrictedDuplicate(t *testing.T, dupID, originalID string) {
	t.Helper()
	dbfx.Exec(t, `UPDATE issue SET status = 'cancelled', duplicate_of_issue_id = $2 WHERE id = $1`, dupID, originalID)
}

func serveAs(handler http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

// TestRestrictedIssueSingleReadsAnswerMissing: for the outsider every
// single-issue read of a hidden issue has the status and body of the same read
// of an issue that does not exist; the grantee and the owner read it.
func TestRestrictedIssueSingleReadsAnswerMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	hiddenID := createIssueInProject(t, "single read hidden "+uuid.NewString()[:8], f.restrictedProjectID)
	missingID := uuid.NewString()
	prefix := testHandler.getIssuePrefix(context.Background(), parseUUID(testWorkspaceID))
	hiddenIdent := prefix + "-" + strconv.Itoa(issueNumber(t, hiddenID))
	missingIdent := prefix + "-999999"

	reads := map[string]struct {
		handler http.HandlerFunc
		method  string
		path    string // %s is replaced by the issue id or identifier
	}{
		"GetIssue":           {testHandler.GetIssue, "GET", "/api/issues/%s"},
		"ListIssueMetadata":  {testHandler.ListIssueMetadata, "GET", "/api/issues/%s/metadata"},
		"ListIssueDuplicate": {testHandler.ListIssueDuplicates, "GET", "/api/issues/%s/duplicates"},
		"ListChildIssues":    {testHandler.ListChildIssues, "GET", "/api/issues/%s/children"},
		"ListIssueWakeups":   {testHandler.ListIssueWakeups, "GET", "/api/issues/%s/wakeups"},
	}
	for name, read := range reads {
		call := func(userID, id string) *httptest.ResponseRecorder {
			req := requestAs(userID, read.method, strings.Replace(read.path, "%s", id, 1), nil)
			return serveAs(read.handler, withURLParam(req, "id", id))
		}
		for label, ids := range map[string][2]string{"uuid": {hiddenID, missingID}, "identifier": {hiddenIdent, missingIdent}} {
			hidden, missing := call(f.outsiderID, ids[0]), call(f.outsiderID, ids[1])
			if missing.Code != http.StatusNotFound {
				t.Fatalf("%s by %s: a missing issue answers %d, want 404", name, label, missing.Code)
			}
			if hidden.Code != missing.Code ||
				strings.ReplaceAll(hidden.Body.String(), ids[0], "ID") != strings.ReplaceAll(missing.Body.String(), ids[1], "ID") {
				t.Errorf("%s by %s as outsider: hidden answers %d %s, missing answers %d %s",
					name, label, hidden.Code, hidden.Body.String(), missing.Code, missing.Body.String())
			}
			for who, userID := range map[string]string{"grantee": f.granteeID, "owner": testUserID} {
				if w := call(userID, ids[0]); w.Code != http.StatusOK {
					t.Errorf("%s by %s as %s: want 200, got %d %s", name, label, who, w.Code, w.Body.String())
				}
			}
		}
	}

	// The duplicate mark of a visible issue that points at a hidden original
	// carries no reference to the original, as if the original were missing.
	openOriginalID := createIssueInProject(t, "open original "+uuid.NewString()[:8], f.workspaceProjectID)
	openDupID := createIssueInProject(t, "open duplicate "+uuid.NewString()[:8], f.workspaceProjectID)
	markRestrictedDuplicate(t, openDupID, hiddenID)
	getDup := func(userID string) string {
		w := serveAs(testHandler.GetIssue, withURLParam(requestAs(userID, "GET", "/api/issues/"+openDupID, nil), "id", openDupID))
		if w.Code != http.StatusOK {
			t.Fatalf("GetIssue duplicate as %s: %d %s", userID, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	if body := getDup(f.outsiderID); strings.Contains(body, hiddenID) || strings.Contains(body, "single read hidden") {
		t.Errorf("GetIssue as outsider names the hidden original: %s", body)
	}
	if body := getDup(testUserID); !strings.Contains(body, "single read hidden") {
		t.Errorf("GetIssue as owner lacks the original reference: %s", body)
	}
	listDuplicates := func(userID, issueID string) string {
		w := serveAs(testHandler.ListIssueDuplicates, withURLParam(requestAs(userID, "GET", "/api/issues/"+issueID+"/duplicates", nil), "id", issueID))
		if w.Code != http.StatusOK {
			t.Fatalf("ListIssueDuplicates as %s: %d %s", userID, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	if body := listDuplicates(f.outsiderID, openDupID); strings.Contains(body, hiddenID) {
		t.Errorf("ListIssueDuplicates as outsider returns the hidden original: %s", body)
	}
	if body := listDuplicates(testUserID, openDupID); !strings.Contains(body, hiddenID) {
		t.Errorf("ListIssueDuplicates as owner lacks the original: %s", body)
	}

	// A hidden issue that duplicates a visible one is not listed under it.
	hiddenDupID := createIssueInProject(t, "hidden duplicate "+uuid.NewString()[:8], f.restrictedProjectID)
	markRestrictedDuplicate(t, hiddenDupID, openOriginalID)
	if body := listDuplicates(f.outsiderID, openOriginalID); strings.Contains(body, hiddenDupID) {
		t.Errorf("ListIssueDuplicates as outsider lists a hidden duplicate: %s", body)
	}
	if body := listDuplicates(testUserID, openOriginalID); !strings.Contains(body, hiddenDupID) {
		t.Errorf("ListIssueDuplicates as owner lacks the duplicate: %s", body)
	}
}

// TestRestrictedProjectViewsAnswerMissing: a saved view or a view preference
// for a project the outsider cannot see answers like one for a missing project.
func TestRestrictedProjectViewsAnswerMissing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	missingID := uuid.NewString()
	create := func(userID, projectID string) *httptest.ResponseRecorder {
		w := serveAs(testHandler.CreateIssueView, requestAs(userID, "POST", "/api/issue-views", map[string]any{
			"name": "restricted view", "scope_type": "project", "scope_id": projectID, "visibility": "private",
			"query": map[string]any{}, "display": map[string]any{},
		}))
		if w.Code == http.StatusCreated {
			var view IssueViewResponse
			json.Unmarshal(w.Body.Bytes(), &view)
			t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue_view WHERE id = $1`, view.ID) })
		}
		return w
	}
	preference := func(userID, projectID string) *httptest.ResponseRecorder {
		return serveAs(testHandler.GetIssueViewPreference, requestAs(userID, "GET",
			"/api/issue-view-preferences?scope_type=project&scope_id="+projectID, nil))
	}
	for name, call := range map[string]func(userID, projectID string) *httptest.ResponseRecorder{
		"CreateIssueView": create, "GetIssueViewPreference": preference,
	} {
		hidden, missing := call(f.outsiderID, f.restrictedProjectID), call(f.outsiderID, missingID)
		if missing.Code != http.StatusNotFound {
			t.Fatalf("%s: a missing project answers %d %s, want 404", name, missing.Code, missing.Body.String())
		}
		if hidden.Code != missing.Code ||
			strings.ReplaceAll(hidden.Body.String(), f.restrictedProjectID, "ID") != strings.ReplaceAll(missing.Body.String(), missingID, "ID") {
			t.Errorf("%s as outsider: hidden project answers %d %s, missing answers %d %s",
				name, hidden.Code, hidden.Body.String(), missing.Code, missing.Body.String())
		}
		if w := call(f.granteeID, f.restrictedProjectID); w.Code != http.StatusOK && w.Code != http.StatusCreated {
			t.Errorf("%s as grantee: want 200 or 201, got %d %s", name, w.Code, w.Body.String())
		}
	}
}

// TestRestrictedIssueListsOmitHidden: lists, counts, facets, groups, search,
// wakeup lists and the run queue leave out the hidden issue for the outsider.
func TestRestrictedIssueListsOmitHidden(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	token := "rl" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	hiddenID := createIssueInProject(t, "hidden "+token, f.restrictedProjectID)
	openID := createIssueInProject(t, "open "+token, f.workspaceProjectID)

	// check runs one read as the outsider and as the owner. The outsider's body
	// must hold the open issue and nothing of the hidden one; the owner's body
	// must hold the hidden one, which proves the fixture reaches the read.
	check := func(name string, read func(userID string) *httptest.ResponseRecorder, hiddenMarkers ...string) {
		t.Helper()
		if len(hiddenMarkers) == 0 {
			hiddenMarkers = []string{hiddenID, "hidden " + token}
		}
		out, owner := read(f.outsiderID), read(testUserID)
		if out.Code != http.StatusOK || owner.Code != http.StatusOK {
			t.Errorf("%s: outsider %d %s, owner %d %s", name, out.Code, out.Body.String(), owner.Code, owner.Body.String())
			return
		}
		for _, marker := range hiddenMarkers {
			if strings.Contains(out.Body.String(), marker) {
				t.Errorf("%s as outsider contains hidden %q: %s", name, marker, out.Body.String())
			}
			if !strings.Contains(owner.Body.String(), marker) {
				t.Errorf("%s as owner lacks %q, so the fixture does not reach this read: %s", name, marker, owner.Body.String())
			}
		}
	}
	get := func(handler http.HandlerFunc, path string) func(string) *httptest.ResponseRecorder {
		return func(userID string) *httptest.ResponseRecorder {
			return serveAs(handler, requestAs(userID, "GET", path, nil))
		}
	}
	post := func(handler http.HandlerFunc, path string, body any) func(string) *httptest.ResponseRecorder {
		return func(userID string) *httptest.ResponseRecorder {
			return serveAs(handler, requestAs(userID, "POST", path, body))
		}
	}

	ws := "workspace_id=" + testWorkspaceID
	check("ListIssues", get(testHandler.ListIssues, "/api/issues?"+ws+"&limit=500"))
	check("QueryIssues", post(testHandler.QueryIssues, "/api/issues/query", map[string]string{"workspace_id": testWorkspaceID, "limit": "500"}))
	check("SearchIssues", get(testHandler.SearchIssues, "/api/issues/search?"+ws+"&q="+url.QueryEscape(token)))
	check("ListGroupedIssues", get(testHandler.ListGroupedIssues, "/api/issues/grouped?"+ws+"&limit=100"))

	query := issueTableQuerySpec{
		Scope:  issueTableScope{Kind: "workspace"},
		Search: token,
		Sort:   issueTableSortRequest{Field: "title", Direction: "asc"},
	}
	check("ListIssueTableRows", post(testHandler.ListIssueTableRows, "/api/issues/table/rows", issueTableRowsRequest{
		Query: query, Group: issueTableGroupSpec{Kind: "none"}, Page: issueTablePageRequest{Limit: 100},
	}))
	check("ListIssueTableRows hierarchy", post(testHandler.ListIssueTableRows, "/api/issues/table/rows", issueTableRowsRequest{
		Query: query, Group: issueTableGroupSpec{Kind: "none"}, Hierarchy: issueTableHierarchyRequest{Enabled: true}, Page: issueTablePageRequest{Limit: 100},
	}))
	check("ListIssueTableGroups project", post(testHandler.ListIssueTableGroups, "/api/issues/table/groups", issueTableGroupsRequest{
		Query: query, Group: issueTableGroupSpec{Kind: "project"}, Page: issueTablePageRequest{Limit: 100},
	}), f.restrictedProjectID)
	check("ListIssueTableFacets project", post(testHandler.ListIssueTableFacets, "/api/issues/table/facets", issueTableFacetsRequest{
		Query: query, Facets: []issueTableFacetSpec{{Kind: "project"}},
	}), f.restrictedProjectID)

	// The total of a facet request counts the visible issues only.
	total := func(userID string) int64 {
		var out struct {
			Total int64 `json:"total"`
		}
		w := post(testHandler.ListIssueTableFacets, "/api/issues/table/facets", issueTableFacetsRequest{
			Query: query, Facets: []issueTableFacetSpec{{Kind: "status"}},
		})(userID)
		if w.Code != http.StatusOK {
			t.Fatalf("ListIssueTableFacets total as %s: %d %s", userID, w.Code, w.Body.String())
		}
		json.Unmarshal(w.Body.Bytes(), &out)
		return out.Total
	}
	if o, p := total(testUserID), total(f.outsiderID); o != 2 || p != 1 {
		t.Errorf("facet total: owner %d (want 2), outsider %d (want 1)", o, p)
	}

	// A visible parent with a hidden child and a visible child.
	parentID := createIssueInProject(t, "parent "+token, f.workspaceProjectID)
	hiddenChildBody := map[string]any{"title": "hidden child " + token, "status": "todo", "priority": "none",
		"project_id": f.restrictedProjectID, "parent_issue_id": parentID}
	w := httptest.NewRecorder()
	testHandler.CreateIssue(w, newRequest("POST", "/api/issues?"+ws, hiddenChildBody))
	if w.Code != http.StatusCreated {
		t.Fatalf("create hidden child: %d %s", w.Code, w.Body.String())
	}
	var hiddenChild IssueResponse
	json.Unmarshal(w.Body.Bytes(), &hiddenChild)
	t.Cleanup(func() { deleteTestIssue(t, hiddenChild.ID) })
	check("ListChildrenByParents", get(testHandler.ListChildrenByParents, "/api/issues/children?"+ws+"&parent_ids="+parentID), hiddenChild.ID)
	check("ListChildIssues", func(userID string) *httptest.ResponseRecorder {
		return serveAs(testHandler.ListChildIssues, withURLParam(requestAs(userID, "GET", "/api/issues/"+parentID+"/children", nil), "id", parentID))
	}, hiddenChild.ID)

	// Wakeup rules of the hidden issue.
	agent := dbfx.Agent(t, "restricted reads target "+token, testRuntimeID)
	svc := service.IssueWakeupService{Tasks: testHandler.TaskService}
	dbfx.Cleanup(t, "DELETE FROM issue_wakeup_receipt WHERE wakeup_id IN (SELECT id FROM issue_wakeup WHERE issue_id IN ($1, $2))", hiddenID, openID)
	dbfx.Cleanup(t, "DELETE FROM issue_wakeup WHERE issue_id IN ($1, $2)", hiddenID, openID)
	dbfx.Cleanup(t, "DELETE FROM activity_log WHERE issue_id IN ($1, $2)", hiddenID, openID)
	rule := func(issueID string) string {
		created, err := svc.Create(context.Background(), parseUUID(issueID), parseUUID(testUserID), pgtype.UUID{},
			service.WakeupInput{AgentID: agent, Kind: "event", EventTypes: []string{"comment.created"}, Instruction: "check"})
		if err != nil {
			t.Fatalf("create wakeup: %v", err)
		}
		return uuidToString(created.ID)
	}
	hiddenRule, openRule := rule(hiddenID), rule(openID)
	dbfx.Exec(t, `UPDATE issue_wakeup SET enabled = false, paused_reason = 'rate', disabled_at = now() WHERE id IN ($1, $2)`, hiddenRule, openRule)
	check("ListWorkspaceWakeups", get(testHandler.ListWorkspaceWakeups, "/api/issue-wakeups?scope=all&limit=100&search="+url.QueryEscape(token)), hiddenRule)
	check("ListPausedWakeups", get(testHandler.ListPausedWakeups, "/api/issue-wakeup-paused"), hiddenRule)
	dbfx.Exec(t, `UPDATE issue_wakeup SET enabled = true, paused_reason = NULL, disabled_at = NULL WHERE id IN ($1, $2)`, hiddenRule, openRule)
	check("ListWorkspaceWakeupSummaries", get(testHandler.ListWorkspaceWakeupSummaries, "/api/issue-wakeup-summaries"), hiddenRule)

	// Property usage counts only the issues the viewer sees.
	propertyID := dbfx.Insert(t, "issue_property", testutil.Cols{"workspace_id": testWorkspaceID, "name": "restricted usage " + token, "type": "text"})
	dbfx.Exec(t, `UPDATE issue SET properties = jsonb_build_object($2::text, 'x') WHERE id = ANY($1::uuid[])`, []string{hiddenID, openID}, propertyID)
	usage := func(userID string) int64 {
		w := serveAs(testHandler.ListProperties, requestAs(userID, "GET", "/api/properties?"+ws, nil))
		var out struct {
			Properties []struct {
				ID         string `json:"id"`
				UsageCount int64  `json:"usage_count"`
			} `json:"properties"`
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatalf("ListProperties as %s: %d %s", userID, w.Code, w.Body.String())
		}
		for _, p := range out.Properties {
			if p.ID == propertyID {
				return p.UsageCount
			}
		}
		t.Fatalf("ListProperties as %s lacks the property", userID)
		return 0
	}
	if o, p := usage(testUserID), usage(f.outsiderID); o != 2 || p != 1 {
		t.Errorf("property usage_count: owner %d (want 2), outsider %d (want 1)", o, p)
	}

	// Trigger preview names the runs of issues the viewer sees only.
	triggerAgent := dbfx.Agent(t, "restricted trigger "+token, testRuntimeID, testutil.Cols{"visibility": "workspace", "permission_mode": "public_to"})
	dbfx.Insert(t, "agent_invocation_target", testutil.Cols{"agent_id": triggerAgent, "target_type": "workspace", "target_id": testWorkspaceID})
	dbfx.Exec(t, `UPDATE issue SET status = 'backlog', assignee_type = 'agent', assignee_id = $2 WHERE id = ANY($1::uuid[])`, []string{hiddenID, openID}, triggerAgent)
	check("PreviewIssueTrigger", post(testHandler.PreviewIssueTrigger, "/api/issues/preview-trigger", map[string]any{
		"issue_ids": []string{hiddenID, openID}, "status": "todo",
	}), hiddenID)

	// Run queue: a queued hidden issue waits on the open one, and a queued open
	// issue waits on the hidden one.
	queuedOpenID := createIssueInProject(t, "queued open "+token, f.workspaceProjectID)
	dbfx.Exec(t, `UPDATE issue SET status = 'backlog', run_queued = true, run_after_issue_ids = ARRAY[$2::uuid] WHERE id = $1`, hiddenID, openID)
	dbfx.Exec(t, `UPDATE issue SET status = 'backlog', run_queued = true, run_after_issue_ids = ARRAY[$2::uuid, $3::uuid] WHERE id = $1`, queuedOpenID, openID, hiddenID)
	check("ListRunQueueWaits", get(testHandler.ListRunQueueWaits, "/api/issues/run-queue/waits?"+ws), hiddenID)
}

// TestParentLookupDoesNotLeakHiddenParent: a child in a workspace project whose
// parent is in a restricted project. The table group context and sort key of a
// parent group, and the promoted parents of a filtered compound group, do not
// carry the parent's title, number or status to the outsider.
func TestParentLookupDoesNotLeakHiddenParent(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	f := newRestrictedFixture(t)
	token := "pl" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	hiddenParentID := createIssueInProject(t, "secret parent "+token, f.restrictedProjectID)
	w := httptest.NewRecorder()
	testHandler.CreateIssue(w, newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title": "visible child " + token, "status": "todo", "priority": "none",
		"project_id": f.workspaceProjectID, "parent_issue_id": hiddenParentID,
	}))
	if w.Code != http.StatusCreated {
		t.Fatalf("create child: %d %s", w.Code, w.Body.String())
	}
	var child IssueResponse
	json.Unmarshal(w.Body.Bytes(), &child)
	t.Cleanup(func() { deleteTestIssue(t, child.ID) })

	query := issueTableQuerySpec{
		Scope:  issueTableScope{Kind: "workspace"},
		Search: token,
		Sort:   issueTableSortRequest{Field: "title", Direction: "asc"},
	}
	byParent := issueTableGroupSpec{Kind: "parent"}
	filtered := issueTableGroupSpec{Kind: "compound", Primary: "parent", Secondary: "status", SecondaryValues: []string{"todo"}}
	groups := func(userID string, group issueTableGroupSpec) string {
		t.Helper()
		w := serveAs(testHandler.ListIssueTableGroups, requestAs(userID, "POST", "/api/issues/table/groups", issueTableGroupsRequest{
			Query: query, Group: group, Page: issueTablePageRequest{Limit: 100},
		}))
		if w.Code != http.StatusOK {
			t.Fatalf("ListIssueTableGroups as %s: %d %s", userID, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	rows := func(userID string, group issueTableGroupSpec, key string) string {
		t.Helper()
		w := serveAs(testHandler.ListIssueTableRows, requestAs(userID, "POST", "/api/issues/table/rows", issueTableRowsRequest{
			Query: query, Group: group, GroupKey: &key, Page: issueTablePageRequest{Limit: 100},
		}))
		if w.Code != http.StatusOK {
			t.Fatalf("ListIssueTableRows as %s: %d %s", userID, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	if body := rows(f.outsiderID, byParent, "parent:"+hiddenParentID); strings.Contains(body, "secret parent") {
		t.Errorf("parent group rows as outsider reveal the hidden parent: %s", body)
	}
	for name, group := range map[string]issueTableGroupSpec{"parent": byParent, "compound parent": filtered} {
		if body := groups(testUserID, group); !strings.Contains(body, "secret parent "+token) {
			t.Fatalf("%s groups as owner lack the parent context, so the fixture does not reach the lookup: %s", name, body)
		}
		if body := groups(f.outsiderID, group); strings.Contains(body, "secret parent") {
			t.Errorf("%s groups as outsider reveal the hidden parent: %s", name, body)
		}
	}
}
