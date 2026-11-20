package staffdeletion

import (
	"context"
	"testing"
)

// A workspace request under a legal hold fails at once with error_code
// legal_hold, and the workspace and its rows stay (spec R6 section 3c.7, rule 5).
func TestStaffDeletionFailsWithLegalHoldAndKeepsData(t *testing.T) {
	ws := workspaceWithOwner(t, "staff job held workspace")
	fx.Exec(t, `INSERT INTO legal_hold (id, scope_kind, scope_id, reason, case_ref)
		VALUES (gen_random_uuid(), 'workspace', $1::uuid, 'staff job test', 'CASE-STAFF')`, ws)
	t.Cleanup(func() { fx.Exec(t, `DELETE FROM legal_hold WHERE scope_id = $1::uuid`, ws) })
	reqID := stageRequest(t, "workspace", ws, "", "pending")

	if n, err := newRunner(nil).RunOnce(context.Background()); err != nil || n != 0 {
		t.Fatalf("RunOnce = %d, %v; want 0, nil", n, err)
	}
	state, attempts, code := requestState(t, reqID)
	if state != "failed" || code != "legal_hold" || attempts != 1 {
		t.Fatalf("request = %q attempts %d code %q, want failed/1/legal_hold", state, attempts, code)
	}
	if n := count(t, `SELECT count(*) FROM workspace WHERE id = $1::uuid`, ws); n != 1 {
		t.Errorf("the held workspace was deleted: rows = %d", n)
	}
}
