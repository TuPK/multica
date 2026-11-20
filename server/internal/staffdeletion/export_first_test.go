package staffdeletion

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// stageExportFirst stages a workspace request with export_first set, the way the
// platform does when staff ask for an export before the deletion (spec R6 3c.9).
func stageExportFirst(t *testing.T, ws string) string {
	t.Helper()
	reqID := stageRequest(t, "workspace", ws, "", "pending")
	fx.Exec(t, `UPDATE platform_deletion_request SET export_first = true WHERE id = $1::uuid`, reqID)
	return reqID
}

// stageArchive writes the workspace_export row the platform queues.
func stageArchive(t *testing.T, ws, status string) {
	t.Helper()
	fx.Exec(t, `INSERT INTO workspace_export (id, workspace_id, requested_by, status, storage_key, finished_at, expires_at)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, $3, CASE WHEN $3 = 'done' THEN 'exports/test.zip' ELSE '' END,
		        CASE WHEN $3 = 'done' THEN now() ELSE NULL END,
		        CASE WHEN $3 = 'done' THEN now() + interval '30 days' ELSE NULL END)`,
		ws, fx.User(t, "export requester", testutil.UniqueEmail("exp-req")), status)
}

// The request waits while the archive is queued, then deletes once it is done.
func TestStaffDeletionWaitsForExportThenMailsSignedLink(t *testing.T) {
	ws := workspaceWithOwner(t, "staff job export wait")
	stageArchive(t, ws, "queued")
	reqID := stageExportFirst(t, ws)

	if n, err := newRunner(nil).RunOnce(context.Background()); err != nil || n != 0 {
		t.Fatalf("RunOnce while queued = %d, %v; want 0, nil", n, err)
	}
	if state, _, _ := requestState(t, reqID); state != "pending" {
		t.Fatalf("request while the export is queued = %q, want pending", state)
	}
	if n := count(t, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, ws); n != 1 {
		t.Fatal("the workspace was deleted before its archive was ready")
	}

	fx.Exec(t, `UPDATE workspace_export SET status = 'done', storage_key = 'exports/test.zip', finished_at = now(), expires_at = now() + interval '30 days' WHERE workspace_id = $1::uuid`, ws)
	if n, err := newRunner(nil).RunOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("RunOnce once done = %d, %v; want 1, nil", n, err)
	}
	if state, _, code := requestState(t, reqID); state != "completed" || code != "" {
		t.Fatalf("request = %q code %q, want completed", state, code)
	}
}

// An archive that does not finish within 24 hours fails the request with
// export_timeout, and the workspace stays.
func TestStaffDeletionExportTimeoutFailsAndKeepsData(t *testing.T) {
	ws := workspaceWithOwner(t, "staff job export timeout")
	stageArchive(t, ws, "queued")
	reqID := stageExportFirst(t, ws)
	fx.Exec(t, `UPDATE platform_deletion_request SET created_at = now() - interval '25 hours' WHERE id = $1::uuid`, reqID)

	newRunner(nil).RunOnce(context.Background())
	state, attempts, code := requestState(t, reqID)
	if state != "failed" || code != "export_timeout" || attempts != 1 {
		t.Fatalf("request = %q attempts %d code %q, want failed/1/export_timeout", state, attempts, code)
	}
	if n := count(t, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, ws); n != 1 {
		t.Fatal("the workspace was deleted after an export timeout")
	}
}

// Without export_first the request does not wait for any archive.
func TestStaffDeletionWithoutExportFirstDoesNotWait(t *testing.T) {
	ws := workspaceWithOwner(t, "staff job no export")
	stageArchive(t, ws, "queued")
	reqID := stageRequest(t, "workspace", ws, "", "pending")

	if n, err := newRunner(nil).RunOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("RunOnce = %d, %v; want 1, nil", n, err)
	}
	if state, _, _ := requestState(t, reqID); state != "completed" {
		t.Fatalf("request = %q, want completed", state)
	}
}
