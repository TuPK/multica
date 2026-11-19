package migrations

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newStaffSandbox returns a pool whose connections work in one isolated
// schema, holding the real platform_staff and audit_event tables built from
// their own migration files.
func newStaffSandbox(t *testing.T, schema string) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration test requires Postgres at DATABASE_URL")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to Postgres: %v", err)
	}
	drop := func() { _, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE") }
	drop()
	t.Cleanup(func() { drop(); pool.Close() })
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create isolated migration schema: %v", err)
	}
	for _, f := range []string{
		"654_audit_event.up.sql",
		"663_platform_staff.up.sql",
		"664_platform_staff_user_idx.up.sql",
	} {
		applyMigrationFile(t, ctx, pool, f)
	}
	return pool
}

type staffRow struct {
	role     string
	tenantID string // "" for NULL
	revoked  bool
}

func insertStaff(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rows map[string]staffRow) {
	t.Helper()
	for id, r := range rows {
		var tenant any
		if r.tenantID != "" {
			tenant = r.tenantID
		}
		revoked := "NULL"
		if r.revoked {
			revoked = "now() - interval '1 day'"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO platform_staff (user_id, role, tenant_id, revoked_at) VALUES ($1, $2, $3, `+revoked+`)`, id, r.role, tenant); err != nil {
			t.Fatalf("insert staff %s: %v", id, err)
		}
	}
}

func readStaff(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string]staffRow {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT user_id::text, role, COALESCE(tenant_id::text, ''), revoked_at IS NOT NULL FROM platform_staff`)
	if err != nil {
		t.Fatalf("read staff: %v", err)
	}
	defer rows.Close()
	out := map[string]staffRow{}
	for rows.Next() {
		var id string
		var r staffRow
		if err := rows.Scan(&id, &r.role, &r.tenantID, &r.revoked); err != nil {
			t.Fatal(err)
		}
		out[id] = r
	}
	return out
}

const (
	sUserOldAdmin     = "00000000-0000-0000-0000-00000000b001"
	sUserOldAdminGone = "00000000-0000-0000-0000-00000000b002"
	sUserScopedAdmin  = "00000000-0000-0000-0000-00000000b003"
	sUserSupport      = "00000000-0000-0000-0000-00000000b004"
	sUserBilling      = "00000000-0000-0000-0000-00000000b005"
	sUserSystemAdmin  = "00000000-0000-0000-0000-00000000b006"
	sTenant           = "00000000-0000-0000-0000-00000000c001"
)

func applyStaffTenantColumn(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	applyMigrationFile(t, ctx, pool, "942_platform_staff_tenant.up.sql")
}

func TestStaffRoleMigrationConvertsOldAdminsOnlyOnce(t *testing.T) {
	ctx := context.Background()
	pool := newStaffSandbox(t, "staff_role_migration_once")
	applyStaffTenantColumn(t, ctx, pool)
	insertStaff(t, ctx, pool, map[string]staffRow{
		sUserOldAdmin:     {role: "admin"},
		sUserOldAdminGone: {role: "admin", revoked: true},
		sUserScopedAdmin:  {role: "admin", tenantID: sTenant},
		sUserSupport:      {role: "support"},
		sUserBilling:      {role: "billing"},
		sUserSystemAdmin:  {role: "system_admin"},
	})

	applyMigrationFile(t, ctx, pool, "944_platform_staff_system_admin.up.sql")
	first := readStaff(t, ctx, pool)
	if got := first[sUserOldAdmin].role; got != "system_admin" {
		t.Errorf("old admin without tenant = %q, want system_admin", got)
	}
	if got := first[sUserOldAdminGone]; got.role != "system_admin" || !got.revoked {
		t.Errorf("revoked old admin = %+v, want system_admin and still revoked", got)
	}

	applyMigrationFile(t, ctx, pool, "944_platform_staff_system_admin.up.sql")
	second := readStaff(t, ctx, pool)
	for id, want := range first {
		if second[id] != want {
			t.Errorf("second run changed %s: %+v -> %+v", id, want, second[id])
		}
	}
}

func TestStaffRoleMigrationLeavesScopedAdminsAndOtherRoles(t *testing.T) {
	ctx := context.Background()
	pool := newStaffSandbox(t, "staff_role_migration_leave")
	applyStaffTenantColumn(t, ctx, pool)
	want := map[string]staffRow{
		sUserScopedAdmin: {role: "admin", tenantID: sTenant},
		sUserSupport:     {role: "support"},
		sUserBilling:     {role: "billing"},
		sUserSystemAdmin: {role: "system_admin"},
	}
	insertStaff(t, ctx, pool, want)

	applyMigrationFile(t, ctx, pool, "944_platform_staff_system_admin.up.sql")

	got := readStaff(t, ctx, pool)
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s = %+v, want %+v", id, got[id], w)
		}
	}
}

func TestStaffRoleMigrationDownRevokesScopedAdminsFirst(t *testing.T) {
	ctx := context.Background()
	pool := newStaffSandbox(t, "staff_role_migration_down")
	applyStaffTenantColumn(t, ctx, pool)
	insertStaff(t, ctx, pool, map[string]staffRow{
		sUserSystemAdmin: {role: "system_admin"},
		sUserScopedAdmin: {role: "admin", tenantID: sTenant},
		sUserSupport:     {role: "support"},
	})

	applyMigrationFile(t, ctx, pool, "944_platform_staff_system_admin.down.sql")

	got := readStaff(t, ctx, pool)
	if r := got[sUserScopedAdmin]; !r.revoked {
		t.Errorf("scoped admin still active after down: %+v (an old platform would read it as a platform-wide admin)", r)
	}
	if r := got[sUserSystemAdmin]; r.role != "admin" || r.revoked {
		t.Errorf("system_admin = %+v, want an active admin", r)
	}
	if r := got[sUserSupport]; r.role != "support" || r.revoked {
		t.Errorf("support = %+v, want unchanged", r)
	}
}

func TestStaffTenantColumnIsNullableWithoutForeignKey(t *testing.T) {
	ctx := context.Background()
	pool := newStaffSandbox(t, "staff_tenant_column")
	applyStaffTenantColumn(t, ctx, pool)
	assertNullableUUIDColumnWithoutConstraints(t, ctx, pool, "platform_staff", "tenant_id")

	applyMigrationFile(t, ctx, pool, "943_platform_staff_tenant_idx.up.sql")
	assertPartialIndex(t, ctx, pool, "platform_staff_tenant_idx", "revoked_at IS NULL", "tenant_id IS NOT NULL")

	applyMigrationFile(t, ctx, pool, "943_platform_staff_tenant_idx.down.sql")
	applyMigrationFile(t, ctx, pool, "942_platform_staff_tenant.down.sql")
	if columnExists(t, ctx, pool, "platform_staff", "tenant_id") {
		t.Error("942 down left tenant_id behind")
	}
}

func TestAuditEventTenantColumnIsNullableWithoutForeignKey(t *testing.T) {
	ctx := context.Background()
	pool := newStaffSandbox(t, "audit_event_tenant_column")
	applyMigrationFile(t, ctx, pool, "945_audit_event_tenant.up.sql")
	assertNullableUUIDColumnWithoutConstraints(t, ctx, pool, "audit_event", "tenant_id")

	applyMigrationFile(t, ctx, pool, "946_audit_event_tenant_created_idx.up.sql")
	assertPartialIndex(t, ctx, pool, "audit_event_tenant_created_idx", "tenant_id IS NOT NULL")

	applyMigrationFile(t, ctx, pool, "946_audit_event_tenant_created_idx.down.sql")
	applyMigrationFile(t, ctx, pool, "945_audit_event_tenant.down.sql")
	if columnExists(t, ctx, pool, "audit_event", "tenant_id") {
		t.Error("945 down left tenant_id behind")
	}
}

func columnExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_attribute
		WHERE attrelid = to_regclass($1) AND attname = $2 AND NOT attisdropped)`, table, column).Scan(&ok); err != nil {
		t.Fatalf("read column %s.%s: %v", table, column, err)
	}
	return ok
}

func assertNullableUUIDColumnWithoutConstraints(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string) {
	t.Helper()
	var typ string
	var notNull bool
	if err := pool.QueryRow(ctx, `
		SELECT format_type(atttypid, atttypmod), attnotnull FROM pg_attribute
		WHERE attrelid = to_regclass($1) AND attname = $2 AND NOT attisdropped`, table, column).Scan(&typ, &notNull); err != nil {
		t.Fatalf("%s.%s is missing: %v", table, column, err)
	}
	if typ != "uuid" || notNull {
		t.Errorf("%s.%s is %s not_null=%v, want a nullable uuid", table, column, typ, notNull)
	}
	var n int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint c
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY (c.conkey)
		WHERE c.conrelid = to_regclass($1) AND a.attname = $2`, table, column).Scan(&n); err != nil {
		t.Fatalf("read constraints: %v", err)
	}
	if n != 0 {
		t.Errorf("%s.%s has %d constraints, want none (no foreign key)", table, column, n)
	}
}

// assertPartialIndex checks the index exists, is valid and its predicate holds
// every fragment.
func assertPartialIndex(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string, fragments ...string) {
	t.Helper()
	var def string
	var valid bool
	if err := pool.QueryRow(ctx, `
		SELECT pg_get_indexdef(i.indexrelid), i.indisvalid FROM pg_index i
		WHERE i.indexrelid = to_regclass($1)`, name).Scan(&def, &valid); err != nil {
		t.Fatalf("index %s is missing: %v", name, err)
	}
	if !valid {
		t.Errorf("index %s is invalid", name)
	}
	where := def
	if i := strings.Index(def, " WHERE "); i >= 0 {
		where = def[i:]
	} else {
		t.Errorf("index %s is not partial: %s", name, def)
	}
	for _, f := range fragments {
		if !strings.Contains(where, strings.NewReplacer("(", "", ")", "").Replace(f)) && !strings.Contains(where, f) {
			t.Errorf("index %s predicate lacks %q: %s", name, f, def)
		}
	}
}
