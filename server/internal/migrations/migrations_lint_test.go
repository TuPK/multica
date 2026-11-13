package migrations

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// legacyDuplicateMigrationStems are migrations 595–616 that already shipped with
// a shared prefix. They are applied on live databases keyed by their full name,
// so they cannot be renamed. Any new duplicate is still an error.
var legacyDuplicateMigrationStems = map[string]bool{
	"600_agent_office_room":                       true,
	"600_wecom_install_session":                   true,
	"601_office_floor":                            true,
	"601_wecom_install_session_due_idx":           true,
	"602_office_floor_pkey_uidx":                  true,
	"602_wecom_install_session_request_idx":       true,
	"603_office_floor_primary_key":                true,
	"603_wecom_install_session_agent_pending_idx": true,
	"604_office_floor_workspace_position_idx":     true,
	"604_wecom_install_session_begin_rate_idx":    true,
	"605_agent_runtime_max_concurrent_tasks":      true,
	"605_squad_office_seat":                       true,
	"606_agent_task_queue_runtime_active_index":   true,
	"606_squad_office_seat_uidx":                  true,
	"607_agent_home_squad":                        true,
	"607_inbox_item_section":                      true,
	"608_drop_agent_office_room":                  true,
	"608_task_usage_credits":                      true,
	"610_proposal":                                true,
	"610_runtime_profile_add_zcode":               true,
	"611_proposal_pkey_index":                     true,
	"611_user_whats_new_seen_version":             true,
	"612_proposal_primary_key":                    true,
	"612_skill_version":                           true,
	"613_proposal_workspace_state_index":          true,
	"613_skill_version_index":                     true,
	"614_project_creator":                         true,
	"614_proposal_squad_state_index":              true,
	"615_attachment_image_dimensions":             true,
	"615_proposal_issue_uidx":                     true,
	"616_issue_origin_proposal":                   true,
	"616_member_pinned":                           true,

	"595_issue_queue_order":                      true,
	"595_issue_view_preference_sidebar":          true,
	"596_issue_queue_order_index":                true,
	"596_issue_view_preference_sidebar_projects": true,
	"597_agent_task_branch_merge":                true,
	"597_issue_unread":                           true,
	"598_issue_unread_unique_index":              true,
	"598_runtime_quickstart_request":             true,
	"599_issue_unread_primary_key":               true,
	"599_runtime_quickstart_request_index":       true,
}

func TestMigrationNumericPrefixesAreUnique(t *testing.T) {
	files := migrationFilesForLint(t, "*.up.sql")

	// Migrations through 128 contain historical duplicate numeric prefixes.
	// From 129 onward, keep the numeric sequence unique so release tooling and
	// operators can identify one schema change unambiguously by its number.
	const firstUniqueMigrationNumber = 129
	stemByNumber := make(map[int]string)
	for _, file := range files {
		stem, _, ok := splitMigrationFilename(filepath.Base(file))
		if !ok {
			continue
		}
		prefix, _, ok := strings.Cut(stem, "_")
		if !ok {
			continue
		}
		number, err := strconv.Atoi(prefix)
		if err != nil || number < firstUniqueMigrationNumber {
			continue
		}
		if previous, exists := stemByNumber[number]; exists {
			if legacyDuplicateMigrationStems[previous] && legacyDuplicateMigrationStems[stem] {
				continue
			}
			t.Errorf("migrations %s and %s share numeric prefix %s", previous, stem, prefix)
			continue
		}
		stemByNumber[number] = stem
	}
}

func TestMigrationFilesHaveMatchingDirections(t *testing.T) {
	files := migrationFilesForLint(t, "*.sql")

	directionsByStem := make(map[string]map[string]bool)
	for _, file := range files {
		stem, direction, ok := splitMigrationFilename(filepath.Base(file))
		if !ok {
			continue
		}
		if directionsByStem[stem] == nil {
			directionsByStem[stem] = make(map[string]bool)
		}
		directionsByStem[stem][direction] = true
	}

	for stem, directions := range directionsByStem {
		if !directions["up"] || !directions["down"] {
			t.Errorf("migration %s must have both .up.sql and .down.sql files", stem)
		}
	}
}

func migrationFilesForLint(t *testing.T, pattern string) []string {
	t.Helper()

	dir := realMigrationsDir(t)
	files, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no migration files matched %s in %s", pattern, dir)
	}
	sort.Strings(files)
	return files
}

func realMigrationsDir(t *testing.T) string {
	t.Helper()

	// Relative to the package directory go test runs in; runtime.Caller
	// would return a module path under -trimpath.
	dir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	return dir
}

func splitMigrationFilename(name string) (stem, direction string, ok bool) {
	for _, candidateDirection := range []string{"up", "down"} {
		suffix := fmt.Sprintf(".%s.sql", candidateDirection)
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix), candidateDirection, true
		}
	}
	return "", "", false
}
