package main

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"
)

// TestSquadPackMigrations_UpDown checks migrations 994 to 997 (what a pack installed): up adds the table
// with the column types of the spec, builds the two indexes valid and attaches the primary key to the
// index 995 built; down removes it all; a second up, and an up that forgets it ran, change nothing.
func TestSquadPackMigrations_UpDown(t *testing.T) {
	adminPool := openTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	schema := createScratchSchema(t, ctx, adminPool, "migrate_squad_pack_")
	pool := openTestPoolWithSearchPath(t, schema)

	versions := []string{
		"994_squad_pack_install", "995_squad_pack_install_pkey_idx", "996_squad_pack_install_pkey", "997_squad_pack_install_squad_idx",
	}
	downVersions := []string{
		"997_squad_pack_install_squad_idx", "996_squad_pack_install_pkey", "995_squad_pack_install_pkey_idx", "994_squad_pack_install",
	}
	columns := []struct{ name, dataType, nullable, def string }{
		{"id", "uuid", "NO", ""},
		{"workspace_id", "uuid", "NO", ""},
		{"squad_id", "uuid", "YES", ""},
		{"pack_id", "text", "NO", "''::text"},
		{"pack_version", "text", "NO", "''::text"},
		{"format", "text", "NO", ""},
		{"digest", "text", "NO", ""},
		{"source", "jsonb", "NO", "'{}'::jsonb"},
		{"roster", "jsonb", "NO", "'{}'::jsonb"},
		{"installed_by", "uuid", "NO", ""},
		{"installed_at", "timestamp with time zone", "NO", "now()"},
	}
	// Attaching the unique index as the primary key renames it to the constraint's name.
	indexes := []string{"squad_pack_install_pkey", "idx_squad_pack_install_squad"}

	run := func(direction string, files []string) {
		t.Helper()
		if err := runMigrations(ctx, pool, runOptions{
			Direction:             direction,
			Files:                 realMigrationFiles(t, files, direction),
			SchemaMigrationsTable: schema + ".schema_migrations",
			AdvisoryLockKey:       int64(rand.Uint64()&0x7fffffffffffffff) | 1,
			Hooks:                 hooksForDirection(direction),
			Conditions:            conditionsForDirection(direction),
		}); err != nil {
			t.Fatalf("migrate %s: %v", direction, err)
		}
	}
	assertColumns := func(want bool) {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = 'squad_pack_install'`, schema).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if !want {
			if count != 0 {
				t.Fatal("squad_pack_install still exists after down")
			}
			return
		}
		if count != 1 {
			t.Fatal("squad_pack_install is missing after up")
		}
		for _, col := range columns {
			var dataType, nullable string
			var def *string
			err := pool.QueryRow(ctx, `
				SELECT data_type, is_nullable, column_default FROM information_schema.columns
				WHERE table_schema = $1 AND table_name = 'squad_pack_install' AND column_name = $2
			`, schema, col.name).Scan(&dataType, &nullable, &def)
			if err != nil {
				t.Fatalf("squad_pack_install.%s missing after up: %v", col.name, err)
			}
			if dataType != col.dataType || nullable != col.nullable {
				t.Fatalf("%s is %s (nullable %s), want %s (nullable %s)", col.name, dataType, nullable, col.dataType, col.nullable)
			}
			if col.def != "" && (def == nil || *def != col.def) {
				t.Fatalf("%s default = %v, want %s", col.name, def, col.def)
			}
		}
	}
	// The primary key is the constraint the unique index 995 built was attached to; no foreign key, no CHECK.
	assertConstraints := func() {
		t.Helper()
		rows, err := pool.Query(ctx, `
			SELECT c.contype::text, COALESCE(i.relname, '')
			FROM pg_constraint c
			JOIN pg_class t ON t.oid = c.conrelid
			JOIN pg_namespace n ON n.oid = t.relnamespace
			LEFT JOIN pg_class i ON i.oid = c.conindid
			WHERE n.nspname = $1 AND t.relname = 'squad_pack_install'
		`, schema)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var got []string
		for rows.Next() {
			var kind, index string
			if err := rows.Scan(&kind, &index); err != nil {
				t.Fatal(err)
			}
			got = append(got, kind+":"+index)
		}
		if len(got) != 1 || got[0] != "p:squad_pack_install_pkey" {
			t.Fatalf("constraints = %v, want only the primary key", got)
		}
		assertIndexExists(t, pool, schema, "squad_pack_install_pkey_uidx", false)
	}

	run("up", versions)
	assertColumns(true)
	assertConstraints()
	for _, name := range indexes {
		assertIndexValidity(t, pool, schema, name, true)
	}

	run("down", downVersions)
	assertColumns(false)
	for _, name := range indexes {
		assertIndexExists(t, pool, schema, name, false)
	}

	// Up again; a second up finds every version recorded and changes nothing.
	run("up", versions)
	assertColumns(true)
	run("up", versions)
	assertColumns(true)
	assertConstraints()
	// Forget the records: the statements that can run twice do (the primary key attaches once).
	if _, err := pool.Exec(ctx, "DELETE FROM "+schema+".schema_migrations"); err != nil {
		t.Fatal(err)
	}
	run("up", []string{"994_squad_pack_install", "997_squad_pack_install_squad_idx"})
	assertColumns(true)
	assertConstraints()
	for _, name := range indexes {
		assertIndexValidity(t, pool, schema, name, true)
	}
}
