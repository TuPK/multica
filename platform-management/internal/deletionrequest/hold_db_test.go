package deletionrequest

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

// Spec R6 section 3c.7 rule 3: a live hold on a workspace refuses its immediate
// deletion with no write, and the refusal is audited on its own.

func TestLegalHoldBlocksPhaseOneWithoutWriting(t *testing.T) {
	f := newFixture(t)
	ws := f.workspace(false)
	f.exec(`INSERT INTO legal_hold (id, scope_kind, scope_id, reason, case_ref) VALUES (gen_random_uuid(), 'workspace', $1, 'court order', 'CASE-9')`, ws)
	t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM legal_hold WHERE scope_id = $1`, ws) })
	svc := New(Deps{Runner: NewPgxRunner(f.pool), Reader: NewPgxReader(f.pool), Cache: f.cache, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})

	_, _, err := svc.RequestWorkspace(context.Background(), f.staff, ws, Input{Reason: "closing", Confirm: f.slug(ws)})
	de := asError(t, err)
	if de.Code != CodeLegalHold || de.Status != 409 {
		t.Fatalf("got %d %s, want 409 legal_hold", de.Status, de.Code)
	}
	if f.requestsFor(ws) != 0 || f.status(ws) != "active" {
		t.Fatal("a held workspace was written to")
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = 'deletion.blocked_by_hold'`, ws); n != 1 {
		t.Fatalf("blocked audit rows = %d", n)
	}
}
