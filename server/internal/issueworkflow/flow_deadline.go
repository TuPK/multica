package issueworkflow

import "time"

// Deadlines of the flow runner (spec section 5.6). The decision is pure: the sweep reads the runs of
// a task, asks what to do and does it.

// RunStats is what the sweep knows about the runs of one issue since its step began.
type RunStats struct {
	// Attempts is the number of runs made since the step began, active ones included.
	Attempts int
	// Active is the number of runs queued, dispatched or running.
	Active int
	// RunningFor is how long the oldest running run has run; zero when none runs.
	RunningFor time.Duration
	// IdleFor is the time since the issue last had a run start or end.
	IdleFor time.Duration
}

// DeadlineLimits are the limits of a flow, from its policy.
type DeadlineLimits struct {
	// Idle is how long an open task may sit without a run before it gets another.
	Idle time.Duration
	// HardCap is how long a run may last.
	HardCap     time.Duration
	MaxAttempts int
}

// LimitsOf reads the limits of a flow's policy, with the defaults for what it leaves out.
func LimitsOf(p Policy) DeadlineLimits {
	p = p.Effective()
	return DeadlineLimits{
		Idle:        time.Duration(p.TaskIdleMinutes) * time.Minute,
		HardCap:     time.Duration(p.TaskHardCapMinutes) * time.Minute,
		MaxAttempts: p.TaskMaxAttempts,
	}
}

// DeadlineKind is what the sweep does about an open task.
type DeadlineKind string

const (
	// DeadlineNone: leave the task alone.
	DeadlineNone DeadlineKind = "none"
	// DeadlineEnqueue: give the task a run (a first one, or a retry).
	DeadlineEnqueue DeadlineKind = "enqueue"
	// DeadlineCancelRun: a run outlived the hard cap; cancel it. Stall is set when it was the last attempt.
	DeadlineCancelRun DeadlineKind = "cancel_run"
	// DeadlineStall: the attempts are used up; block the task and stall the request.
	DeadlineStall DeadlineKind = "stall"
)

// Deadline is the sweep's decision for one open task.
type Deadline struct {
	Kind DeadlineKind
	// Stall, with DeadlineCancelRun: that was the last attempt, so the request stalls once the run is cancelled.
	Stall bool
}

// DecideDeadline decides what to do about an open task with these runs.
func DecideDeadline(l DeadlineLimits, s RunStats) Deadline {
	if s.Active > 0 {
		if s.RunningFor > l.HardCap {
			return Deadline{Kind: DeadlineCancelRun, Stall: s.Attempts >= l.MaxAttempts}
		}
		return Deadline{Kind: DeadlineNone}
	}
	if s.Attempts == 0 {
		return Deadline{Kind: DeadlineEnqueue}
	}
	if s.IdleFor <= l.Idle {
		return Deadline{Kind: DeadlineNone}
	}
	if s.Attempts >= l.MaxAttempts {
		return Deadline{Kind: DeadlineStall}
	}
	return Deadline{Kind: DeadlineEnqueue}
}
