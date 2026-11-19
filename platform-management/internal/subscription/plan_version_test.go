package subscription

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestPutPlanRefusesAStaleExpectedVersion(t *testing.T) {
	f := newDBFixture(t)
	e := f.dbConsole(t)
	p := f.plan(nil)
	f.dropPlanAudits(p.Key)
	path := "/api/platform/plans/" + p.Key
	version := func() int64 {
		t.Helper()
		var v int64
		if err := f.pool.QueryRow(context.Background(), `SELECT version FROM plan WHERE key = $1`, p.Key).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	name := func() string {
		t.Helper()
		var n string
		if err := f.pool.QueryRow(context.Background(), `SELECT name FROM plan WHERE key = $1`, p.Key).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	body := func(newName string, expected string) string {
		b := strings.Replace(teamBody, `"name":"Team"`, `"name":"`+newName+`"`, 1)
		if expected != "" {
			b = strings.Replace(b, "{", `{"expected_version":`+expected+",", 1)
		}
		return b
	}

	start := version()
	wantCode(t, e.do(adminUser, "PUT", path, body("Stale", fmt.Sprint(start+5))), 409, "plan_version_conflict")
	if version() != start || name() == "Stale" {
		t.Fatalf("a refused write changed the row: version %d name %q", version(), name())
	}

	wantStatus(t, e.do(adminUser, "PUT", path, body("Matching", fmt.Sprint(start))), 200)
	if version() != start+1 || name() != "Matching" {
		t.Fatalf("matching write: version %d name %q, want %d Matching", version(), name(), start+1)
	}

	wantCode(t, e.do(adminUser, "PUT", path, body("Again", fmt.Sprint(start))), 409, "plan_version_conflict")

	wantStatus(t, e.do(adminUser, "PUT", path, body("Absent", "")), 200)
	if version() != start+2 || name() != "Absent" {
		t.Fatalf("absent expected_version: version %d name %q, want %d Absent", version(), name(), start+2)
	}
}
