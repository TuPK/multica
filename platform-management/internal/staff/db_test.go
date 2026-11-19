package staff

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// Database tests: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it. Each test makes its own users and
// removes them and their staff rows afterwards.

type dbFixture struct {
	t    *testing.T
	pool *pgxpool.Pool
	q    *db.Queries
}

func newDBFixture(t *testing.T) *dbFixture {
	t.Helper()
	pool := testdb.Open(t)
	return &dbFixture{t: t, pool: pool, q: db.New(pool)}
}

// user creates a Multica user with a random email and returns its id and email.
func (f *dbFixture) user() (pgtype.UUID, string) {
	f.t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		f.t.Fatal(err)
	}
	email := "staff-" + hex.EncodeToString(b) + "@example.test"
	var id pgtype.UUID
	err := f.pool.QueryRow(context.Background(),
		`INSERT INTO "user" (name, email) VALUES ('Staff Test', $1) RETURNING id`, email).Scan(&id)
	if err != nil {
		f.t.Fatalf("create user: %v", err)
	}
	f.t.Cleanup(func() {
		ctx := context.Background()
		f.pool.Exec(ctx, `DELETE FROM platform_staff WHERE user_id = $1`, id)
		f.pool.Exec(ctx, `DELETE FROM platform_account WHERE user_id = $1`, id)
		f.pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, id)
	})
	return id, email
}

func (f *dbFixture) rows(id pgtype.UUID) (active, revoked int) {
	f.t.Helper()
	err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FILTER (WHERE revoked_at IS NULL), count(*) FILTER (WHERE revoked_at IS NOT NULL)
		 FROM platform_staff WHERE user_id = $1`, id).Scan(&active, &revoked)
	if err != nil {
		f.t.Fatal(err)
	}
	return active, revoked
}

func TestDBSeedMakesEnvEmailsSystemAdmin(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	id, email := f.user()

	if err := NewSeeder(f.q, []string{email}, nil).SeedAll(ctx); err != nil {
		t.Fatal(err)
	}
	row, err := f.q.GetActiveStaffByUserID(ctx, id)
	if err != nil {
		t.Fatalf("no active row: %v", err)
	}
	if row.Role != "system_admin" || row.TenantID.Valid || row.CreatedBy.Valid {
		t.Errorf("row = %+v", row)
	}
	// A second run adds nothing.
	if err := NewSeeder(f.q, []string{email}, nil).SeedAll(ctx); err != nil {
		t.Fatal(err)
	}
	if active, revoked := f.rows(id); active != 1 || revoked != 0 {
		t.Errorf("active=%d revoked=%d", active, revoked)
	}
}

func TestDBSeedReactivatesARevokedRow(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	id, email := f.user()

	if _, err := f.q.InsertStaff(ctx, db.InsertStaffParams{UserID: id, Role: "support"}); err != nil {
		t.Fatal(err)
	}
	if n, err := f.q.RevokeStaff(ctx, id); err != nil || n != 1 {
		t.Fatalf("revoke: n=%d err=%v", n, err)
	}
	if err := NewSeeder(f.q, []string{email}, nil).SeedAll(ctx); err != nil {
		t.Fatal(err)
	}
	row, err := f.q.GetActiveStaffByUserID(ctx, id)
	if err != nil {
		t.Fatalf("no active row: %v", err)
	}
	if row.Role != "system_admin" || row.TenantID.Valid || row.RevokedAt.Valid {
		t.Errorf("row = %+v", row)
	}
	if active, revoked := f.rows(id); active != 1 || revoked != 0 {
		t.Errorf("active=%d revoked=%d", active, revoked)
	}
}

func TestDBSeedReactivatesOnlyTheNewestOfSeveralRevokedRows(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	id, email := f.user()
	for i := 0; i < 2; i++ {
		if _, err := f.q.InsertStaff(ctx, db.InsertStaffParams{UserID: id, Role: "billing"}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.q.RevokeStaff(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := NewSeeder(f.q, []string{email}, nil).SeedAll(ctx); err != nil {
		t.Fatal(err)
	}
	if active, revoked := f.rows(id); active != 1 || revoked != 1 {
		t.Errorf("active=%d revoked=%d, want 1 and 1", active, revoked)
	}
}

func TestDBSeedSkipsAnEmailWithoutAnAccount(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	if err := NewSeeder(f.q, []string{"nobody-" + t.Name() + "@example.test"}, nil).SeedAll(ctx); err != nil {
		t.Fatalf("a missing account must not be an error: %v", err)
	}
}

func TestDBPartialUniqueIndex(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	id, _ := f.user()

	// A plain insert of a second active row fails on the index.
	_, err := f.pool.Exec(ctx, `INSERT INTO platform_staff (user_id, role) VALUES ($1, 'support'), ($1, 'admin')`, id)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23505" {
		t.Fatalf("two active rows: err = %v, want a unique violation", err)
	}

	// InsertStaff does nothing when an active row exists.
	if n, err := f.q.InsertStaff(ctx, db.InsertStaffParams{UserID: id, Role: "support"}); err != nil || n != 1 {
		t.Fatalf("first InsertStaff: n=%d err=%v", n, err)
	}
	if n, err := f.q.InsertStaff(ctx, db.InsertStaffParams{UserID: id, Role: "admin"}); err != nil || n != 0 {
		t.Fatalf("second InsertStaff: n=%d err=%v, want 0 rows", n, err)
	}

	// A revoked row and a new active row coexist.
	if _, err := f.q.RevokeStaff(ctx, id); err != nil {
		t.Fatal(err)
	}
	if n, err := f.q.InsertStaff(ctx, db.InsertStaffParams{UserID: id, Role: "billing"}); err != nil || n != 1 {
		t.Fatalf("InsertStaff after revoke: n=%d err=%v", n, err)
	}
	if active, revoked := f.rows(id); active != 1 || revoked != 1 {
		t.Errorf("active=%d revoked=%d", active, revoked)
	}
	// Re-activating while an active row exists fails on the index.
	_, err = f.q.ReactivateStaff(ctx, db.ReactivateStaffParams{UserID: id, Role: "admin"})
	if !isUniqueViolation(err) {
		t.Errorf("ReactivateStaff with an active row: err = %v", err)
	}
}

func TestDBQueriesAgainstTheRealSchema(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	id, email := f.user()
	store := NewStore(f.q)

	gotID, found, err := store.UserIDByEmail(ctx, email)
	if err != nil || !found || gotID != uuidString(id) {
		t.Fatalf("UserIDByEmail: %q %v %v", gotID, found, err)
	}
	if got, err := store.EmailByUserID(ctx, uuidString(id)); err != nil || got != email {
		t.Fatalf("EmailByUserID: %q %v", got, err)
	}
	if _, found, err := store.UserIDByEmail(ctx, "nobody@example.test"); err != nil || found {
		t.Fatalf("unknown email: found=%v err=%v", found, err)
	}

	if _, _, found, err := store.ActiveStaff(ctx, uuidString(id)); err != nil || found {
		t.Fatalf("ActiveStaff before insert: found=%v err=%v", found, err)
	}
	if _, err := f.q.InsertStaff(ctx, db.InsertStaffParams{UserID: id, Role: "billing", CreatedBy: id}); err != nil {
		t.Fatal(err)
	}
	role, _, found, err := store.ActiveStaff(ctx, uuidString(id))
	if err != nil || !found || role != RoleBilling {
		t.Fatalf("ActiveStaff: %q %v %v", role, found, err)
	}
	list, err := f.q.ListStaff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var listed bool
	for _, r := range list {
		if r.UserID == id {
			listed = r.Role == "billing" && r.CreatedBy == id
		}
	}
	if !listed {
		t.Error("ListStaff did not return the new row")
	}

	if _, found, err := store.AccountState(ctx, uuidString(id)); err != nil || found {
		t.Fatalf("AccountState without a row: found=%v err=%v", found, err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO platform_account (user_id, status) VALUES ($1, 'suspended')`, id); err != nil {
		t.Fatal(err)
	}
	rec, found, err := store.AccountState(ctx, uuidString(id))
	if err != nil || !found || string(rec.Status) != "suspended" {
		t.Fatalf("AccountState: %+v %v %v", rec, found, err)
	}

	if n, err := f.q.RevokeStaff(ctx, id); err != nil || n != 1 {
		t.Fatalf("RevokeStaff: n=%d err=%v", n, err)
	}
	if n, err := f.q.RevokeStaff(ctx, id); err != nil || n != 0 {
		t.Fatalf("RevokeStaff again: n=%d err=%v", n, err)
	}
	list, _ = f.q.ListStaff(ctx)
	for _, r := range list {
		if r.UserID == id {
			t.Error("ListStaff returned a revoked row")
		}
	}
}

func TestDBMFAStoreReadsAndUsesTheConfirmedRow(t *testing.T) {
	f := newDBFixture(t)
	ctx := context.Background()
	id, _ := f.user()
	userID := uuidString(id)
	store := NewMFAStore(f.q)

	if _, _, found, err := store.ConfirmedMFA(ctx, userID); err != nil || found {
		t.Fatalf("no row: found %v, err %v", found, err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO user_mfa (user_id, secret_enc) VALUES ($1, '\x0102'::bytea)`, id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.pool.Exec(ctx, `DELETE FROM user_mfa WHERE user_id = $1`, id) })

	// A pending enrolment is not a second factor.
	if _, _, found, err := store.ConfirmedMFA(ctx, userID); err != nil || found {
		t.Fatalf("pending row: found %v, err %v", found, err)
	}
	if ok, err := store.UseMFAStep(ctx, userID, 10); err != nil || ok {
		t.Fatalf("pending row used a step: %v, %v", ok, err)
	}

	if _, err := f.pool.Exec(ctx, `UPDATE user_mfa SET confirmed_at = now(), last_used_step = 5 WHERE user_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	sealed, last, found, err := store.ConfirmedMFA(ctx, userID)
	if err != nil || !found || last != 5 || len(sealed) != 2 {
		t.Fatalf("confirmed row: %v %d %v %v", sealed, last, found, err)
	}
	for _, tc := range []struct {
		step int64
		want bool
	}{{10, true}, {10, false}, {9, false}, {11, true}} {
		if ok, err := store.UseMFAStep(ctx, userID, tc.step); err != nil || ok != tc.want {
			t.Fatalf("UseMFAStep(%d) = %v, %v; want %v", tc.step, ok, err, tc.want)
		}
	}
	if _, err := store.UseMFAStep(ctx, "not-a-uuid", 1); err == nil {
		t.Fatal("an invalid user id was accepted")
	}

	secret, found, err := store.AnyConfirmedSecret(ctx)
	if err != nil || !found || len(secret) == 0 {
		t.Fatalf("AnyConfirmedSecret: %v %v %v", secret, found, err)
	}
}
