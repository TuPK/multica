package scheduler

import (
	"context"
	"testing"
)

type fakeHubSourceSyncer struct{}

func (fakeHubSourceSyncer) SyncHubSourceAuto(context.Context, string) error { return nil }

// The job's timing must satisfy the manager's invariants (run < stale, heartbeat <
// stale), or Register refuses it at startup. The due-source query itself is tested
// against the database in the handler package.
func TestHubSourceSyncJobRegisters(t *testing.T) {
	m := NewManager(nil, Options{})
	if err := m.Register(HubSourceSyncJob(nil, fakeHubSourceSyncer{})); err != nil {
		t.Fatalf("register hub source sync: %v", err)
	}
	if err := m.Register(HubSourceSyncJob(nil, fakeHubSourceSyncer{})); err == nil {
		t.Fatal("a second registration of the same job name must be refused")
	}
}
