package main

import (
	"strings"
	"testing"
)

const (
	testDB   = "postgres://multica:pw@localhost:5432/multica_platform_test?sslmode=disable"
	sharedDB = "postgres://multica:pw@localhost:5432/multica?sslmode=disable"
)

func TestGuard(t *testing.T) {
	cases := []struct {
		name            string
		dsn             string
		allowProduction bool
		wantRefused     bool
	}{
		{"test database", testDB, false, false},
		{"shared development database refused", sharedDB, false, true},
		{"production name refused", "postgres://multica:pw@localhost:5432/platform?sslmode=disable", false, true},
		{"production name allowed with the flag", "postgres://multica:pw@localhost:5432/platform?sslmode=disable", true, false},
		{"shared database allowed with the flag", sharedDB, true, false},
		{"empty refused", "", false, true},
		{"empty refused even with the flag", "", true, true},
		{"unparsable refused", "://nope", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := guard(tc.dsn, tc.allowProduction)
			if (err != nil) != tc.wantRefused {
				t.Fatalf("guard error = %v, want refused=%v", err, tc.wantRefused)
			}
		})
	}
}

func TestGuardErrorDoesNotLeakPassword(t *testing.T) {
	err := guard("postgres://multica:hunter2@localhost:5432/platform", false)
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error = %v; want a refusal without the password", err)
	}
}

func TestRunRefusesProductionNameWithoutFlag(t *testing.T) {
	getenv := func(string) string { return "" }
	err := run([]string{"--database-url", "postgres://multica:pw@localhost:1/platform"}, getenv)
	if err == nil || !strings.Contains(err.Error(), "--allow-production") {
		t.Fatalf("run error = %v; want a refusal that names --allow-production", err)
	}
}

func TestRunReadsDatabaseURLFromEnvironment(t *testing.T) {
	getenv := func(name string) string {
		if name == "DATABASE_URL" {
			return sharedDB
		}
		return ""
	}
	err := run(nil, getenv)
	if err == nil || !strings.Contains(err.Error(), "--allow-production") {
		t.Fatalf("run error = %v; want the environment DSN refused by the guard", err)
	}
}

func TestPlatformBackfillTenantsSubcommandParsesArguments(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    options
		wantErr string
	}{
		{"no command is the subscription backfill", []string{"--database-url", testDB}, options{command: commandSubscriptions, databaseURL: testDB}, ""},
		{"tenants", []string{"tenants"}, options{command: commandTenants}, ""},
		{"tenants with flags after it", []string{"tenants", "--database-url", testDB, "--allow-production"}, options{command: commandTenants, databaseURL: testDB, allowProduction: true}, ""},
		{"subscriptions by name", []string{"subscriptions", "--allow-production"}, options{command: commandSubscriptions, allowProduction: true}, ""},
		{"unknown command", []string{"orders"}, options{}, "unknown command"},
		{"unknown flag", []string{"tenants", "--nope"}, options{}, "flag provided but not defined"},
		{"stray argument", []string{"tenants", "extra"}, options{}, "unexpected argument"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseArgs(tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("parseArgs(%v) error = %v, want one containing %q", tc.args, err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("parseArgs(%v) = %+v, %v; want %+v", tc.args, got, err, tc.want)
			}
		})
	}
}

// The tenants command goes through the same guard as the subscription backfill.
func TestRunTenantsRefusesProductionNameWithoutFlag(t *testing.T) {
	err := run([]string{"tenants", "--database-url", "postgres://multica:pw@localhost:1/platform"}, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "--allow-production") {
		t.Fatalf("run error = %v; want a refusal that names --allow-production", err)
	}
}
