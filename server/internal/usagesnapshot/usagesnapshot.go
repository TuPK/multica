package usagesnapshot

import (
	"time"
)

// RunDelay is how long after UTC midnight the job runs, so that the day it
// computes is closed. The scheduler job waits this long; DayFor assumes it.
const RunDelay = time.Hour

// MaxBatch is the most workspaces one transaction writes.
const MaxBatch = 500

// Counts are the numbers of one workspace for one day. Each field is produced
// by the query its Metric names.
type Counts struct {
	Members       int32
	Agents        int32
	Runtimes      int32
	IssuesTotal   int32
	IssuesCreated int32
	TasksRun      int32
	StorageBytes  int64
}

// Row is one platform_usage_daily row.
type Row struct {
	WorkspaceID string
	Day         time.Time // midnight UTC of the day
	Counts
}

// Bounds says which edges of the day a count's query applies.
type Bounds int

const (
	// BoundsEnd counts the state at the end of the day: rows created before
	// day_end.
	BoundsEnd Bounds = iota
	// BoundsWindow counts what happened in the day: day_start <= x < day_end.
	BoundsWindow
)

// Metric is the definition of one count: the platform_usage_daily column, the
// sqlc query in pkg/db/queries/platform_usage_daily.sql that produces it, and
// the bounds that query applies. A test reads the query text and checks that it
// filters by workspace and uses these bounds.
type Metric struct {
	Column string
	Query  string
	Bounds Bounds
}

// Metrics lists every count, in column order.
var Metrics = []Metric{
	{"members", "CountUsageMembers", BoundsEnd},
	{"agents", "CountUsageAgents", BoundsEnd},
	{"runtimes", "CountUsageRuntimes", BoundsEnd},
	{"issues_total", "CountUsageIssuesTotal", BoundsEnd},
	{"issues_created", "CountUsageIssuesCreated", BoundsWindow},
	{"tasks_run", "CountUsageTasksRun", BoundsWindow},
	{"storage_bytes", "SumUsageStorageBytes", BoundsEnd},
}

// Window is a UTC calendar day as the half-open interval [Start, End).
type Window struct {
	Start, End time.Time
}

// DayWindow returns the window of the UTC date that t falls on.
func DayWindow(t time.Time) Window {
	start := midnightUTC(t)
	return Window{Start: start, End: start.AddDate(0, 0, 1)}
}

// Contains reports whether t is on the window's day: Start is in, End is out.
func (w Window) Contains(t time.Time) bool {
	return !t.Before(w.Start) && t.Before(w.End)
}

func midnightUTC(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// DayFor returns the day (midnight UTC) a run at now computes. The job is due
// RunDelay after midnight UTC of day D, and computes D-1. A run before that
// moment belongs to the previous plan and computes D-2.
func DayFor(now time.Time) time.Time {
	return midnightUTC(now.UTC().Add(-RunDelay)).AddDate(0, 0, -1)
}
