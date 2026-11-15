package auth

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/platform-management/accountguard"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Database-backed: runs only when PLATFORM_TEST_DATABASE_URL points at a
// dedicated *_test database.
func TestAccountGuardReadsPlatformAccount(t *testing.T) {
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
	q := db.New(pool)

	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO "user" (name, email) VALUES ('Account Guard Test', 'account-guard-test@multica.ai') RETURNING id::text`,
	).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM platform_account WHERE user_id = $1::uuid`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1::uuid`, userID)
	})

	g := NewAccountGuard(q, nil)

	rec, err := g.State(ctx, userID)
	if err != nil || rec.Status != accountguard.Active || !rec.SessionsValidAfter.IsZero() {
		t.Fatalf("no row: got %+v, %v; want active", rec, err)
	}

	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetAccountState(ctx, db.SetAccountStateParams{UserID: uid, Status: "suspended", Reason: "test"}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	rec, err = g.State(ctx, userID)
	if err != nil || rec.Status != accountguard.Suspended {
		t.Fatalf("suspended: got %+v, %v", rec, err)
	}

	// The upsert replaces the row rather than adding a second one.
	if _, err := q.SetAccountState(ctx, db.SetAccountStateParams{UserID: uid, Status: "active"}); err != nil {
		t.Fatalf("set state again: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM platform_account WHERE user_id = $1::uuid`, userID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows = %d, err %v; want 1", n, err)
	}
}
