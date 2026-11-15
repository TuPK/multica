package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forbidden is split so that this file does not contain the string itself.
var forbidden = strings.ToLower("dev" + "_verification")

// filesWithForbidden returns the files under root whose content, lowercased,
// contains the forbidden string.
func filesWithForbidden(t *testing.T, root string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(string(body)), forbidden) {
			hits = append(hits, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func TestScannerFindsTheForbiddenString(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("x := os.Getenv(\"MULTICA_"+strings.ToUpper(forbidden)+"_CODE\")"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("clean"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hits := filesWithForbidden(t, dir); len(hits) != 1 || filepath.Base(hits[0]) != "a.go" {
		t.Fatalf("hits = %v", hits)
	}
}

// The platform has no development sign-in path: no fixed code, no bypass.
func TestPlatformHasNoDevelopmentVerificationPath(t *testing.T) {
	if hits := filesWithForbidden(t, "../.."); len(hits) > 0 {
		t.Fatalf("files mention the development verification path: %v", hits)
	}
}
