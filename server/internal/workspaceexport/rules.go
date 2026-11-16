package workspaceexport

import (
	"errors"
	"time"
)

// Export statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
	StatusExpired = "expired"
)

const (
	// LinkValidity is how long a finished export can be downloaded.
	LinkValidity = 7 * 24 * time.Hour
	// DailyWindow is the span in which a workspace may create one export.
	DailyWindow = 24 * time.Hour
)

// Refusals of a new export. The handler maps them to 409 export_in_progress and
// 429 export_daily_limit.
var (
	ErrInProgress = errors.New("an export is already queued or running")
	ErrDailyLimit = errors.New("the workspace exported within the last 24 hours")
)

// Export is one workspace_export row.
type Export struct {
	ID          string
	WorkspaceID string
	RequestedBy string
	Status      string
	StorageKey  string
	SizeBytes   int64
	Error       string
	CreatedAt   time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
	ExpiresAt   *time.Time
}

// Latest is the newest export of a workspace, whatever its status.
type Latest struct {
	Status    string
	CreatedAt time.Time
}

// Admit decides whether a workspace may create an export now, given its newest
// export (nil when it has none). An export that is queued or running is
// refused first; any export created within DailyWindow, failed ones included,
// counts against the daily limit.
func Admit(now time.Time, latest *Latest) error {
	if latest == nil {
		return nil
	}
	if latest.Status == StatusQueued || latest.Status == StatusRunning {
		return ErrInProgress
	}
	if now.Sub(latest.CreatedAt) < DailyWindow {
		return ErrDailyLimit
	}
	return nil
}

// ExpiresAt is when the link of an export finished at finishedAt stops working.
func ExpiresAt(finishedAt time.Time) time.Time {
	return finishedAt.Add(LinkValidity)
}

// EffectiveStatus is the status to show at now. A done export past its expiry
// reads as expired even before the job has marked it.
func EffectiveStatus(e Export, now time.Time) string {
	if e.Status == StatusDone && e.ExpiresAt != nil && !now.Before(*e.ExpiresAt) {
		return StatusExpired
	}
	return e.Status
}

// DownloadWindow returns how long a download link may stay valid, which is the
// time left until the export expires. It reports false when no link may be
// issued: the export is not done, has no expiry, or has expired.
func DownloadWindow(e Export, now time.Time) (time.Duration, bool) {
	if EffectiveStatus(e, now) != StatusDone || e.ExpiresAt == nil {
		return 0, false
	}
	return e.ExpiresAt.Sub(now), true
}
