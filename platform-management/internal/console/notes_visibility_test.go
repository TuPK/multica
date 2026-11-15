package console

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Notes live in platform_note and are read by the console only. The Multica
// server, whose routes workspace members reach, must not name the table. This
// is a source scan; the database test covers the stored rows.
const serverDir = "../../../server"

var notesName = regexp.MustCompile(`\bplatform_note\b|\bPlatformNote\b`)

func TestMulticaServerNeverReadsPlatformNotes(t *testing.T) {
	if _, err := os.Stat(serverDir); err != nil {
		t.Fatalf("server directory: %v", err)
	}
	var scanned int
	err := filepath.WalkDir(serverDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		slash := filepath.ToSlash(path)
		if d.IsDir() {
			// The migrations create the table; the generated models list every
			// table in the schema, this one included once the server regenerates.
			if d.Name() == "migrations" || d.Name() == "node_modules" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".sql") {
			return nil
		}
		if strings.HasSuffix(slash, "pkg/db/generated/models.go") {
			return nil
		}
		scanned++
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if notesName.Match(body) {
			t.Errorf("%s names the notes table", slash)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 100 {
		t.Fatalf("scanned only %d server files; the path is wrong", scanned)
	}
}
