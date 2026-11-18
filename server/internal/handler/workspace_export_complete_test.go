package handler

// The workspace export belongs to the owner, who sees every project (spec 5.11
// and 7.1 rule 4). Narrowing derived reads to the viewer's scope must not
// thin it out.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/workspaceexport"
)

// TestWorkspaceExportIsOwnerOnlyAndComplete: a member and an admin are refused
// on the export routes; the owner's archive holds a restricted project, its
// issue and a comment of that issue.
func TestWorkspaceExportIsOwnerOnlyAndComplete(t *testing.T) {
	requirePlatformTestDatabase(t)
	marker := uuid.NewString()[:8]

	// Not the owner: refused, and nothing is queued.
	for _, role := range []string{"member", "admin"} {
		wsID := exportWorkspace(t, role)
		testutil.Call(t, testHandler.CreateWorkspaceExport, createExportRequest(wsID)).Want(http.StatusForbidden)
		if n := dbfx.Count(t, `SELECT count(*) FROM workspace_export WHERE workspace_id = $1::uuid`, wsID); n != 0 {
			t.Fatalf("%s: a refused request queued %d exports", role, n)
		}
	}

	wsID := exportWorkspace(t, "owner")
	restricted := dbfx.Project(t, "EXPORT-PROJECT-"+marker, testutil.Cols{"workspace_id": wsID, "access": "restricted"})
	issueID := dbfx.Issue(t, "EXPORT-ISSUE-"+marker, testutil.Cols{"workspace_id": wsID, "project_id": restricted})
	dbfx.Comment(t, issueID, "EXPORT-COMMENT-"+marker, testutil.Cols{"workspace_id": wsID})

	testutil.Call(t, testHandler.CreateWorkspaceExport, createExportRequest(wsID)).Want(http.StatusAccepted)
	data, _, err := workspaceexport.BuildArchive(context.Background(), workspaceexport.NewDBSource(testHandler.Queries), wsID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"EXPORT-PROJECT-" + marker, "EXPORT-ISSUE-" + marker, "EXPORT-COMMENT-" + marker} {
		if !archiveHas(t, data, needle) {
			t.Errorf("the owner's archive lacks %q", needle)
		}
	}
}
