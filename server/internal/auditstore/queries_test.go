package auditstore

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// allowedAuditUpdates are the only two UPDATE statements audit_event.sql may
// hold. Both rename a label when an account is deleted; neither touches the
// action, the ids, the detail or the time.
var allowedAuditUpdates = []string{
	"UPDATE audit_event SET actor_label = 'deleted user' WHERE actor_id = $1 AND actor_type = 'member'",
	"UPDATE audit_event SET target_label = 'deleted user' WHERE target_type = 'user' AND target_id = $1",
}

var (
	sqlComment = regexp.MustCompile(`--[^\n]*`)
	updateStmt = regexp.MustCompile(`(?is)\bUPDATE\b[^;]*;`)
)

// normalizeSQL drops comments and collapses whitespace so a statement compares
// equal however it is wrapped.
func normalizeSQL(s string) string {
	return strings.Join(strings.Fields(strings.TrimSuffix(strings.TrimSpace(s), ";")), " ")
}

// audit_event is append-only apart from the two renames: its query file may
// hold INSERT, SELECT and DELETE (retention) statements and exactly the two
// allowed UPDATE statements.
func TestAuditEventQueriesOnlyAllowedUpdates(t *testing.T) {
	b, err := os.ReadFile("../../pkg/db/queries/audit_event.sql")
	if err != nil {
		t.Fatal(err)
	}
	found := updateStmt.FindAllString(sqlComment.ReplaceAllString(string(b), " "), -1)
	if len(found) != len(allowedAuditUpdates) {
		t.Fatalf("audit_event.sql has %d UPDATE statements, want exactly %d", len(found), len(allowedAuditUpdates))
	}
	for i, want := range allowedAuditUpdates {
		if got := normalizeSQL(found[i]); got != want {
			t.Errorf("UPDATE #%d is %q, want %q", i+1, got, want)
		}
	}
}

func TestUpdateGuardRejectsOtherUpdates(t *testing.T) {
	sql := sqlComment.ReplaceAllString("UPDATE audit_event SET action = 'x' WHERE id = $1;", " ")
	found := updateStmt.FindAllString(sql, -1)
	if len(found) != 1 || normalizeSQL(found[0]) == allowedAuditUpdates[0] || normalizeSQL(found[0]) == allowedAuditUpdates[1] {
		t.Fatal("the guard would accept an UPDATE it must reject")
	}
}
