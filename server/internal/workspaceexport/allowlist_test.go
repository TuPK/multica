package workspaceexport

import (
	"strings"
	"testing"
)

func TestEveryTableHasAnAllowlist(t *testing.T) {
	want := []string{"workspace", "members", "projects", "issues", "comments", "labels", "statuses", "attachments"}
	if len(Tables) != len(want) {
		t.Fatalf("Tables = %v, want %v", Tables, want)
	}
	for i, table := range want {
		if Tables[i] != table {
			t.Fatalf("Tables[%d] = %q, want %q", i, Tables[i], table)
		}
		if len(AllowedColumns(table)) == 0 {
			t.Errorf("table %q has an empty allowlist", table)
		}
	}
	if AllowedColumns("agent") != nil {
		t.Error("an unknown table must have no allowed columns")
	}
}

func TestNoAllowlistNamesASecretColumn(t *testing.T) {
	forbidden := []string{"custom_env", "mcp", "token", "secret", "password", "credential", "api_key"}
	for _, table := range Tables {
		for _, col := range AllowedColumns(table) {
			for _, bad := range forbidden {
				if strings.Contains(strings.ToLower(col), bad) {
					t.Errorf("table %q allows column %q, which names a secret (%q)", table, col, bad)
				}
			}
		}
	}
}

func TestAllowedColumnsReturnsACopy(t *testing.T) {
	cols := AllowedColumns("issues")
	cols[0] = "custom_env"
	if AllowedColumns("issues")[0] == "custom_env" {
		t.Fatal("AllowedColumns exposes the package's list")
	}
}

func TestEachTableCarriesItsWorkspaceKey(t *testing.T) {
	// The builder drops rows of another workspace by this column, so every
	// table except the workspace itself must allow it.
	for _, table := range Tables {
		key := workspaceKey(table)
		found := false
		for _, col := range AllowedColumns(table) {
			if col == key {
				found = true
			}
		}
		if !found {
			t.Errorf("table %q does not allow its workspace key %q", table, key)
		}
	}
}
