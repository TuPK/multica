package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/migrations"
)

// TestSSODomainMigrationsAreRegistered pins migrations 693 to 695 (platform spec
// 6.3, domain verification): the table with no foreign key, each unique index in
// its own file built concurrently, and a cleanup entry for each index.
func TestSSODomainMigrationsAreRegistered(t *testing.T) {
	dir, err := migrations.ResolveDir()
	if err != nil {
		t.Fatal(err)
	}
	read := func(version, direction string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, version+"."+direction+".sql"))
		if err != nil {
			t.Fatalf("%s %s: %v", version, direction, err)
		}
		return string(b)
	}

	table := read("693_sso_domain", "up")
	if !strings.Contains(table, "CREATE TABLE") || strings.Contains(strings.ToUpper(table), "REFERENCES") || strings.Contains(strings.ToUpper(table), "INDEX") {
		t.Errorf("693_sso_domain must create the table only, with no foreign key and no index:\n%s", table)
	}
	read("693_sso_domain", "down")

	for version, index := range map[string]string{
		"694_sso_domain_verified_domain_uidx":  "sso_domain_verified_domain_uidx",
		"695_sso_domain_workspace_domain_uidx": "sso_domain_workspace_domain_uidx",
	} {
		up := read(version, "up")
		if strings.Count(up, ";") != 1 || !strings.Contains(up, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS "+index+" ") {
			t.Errorf("%s up must be one CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS %s statement:\n%s", version, index, up)
		}
		if down := read(version, "down"); !strings.Contains(down, "DROP INDEX CONCURRENTLY IF EXISTS "+index) {
			t.Errorf("%s down does not drop %s:\n%s", version, index, down)
		}
		if got := concurrentIndexCleanups[version]; got != index {
			t.Errorf("concurrentIndexCleanups[%q] = %q, want %q", version, got, index)
		}
	}
	if !strings.Contains(read("694_sso_domain_verified_domain_uidx", "up"), "WHERE verified_at IS NOT NULL") {
		t.Error("694 must be unique on domain only for verified rows")
	}
}
