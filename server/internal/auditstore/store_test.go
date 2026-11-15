package auditstore

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/platform-management/audit"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Database-backed: runs only when PLATFORM_TEST_DATABASE_URL points at a
// dedicated *_test database.
func TestRecordWritesRedactedRow(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	const requestID = "auditstore-test-request"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_event WHERE request_id = $1`, requestID)
	})

	err = audit.Record(ctx, New(db.New(pool)), audit.Entry{
		ActorType: audit.ActorSystem,
		Action:    audit.ActionWorkspaceCreated,
		RequestID: requestID,
		Detail:    map[string]any{"name": "w", "api_token": "secret-value"},
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	var action, name, token string
	var workspaceNull, actorNull bool
	if err := pool.QueryRow(ctx, `
		SELECT action, detail->>'name', detail->>'api_token', workspace_id IS NULL, actor_id IS NULL
		FROM audit_event WHERE request_id = $1`, requestID,
	).Scan(&action, &name, &token, &workspaceNull, &actorNull); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if action != audit.ActionWorkspaceCreated || name != "w" || token != audit.Redacted || !workspaceNull || !actorNull {
		t.Fatalf("unexpected row: %s %s %s %v %v", action, name, token, workspaceNull, actorNull)
	}
}

func TestTxStoreRollsBackWithTransaction(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	const requestID = "auditstore-test-rollback"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_event WHERE request_id = $1`, requestID)
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := audit.Record(ctx, NewTx(db.New(pool), tx), audit.Entry{
		ActorType: audit.ActorSystem, Action: audit.ActionMemberLeft, RequestID: requestID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE request_id = $1`, requestID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows after rollback = %d, err %v; want 0", n, err)
	}
}
