package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/workspaceexport"
)

// Database-backed: they run only against the dedicated *_test database named by
// PLATFORM_TEST_DATABASE_URL (see requirePlatformTestDatabase). The pure rules
// (allowlist, archive, daily limit, expiry, job transitions) are tested in
// internal/workspaceexport. These tests were written without a database
// available and have not been run.

func exportWorkspace(t *testing.T, role string) string {
	t.Helper()
	wsID := dbfx.Workspace(t, "Export "+role, "wsexp-"+uuid.NewString()[:8])
	dbfx.Member(t, wsID, testUserID, role)
	dbfx.Cleanup(t, `DELETE FROM workspace_export WHERE workspace_id = $1::uuid`, wsID)
	dbfx.Cleanup(t, `DELETE FROM platform_workspace WHERE workspace_id = $1::uuid`, wsID)
	return wsID
}

func createExportRequest(wsID string) *http.Request {
	return withURLParam(newRequest(http.MethodPost, "/api/workspaces/"+wsID+"/exports", nil), "id", wsID)
}

func listExportsRequest(wsID string) *http.Request {
	return withURLParam(newRequest(http.MethodGet, "/api/workspaces/"+wsID+"/exports", nil), "id", wsID)
}

func getExportRequest(wsID, exportID string) *http.Request {
	return withURLParams(newRequest(http.MethodGet, "/api/workspaces/"+wsID+"/exports/"+exportID, nil),
		"id", wsID, "exportId", exportID)
}

type recordingObjects struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (o *recordingObjects) Upload(_ context.Context, key string, data []byte, _, _ string) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.data == nil {
		o.data = map[string][]byte{}
	}
	o.data[key] = data
	return key, nil
}
func (o *recordingObjects) DeleteObject(context.Context, string) error { return nil }

func runExportJob(t *testing.T, objects workspaceexport.Objects) {
	t.Helper()
	r := &workspaceexport.Runner{
		Store:   workspaceexport.NewDBStore(testHandler.Queries),
		Source:  workspaceexport.NewDBSource(testHandler.Queries),
		Objects: objects,
		Now:     time.Now,
	}
	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSecondExportAtOnceGetsExportInProgress(t *testing.T) {
	requirePlatformTestDatabase(t)
	wsID := exportWorkspace(t, "owner")

	testutil.Call(t, testHandler.CreateWorkspaceExport, createExportRequest(wsID)).Want(http.StatusAccepted)
	resp := testutil.Call(t, testHandler.CreateWorkspaceExport, createExportRequest(wsID)).Want(http.StatusConflict)

	var out map[string]string
	resp.JSON(&out)
	if out["code"] != "export_in_progress" {
		t.Fatalf("code = %q, want export_in_progress", out["code"])
	}
}

func TestConcurrentExportRequestsCreateOne(t *testing.T) {
	requirePlatformTestDatabase(t)
	wsID := exportWorkspace(t, "owner")

	codes := make(chan int, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := testutil.Call(t, testHandler.CreateWorkspaceExport, createExportRequest(wsID))
			codes <- resp.Code
		}()
	}
	wg.Wait()
	close(codes)
	accepted := 0
	for c := range codes {
		switch c {
		case http.StatusAccepted:
			accepted++
		case http.StatusConflict:
		default:
			t.Errorf("status %d, want 202 or 409", c)
		}
	}
	if accepted != 1 {
		t.Fatalf("%d requests were accepted, want 1", accepted)
	}
}

func TestExportWithinADayOfTheLastGetsExportDailyLimit(t *testing.T) {
	requirePlatformTestDatabase(t)
	wsID := exportWorkspace(t, "owner")
	dbfx.Exec(t, `INSERT INTO workspace_export (id, workspace_id, requested_by, status, created_at)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, 'done', now() - interval '23 hours')`, wsID, testUserID)

	resp := testutil.Call(t, testHandler.CreateWorkspaceExport, createExportRequest(wsID)).Want(http.StatusTooManyRequests)
	var out map[string]string
	resp.JSON(&out)
	if out["code"] != "export_daily_limit" {
		t.Fatalf("code = %q, want export_daily_limit", out["code"])
	}
}

func TestExportAfterADayIsAccepted(t *testing.T) {
	requirePlatformTestDatabase(t)
	wsID := exportWorkspace(t, "owner")
	dbfx.Exec(t, `INSERT INTO workspace_export (id, workspace_id, requested_by, status, created_at)
		VALUES (gen_random_uuid(), $1::uuid, $2::uuid, 'done', now() - interval '25 hours')`, wsID, testUserID)

	testutil.Call(t, testHandler.CreateWorkspaceExport, createExportRequest(wsID)).Want(http.StatusAccepted)
}

func TestAMemberAndAnAdminGet403OnAllThreeExportRoutes(t *testing.T) {
	requirePlatformTestDatabase(t)
	for _, role := range []string{"member", "admin"} {
		wsID := exportWorkspace(t, role)
		dbfx.Exec(t, `INSERT INTO workspace_export (id, workspace_id, requested_by, status)
			VALUES (gen_random_uuid(), $1::uuid, $2::uuid, 'done')`, wsID, testUserID)
		var exportID string
		if err := testPool.QueryRow(context.Background(),
			`SELECT id::text FROM workspace_export WHERE workspace_id = $1::uuid`, wsID).Scan(&exportID); err != nil {
			t.Fatal(err)
		}

		testutil.Call(t, testHandler.CreateWorkspaceExport, createExportRequest(wsID)).Want(http.StatusForbidden)
		testutil.Call(t, testHandler.ListWorkspaceExports, listExportsRequest(wsID)).Want(http.StatusForbidden)
		testutil.Call(t, testHandler.GetWorkspaceExport, getExportRequest(wsID, exportID)).Want(http.StatusForbidden)
		if n := dbfx.Count(t, `SELECT count(*) FROM workspace_export WHERE workspace_id = $1::uuid`, wsID); n != 1 {
			t.Fatalf("%s: %d export rows, want only the seeded one", role, n)
		}
	}
}

func TestExportOfASuspendedWorkspaceCompletes(t *testing.T) {
	requirePlatformTestDatabase(t)
	wsID := exportWorkspace(t, "owner")
	dbfx.Exec(t, `INSERT INTO platform_workspace (workspace_id, status, reason)
		VALUES ($1::uuid, 'suspended', 'test')`, wsID)
	dbfx.Issue(t, "Suspended issue", map[string]any{"workspace_id": wsID})

	var created WorkspaceExportResponse
	testutil.Call(t, testHandler.CreateWorkspaceExport, createExportRequest(wsID)).Want(http.StatusAccepted).JSON(&created)
	runExportJob(t, &recordingObjects{})

	var got WorkspaceExportResponse
	testutil.Call(t, testHandler.GetWorkspaceExport, getExportRequest(wsID, created.ID)).Want(http.StatusOK).JSON(&got)
	if got.Status != "done" || got.ExpiresAt == nil {
		t.Fatalf("export = %+v, want done with an expiry", got)
	}
	var state string
	if err := testPool.QueryRow(context.Background(),
		`SELECT status FROM platform_workspace WHERE workspace_id = $1::uuid`, wsID).Scan(&state); err != nil || state != "suspended" {
		t.Fatalf("platform_workspace status = %q, %v; the export must not change it", state, err)
	}
}

func TestExportDoesNotMixTwoWorkspaces(t *testing.T) {
	requirePlatformTestDatabase(t)
	a := exportWorkspace(t, "owner")
	b := exportWorkspace(t, "owner")
	dbfx.Issue(t, "ISSUE-OF-A", map[string]any{"workspace_id": a})
	dbfx.Issue(t, "ISSUE-OF-B", map[string]any{"workspace_id": b})

	var created WorkspaceExportResponse
	testutil.Call(t, testHandler.CreateWorkspaceExport, createExportRequest(a)).Want(http.StatusAccepted).JSON(&created)
	objects := &recordingObjects{}
	runExportJob(t, objects)

	if len(objects.data) == 0 {
		t.Fatal("nothing was uploaded")
	}
	data, _, err := workspaceexport.BuildArchive(context.Background(), workspaceexport.NewDBSource(testHandler.Queries), a, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !archiveHas(t, data, "ISSUE-OF-A") || archiveHas(t, data, "ISSUE-OF-B") {
		t.Fatal("the archive of A must hold A's issue and never B's")
	}
}

func archiveHas(t *testing.T, zipped []byte, needle string) bool {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), needle) {
			return true
		}
	}
	return false
}
