package workspacedeletion

import (
	"strings"
	"testing"
	"time"
)

// fakeNow is the fixed clock every test uses; the package never reads the real one.
var fakeNow = time.Date(2030, 3, 1, 12, 0, 0, 0, time.UTC)

func TestDecideWithZeroDaysSchedulesNothing(t *testing.T) {
	if got := Decide(fakeNow, 0); got != nil {
		t.Fatalf("Decide(now, 0) = %v, want nil", *got)
	}
}

func TestDecideAddsWholeDays(t *testing.T) {
	got := Decide(fakeNow, 14)
	if got == nil {
		t.Fatal("Decide(now, 14) = nil")
	}
	if want := fakeNow.Add(14 * 24 * time.Hour); !got.Equal(want) {
		t.Fatalf("Decide(now, 14) = %v, want %v", got, want)
	}
}

func TestDecideOneDayIsTheSmallestSchedule(t *testing.T) {
	got := Decide(fakeNow, 1)
	if got == nil || !got.Equal(fakeNow.Add(24*time.Hour)) {
		t.Fatalf("Decide(now, 1) = %v", got)
	}
}

func TestDecideDoesNotMoveTheCallersClock(t *testing.T) {
	now := fakeNow
	_ = Decide(now, 14)
	if !now.Equal(fakeNow) {
		t.Fatalf("now changed to %v", now)
	}
}

func TestDueBoundaryIsInclusive(t *testing.T) {
	at := fakeNow
	if !Due(at, at) {
		t.Fatal("Due at the schedule time = false, want true")
	}
	if Due(at.Add(-time.Second), at) {
		t.Fatal("Due one second before the schedule = true, want false")
	}
	if !Due(at.Add(time.Second), at) {
		t.Fatal("Due one second after the schedule = false, want true")
	}
}

func TestDecideThenDueAfterTheWait(t *testing.T) {
	at := Decide(fakeNow, 14)
	if at == nil {
		t.Fatal("Decide returned nil")
	}
	if Due(fakeNow.Add(14*24*time.Hour-time.Second), *at) {
		t.Fatal("due one second early")
	}
	if !Due(fakeNow.Add(14*24*time.Hour), *at) {
		t.Fatal("not due when the 14 days end")
	}
}

func TestParseDays(t *testing.T) {
	ok := []struct {
		raw  string
		want int
	}{
		{"", 0},
		{"   ", 0},
		{"0", 0},
		{"14", 14},
		{" 30 ", 30},
	}
	for _, c := range ok {
		got, err := ParseDays(c.raw)
		if err != nil || got != c.want {
			t.Errorf("ParseDays(%q) = %d, %v; want %d, nil", c.raw, got, err, c.want)
		}
	}
	for _, raw := range []string{"-1", "abc", "1.5", "14d", "+-3"} {
		got, err := ParseDays(raw)
		if err == nil {
			t.Errorf("ParseDays(%q) = %d, nil; want an error", raw, got)
			continue
		}
		if !strings.Contains(err.Error(), "PLATFORM_WORKSPACE_DELETION_DAYS") {
			t.Errorf("ParseDays(%q) error %q does not name the variable", raw, err)
		}
	}
}

func TestDaysFromEnv(t *testing.T) {
	t.Setenv(EnvDays, "")
	if got, err := DaysFromEnv(); err != nil || got != 0 {
		t.Fatalf("empty = %d, %v; want 0, nil", got, err)
	}
	t.Setenv(EnvDays, "14")
	if got, err := DaysFromEnv(); err != nil || got != 14 {
		t.Fatalf("14 = %d, %v; want 14, nil", got, err)
	}
	t.Setenv(EnvDays, "-1")
	if _, err := DaysFromEnv(); err == nil || !strings.Contains(err.Error(), EnvDays) {
		t.Fatalf("-1 error = %v, want one naming %s", err, EnvDays)
	}
}
