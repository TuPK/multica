package migrations

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/server/internal/selfexec"
)

const maxSearchDepth = 4

var candidateLeaves = []string{
	"migrations",
	filepath.Join("server", "migrations"),
}

// ResolveDir returns the first migrations directory that exists from the
// current working directory.
func ResolveDir() (string, error) {
	seen := make(map[string]bool)
	for _, root := range searchRoots() {
		base := root
		for range maxSearchDepth + 1 {
			for _, leaf := range candidateLeaves {
				dir := filepath.Clean(filepath.Join(base, leaf))
				if seen[dir] {
					continue
				}
				seen[dir] = true
				info, err := os.Stat(dir)
				if err == nil && info.IsDir() {
					return dir, nil
				}
			}
			base = filepath.Join(base, "..")
		}
	}
	return "", fmt.Errorf("migrations directory not found")
}

func searchRoots() []string {
	roots := []string{"."}
	if exe, err := selfexec.Resolve(); err == nil {
		roots = append(roots, filepath.Dir(exe))
	}
	return roots
}

// Files returns sorted migration files for the given direction ("up" or
// "down").
func Files(direction string) ([]string, error) {
	dir, err := ResolveDir()
	if err != nil {
		return nil, err
	}

	suffix := "." + direction + ".sql"
	files, err := filepath.Glob(filepath.Join(dir, "*"+suffix))
	if err != nil {
		return nil, err
	}

	sortMigrations(files)
	if direction == "down" {
		reverseFiles(files)
	}
	return files, nil
}

// sortMigrations orders up migrations by their leading number, so 830 runs before 1000 and 1107,
// and by name when two share a number. Ordering by the whole name would put 1000 before 830, which
// is wrong for a migration that alters a table an earlier number creates.
func sortMigrations(files []string) {
	sort.SliceStable(files, func(i, j int) bool {
		return migrationLess(files[i], files[j])
	})
}

// migrationLess is the up order: the leading number, then the base name.
func migrationLess(a, b string) bool {
	na, nb := migrationNumber(a), migrationNumber(b)
	if na != nb {
		return na < nb
	}
	return filepath.Base(a) < filepath.Base(b)
}

// migrationNumber is the integer a migration file's name starts with. A name with no leading number
// sorts after every numbered one.
func migrationNumber(path string) int {
	base := filepath.Base(path)
	end := 0
	for end < len(base) && base[end] >= '0' && base[end] <= '9' {
		end++
	}
	if end == 0 {
		return math.MaxInt
	}
	n, err := strconv.Atoi(base[:end])
	if err != nil {
		return math.MaxInt
	}
	return n
}

// reverseFiles reverses a sorted list in place: down migrations run in the reverse of up order.
func reverseFiles(files []string) {
	for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
		files[i], files[j] = files[j], files[i]
	}
}

// AllVersions returns every "up" migration version found on disk, in apply
// order. The readiness check verifies that all of them are recorded in
// schema_migrations — checking only the lexically-last version would miss an
// out-of-order migration (one numbered below an already-applied later one),
// letting a server report ready while running against a schema that lacks it.
func AllVersions() ([]string, error) {
	files, err := Files("up")
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no up migrations found")
	}
	versions := make([]string, len(files))
	for i, f := range files {
		versions[i] = ExtractVersion(f)
	}
	return versions, nil
}

// ExtractVersion strips the .up.sql / .down.sql suffix from a migration file.
func ExtractVersion(filename string) string {
	base := filepath.Base(filename)
	base = strings.TrimSuffix(base, ".up.sql")
	base = strings.TrimSuffix(base, ".down.sql")
	return base
}
