package migrations

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newTenantSandbox returns a pool working in one isolated schema that holds a
// minimal workspace table, the shape 001_init gives it for the columns the
// tenant migrations read.
func newTenantSandbox(t *testing.T, schema string) *pgxpool.Pool {
	t.Helper()
	pool := newSandboxPool(t, schema)
	if _, err := pool.Exec(context.Background(), `
		CREATE TABLE workspace (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			name text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	return pool
}

// newSandboxPool is a pool whose connections all resolve unqualified table names
// inside the schema, which it creates empty and drops at the end of the test.
func newSandboxPool(t *testing.T, schema string) *pgxpool.Pool {
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
	return pool
}

var tenantMigrations = []string{
	"1016_tenant", "1017_tenant_id_uidx", "1018_tenant_workspace",
	"1019_tenant_workspace_ws_uidx", "1020_tenant_workspace_tenant_idx",
}

func applyTenantSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, m := range tenantMigrations {
		applyMigrationFile(t, context.Background(), pool, m+".up.sql")
	}
}

func scalarCount(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

const (
	twsA  = "00000000-0000-0000-0000-00000000e001"
	twsB  = "00000000-0000-0000-0000-00000000e002"
	twsC  = "00000000-0000-0000-0000-00000000e003"
	twsD  = "00000000-0000-0000-0000-00000000e004" // already linked to a tenant of its own
	twsE  = "00000000-0000-0000-0000-00000000e005"
	twsF  = "00000000-0000-0000-0000-00000000e006"
	tKept = "00000000-0000-0000-0000-00000000f001"
)

func TestTenantBackfillIsIdempotentAndKeysTenantByWorkspace(t *testing.T) {
	ctx := context.Background()
	pool := newTenantSandbox(t, "tenant_backfill_migration")
	applyTenantSchema(t, pool)
	for id, name := range map[string]string{twsA: "A", twsB: "B", twsC: "C", twsD: "D"} {
		if _, err := pool.Exec(ctx, `INSERT INTO workspace (id, name) VALUES ($1, $2)`, id, name); err != nil {
			t.Fatal(err)
		}
	}
	// D is already linked to a tenant that is not keyed by its own id.
	if _, err := pool.Exec(ctx, `INSERT INTO tenant (id, name) VALUES ($1, 'kept')`, tKept); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tenant_workspace (workspace_id, tenant_id) VALUES ($1, $2)`, twsD, tKept); err != nil {
		t.Fatal(err)
	}

	applyMigrationFile(t, ctx, pool, "1021_tenant_backfill.up.sql")
	applyMigrationFile(t, ctx, pool, "1021_tenant_backfill.up.sql")

	for id, name := range map[string]string{twsA: "A", twsB: "B", twsC: "C"} {
		if n := scalarCount(t, pool, `SELECT count(*) FROM tenant WHERE id = $1 AND name = $2`, id, name); n != 1 {
			t.Errorf("workspace %s has %d tenants keyed by its id, want 1", id, n)
		}
		if n := scalarCount(t, pool, `SELECT count(*) FROM tenant_workspace WHERE workspace_id = $1 AND tenant_id = $1`, id); n != 1 {
			t.Errorf("workspace %s has %d links to its own tenant, want 1", id, n)
		}
	}
	if n := scalarCount(t, pool, `SELECT count(*) FROM tenant_workspace WHERE workspace_id = $1 AND tenant_id = $2`, twsD, tKept); n != 1 {
		t.Errorf("the existing link of %s changed", twsD)
	}
	if n := scalarCount(t, pool, `SELECT count(*) FROM tenant WHERE id = $1`, twsD); n != 0 {
		t.Errorf("a linked workspace got a second tenant")
	}
	if n := scalarCount(t, pool, `SELECT count(*) FROM tenant`); n != 4 {
		t.Errorf("tenants = %d, want 4 (3 new, 1 existing)", n)
	}

	// Two runs at once, over workspaces that are still unlinked.
	for id, name := range map[string]string{twsE: "E", twsF: "F"} {
		if _, err := pool.Exec(ctx, `INSERT INTO workspace (id, name) VALUES ($1, $2)`, id, name); err != nil {
			t.Fatal(err)
		}
	}
	sql := readMigrationFile(t, "1021_tenant_backfill.up.sql")
	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = pool.Exec(ctx, sql)
		}()
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("parallel run %d: %v", i, err)
		}
	}
	for _, id := range []string{twsE, twsF} {
		if n := scalarCount(t, pool, `SELECT count(*) FROM tenant WHERE id = $1`, id); n != 1 {
			t.Errorf("workspace %s has %d tenants after two parallel runs, want 1", id, n)
		}
		if n := scalarCount(t, pool, `SELECT count(*) FROM tenant_workspace WHERE workspace_id = $1`, id); n != 1 {
			t.Errorf("workspace %s has %d links after two parallel runs, want 1", id, n)
		}
	}
}

// The tenant tables have no foreign key, no primary key and no unique
// constraint: relations and uniqueness are the application's and the unique
// indexes' (AGENTS.md database rules).
func TestTenantTablesHaveNoForeignKeysOrInlineConstraints(t *testing.T) {
	ctx := context.Background()
	pool := newTenantSandbox(t, "tenant_tables_constraints")
	applyTenantSchema(t, pool)
	for _, table := range []string{"tenant", "tenant_workspace"} {
		if n := scalarCount(t, pool, `
			SELECT count(*) FROM pg_constraint
			WHERE conrelid = to_regclass($1) AND contype IN ('f', 'p', 'u')`, table); n != 0 {
			t.Errorf("%s has %d foreign key, primary key or unique constraints, want none", table, n)
		}
	}
	for name, unique := range map[string]bool{"tenant_id_uidx": true, "tenant_workspace_ws_uidx": true, "tenant_workspace_tenant_idx": false} {
		var isUnique, valid bool
		if err := pool.QueryRow(ctx, `SELECT i.indisunique, i.indisvalid FROM pg_index i WHERE i.indexrelid = to_regclass($1)`, name).Scan(&isUnique, &valid); err != nil {
			t.Errorf("index %s is missing: %v", name, err)
			continue
		}
		if isUnique != unique || !valid {
			t.Errorf("index %s unique=%v valid=%v, want unique=%v valid", name, isUnique, valid, unique)
		}
	}
	var status string
	var notNull bool
	if err := pool.QueryRow(ctx, `
		SELECT pg_get_expr(d.adbin, d.adrelid), a.attnotnull FROM pg_attribute a
		JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE a.attrelid = to_regclass('tenant') AND a.attname = 'status'`).Scan(&status, &notNull); err != nil {
		t.Fatalf("tenant.status: %v", err)
	}
	if status != "'active'::text" || !notNull {
		t.Errorf("tenant.status default %q not_null %v, want 'active' and NOT NULL", status, notNull)
	}
	for _, col := range []string{"status_reason", "status_changed_at", "status_changed_by", "created_by"} {
		if !columnExists(t, ctx, pool, "tenant", col) {
			t.Errorf("tenant.%s is missing", col)
		}
	}
}

// workspace is Delegora's table and gets no tenant column (D6, spec R6 section 5.1).
func TestWorkspaceTableHasNoTenantColumn(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration test requires Postgres at DATABASE_URL")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if n := scalarCount(t, pool, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'workspace' AND column_name LIKE '%tenant%'`); n != 0 {
		t.Errorf("workspace has %d tenant columns, want none", n)
	}
}

func TestTenantDownMigrationsDropWhatTheyCreate(t *testing.T) {
	ctx := context.Background()
	pool := newTenantSandbox(t, "tenant_down_migrations")
	applyTenantSchema(t, pool)
	for i := len(tenantMigrations) - 1; i >= 0; i-- {
		applyMigrationFile(t, ctx, pool, tenantMigrations[i]+".down.sql")
	}
	for _, table := range []string{"tenant", "tenant_workspace"} {
		if n := scalarCount(t, pool, `SELECT count(*) FROM pg_class WHERE oid = to_regclass($1)`, table); n != 0 {
			t.Errorf("%s survived its down migration", table)
		}
	}
	// 1021 is a data copy and has nothing to undo; its down file must still run.
	applyMigrationFile(t, ctx, pool, "1021_tenant_backfill.down.sql")
}

func TestPlanMaxWorkspacesMigrationIsIdempotentAndKeepsExistingKey(t *testing.T) {
	ctx := context.Background()
	pool := newSandboxPool(t, "plan_max_workspaces_migration")
	// 941 sorts before 953, so only the columns of 670_plan exist when it runs.
	applyMigrationFile(t, ctx, pool, "670_plan.up.sql")
	for _, key := range []string{"free", "pro", "business", "enterprise", "unlimited", "payg", "team"} {
		if _, err := pool.Exec(ctx, `INSERT INTO plan (key, name, currency, limits) VALUES ($1, $1, 'VND', '{"agents": 5}')`, key); err != nil {
			t.Fatal(err)
		}
	}
	// A key staff already set is not overwritten.
	if _, err := pool.Exec(ctx, `UPDATE plan SET limits = limits || '{"max_workspaces": 7}' WHERE key = 'pro'`); err != nil {
		t.Fatal(err)
	}

	applyMigrationFile(t, ctx, pool, "941_plan_max_workspaces.up.sql")
	snapshot := func() map[string]string {
		rows, err := pool.Query(ctx, `SELECT key, limits::text FROM plan`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]string{}
		for rows.Next() {
			var k, l string
			if err := rows.Scan(&k, &l); err != nil {
				t.Fatal(err)
			}
			out[k] = l
		}
		return out
	}
	first := snapshot()
	applyMigrationFile(t, ctx, pool, "941_plan_max_workspaces.up.sql")
	for k, v := range snapshot() {
		if first[k] != v {
			t.Errorf("second run changed %s: %s -> %s", k, first[k], v)
		}
	}

	value := func(key string) (present bool, kind, text string) {
		var hasKey bool
		var jsonKind, raw string
		if err := pool.QueryRow(ctx, `
			SELECT limits ? 'max_workspaces', COALESCE(jsonb_typeof(limits -> 'max_workspaces'), ''), COALESCE((limits -> 'max_workspaces')::text, '')
			FROM plan WHERE key = $1`, key).Scan(&hasKey, &jsonKind, &raw); err != nil {
			t.Fatal(err)
		}
		return hasKey, jsonKind, raw
	}
	for key, want := range map[string]string{"free": "1", "pro": "7", "business": "10", "payg": "5"} {
		if present, kind, got := value(key); !present || kind != "number" || got != want {
			t.Errorf("%s max_workspaces = %q (%s, present %v), want %s", key, got, kind, present, want)
		}
	}
	for _, key := range []string{"enterprise", "unlimited"} {
		if present, kind, _ := value(key); !present || kind != "null" {
			t.Errorf("%s max_workspaces present=%v kind=%q, want an explicit JSON null", key, present, kind)
		}
	}
	if present, _, _ := value("team"); present {
		t.Error("a plan outside the catalogue got max_workspaces")
	}
	if n := scalarCount(t, pool, `SELECT count(*) FROM plan WHERE NOT (limits ? 'agents')`); n != 0 {
		t.Errorf("%d plans lost their other limits", n)
	}

	up := readMigrationFile(t, "941_plan_max_workspaces.up.sql")
	if strings.Contains(strings.ToLower(up), "version") {
		t.Error("941 runs before 953 on a new database and must not touch plan.version")
	}

	applyMigrationFile(t, ctx, pool, "941_plan_max_workspaces.down.sql")
	for _, key := range []string{"free", "pro", "enterprise"} {
		if present, _, _ := value(key); present {
			t.Errorf("%s keeps max_workspaces after the down migration", key)
		}
	}
}
