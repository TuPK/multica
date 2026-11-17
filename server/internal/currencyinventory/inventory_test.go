package currencyinventory

import (
	"bufio"
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
	docPath  = "docs/engineering/r4-currency-inventory.md"
)

var (
	scanRoots = []string{"server", "platform-management", "packages"}
	skipDirs  = map[string]bool{
		"node_modules": true, "dist": true, "generated": true,
		"testdata": true, ".git": true, "__tests__": true,
	}
	skipSuffixes = []string{
		"_test.go", ".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx", ".sql.go",
	}
	scanExts   = map[string]bool{".go": true, ".sql": true, ".ts": true, ".tsx": true, ".js": true, ".mjs": true, ".json": true}
	validState = map[string]bool{"todo": true, "ruled": true}

	// terms are checked in order; every matching term is recorded per file.
	terms = []struct {
		name string
		res  []*regexp.Regexp
	}{
		{"currency", []*regexp.Regexp{regexp.MustCompile(`(?i)(?:^|[^n])currenc[yi]`)}},
		{"vnd", []*regexp.Regexp{regexp.MustCompile(`(?:^|[^A-Za-z])(?:VND|vnd)(?:[^A-Za-z]|$)`), regexp.MustCompile(`₫`)}},
		{"usd", []*regexp.Regexp{regexp.MustCompile(`(?:^|[^A-Za-z])USD(?:[^A-Za-z]|$)`), regexp.MustCompile(`"usd"`)}},
		{"cad", []*regexp.Regexp{regexp.MustCompile(`(?:^|[^A-Za-z])(?:CAD|cad)(?:[^A-Za-z]|$)`)}},
		{"money-format", []*regexp.Regexp{regexp.MustCompile(`formatStripeMinorAmount|formatMoney`)}},
	}
)

// row is one inventory table line in the doc.
type row struct{ File, Term, Func, Kind, Holds, Status, Note string }

func (r row) key() string { return r.File + " " + r.Term }

func skipFile(rel string) bool {
	for _, s := range skipSuffixes {
		if strings.HasSuffix(rel, s) {
			return true
		}
	}
	if !scanExts[filepath.Ext(rel)] {
		return true
	}
	return filepath.Ext(rel) == ".json" && strings.Contains(rel, "contract/")
}

// scan returns the set of unit keys (file + " " + term) under root.
func scan(t *testing.T, root string) map[string]bool {
	t.Helper()
	units := map[string]bool{}
	for _, sr := range scanRoots {
		base := filepath.Join(root, sr)
		if _, err := os.Stat(base); err != nil {
			t.Fatalf("scan root %s: %v", base, err)
		}
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				return rerr
			}
			rel = filepath.ToSlash(rel)
			if skipFile(rel) {
				return nil
			}
			f, oerr := os.Open(path)
			if oerr != nil {
				return oerr
			}
			defer f.Close()
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
			for sc.Scan() {
				line := sc.Text()
				for _, term := range terms {
					for _, re := range term.res {
						if re.MatchString(line) {
							units[rel+" "+term.name] = true
							break
						}
					}
				}
			}
			return sc.Err()
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return units
}

func cell(s string) string { return strings.Trim(strings.TrimSpace(s), "`") }

// parseDoc reads the seven-column table rows whose first cell is a backticked
// path. A missing doc yields an empty table.
func parseDoc(t *testing.T) []row {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, docPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("inventory doc: %v", err)
	}
	var rows []row
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		c := strings.Split(strings.Trim(line, "|"), "|")
		if len(c) != 7 {
			continue
		}
		rows = append(rows, row{cell(c[0]), cell(c[1]), cell(c[2]), cell(c[3]), cell(c[4]), cell(c[5]), cell(c[6])})
	}
	return rows
}

func TestEveryCurrencyUseIsListed(t *testing.T) {
	units := scan(t, repoRoot)
	rows := parseDoc(t)
	listed := map[string]bool{}
	for _, r := range rows {
		listed[r.key()] = true
	}
	var missing, stale []string
	for k := range units {
		if !listed[k] {
			missing = append(missing, "not listed: "+k)
		}
	}
	if len(rows) > 0 {
		for k := range listed {
			if !units[k] {
				stale = append(stale, "stale row: "+k)
			}
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	for _, m := range append(missing, stale...) {
		t.Error(m)
	}
}

func TestCurrencyInventoryRowsAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range parseDoc(t) {
		if !validState[r.Status] {
			t.Errorf("row %s: unknown status %q", r.key(), r.Status)
		}
		if r.Holds == "" || r.Func == "" {
			t.Errorf("row %s: Holds and Function or field must not be empty", r.key())
		}
		if seen[r.key()] {
			t.Errorf("duplicate row: %s", r.key())
		}
		seen[r.key()] = true
	}
}

func TestCurrencyScanFindsKnownSources(t *testing.T) {
	var server, packages int
	for k := range scan(t, repoRoot) {
		switch {
		case strings.HasPrefix(k, "server/"):
			server++
		case strings.HasPrefix(k, "packages/"):
			packages++
		}
	}
	if server == 0 || packages == 0 {
		t.Fatalf("scan found %d units in server and %d in packages; the roots are wrong", server, packages)
	}
}
