package legalhold

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database. Each test removes the holds and audit rows it wrote.

func newHoldFixture(t *testing.T) (*Service, *pgxpool.Pool, string) {
	t.Helper()
	pool := testdb.Open(t)
	var scope string
	if err := pool.QueryRow(context.Background(), `SELECT gen_random_uuid()::text`).Scan(&scope); err != nil {
		t.Fatalf("make scope id: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		pool.Exec(ctx, `DELETE FROM audit_event WHERE target_type = 'legal_hold' AND detail->>'legal_hold_id' IN (SELECT id::text FROM legal_hold WHERE scope_id = $1)`, scope)
		pool.Exec(ctx, `DELETE FROM legal_hold WHERE scope_id = $1`, scope)
	})
	return New(pool), pool, scope
}

// Spec R6 section 3c.7 rule 4: the audit row of a placed hold names the hold
// only; the reason and the case reference stay out of it.
func TestLegalHoldAuditDetailHasOnlyHoldID(t *testing.T) {
	svc, pool, scope := newHoldFixture(t)
	const reason = "court order 2026-11-20 for matter"
	const caseRef = "CASE-AUDIT-7741"

	h, err := svc.Place(context.Background(), Actor{Label: "staff@example.test"}, KindWorkspace, scope, "", reason, caseRef)
	if err != nil {
		t.Fatalf("place: %v", err)
	}

	var raw []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT detail FROM audit_event WHERE action = 'legal_hold.placed' AND target_id = $1`, h.ID).Scan(&raw); err != nil {
		t.Fatalf("read audit row: %v", err)
	}
	var detail map[string]any
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("audit detail is not JSON: %v", err)
	}
	if len(detail) != 1 || detail["legal_hold_id"] != h.ID {
		t.Fatalf("audit detail = %v, want only legal_hold_id=%s", detail, h.ID)
	}
	for _, leaked := range []string{reason, caseRef} {
		if strings.Contains(string(raw), leaked) {
			t.Fatalf("audit detail leaks %q: %s", leaked, raw)
		}
	}
}

// Spec R6 section 3c.7: a second live hold on the same scope is 409 legal_hold_exists.
func TestSecondHoldOnSameScopeIs409(t *testing.T) {
	svc, pool, scope := newHoldFixture(t)
	ctx := context.Background()

	if _, err := svc.Place(ctx, Actor{}, KindWorkspace, scope, "", "first order", "CASE-1"); err != nil {
		t.Fatalf("first place: %v", err)
	}
	_, err := svc.Place(ctx, Actor{}, KindWorkspace, scope, "", "second order", "CASE-2")
	var le *Error
	if !errors.As(err, &le) {
		t.Fatalf("second place error = %v, want *legalhold.Error", err)
	}
	if le.Status != http.StatusConflict || le.Code != CodeExists {
		t.Fatalf("second place = %d %s, want 409 %s", le.Status, le.Code, CodeExists)
	}

	var live int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM legal_hold WHERE scope_id = $1 AND released_at IS NULL`, scope).Scan(&live); err != nil {
		t.Fatalf("count live holds: %v", err)
	}
	if live != 1 {
		t.Fatalf("live holds on scope = %d, want 1", live)
	}
}

// Spec R6 section 3c.7: a released hold stays as history with released_at set,
// and releasing it again finds nothing live.
func TestReleaseKeepsRowAsHistory(t *testing.T) {
	svc, pool, scope := newHoldFixture(t)
	ctx := context.Background()

	h, err := svc.Place(ctx, Actor{}, KindWorkspace, scope, "", "hold for review", "CASE-H")
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	released, err := svc.Release(ctx, Actor{}, h.ID, "")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if !released.Released {
		t.Fatalf("release result Released = false")
	}

	var rows int
	var releasedAt *string
	if err := pool.QueryRow(ctx,
		`SELECT count(*), max(released_at::text) FROM legal_hold WHERE id = $1`, h.ID).Scan(&rows, &releasedAt); err != nil {
		t.Fatalf("read history row: %v", err)
	}
	if rows != 1 || releasedAt == nil {
		t.Fatalf("history rows = %d, released_at set = %v; want the row kept with released_at", rows, releasedAt != nil)
	}

	if _, err := svc.Release(ctx, Actor{}, h.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second release error = %v, want ErrNotFound", err)
	}
}
