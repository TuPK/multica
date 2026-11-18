package audit

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	sqlLineComment  = regexp.MustCompile(`--[^\n]*`)
	auditMutation   = regexp.MustCompile(`(?is)\b(UPDATE|DELETE|TRUNCATE|ALTER)\b`)
	auditTableUsage = regexp.MustCompile(`(?i)\baudit_event\b`)
)

// auditMutations returns the statements in sql that change audit_event.
func auditMutations(sql string) []string {
	var out []string
	for _, stmt := range strings.Split(sqlLineComment.ReplaceAllString(sql, " "), ";") {
		if auditTableUsage.MatchString(stmt) && auditMutation.MatchString(stmt) {
			out = append(out, strings.Join(strings.Fields(stmt), " "))
		}
	}
	return out
}

// Spec: the audit log is append-only. No platform query may UPDATE, DELETE,
// TRUNCATE or ALTER audit_event; unlike the server's audit_event.sql, there
// is no allowed exception.
func TestPlatformAuditQueriesAreInsertAndSelectOnly(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "queries", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no platform query files: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range auditMutations(string(b)) {
			t.Errorf("%s changes audit_event: %q", filepath.Base(f), stmt)
		}
	}
}

func TestAuditMutationScanFlagsWrites(t *testing.T) {
	for _, sql := range []string{
		"UPDATE audit_event SET action = 'x';",
		"DELETE FROM audit_event WHERE id = $1;",
		"TRUNCATE audit_event;",
		"ALTER TABLE audit_event ADD COLUMN x int;",
	} {
		if len(auditMutations(sql)) != 1 {
			t.Errorf("scan missed %q", sql)
		}
	}
	if got := auditMutations("-- DELETE FROM audit_event;\nSELECT * FROM audit_event; UPDATE plan SET x = 1;"); len(got) != 0 {
		t.Errorf("scan flagged reads or other tables: %v", got)
	}
}
