package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/storage"
)

// Hub staging garbage collection. A preview stages its zip and metadata under
// staging/{workspace}/ until it is committed or its TTL passes, and nothing in the
// request path removes a preview that is never committed. This sweep removes those
// leftovers. It complements the bucket lifecycle rule on the staging/ prefix
// (docs/engineering/runbooks/hub-object-storage.md) and does not replace it: the
// rule works without a server, and this sweep also covers local disk, where no
// rule exists.

const (
	// JobNameHubStagingGC is persisted in sys_cron_executions and must stay stable.
	JobNameHubStagingGC = "hub_staging_gc"

	// HubStagingPrefix is the key prefix of staged previews. Nothing outside it is swept.
	HubStagingPrefix = "staging/"

	// HubStagingMaxAge is how old a staged object must be before the sweep deletes
	// it: the 30 minute preview TTL plus a margin for a commit that started just
	// before its preview expired.
	HubStagingMaxAge = 60 * time.Minute

	hubStagingGCRunTimeout = 10 * time.Minute
)

// SweepHubStaging deletes the staged objects older than maxAge at now. It returns
// how many it deleted. A store without listing has nothing it can sweep, so the
// call does nothing and returns zero. Deleting is idempotent, so a sweep that
// overlaps another instance's sweep is harmless.
func SweepHubStaging(ctx context.Context, store storage.Storage, now time.Time, maxAge time.Duration) (int, error) {
	lister, ok := store.(storage.Lister)
	if !ok {
		return 0, nil
	}
	objects, err := lister.ListPrefix(ctx, HubStagingPrefix)
	if err != nil {
		return 0, fmt.Errorf("list hub staging: %w", err)
	}
	deleted := 0
	var errs []error
	for _, key := range expiredStagingKeys(objects, now, maxAge) {
		if err := store.DeleteObject(ctx, key); err != nil {
			errs = append(errs, fmt.Errorf("delete %s: %w", key, err))
			continue
		}
		deleted++
	}
	return deleted, errors.Join(errs...)
}

// expiredStagingKeys selects the staged keys whose last modification is before
// now-maxAge. An object modified exactly at the cutoff is kept. Keys outside the
// staging prefix are never selected.
func expiredStagingKeys(objects []storage.ObjectInfo, now time.Time, maxAge time.Duration) []string {
	cutoff := now.Add(-maxAge)
	var keys []string
	for _, o := range objects {
		if !strings.HasPrefix(o.Key, HubStagingPrefix) {
			continue
		}
		if !o.LastModified.Before(cutoff) {
			continue
		}
		keys = append(keys, o.Key)
	}
	return keys
}

// HubStagingGCJob sweeps expired previews from the Hub store once an hour. The
// scope is global, so one instance holds the lease for each run; a second
// instance skips the run instead of sweeping in parallel.
func HubStagingGCJob(store storage.Storage) JobSpec {
	return JobSpec{
		Name:              JobNameHubStagingGC,
		Cadence:           time.Hour,
		CatchUpMode:       CatchUpLatestOnly,
		RunTimeout:        hubStagingGCRunTimeout,
		StaleTimeout:      hubStagingGCRunTimeout + 5*time.Minute,
		HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       3,
		RetryBackoff:      []time.Duration{time.Minute, 5 * time.Minute},
		Scopes:            StaticScopes(ScopeGlobal),
		Handler: func(ctx context.Context, _ HandlerInput) (HandlerResult, error) {
			n, err := SweepHubStaging(ctx, store, time.Now(), HubStagingMaxAge)
			return HandlerResult{RowsAffected: int64(n)}, err
		},
	}
}
