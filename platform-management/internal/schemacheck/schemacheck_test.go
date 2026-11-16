package schemacheck

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

const migrationsDir = "../../../server/migrations"

func TestRequiredVersionsAreSortedUniqueAndExist(t *testing.T) {
	seen := map[string]bool{}
	for i, v := range Required {
		if seen[v] {
			t.Errorf("duplicate version %s", v)
		}
		seen[v] = true
		if i > 0 && Required[i-1] >= v {
			t.Errorf("%s is not after %s; keep the list sorted", v, Required[i-1])
		}
		if _, err := os.Stat(filepath.Join(migrationsDir, v+".up.sql")); err != nil {
			t.Errorf("required version %s has no migration file: %v", v, err)
		}
	}
	if len(Required) == 0 {
		t.Fatal("Required is empty")
	}
}

// The platform's own tables are the ones with these names. A migration that
// touches one without its version in Required would let the service start
// against a schema that lacks the change.
var platformTable = regexp.MustCompile(`^(platform_.*|plan|workspace_subscription|billing_order|billing_notice|workspace_export|platform_usage_daily|platform_seat_operation)$`)

var (
	sqlLineComment  = regexp.MustCompile(`--[^\n]*`)
	sqlBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	tableStatement  = regexp.MustCompile(`(?i)\b(?:create|alter|drop)\s+table\s+(?:if\s+(?:not\s+)?exists\s+)?(?:only\s+)?(?:public\.)?"?(\w+)"?`)
	indexStatement  = regexp.MustCompile(`(?i)\bcreate\s+(?:unique\s+)?index\s+(?:concurrently\s+)?(?:if\s+not\s+exists\s+)?(?:\w+\s+)?on\s+(?:only\s+)?(?:public\.)?"?(\w+)"?`)
)

// tablesTouched returns the table names a migration creates, alters, drops,
// or builds an index on.
func tablesTouched(sql string) []string {
	sql = sqlBlockComment.ReplaceAllString(sql, " ")
	sql = sqlLineComment.ReplaceAllString(sql, " ")
	var out []string
	for _, re := range []*regexp.Regexp{tableStatement, indexStatement} {
		for _, m := range re.FindAllStringSubmatch(sql, -1) {
			out = append(out, strings.ToLower(m[1]))
		}
	}
	return out
}

// unlistedMigrations reads the *.up.sql files numbered lo to hi in dir and
// returns the versions that touch a platform table but are not in required.
func unlistedMigrations(dir string, lo, hi int, required []string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return nil, err
	}
	listed := map[string]bool{}
	for _, v := range required {
		listed[v] = true
	}
	var out []string
	for _, f := range files {
		version := strings.TrimSuffix(filepath.Base(f), ".up.sql")
		num, _, _ := strings.Cut(version, "_")
		n, err := strconv.Atoi(num)
		if err != nil || n < lo || n > hi {
			continue
		}
		body, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, table := range tablesTouched(string(body)) {
			if platformTable.MatchString(table) && !listed[version] {
				out = append(out, version)
				break
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func TestEveryPlatformTableMigrationIsRequired(t *testing.T) {
	unlisted, err := unlistedMigrations(migrationsDir, 651, 699, Required)
	if err != nil {
		t.Fatal(err)
	}
	if len(unlisted) > 0 {
		t.Fatalf("migrations touch platform tables but are missing from schemacheck.Required: %v", unlisted)
	}

	// The scan must see the real platform migrations, or it proves nothing.
	probe, err := unlistedMigrations(migrationsDir, 651, 699, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(probe) == 0 {
		t.Fatal("scan found no platform-table migration in 651 to 699; the patterns are not matching")
	}
}

func TestScanReportsAnUnlistedMigration(t *testing.T) {
	dir := t.TempDir()
	write := func(name, sql string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(sql), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("661_platform_session.up.sql", "CREATE TABLE IF NOT EXISTS platform_session (id uuid);")
	write("662_plan_alter.up.sql", "-- note\nALTER TABLE plan ADD COLUMN x int;")
	write("663_billing_notice_idx.up.sql", "CREATE INDEX CONCURRENTLY IF NOT EXISTS billing_notice_idx ON billing_notice (id);")
	write("664_listed.up.sql", "CREATE TABLE platform_listed (id uuid);")
	write("665_other_table.up.sql", "CREATE TABLE issue_extra (id uuid);")
	write("666_comment_only.up.sql", "-- CREATE TABLE platform_commented (id uuid);\n/* ALTER TABLE plan ADD x int; */ SELECT 1;")
	write("700_out_of_range.up.sql", "CREATE TABLE platform_late (id uuid);")
	write("650_below_range.up.sql", "CREATE TABLE platform_early (id uuid);")
	write("661_platform_session.down.sql", "DROP TABLE platform_session;")

	got, err := unlistedMigrations(dir, 651, 699, []string{"664_listed"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"661_platform_session", "662_plan_alter", "663_billing_notice_idx"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unlisted = %v, want %v", got, want)
	}
}

func TestMissing(t *testing.T) {
	required := []string{"651_a", "652_b", "653_c"}
	got := missing(required, []string{"652_b", "999_other"})
	if want := []string{"651_a", "653_c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}
	if got := missing(required, required); len(got) != 0 {
		t.Fatalf("missing = %v, want none", got)
	}
}

func TestMissingErrorListsVersions(t *testing.T) {
	err := error(&MissingError{Versions: []string{"651_a", "653_c"}})
	var me *MissingError
	if !errors.As(err, &me) {
		t.Fatal("not a *MissingError")
	}
	for _, v := range []string{"651_a", "653_c"} {
		if !strings.Contains(err.Error(), v) {
			t.Errorf("error %q does not list %s", err, v)
		}
	}
}

// Database test: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it.
func TestCheckAgainstDatabase(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	// A temporary schema_migrations table shadows the real one for this
	// transaction only, so the test never reads or changes recorded versions.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE schema_migrations (version text PRIMARY KEY) ON COMMIT DROP`); err != nil {
		t.Fatal(err)
	}

	// All but the last required version recorded: the last one is reported.
	for _, v := range Required[:len(Required)-1] {
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, v); err != nil {
			t.Fatal(err)
		}
	}
	err = Check(ctx, tx, Required)
	var me *MissingError
	if !errors.As(err, &me) {
		t.Fatalf("Check err = %v, want *MissingError", err)
	}
	if want := []string{Required[len(Required)-1]}; !reflect.DeepEqual(me.Versions, want) {
		t.Fatalf("missing = %v, want %v", me.Versions, want)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, Required[len(Required)-1]); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, tx, Required); err != nil {
		t.Fatalf("Check with every version recorded: %v", err)
	}
}

// The plan and subscription tables are read by the policy endpoint on every
// request, so the service must refuse to start without them.
func TestRequiredListsThePlanAndSubscriptionMigrations(t *testing.T) {
	for _, want := range []string{
		"670_plan",
		"671_plan_key_uidx",
		"672_workspace_subscription",
		"673_workspace_subscription_uidx",
		"686_workspace_subscription_status_idx",
	} {
		if !slices.Contains(Required, want) {
			t.Errorf("Required does not list %s", want)
		}
	}
}

func TestRequiredIncludesSeatOperationMigrations(t *testing.T) {
	for _, v := range []string{
		"674_platform_seat_operation",
		"675_platform_seat_operation_uidx",
		"676_platform_seat_operation_state_idx",
	} {
		if !slices.Contains(Required, v) {
			t.Errorf("Required lacks %s", v)
		}
	}
}

func TestRequiredCoversTheBillingOrderMigrations(t *testing.T) {
	for _, v := range []string{
		"677_billing_order",
		"678_billing_order_id_uidx",
		"679_billing_order_idem_uidx",
		"680_billing_order_transfer_uidx",
		"681_billing_order_status_idx",
		"682_billing_order_workspace_idx",
	} {
		if !slices.Contains(Required, v) {
			t.Errorf("Required lacks %s", v)
		}
	}
}

func TestRequiredCoversTheBillingNoticeMigrations(t *testing.T) {
	for _, v := range []string{
		"683_billing_notice",
		"684_billing_notice_key_uidx",
	} {
		if !slices.Contains(Required, v) {
			t.Errorf("Required lacks %s", v)
		}
	}
}

func TestUsageDailyMigrationsAreRequired(t *testing.T) {
	for _, v := range []string{"689_platform_usage_daily", "690_platform_usage_daily_uidx"} {
		if !slices.Contains(Required, v) {
			t.Errorf("%s is not in Required", v)
		}
	}
}
