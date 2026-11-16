package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forbidden is Multica's own development code variable. The platform reads only
// its own gated PLATFORM_DEV_VERIFICATION_CODE; make dev-full copies the value.
// The string is split so that this file does not contain it.
var forbidden = strings.ToLower("multica" + "_dev_verification")

// filesWithForbidden returns the files under root whose content, lowercased,
// contains the forbidden string. It skips .git and node_modules: the latter
// holds the console's installed packages, which are not this module's source.
func filesWithForbidden(t *testing.T, root string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
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

func TestScannerSkipsInstalledPackages(t *testing.T) {
	dir := t.TempDir()
	pkg := filepath.Join(dir, "console", "node_modules", "dep")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "index.js"), []byte(forbidden), 0o644); err != nil {
		t.Fatal(err)
	}
	// A package manager links workspace packages as directory symlinks.
	if err := os.Symlink(pkg, filepath.Join(dir, "console", "node_modules", "link")); err != nil {
		t.Fatal(err)
	}
	if hits := filesWithForbidden(t, dir); len(hits) != 0 {
		t.Fatalf("hits = %v", hits)
	}
}

// The platform never reads Multica's development code variable.
func TestPlatformDoesNotReadMulticaDevCodeVariable(t *testing.T) {
	if hits := filesWithForbidden(t, "../.."); len(hits) > 0 {
		t.Fatalf("files mention the Multica development code variable: %v", hits)
	}
}
