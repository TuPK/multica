package migrations

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestOutcomeAppendOnlyVerifications: outcome_verification is append-only (spec
// 2.3). No query under pkg/db/queries may update or delete a verification; a new
// verification is a new row, and a settled verdict changes only through a new
// verification of the member's own, as governance_event does.
func TestOutcomeAppendOnlyVerifications(t *testing.T) {
	createsTable := regexp.MustCompile(`(?is)\bCREATE\s+TABLE\s+(IF\s+NOT\s+EXISTS\s+)?outcome_verification\b`)
	migrations, err := filepath.Glob(filepath.Join(realMigrationsDir(t), "*.up.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("no up migrations found: %v", err)
	}
	created := false
	for _, path := range migrations {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if createsTable.Match(sqlComment.ReplaceAll(body, nil)) {
			created = true
		}
	}
	if !created {
		t.Fatal("no migration creates outcome_verification: the append-only check has no table to guard")
	}

	queries, err := filepath.Glob(filepath.Join("..", "..", "pkg", "db", "queries", "*.sql"))
	if err != nil || len(queries) == 0 {
		t.Fatalf("no query files found: %v", err)
	}
	mutation := regexp.MustCompile(`(?is)\b(UPDATE|DELETE\s+FROM)\s+outcome_verification\b`)
	for _, path := range queries {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if m := mutation.Find(sqlComment.ReplaceAll(body, nil)); m != nil {
			t.Errorf("%s mutates outcome_verification (%q); verifications are append-only (spec 2.3)", filepath.Base(path), m)
		}
	}
}
