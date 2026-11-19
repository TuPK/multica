package plan

import (
	"strings"
	"testing"
	"time"
)

// Spec R6 section 3b.1 rule 2: description is optional, at most 500 characters.
func TestPlanDescriptionMaxLength(t *testing.T) {
	base := Plan{Key: "pro", Name: "Pro", Currency: "vnd", Limits: Limits{}}
	ok := base
	ok.Description = strings.Repeat("ớ", 500)
	if err := Validate(ok); err != nil {
		t.Fatalf("500 characters must be accepted: %v", err)
	}
	long := base
	long.Description = strings.Repeat("a", 501)
	if err := Validate(long); err == nil {
		t.Fatal("501 characters must be refused")
	}
}

// Spec R6 section 3b.1 rule 5: deactivate sets archived_at once; activate clears it.
func TestWithArchivedIsIdempotent(t *testing.T) {
	p := Plan{Key: "pro"}
	now := mustTime(t)
	off, changed := WithArchived(p, true, now)
	if !changed || off.ArchivedAt == nil || !off.ArchivedAt.Equal(now) {
		t.Fatalf("deactivate: changed=%v archived_at=%v", changed, off.ArchivedAt)
	}
	again, changed := WithArchived(off, true, now.Add(1))
	if changed || !again.ArchivedAt.Equal(now) {
		t.Fatalf("second deactivate must not change anything: changed=%v", changed)
	}
	on, changed := WithArchived(again, false, now.Add(2))
	if !changed || on.ArchivedAt != nil {
		t.Fatalf("activate: changed=%v archived_at=%v", changed, on.ArchivedAt)
	}
	if _, changed := WithArchived(on, false, now.Add(3)); changed {
		t.Fatal("second activate must not change anything")
	}
}

func mustTime(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
}
