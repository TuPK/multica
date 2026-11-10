package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Scheduled sync of hub sources (spec 5.6). Each enabled source with an
// auto-sync interval is one scope. The scheduler's lease is per (job, scope, plan),
// so one instance runs a source's sync for a given minute; the handler's
// per-source advisory lock stops a manual sync from overlapping it.

const (
	// JobNameHubSourceSync is persisted in sys_cron_executions and must stay stable.
	JobNameHubSourceSync = "hub_source_sync"
	// ScopeKindHubSource is the scope kind of one source.
	ScopeKindHubSource = "hub_source"

	hubSourceSyncScanLimit  = 100
	hubSourceSyncRunTimeout = 5 * time.Minute
)

// HubSourceSyncer runs one scheduled sync of a source. The handler implements it.
// It returns nil when the source is not due, or when another sync holds it.
type HubSourceSyncer interface {
	SyncHubSourceAuto(ctx context.Context, sourceID string) error
}

// HubSourceSyncJob syncs due hub sources. The sweep runs every minute, and a source
// that is still due after a failed run is picked up again on the next minute.
func HubSourceSyncJob(queries *db.Queries, syncer HubSourceSyncer) JobSpec {
	return JobSpec{
		Name:              JobNameHubSourceSync,
		Cadence:           time.Minute,
		CatchUpMode:       CatchUpLatestOnly,
		RunTimeout:        hubSourceSyncRunTimeout,
		StaleTimeout:      hubSourceSyncRunTimeout + 2*time.Minute,
		HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       1,
		Scopes:            hubSourceSyncScopes(queries),
		Handler: func(ctx context.Context, in HandlerInput) (HandlerResult, error) {
			if err := syncer.SyncHubSourceAuto(ctx, in.Scope.ID); err != nil {
				return HandlerResult{}, fmt.Errorf("hub source sync %s: %w", in.Scope.ID, err)
			}
			return HandlerResult{}, nil
		},
	}
}

// hubSourceSyncScopes lists the sources due at the time of the sweep. The due rule
// is the one in the ListDueHubSources query.
func hubSourceSyncScopes(queries *db.Queries) ScopeProvider {
	return func(ctx context.Context, _ time.Time) ([]Scope, error) {
		sources, err := queries.ListDueHubSources(ctx, hubSourceSyncScanLimit)
		if err != nil {
			return nil, fmt.Errorf("hub source sync scope: list due sources: %w", err)
		}
		scopes := make([]Scope, 0, len(sources))
		for _, s := range sources {
			scopes = append(scopes, Scope{Kind: ScopeKindHubSource, ID: uuid.UUID(s.ID.Bytes).String()})
		}
		return scopes, nil
	}
}
