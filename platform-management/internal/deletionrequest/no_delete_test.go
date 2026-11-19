package deletionrequest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoDeleteStatementsOnWorkspaceContentInPlatformCode checks that phase 1 deletes
// no workspace content: the data is removed by Multica's existing deletion path
// (spec R6 section 6.2, rule 4). It scans this package and its own queries file.
func TestNoDeleteStatementsOnWorkspaceContentInPlatformCode(t *testing.T) {
	var files []string
	goFiles, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range goFiles {
		if !strings.HasSuffix(f, "_test.go") {
			files = append(files, f)
		}
	}
	files = append(files, "../../queries/deletion_request.sql")
	if len(files) == 0 {
		t.Fatal("no source files found")
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToUpper(string(raw)), "DELETE FROM") {
			t.Errorf("%s contains a DELETE statement; the deletion path is Multica's", f)
		}
	}
}
