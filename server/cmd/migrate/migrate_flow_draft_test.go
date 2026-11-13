package main

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"
)

// TestFlowDraftMigrations_UpDown checks migrations 988 to 993 (versions of a flow
// workflow and its drafts): up adds the four columns and the draft table with the
// column types of the spec, builds the three indexes valid and attaches the
// primary key to the index 991 built; down removes it all; a second up, and an up
// that forgets it ran, change nothing.
func TestFlowDraftMigrations_UpDown(t *testing.T) {
	adminPool := openTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	schema := createScratchSchema(t, ctx, adminPool, "migrate_flow_draft_")
	pool := openTestPoolWithSearchPath(t, schema)
	if _, err := pool.Exec(ctx, `CREATE TABLE issue_workflow (id UUID PRIMARY KEY)`); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	versions := []string{
		"988_issue_workflow_versions", "989_issue_workflow_lineage_version_idx", "990_issue_workflow_draft",
		"991_issue_workflow_draft_pkey_idx", "992_issue_workflow_draft_pkey", "993_issue_workflow_draft_squad_idx",
	}
	downVersions := []string{
		"993_issue_workflow_draft_squad_idx", "992_issue_workflow_draft_pkey", "991_issue_workflow_draft_pkey_idx",
		"990_issue_workflow_draft", "989_issue_workflow_lineage_version_idx", "988_issue_workflow_versions",
	}
	columns := []struct {
		table, name, dataType, nullable string
	}{
		{"issue_workflow", "lineage_id", "uuid", "YES"},
		{"issue_workflow", "version", "integer", "YES"},
		{"issue_workflow", "published_at", "timestamp with time zone", "YES"},
		{"issue_workflow", "published_by", "uuid", "YES"},
		{"issue_workflow_draft", "id", "uuid", "NO"},
		{"issue_workflow_draft", "workspace_id", "uuid", "NO"},
		{"issue_workflow_draft", "squad_id", "uuid", "NO"},
		{"issue_workflow_draft", "lineage_id", "uuid", "YES"},
		{"issue_workflow_draft", "base_workflow_id", "uuid", "YES"},
		{"issue_workflow_draft", "name", "text", "NO"},
		{"issue_workflow_draft", "doc", "jsonb", "NO"},
		{"issue_workflow_draft", "revision", "integer", "NO"},
		{"issue_workflow_draft", "origin", "text", "NO"},
		{"issue_workflow_draft", "source_ref", "jsonb", "YES"},
		{"issue_workflow_draft", "created_by", "uuid", "NO"},
		{"issue_workflow_draft", "updated_by", "uuid", "NO"},
		{"issue_workflow_draft", "created_at", "timestamp with time zone", "NO"},
		{"issue_workflow_draft", "updated_at", "timestamp with time zone", "NO"},
	}
	// Attaching the unique index as the primary key renames it to the constraint's name.
	indexes := []string{"idx_issue_workflow_lineage_version", "issue_workflow_draft_pkey", "idx_issue_workflow_draft_squad"}

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
		for _, col := range columns {
			var dataType, nullable string
			err := pool.QueryRow(ctx, `
				SELECT data_type, is_nullable FROM information_schema.columns
				WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
			`, schema, col.table, col.name).Scan(&dataType, &nullable)
			if !want {
				if err == nil {
					t.Fatalf("%s.%s still exists after down", col.table, col.name)
				}
				continue
			}
			if err != nil {
				t.Fatalf("%s.%s missing after up: %v", col.table, col.name, err)
			}
			if dataType != col.dataType || nullable != col.nullable {
				t.Fatalf("%s.%s is %s (nullable %s), want %s (nullable %s)", col.table, col.name, dataType, nullable, col.dataType, col.nullable)
			}
		}
	}
	assertDraftDefaults := func() {
		t.Helper()
		var revision, origin string
		if err := pool.QueryRow(ctx, `
			SELECT
			  (SELECT column_default FROM information_schema.columns WHERE table_schema = $1 AND table_name = 'issue_workflow_draft' AND column_name = 'revision'),
			  (SELECT column_default FROM information_schema.columns WHERE table_schema = $1 AND table_name = 'issue_workflow_draft' AND column_name = 'origin')
		`, schema).Scan(&revision, &origin); err != nil {
			t.Fatal(err)
		}
		if revision != "1" || origin != "'manual'::text" {
			t.Fatalf("defaults revision/origin = %q/%q, want 1/'manual'", revision, origin)
		}
	}
	// The primary key is the constraint the unique index 991 built was attached to; no foreign key, no CHECK.
	assertConstraints := func() {
		t.Helper()
		rows, err := pool.Query(ctx, `
			SELECT c.contype::text, COALESCE(i.relname, '')
			FROM pg_constraint c
			JOIN pg_class t ON t.oid = c.conrelid
			JOIN pg_namespace n ON n.oid = t.relnamespace
			LEFT JOIN pg_class i ON i.oid = c.conindid
			WHERE n.nspname = $1 AND t.relname = 'issue_workflow_draft'
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
		if len(got) != 1 || got[0] != "p:issue_workflow_draft_pkey" {
			t.Fatalf("constraints = %v, want only the primary key", got)
		}
		// The index 991 built is the one the key took over: it is gone under its own name.
		assertIndexExists(t, pool, schema, "issue_workflow_draft_pkey_uidx", false)
	}

	run("up", versions)
	assertColumns(true)
	assertDraftDefaults()
	assertConstraints()
	for _, name := range indexes {
		assertIndexValidity(t, pool, schema, name, true)
	}

	run("down", downVersions)
	assertColumns(false)
	for _, name := range indexes {
		assertIndexExists(t, pool, schema, name, false)
	}
	var tables int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = 'issue_workflow_draft'`, schema).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("the draft table still exists after down (%d, %v)", tables, err)
	}

	// Up again; a second up finds every version recorded and changes nothing.
	run("up", versions)
	assertColumns(true)
	run("up", versions)
	assertColumns(true)
	assertConstraints()
	// Forget the records: the statements that can run twice do (the primary key attaches once, as 575 does).
	if _, err := pool.Exec(ctx, "DELETE FROM "+schema+".schema_migrations"); err != nil {
		t.Fatal(err)
	}
	run("up", []string{"988_issue_workflow_versions", "989_issue_workflow_lineage_version_idx", "990_issue_workflow_draft", "993_issue_workflow_draft_squad_idx"})
	assertColumns(true)
	assertConstraints()
	for _, name := range indexes {
		assertIndexValidity(t, pool, schema, name, true)
	}
}
