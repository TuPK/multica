package main

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"
)

// TestFlowMigrations_UpDown checks migrations 985 to 987 (the enforced squad
// workflow engine): up adds the eight columns with their types and the two
// valid indexes, down removes them all, and a second up is a no-op rerun.
func TestFlowMigrations_UpDown(t *testing.T) {
	adminPool := openTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	schema := createScratchSchema(t, ctx, adminPool, "migrate_flow_columns_")
	pool := openTestPoolWithSearchPath(t, schema)
	for _, statement := range []string{
		`CREATE TABLE issue (id UUID PRIMARY KEY, workspace_id UUID NOT NULL, parent_issue_id UUID)`,
		`CREATE TABLE issue_workflow (id UUID PRIMARY KEY)`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}

	versions := []string{"985_issue_flow_columns", "986_issue_flow_task_once_idx", "987_issue_flow_open_idx"}
	downVersions := []string{"987_issue_flow_open_idx", "986_issue_flow_task_once_idx", "985_issue_flow_columns"}
	columns := []struct {
		table, name, dataType string
	}{
		{"issue", "outcome", "text"},
		{"issue", "flow", "jsonb"},
		{"issue", "flow_step", "text"},
		{"issue", "flow_task", "text"},
		{"issue", "flow_round", "integer"},
		{"issue_workflow", "flow_config", "jsonb"},
		{"issue_workflow", "template_ref", "text"},
		{"issue_workflow", "flow_auto", "boolean"},
	}
	indexes := []string{"idx_issue_flow_task_once", "idx_issue_flow_open"}

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
			var dataType string
			err := pool.QueryRow(ctx, `
				SELECT data_type FROM information_schema.columns
				WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
			`, schema, col.table, col.name).Scan(&dataType)
			if !want {
				if err == nil {
					t.Fatalf("%s.%s still exists after down", col.table, col.name)
				}
				continue
			}
			if err != nil {
				t.Fatalf("%s.%s missing after up: %v", col.table, col.name, err)
			}
			if dataType != col.dataType {
				t.Fatalf("%s.%s type = %s, want %s", col.table, col.name, dataType, col.dataType)
			}
		}
	}

	run("up", versions)
	assertColumns(true)
	for _, name := range indexes {
		assertIndexValidity(t, pool, schema, name, true)
	}
	var autoDefault, autoNullable string
	if err := pool.QueryRow(ctx, `
		SELECT column_default, is_nullable FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'issue_workflow' AND column_name = 'flow_auto'
	`, schema).Scan(&autoDefault, &autoNullable); err != nil {
		t.Fatal(err)
	}
	if autoDefault != "false" || autoNullable != "NO" {
		t.Fatalf("flow_auto default/nullable = %q/%q, want false/NO", autoDefault, autoNullable)
	}

	run("down", downVersions)
	assertColumns(false)
	for _, name := range indexes {
		assertIndexExists(t, pool, schema, name, false)
	}

	// Up again, then forget the records and rerun: every statement is idempotent.
	run("up", versions)
	assertColumns(true)
	if _, err := pool.Exec(ctx, "DELETE FROM "+schema+".schema_migrations"); err != nil {
		t.Fatal(err)
	}
	run("up", versions)
	assertColumns(true)
	for _, name := range indexes {
		assertIndexValidity(t, pool, schema, name, true)
	}
}
