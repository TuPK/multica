package auditstore

import (
	"os"
	"regexp"
	"testing"
)

// audit_event is append-only: its query file may hold INSERT, SELECT and
// DELETE (retention) statements, never UPDATE.
func TestAuditEventQueriesNeverUpdate(t *testing.T) {
	b, err := os.ReadFile("../../pkg/db/queries/audit_event.sql")
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`(?i)\bUPDATE\b`).Match(b) {
		t.Fatal("audit_event.sql must not contain UPDATE")
	}
}
