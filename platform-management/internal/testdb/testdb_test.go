package testdb

import (
	"strings"
	"testing"
)

func TestGuard(t *testing.T) {
	const ok = "postgres://u:p@localhost:5432/platform_test?sslmode=disable"
	cases := []struct {
		name      string
		testDSN   string
		databaseU string
		wantSkip  string // substring of the reason; empty means allowed
	}{
		{"allowed", ok, ok, ""},
		{"unset test dsn", "", ok, "PLATFORM_TEST_DATABASE_URL is not set"},
		{"unset database url", ok, "", "DATABASE_URL"},
		{"database url differs", ok, "postgres://u:p@localhost:5432/other_test", "DATABASE_URL"},
		{"name without _test", "postgres://u:p@localhost/platform", "postgres://u:p@localhost/platform", "_test"},
		{"multica database", "postgres://u:p@localhost/multica", "postgres://u:p@localhost/multica", "_test"},
		{"multica_test is a test database", "postgres://u:p@localhost/multica_test", "postgres://u:p@localhost/multica_test", ""},
		{"unparsable", "://nope", "://nope", "PLATFORM_TEST_DATABASE_URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := guard(tc.testDSN, tc.databaseU)
			if tc.wantSkip == "" {
				if got != "" {
					t.Fatalf("guard = %q, want allowed", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantSkip) {
				t.Fatalf("guard = %q, want it to mention %q", got, tc.wantSkip)
			}
		})
	}
}

func TestGuardReasonDoesNotLeakCredentials(t *testing.T) {
	got := guard("postgres://user:hunter2@localhost/prod", "postgres://user:hunter2@localhost/prod")
	if got == "" || strings.Contains(got, "hunter2") {
		t.Fatalf("reason = %q", got)
	}
}
