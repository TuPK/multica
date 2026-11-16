package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/migrations"
)

// TestSSOConnectionMigrationsAreRegistered pins migrations 696 and 697 (platform
// spec 6.3, the OIDC connection): the table with no foreign key and no index,
// the unique index in its own file built concurrently, and a cleanup entry.
func TestSSOConnectionMigrationsAreRegistered(t *testing.T) {
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

	table := read("696_sso_connection", "up")
	upper := strings.ToUpper(table)
	if !strings.Contains(upper, "CREATE TABLE SSO_CONNECTION") || strings.Contains(upper, "REFERENCES") || strings.Contains(upper, "INDEX") ||
		strings.Contains(upper, "ON DELETE") || strings.Contains(upper, "ON UPDATE") {
		t.Errorf("696_sso_connection must create the table only, with no foreign key, cascade or index:\n%s", table)
	}
	for _, col := range []string{"id ", "workspace_id ", "issuer ", "client_id ", "client_secret_enc ", "default_role ", "status ", "created_by "} {
		if !strings.Contains(table, col) {
			t.Errorf("696_sso_connection has no column %q", strings.TrimSpace(col))
		}
	}
	if !strings.Contains(read("696_sso_connection", "down"), "DROP TABLE IF EXISTS sso_connection") {
		t.Error("696 down does not drop the table")
	}

	const version, index = "697_sso_connection_workspace_uidx", "sso_connection_workspace_uidx"
	up := read(version, "up")
	if strings.Count(up, ";") != 1 || !strings.Contains(up, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS "+index+" ON sso_connection (workspace_id)") {
		t.Errorf("%s up must be one CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS statement on workspace_id:\n%s", version, up)
	}
	if down := read(version, "down"); !strings.Contains(down, "DROP INDEX CONCURRENTLY IF EXISTS "+index) {
		t.Errorf("%s down does not drop %s:\n%s", version, index, down)
	}
	if got := concurrentIndexCleanups[version]; got != index {
		t.Errorf("concurrentIndexCleanups[%q] = %q, want %q", version, got, index)
	}
}
