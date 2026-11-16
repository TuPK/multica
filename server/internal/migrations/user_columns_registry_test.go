package migrations

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/accountdeletion"
)

// userColumnNames are the exact column names that can hold a user id, besides
// the user_id suffix rule.
var userColumnNames = map[string]bool{
	"user_id":    true,
	"owner_id":   true,
	"creator_id": true,
	"created_by": true,
	"inviter_id": true,
	"added_by":   true,
}

func isUserColumnName(name string) bool {
	return userColumnNames[name] || strings.HasSuffix(name, "_user_id")
}

// TestUserColumnRegistry fails when a migration adds a UUID column that can
// point at a user and the account-deletion registry does not name it. It reads
// the SQL text only; no database is involved.
//
// Limits: it reads CREATE TABLE and ALTER TABLE ... ADD COLUMN statements in
// file order. It does not understand DROP COLUMN, RENAME COLUMN or
// ALTER COLUMN ... TYPE, and it does not look inside DO blocks or dynamic SQL.
// A dropped or renamed column therefore still has to be in the registry, and a
// column created under a name that does not match the rule is never seen.
func TestUserColumnRegistry(t *testing.T) {
	found := userColumnsFromMigrations(t, "../../migrations")
	if len(found) == 0 {
		t.Fatal("no user columns found; the parser or the path is broken")
	}

	registered := map[string]accountdeletion.Column{}
	for _, c := range accountdeletion.Columns {
		registered[c.Table+"."+c.Column] = c
	}

	var missing []string
	for key := range found {
		if _, ok := registered[key]; !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("%d user columns are not in platform-management/accountdeletion/registry.go; classify each as delete, anonymize, keep or not_a_user:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}

	var stale []string
	for key := range registered {
		if _, ok := found[key]; !ok {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("%d registry entries match no migration column:\n  %s", len(stale), strings.Join(stale, "\n  "))
	}
}

// TestUserColumnParser pins the statement shapes the registry test depends on.
func TestUserColumnParser(t *testing.T) {
	sql := `
-- comment: CREATE TABLE ignored (user_id UUID);
CREATE TABLE IF NOT EXISTS public."thing" (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL, /* inline, with comma */
    owner_id uuid REFERENCES "user"(id),
    note TEXT DEFAULT 'a,b',
    created_by TEXT,
    amount NUMERIC(10, 2),
    UNIQUE (user_id, note)
);
ALTER TABLE thing ADD COLUMN IF NOT EXISTS reviewer_user_id UUID,
    ADD COLUMN inviter_id UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE ONLY other ADD COLUMN creator_id UUID;
ALTER TABLE other ADD COLUMN other_id UUID;
`
	got := map[string]bool{}
	scanUserColumns(sql, got)
	want := []string{"thing.user_id", "thing.owner_id", "thing.reviewer_user_id", "thing.inviter_id", "other.creator_id"}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing %s; got %v", w, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %v, want exactly %v", got, want)
	}
}

func userColumnsFromMigrations(t *testing.T, dir string) map[string]bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	found := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		scanUserColumns(string(b), found)
	}
	return found
}

var (
	lineComment  = regexp.MustCompile(`--[^\n]*`)
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	createTable  = regexp.MustCompile(`(?is)\bCREATE\s+(?:UNLOGGED\s+|TEMP(?:ORARY)?\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?((?:"?\w+"?\.)?"?\w+"?)\s*\(`)
	alterTable   = regexp.MustCompile(`(?is)\bALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:ONLY\s+)?((?:"?\w+"?\.)?"?\w+"?)\s+((?:[^;']|'[^']*')*?)(?:;|$)`)
	addColumn    = regexp.MustCompile(`(?is)^\s*ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?"?(\w+)"?\s+(\S+)`)
)

// scanUserColumns adds "table.column" for every UUID column in sql whose name
// can hold a user id.
func scanUserColumns(sql string, found map[string]bool) {
	sql = blockComment.ReplaceAllString(sql, " ")
	sql = lineComment.ReplaceAllString(sql, " ")

	for _, m := range createTable.FindAllStringSubmatchIndex(sql, -1) {
		table := tableName(sql[m[2]:m[3]])
		open := m[1] - 1
		body, ok := parenBody(sql, open)
		if !ok {
			continue
		}
		for _, def := range splitTopLevel(body) {
			if name, typ, ok := columnDef(def); ok {
				record(found, table, name, typ)
			}
		}
	}

	for _, m := range alterTable.FindAllStringSubmatch(sql, -1) {
		table := tableName(m[1])
		for _, action := range splitTopLevel(m[2]) {
			if am := addColumn.FindStringSubmatch(action); am != nil {
				record(found, table, strings.ToLower(am[1]), am[2])
			}
		}
	}
}

func record(found map[string]bool, table, column, typ string) {
	if !isUserColumnName(column) {
		return
	}
	typ = strings.ToLower(strings.TrimRight(typ, ","))
	if typ != "uuid" && typ != "uuid[]" {
		return
	}
	found[table+"."+column] = true
}

func tableName(raw string) string {
	if i := strings.LastIndex(raw, "."); i >= 0 {
		raw = raw[i+1:]
	}
	return strings.ToLower(strings.Trim(raw, `"`))
}

// constraintStarts are the table-level clauses that open a CREATE TABLE item
// without defining a column.
var constraintStarts = map[string]bool{
	"constraint": true, "primary": true, "foreign": true, "unique": true,
	"check": true, "exclude": true, "like": true,
}

func columnDef(def string) (name, typ string, ok bool) {
	fields := strings.Fields(def)
	if len(fields) < 2 {
		return "", "", false
	}
	first := strings.ToLower(fields[0])
	if constraintStarts[first] {
		return "", "", false
	}
	return strings.ToLower(strings.Trim(fields[0], `"`)), fields[1], true
}

// parenBody returns the text between the parenthesis at open and its match,
// skipping quoted strings.
func parenBody(s string, open int) (string, bool) {
	depth := 0
	inStr := false
	for i := open; i < len(s); i++ {
		c := s[i]
		switch {
		case inStr:
			if c == '\'' {
				inStr = false
			}
		case c == '\'':
			inStr = true
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return s[open+1 : i], true
			}
		}
	}
	return "", false
}

// splitTopLevel splits on commas outside parentheses and quoted strings.
func splitTopLevel(s string) []string {
	var parts []string
	depth := 0
	inStr := false
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inStr:
			if c == '\'' {
				inStr = false
			}
		case c == '\'':
			inStr = true
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}
