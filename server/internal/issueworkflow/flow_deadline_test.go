package issueworkflow

import (
	"testing"
	"time"
)

func TestDecideDeadline(t *testing.T) {
	limits := DeadlineLimits{Idle: 120 * time.Minute, HardCap: 240 * time.Minute, MaxAttempts: 2}
	cases := []struct {
		name string
		in   RunStats
		want Deadline
	}{
		{"a task with no run yet gets one", RunStats{}, Deadline{Kind: DeadlineEnqueue}},
		{"a run in progress is left alone", RunStats{Attempts: 1, Active: 1, RunningFor: time.Hour}, Deadline{Kind: DeadlineNone}},
		{"a queued run is left alone", RunStats{Attempts: 1, Active: 1}, Deadline{Kind: DeadlineNone}},
		{"a run past the hard cap is cancelled", RunStats{Attempts: 1, Active: 1, RunningFor: 5 * time.Hour}, Deadline{Kind: DeadlineCancelRun}},
		{"the last attempt past the hard cap stalls after the cancel", RunStats{Attempts: 2, Active: 1, RunningFor: 5 * time.Hour}, Deadline{Kind: DeadlineCancelRun, Stall: true}},
		{"an idle task within the timeout waits", RunStats{Attempts: 1, IdleFor: time.Hour}, Deadline{Kind: DeadlineNone}},
		{"an idle task past the timeout is retried", RunStats{Attempts: 1, IdleFor: 3 * time.Hour}, Deadline{Kind: DeadlineEnqueue}},
		{"an idle task with no attempts left stalls", RunStats{Attempts: 2, IdleFor: 3 * time.Hour}, Deadline{Kind: DeadlineStall}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecideDeadline(limits, tc.in); got != tc.want {
				t.Fatalf("DecideDeadline(%+v) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestLimitsOfUsesThePolicyAndTheDefaults(t *testing.T) {
	got := LimitsOf(Policy{TaskIdleMinutes: 10, TaskMaxAttempts: 3})
	if got.Idle != 10*time.Minute || got.MaxAttempts != 3 || got.HardCap != time.Duration(DefaultTaskHardCapMinutes)*time.Minute {
		t.Fatalf("limits = %+v", got)
	}
}
