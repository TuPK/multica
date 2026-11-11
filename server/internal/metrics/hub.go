package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Skill and plugin hub metrics. Result and operation labels are closed sets (see
// the Hub* constants); source and item identifiers belong in logs, never here.

// Result values shared by the hub series.
const (
	HubResultOK      = "ok"
	HubResultError   = "error"
	HubResultSkipped = "skipped"
)

// Operations of the Hub's object store, for multica_hub_storage_errors_total.
const (
	HubStorageOpPut    = "put"
	HubStorageOpGet    = "get"
	HubStorageOpDelete = "delete"
)

// hubKinds are the item kinds an import commit can create.
var hubKinds = []string{"skill", "plugin"}

type hubMetrics struct {
	syncDuration     prometheus.Histogram
	syncRuns         *prometheus.CounterVec
	syncItemsChecked prometheus.Counter
	syncUpdatesFound prometheus.Counter
	importCommits    *prometheus.CounterVec
	bundleBuilds     *prometheus.CounterVec
	storageErrors    *prometheus.CounterVec
}

func newHubMetrics() *hubMetrics {
	m := &hubMetrics{
		syncDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "multica", Subsystem: "hub", Name: "sync_duration_seconds",
			Help:    "Duration of one source sync, from its lock to its recorded result: every item of the source checked against its remote.",
			Buckets: prometheus.DefBuckets,
		}),
		syncRuns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "multica", Subsystem: "hub", Name: "sync_runs_total",
			Help: "Source syncs by result: ok when every item was checked, error when at least one was not or the sync could not start.",
		}, metricLabels("multica_hub_sync_runs_total")),
		syncItemsChecked: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "multica", Subsystem: "hub", Name: "sync_items_checked_total",
			Help: "Items checked against their source's remote by source syncs, whether the check succeeded or not.",
		}),
		syncUpdatesFound: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "multica", Subsystem: "hub", Name: "sync_updates_found_total",
			Help: "Items a source sync found with a newer remote version.",
		}),
		importCommits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "multica", Subsystem: "hub", Name: "import_commits_total",
			Help: "Hub import commits by item kind and result: ok when the item was written, skipped when the conflict strategy kept the existing one, error when the commit failed.",
		}, metricLabels("multica_hub_import_commits_total")),
		bundleBuilds: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "multica", Subsystem: "hub", Name: "bundle_builds_total",
			Help: "Hub bundle builds by result: ok when an item's bundle was built and stored, error when it could not be.",
		}, metricLabels("multica_hub_bundle_builds_total")),
		storageErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "multica", Subsystem: "hub", Name: "storage_errors_total",
			Help: "Failed operations on the Hub object store, by operation (put, get, delete).",
		}, metricLabels("multica_hub_storage_errors_total")),
	}
	// Prewarm every series so a series that has not fired since start reads as zero
	// rather than missing, the same as the other business counters.
	for _, result := range []string{HubResultOK, HubResultError} {
		m.syncRuns.WithLabelValues(result).Add(0)
		m.bundleBuilds.WithLabelValues(result).Add(0)
	}
	for _, kind := range hubKinds {
		for _, result := range []string{HubResultOK, HubResultSkipped, HubResultError} {
			m.importCommits.WithLabelValues(kind, result).Add(0)
		}
	}
	for _, op := range []string{HubStorageOpPut, HubStorageOpGet, HubStorageOpDelete} {
		m.storageErrors.WithLabelValues(op).Add(0)
	}
	return m
}

func (m *hubMetrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.syncDuration, m.syncRuns, m.syncItemsChecked, m.syncUpdatesFound,
		m.importCommits, m.bundleBuilds, m.storageErrors,
	}
}

// RecordHubSync records one finished source sync.
func (m *BusinessMetrics) RecordHubSync(result string, duration time.Duration, checked, updates int) {
	if m == nil || m.hub == nil {
		return
	}
	m.hub.syncDuration.Observe(duration.Seconds())
	m.hub.syncRuns.WithLabelValues(result).Inc()
	m.hub.syncItemsChecked.Add(float64(checked))
	m.hub.syncUpdatesFound.Add(float64(updates))
}

// RecordHubImportCommit records one import commit by item kind and result.
func (m *BusinessMetrics) RecordHubImportCommit(kind, result string) {
	if m == nil || m.hub == nil {
		return
	}
	m.hub.importCommits.WithLabelValues(kind, result).Inc()
}

// RecordHubBundleBuild records one bundle build by result.
func (m *BusinessMetrics) RecordHubBundleBuild(result string) {
	if m == nil || m.hub == nil {
		return
	}
	m.hub.bundleBuilds.WithLabelValues(result).Inc()
}

// RecordHubStorageError records one failed operation on the Hub object store.
func (m *BusinessMetrics) RecordHubStorageError(operation string) {
	if m == nil || m.hub == nil {
		return
	}
	m.hub.storageErrors.WithLabelValues(operation).Inc()
}
