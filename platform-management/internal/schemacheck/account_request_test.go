package schemacheck

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func migrationSQL(t *testing.T, version, direction string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(migrationsDir, version+"."+direction+".sql"))
	if err != nil {
		t.Fatalf("migration %s.%s: %v", version, direction, err)
	}
	sql := sqlLineComment.ReplaceAllString(sqlBlockComment.ReplaceAllString(string(raw), " "), " ")
	return strings.Join(strings.Fields(strings.ToLower(sql)), " ")
}

// 966 to 968 are platform_account_request: the table with no key and no index
// (966), then the two unique indexes, one statement each (spec 7b.5, 7b.6).
func TestAccountRequestMigrationsAreRequired(t *testing.T) {
	for _, v := range []string{
		"966_platform_account_request",
		"967_platform_account_request_id_uidx",
		"968_platform_account_request_email_uidx",
	} {
		if !slices.Contains(Required, v) {
			t.Errorf("Required does not list %s", v)
		}
	}
}

func TestAccountRequestTableMigrationHasTheSpecColumnsAndNoKeys(t *testing.T) {
	up := migrationSQL(t, "966_platform_account_request", "up")
	for _, col := range []string{
		"id uuid", "kind", "email", "display_name", "workspace_name", "workspace_slug", "plan_key", "seats",
		"interval", "trial", "comp_reason", "billing_profile jsonb", "state", "error_code", "attempts",
		"user_id uuid", "workspace_id uuid", "created_by uuid", "created_at", "updated_at", "completed_at",
		"welcome_sent_at", "welcome_attempts",
	} {
		if !strings.Contains(up, col) {
			t.Errorf("966 up lacks column %s", col)
		}
	}
	for _, banned := range []string{"references", "primary key", "create index", "create unique index", "on delete", "unique"} {
		if strings.Contains(up, banned) {
			t.Errorf("966 up contains %q: no key, index or cascade belongs in the table migration", banned)
		}
	}
	if down := migrationSQL(t, "966_platform_account_request", "down"); !strings.Contains(down, "drop table if exists platform_account_request") {
		t.Errorf("966 down = %q", down)
	}
}

func TestAccountRequestIndexMigrationsAreOneConcurrentStatementEach(t *testing.T) {
	for version, c := range map[string]struct{ up, down string }{
		"967_platform_account_request_id_uidx": {
			"create unique index concurrently if not exists platform_account_request_id_uidx on platform_account_request (id);",
			"drop index concurrently if exists platform_account_request_id_uidx",
		},
		"968_platform_account_request_email_uidx": {
			"create unique index concurrently if not exists platform_account_request_email_uidx on platform_account_request (email) where state in ('pending', 'provisioned');",
			"drop index concurrently if exists platform_account_request_email_uidx",
		},
	} {
		if got := migrationSQL(t, version, "up"); got != c.up {
			t.Errorf("%s up = %q, want %q", version, got, c.up)
		}
		if got := migrationSQL(t, version, "down"); !strings.Contains(got, c.down) {
			t.Errorf("%s down = %q, want %q", version, got, c.down)
		}
	}
}
