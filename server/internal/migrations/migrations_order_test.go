package migrations

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The runner sorts migration files as strings (migrations.go), so a four-digit
// number such as 1016 sorts before 101 to 999 and runs early on a new
// database. A four-digit migration may therefore only touch a table that a file
// sorting before it creates; otherwise it fails on a fresh install although it
// passes on a database that already has the table.
var (
	createTableRe = regexp.MustCompile(`(?is)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?"?([a-z_][a-z0-9_]*)"?`)
	touchedTable  = regexp.MustCompile(`(?is)\b(?:ALTER\s+TABLE(?:\s+IF\s+EXISTS)?(?:\s+ONLY)?|INSERT\s+INTO|UPDATE|FROM|JOIN|CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:CONCURRENTLY\s+)?(?:IF\s+NOT\s+EXISTS\s+)?[a-z0-9_]+\s+ON)\s+"?([a-z_][a-z0-9_]*)"?`)
	sqlComment    = regexp.MustCompile(`(?m)--[^\n]*`)
)

// firstFourDigitMigration is the first migration the rule is enforced on: the
// tenant migrations of spec R6. Earlier four-digit files predate the rule.
const firstFourDigitMigration = 1016

// triggerKeywords are SQL words the touchedTable pattern reads as a table name:
// "UPDATE OF status" in a trigger definition gives "of", and "IS DISTINCT FROM
// NEW.status" gives "new". None of them is a table, so a valid trigger in a
// four-digit migration is not reported. A real write to a table created later
// is still reported (TestOrderScanFlagsATableCreatedLater).
var triggerKeywords = map[string]bool{
	"of": true, "new": true, "old": true, "only": true, "when": true,
	"for": true, "each": true, "row": true, "function": true,
}

// orderViolations returns, for each four-digit migration in files (name to
// content), the tables it touches that no earlier file in sort order creates.
func orderViolations(files map[string]string) []string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	created := map[string]bool{}
	var out []string
	for _, name := range names {
		body := sqlComment.ReplaceAllString(files[name], " ")
		own := map[string]bool{}
		for _, m := range createTableRe.FindAllStringSubmatch(body, -1) {
			own[strings.ToLower(m[1])] = true
		}
		prefix, _, _ := strings.Cut(name, "_")
		if n, err := strconv.Atoi(prefix); err == nil && len(prefix) == 4 && n >= firstFourDigitMigration {
			for _, m := range touchedTable.FindAllStringSubmatch(body, -1) {
				table := strings.ToLower(m[1])
				if triggerKeywords[table] {
					continue
				}
				if !created[table] && !own[table] {
					out = append(out, name+" touches "+table)
				}
			}
		}
		for t := range own {
			created[t] = true
		}
	}
	sort.Strings(out)
	return out
}

func TestFourDigitMigrationsOnlyTouchTablesCreatedEarlierInSortOrder(t *testing.T) {
	dir := realMigrationsDir(t)
	matches, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no migrations in %s: %v", dir, err)
	}
	files := map[string]string{}
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Base(m)] = string(b)
	}
	for _, v := range orderViolations(files) {
		t.Errorf("%s: it sorts before that table's creation on a new database", v)
	}
	if _, ok := files["1016_tenant.up.sql"]; !ok {
		t.Error("1016_tenant.up.sql is missing, so the scan checked no tenant migration")
	}
}

// The scan must flag the mistake it exists for: 1021 reading a table from a
// migration that sorts after it.
func TestOrderScanFlagsATableCreatedLater(t *testing.T) {
	got := orderViolations(map[string]string{
		"001_init.up.sql":                   "CREATE TABLE workspace (id uuid);",
		"672_workspace_subscription.up.sql": "CREATE TABLE IF NOT EXISTS workspace_subscription (workspace_id uuid);",
		"1016_tenant.up.sql":                "CREATE TABLE IF NOT EXISTS tenant (id uuid);",
		"1021_tenant_backfill.up.sql": `-- reads workspace_subscription in a comment: ignored
			INSERT INTO tenant (id) SELECT w.id FROM workspace w JOIN workspace_subscription s ON s.workspace_id = w.id;`,
	})
	if len(got) != 1 || got[0] != "1021_tenant_backfill.up.sql touches workspace_subscription" {
		t.Fatalf("violations = %v, want only workspace_subscription in 1021", got)
	}
	clean := orderViolations(map[string]string{
		"001_init.up.sql":             "CREATE TABLE workspace (id uuid);",
		"1016_tenant.up.sql":          "CREATE TABLE IF NOT EXISTS tenant (id uuid);",
		"1021_tenant_backfill.up.sql": "INSERT INTO tenant (id) SELECT w.id FROM workspace w WHERE NOT EXISTS (SELECT 1 FROM tenant t WHERE t.id = w.id);",
	})
	if len(clean) != 0 {
		t.Fatalf("violations = %v, want none", clean)
	}
}

// A trigger definition is DDL on an existing table, so it is not a write to a
// later table. Its keywords (OF, NEW, OLD, ...) must not be read as table names.
func TestOrderScanIgnoresTriggerKeywords(t *testing.T) {
	got := orderViolations(map[string]string{
		"001_init.up.sql": "CREATE TABLE issue (id uuid, status text);",
		"1044_trigger.up.sql": `CREATE OR REPLACE FUNCTION f() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN RETURN NEW; END; $$;
			CREATE TRIGGER t_ai AFTER UPDATE OF status ON issue FOR EACH ROW
			WHEN (OLD.status IS DISTINCT FROM NEW.status) EXECUTE FUNCTION f();`,
	})
	if len(got) != 0 {
		t.Fatalf("trigger keywords reported as tables: %v", got)
	}
}
