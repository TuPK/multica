package handler

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
)

// hubMetricValue reads one series of a family from a registry. A missing series is zero.
func hubMetricValue(t *testing.T, reg prometheus.Gatherer, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, m := range family.GetMetric() {
			if labelsMatch(m, labels) {
				return counterOrCount(m)
			}
		}
	}
	return 0
}

func labelsMatch(m *dto.Metric, want map[string]string) bool {
	got := map[string]string{}
	for _, pair := range m.GetLabel() {
		got[pair.GetName()] = pair.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

func counterOrCount(m *dto.Metric) float64 {
	if m.GetCounter() != nil {
		return m.GetCounter().GetValue()
	}
	return 0
}

// A source sync and an import commit each move their hub series.
func TestHubMetricsFollowSyncAndImportCommit(t *testing.T) {
	setupHubImportTest(t)
	reg := prometheus.NewRegistry()
	m := obsmetrics.NewBusinessMetrics()
	reg.MustRegister(m.Collectors()...)
	prev := testHandler.Metrics
	testHandler.Metrics = m
	t.Cleanup(func() { testHandler.Metrics = prev })

	f := hubSyncWorkspace(t)
	owner := f.users[hubRoleOwner]
	remote := newHubSyncRemote(t)
	remote.set("/skill.zip", hubSyncSkillZip(t, "metered-skill", "Counts syncs.", "Body."))
	sourceURL := remote.url("/skill.zip")

	commitsBefore := hubMetricValue(t, reg, "multica_hub_import_commits_total", map[string]string{"kind": "skill", "result": "ok"})
	importHubSkillForSync(t, f.ws, owner, sourceURL)
	if got := hubMetricValue(t, reg, "multica_hub_import_commits_total", map[string]string{"kind": "skill", "result": "ok"}); got != commitsBefore+1 {
		t.Fatalf("skill ok commits = %v, want %v", got, commitsBefore+1)
	}

	runsBefore := hubMetricValue(t, reg, "multica_hub_sync_runs_total", map[string]string{"result": "ok"})
	checkedBefore := hubMetricValue(t, reg, "multica_hub_sync_items_checked_total", nil)
	syncHubSourceForTest(t, f.ws, owner, hubSyncSourceID(t, f.ws, sourceURL)).Want(200)
	if got := hubMetricValue(t, reg, "multica_hub_sync_runs_total", map[string]string{"result": "ok"}); got != runsBefore+1 {
		t.Fatalf("ok sync runs = %v, want %v", got, runsBefore+1)
	}
	if got := hubMetricValue(t, reg, "multica_hub_sync_items_checked_total", nil); got != checkedBefore+1 {
		t.Fatalf("items checked = %v, want %v", got, checkedBefore+1)
	}
}
