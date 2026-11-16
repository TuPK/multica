package scheduler

import (
	"context"
	"time"
)

// JobNameWorkspaceExport is the canonical name used in audit rows. Stable
// across releases.
const JobNameWorkspaceExport = "workspace_export"

// WorkspaceExportRunner builds the queued workspace exports and expires old
// ones. internal/workspaceexport implements it.
type WorkspaceExportRunner interface {
	// RunOnce returns how many queued exports it claimed.
	RunOnce(ctx context.Context) (int, error)
}

// WorkspaceExportJob runs every minute. A run claims one queued export at a
// time, so a second server process running the same job does not build an
// export twice, and a run that fails to build an export marks that export
// failed instead of failing the run.
func WorkspaceExportJob(runner WorkspaceExportRunner) JobSpec {
	return JobSpec{
		Name:              JobNameWorkspaceExport,
		Cadence:           time.Minute,
		CatchUpMode:       CatchUpLatestOnly,
		CatchUpWindow:     time.Minute,
		RunTimeout:        30 * time.Minute,
		StaleTimeout:      35 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       1,
		Scopes:            StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			n, err := runner.RunOnce(ctx)
			if err != nil {
				return HandlerResult{}, err
			}
			return HandlerResult{RowsAffected: int64(n)}, nil
		},
	}
}
