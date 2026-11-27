package migrations

import (
	"path/filepath"
	"testing"
)

// The runner applies files in the order sortMigrations gives. A migration that alters a table an
// earlier number creates must run after that number, so the order is by the leading number. These
// tests need no database.

func sortedBaseNames(names ...string) []string {
	paths := make([]string, len(names))
	for i, n := range names {
		paths[i] = filepath.Join("/migrations", n)
	}
	sortMigrations(paths)
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}

func TestMigrationsSortByNumberNotByName(t *testing.T) {
	got := sortedBaseNames("1107_org_change_application_runtime_type.up.sql", "1000_x.up.sql", "830_org_change_application.up.sql", "870_org_change_application_rollback_types.up.sql")
	want := []string{"830_org_change_application.up.sql", "870_org_change_application_rollback_types.up.sql", "1000_x.up.sql", "1107_org_change_application_runtime_type.up.sql"}
	assertOrder(t, got, want)
}

func TestMigrationsSharingANumberSortByName(t *testing.T) {
	got := sortedBaseNames("109_lark_integration.up.sql", "109_agent_task_waiting_local_directory.up.sql", "109_drop_agent_skills_local.up.sql")
	want := []string{"109_agent_task_waiting_local_directory.up.sql", "109_drop_agent_skills_local.up.sql", "109_lark_integration.up.sql"}
	assertOrder(t, got, want)
}

func TestMigrationsWithoutANumberSortLast(t *testing.T) {
	got := sortedBaseNames("README.up.sql", "5_x.up.sql")
	want := []string{"5_x.up.sql", "README.up.sql"}
	assertOrder(t, got, want)
}

// A down migration runs the reverse of the up order: 1107 is undone before 870, and 870 before 830.
func TestDownMigrationsRunInReverseUpOrder(t *testing.T) {
	paths := []string{
		filepath.Join("/m", "830_org_change_application.down.sql"),
		filepath.Join("/m", "1107_org_change_application_runtime_type.down.sql"),
		filepath.Join("/m", "870_org_change_application_rollback_types.down.sql"),
	}
	sortMigrations(paths)
	reverseFiles(paths)
	got := make([]string, len(paths))
	for i, p := range paths {
		got[i] = filepath.Base(p)
	}
	want := []string{"1107_org_change_application_runtime_type.down.sql", "870_org_change_application_rollback_types.down.sql", "830_org_change_application.down.sql"}
	assertOrder(t, got, want)
}

// The real directory: the org change table is created by 830, rewritten by 870, and altered by
// the runtime change migrations 1116 and 1117, in that order.
func TestRealMigrationsRunTheOrgChangeTableInDependencyOrder(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join(realMigrationsDir(t), "*.up.sql"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no up migrations: %v", err)
	}
	sortMigrations(matches)
	index := map[string]int{}
	for i, f := range matches {
		index[filepath.Base(f)] = i
	}
	chain := []string{
		"830_org_change_application.up.sql",
		"870_org_change_application_rollback_types.up.sql",
		"1116_org_change_application_runtime_type.up.sql",
		"1117_org_change_application_shadow_actor.up.sql",
	}
	for i := 1; i < len(chain); i++ {
		a, okA := index[chain[i-1]]
		b, okB := index[chain[i]]
		if !okA || !okB {
			t.Fatalf("%s or %s is missing from the up migrations", chain[i-1], chain[i])
		}
		if a >= b {
			t.Errorf("%s runs at %d, not before %s at %d", chain[i-1], a, chain[i], b)
		}
	}
	if index["001_init.up.sql"] >= index["1116_org_change_application_runtime_type.up.sql"] {
		t.Error("001_init must run before 1116")
	}
}

func assertOrder(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
