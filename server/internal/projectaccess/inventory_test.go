package projectaccess

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The test runs with the package directory as cwd.
const (
	repoRoot = "../../.."
	docPath  = "docs/engineering/r4-read-path-inventory.md"
)

var (
	readerRE   = regexp.MustCompile(`(?i)\b(from|join)\s+(issue|project)\b`)
	queryNamRE = regexp.MustCompile(`^--\s*name:\s*(\w+)\s+:\w+`)
	validState = map[string]bool{"todo": true, "scoped": true, "exempt": true}
)

// unit is one reader: a SQL query, or a Go function that calls such a query
// or embeds raw SQL. Kind is informational; the key is file + function.
type unit struct{ File, Func, Kind string }

func (u unit) key() string { return u.File + " " + u.Func }

// row is one line of an inventory table in the doc.
type row struct{ File, Func, Kind, Status, Note string }

func rel(t *testing.T, root, path string) string {
	t.Helper()
	r, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(r)
}

// scanSQL returns the queries under root/server/pkg/db/queries that read
// the issue or project table.
func scanSQL(t *testing.T, root string) []unit {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "server/pkg/db/queries/*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	var out []unit
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		name, body := "", []string{}
		flush := func() {
			if name != "" && readerRE.MatchString(strings.Join(body, "\n")) {
				out = append(out, unit{rel(t, root, f), name, "sql-query"})
			}
		}
		for _, line := range strings.Split(string(data), "\n") {
			if m := queryNamRE.FindStringSubmatch(line); m != nil {
				flush()
				name, body = m[1], body[:0]
				continue
			}
			body = append(body, line)
		}
		flush()
	}
	return out
}

func declName(d ast.Decl) string {
	fn, ok := d.(*ast.FuncDecl)
	if !ok {
		return "<file-level>"
	}
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	typ := fn.Recv.List[0].Type
	if s, ok := typ.(*ast.StarExpr); ok {
		typ = s.X
	}
	if i, ok := typ.(*ast.IndexExpr); ok {
		typ = i.X
	}
	if id, ok := typ.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// scanGo returns functions under root/server/internal that call one of the
// reader queries or contain a string literal reading issue or project.
func scanGo(t *testing.T, root string, queries []unit) []unit {
	t.Helper()
	names := map[string]bool{}
	for _, q := range queries {
		names[q.Func] = true
	}
	var out []unit
	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return perr
		}
		for _, decl := range file.Decls {
			fn, r := declName(decl), rel(t, root, path)
			ast.Inspect(decl, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					if sel, ok := x.Fun.(*ast.SelectorExpr); ok && names[sel.Sel.Name] {
						out = append(out, unit{r, fn, "go-call"})
					}
				case *ast.BasicLit:
					if x.Kind == token.STRING && readerRE.MatchString(x.Value) {
						out = append(out, unit{r, fn, "raw-sql"})
					}
				}
				return true
			})
		}
		return nil
	}
	if err := filepath.WalkDir(filepath.Join(root, "server/internal"), walk); err != nil {
		t.Fatal(err)
	}
	return out
}

// scanAll merges the three scans into one unit per key, sorted by key.
func scanAll(t *testing.T, root string) []unit {
	t.Helper()
	sqlUnits := scanSQL(t, root)
	kinds := map[string][]string{}
	byKey := map[string]unit{}
	for _, u := range append(sqlUnits, scanGo(t, root, sqlUnits)...) {
		byKey[u.key()] = u
		if !contains(kinds[u.key()], u.Kind) {
			kinds[u.key()] = append(kinds[u.key()], u.Kind)
		}
	}
	var out []unit
	for k, u := range byKey {
		sort.Strings(kinds[k])
		u.Kind = strings.Join(kinds[k], "+")
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func cell(s string) string { return strings.Trim(strings.TrimSpace(s), "`") }

// parseDoc reads every five-column table row of the inventory doc.
func parseDoc(t *testing.T) []row {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, docPath))
	if err != nil {
		t.Fatalf("inventory doc: %v", err)
	}
	var rows []row
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		c := strings.Split(strings.Trim(line, "|"), "|")
		if len(c) != 5 || cell(c[0]) == "File" || strings.HasPrefix(cell(c[0]), "---") {
			continue
		}
		rows = append(rows, row{cell(c[0]), cell(c[1]), cell(c[2]), cell(c[3]), cell(c[4])})
	}
	return rows
}

// missingFromDoc returns the scanned units that have no scanned-kind row in
// the doc.
func missingFromDoc(units []unit, rows []row) []unit {
	listed := map[string]bool{}
	for _, r := range rows {
		if r.Kind != "manual" {
			listed[r.File+" "+r.Func] = true
		}
	}
	var out []unit
	for _, u := range units {
		if !listed[u.key()] {
			out = append(out, u)
		}
	}
	return out
}

func TestEveryIssueAndProjectReaderIsClassified(t *testing.T) {
	rows := parseDoc(t)
	units := scanAll(t, repoRoot)
	if len(units) == 0 {
		t.Fatal("scanner found no readers; the scan paths are wrong")
	}
	for _, u := range missingFromDoc(units, rows) {
		t.Errorf("reader not listed in %s: %s [%s]", docPath, u.key(), u.Kind)
	}
	for _, r := range rows {
		if !validState[r.Status] {
			t.Errorf("row %s %s: unknown status %q", r.File, r.Func, r.Status)
		}
		if r.Status == "exempt" && r.Note == "" {
			t.Errorf("row %s %s: exempt needs a written reason in Note", r.File, r.Func)
		}
	}
}

func TestInventoryDetectsNewQuery(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "server/pkg/db/queries")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := "-- name: FakeNewIssueReader :many\nSELECT id FROM issue;\n\n" +
		"-- name: FakeReactionReader :many\nSELECT id FROM issue_reaction;\n"
	if err := os.WriteFile(filepath.Join(dir, "fake.sql"), []byte(fake), 0o644); err != nil {
		t.Fatal(err)
	}
	units := scanSQL(t, root)
	if len(units) != 1 || units[0].Func != "FakeNewIssueReader" {
		t.Fatalf("scanner must find only FakeNewIssueReader, got %+v", units)
	}
	missing := missingFromDoc(units, parseDoc(t))
	if len(missing) != 1 || missing[0] != units[0] {
		t.Fatalf("a new issue reader must be reported as unlisted, got %v", missing)
	}
}

// TestNoTodoLeft fails while any row of the inventory is still todo: every read
// path must be scoped or exempt with a written reason (spec 7.1, P41.10).
func TestNoTodoLeft(t *testing.T) {
	for _, r := range parseDoc(t) {
		if r.Status == "todo" {
			t.Errorf("still todo: %s %s", r.File, r.Func)
		}
	}
}
