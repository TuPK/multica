package console

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// I6: the console API never reads issue, comment, chat, attachment, agent
// configuration or secret data. These tests scan the console's Go files and
// queries for the name of any table that holds such content. They need no
// database.

// contentPrefixes start the name of every table that holds content.
var contentPrefixes = []string{
	"issue", "comment", "chat", "attachment", "agent",
	"secret", "skill", "daemon", "task", "autopilot",
}

var wordPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

var (
	sqlLineComment  = regexp.MustCompile(`--[^\n]*`)
	sqlBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

// isContentName reports whether word names a content table.
func isContentName(word string) bool {
	w := strings.ToLower(word)
	if strings.Contains(w, "secret") {
		return true
	}
	for _, p := range contentPrefixes {
		if strings.HasPrefix(w, p) {
			return true
		}
	}
	return false
}

// contentWords returns the words of text that name a content table.
func contentWords(text string) []string {
	var out []string
	for _, w := range wordPattern.FindAllString(text, -1) {
		if isContentName(w) {
			out = append(out, w)
		}
	}
	return out
}

// scanSQL checks SQL text. Comments are not code and are skipped.
func scanSQL(sql string) []string {
	sql = sqlBlockComment.ReplaceAllString(sql, " ")
	sql = sqlLineComment.ReplaceAllString(sql, " ")
	return contentWords(sql)
}

// scanGo checks the string literals of one Go file, where a table name would
// have to be written, and every db.<Name> reference, where a generated type or
// query for a content table would be named.
func scanGo(t *testing.T, name string, src []byte) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BasicLit:
			if x.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(x.Value)
			if err != nil {
				text = x.Value
			}
			out = append(out, contentWords(text)...)
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok && id.Name == "db" && isContentName(x.Sel.Name) {
				out = append(out, "db."+x.Sel.Name)
			}
		}
		return true
	})
	return out
}

func TestScannerFindsContentTables(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
		want int
	}{
		{"issue table", `SELECT id FROM issue`, 1},
		{"join on comment", `SELECT 1 FROM workspace w JOIN comment c ON c.workspace_id = w.id`, 1},
		{"prefix match", `SELECT 1 FROM agent_task_queue`, 1},
		{"chat_message", `DELETE FROM chat_message`, 1},
		{"contains secret", `SELECT 1 FROM plugin_secret`, 1},
		{"quoted table", `SELECT 1 FROM "attachment"`, 1},
		{"mixed case", `SELECT 1 FROM Skill_File`, 1},
		{"autopilot_run", `SELECT 1 FROM autopilot_run`, 1},
		{"daemon_token", `SELECT 1 FROM daemon_token`, 1},
		{"task_usage", `SELECT 1 FROM task_usage`, 1},
		{"allowed tables", `SELECT w.id, u.email, m.role, e.user_agent FROM workspace w JOIN member m ON m.workspace_id = w.id JOIN "user" u ON u.id = m.user_id JOIN audit_event e ON e.workspace_id = w.id JOIN platform_note n ON n.workspace_id = w.id`, 0},
		{"line comment is skipped", "SELECT 1 -- never reads the issue table\nFROM workspace", 0},
		{"block comment is skipped", `SELECT 1 /* comment on tasks */ FROM workspace`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := scanSQL(tc.sql); len(got) != tc.want {
				t.Fatalf("scanSQL(%q) = %v, want %d hits", tc.sql, got, tc.want)
			}
		})
	}
}

func TestScannerFindsContentTablesInGo(t *testing.T) {
	bad := `package x
import "context"
func f(ctx context.Context, q Querier) {
	q.Exec(ctx, "DELETE FROM issue WHERE id = $1")
	_ = db.Comment{}
	_ = ` + "`SELECT * FROM chat_session`" + `
}`
	if got := scanGo(t, "bad.go", []byte(bad)); len(got) != 3 {
		t.Fatalf("hits = %v, want 3", got)
	}
	good := `package x
func f() {
	// A comment may say issue or agent.
	var issueCount int
	_ = db.Workspace{}
	_ = "SELECT id FROM workspace"
	_ = issueCount
}`
	if got := scanGo(t, "good.go", []byte(good)); len(got) != 0 {
		t.Fatalf("hits = %v, want none", got)
	}
}

// The scanned set must not be empty, or the test would pass on a wrong path.
func TestConsoleFilesAreScannedAndNameNoContentTable(t *testing.T) {
	goFiles, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	sqlFiles, err := filepath.Glob("../../queries/console*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(goFiles) < 2 || len(sqlFiles) == 0 {
		t.Fatalf("scan found %d Go files and %d SQL files", len(goFiles), len(sqlFiles))
	}
	// The note queries are the console's too.
	sqlFiles = append(sqlFiles, "../../queries/platform_note.sql")

	for _, name := range goFiles {
		// This file holds the list of forbidden names.
		if name == "isolation_test.go" {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if hits := scanGo(t, name, src); len(hits) > 0 {
			t.Errorf("%s names a content table: %v", name, hits)
		}
	}
	for _, name := range sqlFiles {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if hits := scanSQL(string(src)); len(hits) > 0 {
			t.Errorf("%s names a content table: %v", name, hits)
		}
	}
}
