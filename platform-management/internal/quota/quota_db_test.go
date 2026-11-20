package quota

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/plan"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests of quota requests (spec R6 section 3c.5). They run only when
// testdb.Open allows the test database and remove the rows they wrote.

type requestFixture struct {
	pool *pgxpool.Pool
	svc  *Service
	ws   string
}

// newRequestFixture makes a workspace with no subscription row, so it is on the
// free plan, and a service over the test database.
func newRequestFixture(t *testing.T) *requestFixture {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()
	var ws string
	if err := pool.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ('Quota Test', 'qt-' || substr(md5(random()::text), 1, 12)) RETURNING id::text`).Scan(&ws); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM audit_event WHERE workspace_id = $1`, ws)
		pool.Exec(ctx, `DELETE FROM quota_request WHERE workspace_id = $1`, ws)
		pool.Exec(ctx, `DELETE FROM workspace_subscription WHERE workspace_id = $1`, ws)
		pool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, ws)
	})
	return &requestFixture{pool: pool, svc: New(Deps{Pool: pool}), ws: ws}
}

func (f *requestFixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func (f *requestFixture) state(t *testing.T, id string) string {
	t.Helper()
	var s string
	if err := f.pool.QueryRow(context.Background(), `SELECT state FROM quota_request WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatalf("read state: %v", err)
	}
	return s
}

// Spec R6 section 3c.5 rule 3: approving writes the workspace's override, the
// request's new state and its audit row.
func TestApproveWritesOverrideAndAuditInOneTransaction(t *testing.T) {
	f := newRequestFixture(t)
	ctx := context.Background()
	req, err := f.svc.Create(ctx, Actor{Label: "owner@example.test"}, f.ws, "", plan.GateAgents, 50, "need more agents")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	out, err := f.svc.Approve(ctx, Actor{Label: "staff@example.test"}, req.ID, nil, "approved for the launch")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if out.State != StateApproved || out.ApprovedValue == nil || *out.ApprovedValue != 50 {
		t.Fatalf("approved request = state %s value %v", out.State, out.ApprovedValue)
	}
	var overrideValue string
	if err := f.pool.QueryRow(ctx, `SELECT COALESCE(limit_overrides->>'agents', '') FROM workspace_subscription WHERE workspace_id = $1`, f.ws).Scan(&overrideValue); err != nil {
		t.Fatalf("read override: %v", err)
	}
	if overrideValue != "50" {
		t.Fatalf("override for agents = %q, want 50", overrideValue)
	}
	if n := f.count(t, `SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = 'quota.request_approved'`, f.ws); n != 1 {
		t.Fatalf("approval audit rows = %d, want 1", n)
	}
}

// Spec R6 section 3c.5 rule 3: a denial needs a reason. A blank one is refused
// with 400 and the request stays pending.
func TestDenyRequiresReason(t *testing.T) {
	f := newRequestFixture(t)
	ctx := context.Background()
	req, err := f.svc.Create(ctx, Actor{Label: "owner@example.test"}, f.ws, "", plan.GateAgents, 50, "need more agents")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = f.svc.Deny(ctx, Actor{Label: "staff@example.test"}, req.ID, "   ")
	var qe *Error
	if !errors.As(err, &qe) || qe.Code != CodeInvalid || qe.Status != http.StatusBadRequest {
		t.Fatalf("deny with a blank reason: %v, want 400 %s", err, CodeInvalid)
	}
	if s := f.state(t, req.ID); s != StatePending {
		t.Fatalf("state after a refused denial = %s, want pending", s)
	}
	if n := f.count(t, `SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = 'quota.request_denied'`, f.ws); n != 0 {
		t.Fatalf("denial audit rows after a refused denial = %d", n)
	}

	if _, err := f.svc.Deny(ctx, Actor{Label: "staff@example.test"}, req.ID, "not this quarter"); err != nil {
		t.Fatalf("deny with a reason: %v", err)
	}
	if s := f.state(t, req.ID); s != StateDenied {
		t.Fatalf("state after denial = %s, want denied", s)
	}
}
