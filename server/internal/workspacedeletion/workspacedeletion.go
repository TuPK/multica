// Package workspacedeletion holds the rules for deleting a workspace after a
// grace period: when the deletion is scheduled, when it is due, and how the
// period is configured. It has no database, HTTP or clock dependency; callers
// pass the time in.
package workspacedeletion

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvDays names the setting that turns the grace period on. 0 or unset means
// a workspace is deleted at once.
const EnvDays = "PLATFORM_WORKSPACE_DELETION_DAYS"

// Reason is stored in platform_workspace.reason when the owner schedules the deletion.
const Reason = "owner_requested"

// StatusPending is the platform_workspace status of a workspace waiting for deletion.
const StatusPending = "pending_deletion"

// Decide returns when a deletion requested at now is carried out, or nil when
// days is 0 and the workspace is deleted at once.
func Decide(now time.Time, days int) (scheduleAt *time.Time) {
	if days <= 0 {
		return nil
	}
	at := now.Add(time.Duration(days) * 24 * time.Hour)
	return &at
}

// Due reports whether a deletion scheduled for scheduledAt may run at now. The
// schedule time itself is due.
func Due(now time.Time, scheduledAt time.Time) bool {
	return !scheduledAt.After(now)
}

// ParseDays reads the value of EnvDays. An empty value means 0; anything that
// is not a whole number of at least 0 is an error that names the variable.
func ParseDays(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a whole number of days, got %q", EnvDays, raw)
	}
	if n < 0 {
		return 0, fmt.Errorf("%s must be 0 or more, got %d", EnvDays, n)
	}
	return n, nil
}

// DaysFromEnv reads and parses EnvDays from the process environment.
func DaysFromEnv() (int, error) {
	return ParseDays(os.Getenv(EnvDays))
}
