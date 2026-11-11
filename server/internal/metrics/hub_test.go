package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// hubSeriesCount returns how many series of one family the registry exposes.
func hubSeriesCount(t *testing.T, reg prometheus.Gatherer, name string) int {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() == name {
			return len(family.GetMetric())
		}
	}
	return 0
}

func TestHubMetricsRegistryExposesEveryFamilyPrewarmed(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewBusinessMetrics().Collectors()...)

	want := map[string]int{
		"multica_hub_sync_duration_seconds":    1,
		"multica_hub_sync_runs_total":          2,
		"multica_hub_sync_items_checked_total": 1,
		"multica_hub_sync_updates_found_total": 1,
		"multica_hub_import_commits_total":     6,
		"multica_hub_bundle_builds_total":      2,
		"multica_hub_storage_errors_total":     3,
	}
	for name, series := range want {
		if got := hubSeriesCount(t, reg, name); got != series {
			t.Errorf("%s exposes %d series, want %d", name, got, series)
		}
	}
}

func TestHubMetricsRecordAndLabelValues(t *testing.T) {
	m := NewBusinessMetrics()
	reg := prometheus.NewRegistry()
	reg.MustRegister(m.Collectors()...)

	m.RecordHubSync(HubResultOK, 250*time.Millisecond, 4, 1)
	m.RecordHubSync(HubResultError, time.Second, 2, 0)
	m.RecordHubImportCommit("skill", HubResultOK)
	m.RecordHubImportCommit("plugin", HubResultSkipped)
	m.RecordHubBundleBuild(HubResultError)
	m.RecordHubStorageError(HubStorageOpGet)

	if got := testutil.ToFloat64(m.hub.syncRuns.WithLabelValues(HubResultOK)); got != 1 {
		t.Errorf("sync runs ok = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.hub.syncRuns.WithLabelValues(HubResultError)); got != 1 {
		t.Errorf("sync runs error = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.hub.syncItemsChecked); got != 6 {
		t.Errorf("items checked = %v, want 6", got)
	}
	if got := testutil.ToFloat64(m.hub.syncUpdatesFound); got != 1 {
		t.Errorf("updates found = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.hub.importCommits.WithLabelValues("plugin", HubResultSkipped)); got != 1 {
		t.Errorf("plugin skipped commits = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.hub.bundleBuilds.WithLabelValues(HubResultError)); got != 1 {
		t.Errorf("bundle build errors = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.hub.storageErrors.WithLabelValues(HubStorageOpGet)); got != 1 {
		t.Errorf("storage get errors = %v, want 1", got)
	}
}

func TestHubMetricsNilSafe(t *testing.T) {
	var m *BusinessMetrics
	m.RecordHubSync(HubResultOK, time.Second, 1, 1)
	m.RecordHubImportCommit("skill", HubResultOK)
	m.RecordHubBundleBuild(HubResultOK)
	m.RecordHubStorageError(HubStorageOpPut)
}
