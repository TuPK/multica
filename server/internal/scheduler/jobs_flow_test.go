package scheduler

import (
	"context"
	"testing"
	"time"
)

type fakeFlowAdvancer struct{ calls int }

func (f *fakeFlowAdvancer) SweepFlowRequests(context.Context) (int, error) {
	f.calls++
	return 0, nil
}

// The sweep job is the runner's safety net: a short cadence, a run timeout under the cadence's
// lease, and one global scope. No database is involved.
func TestFlowSweepJobSpec(t *testing.T) {
	advancer := &fakeFlowAdvancer{}
	spec := FlowSweepJob(advancer)

	if spec.Name != "flow_sweep" || spec.Cadence != 30*time.Second {
		t.Fatalf("name %q, cadence %v; want flow_sweep every 30s", spec.Name, spec.Cadence)
	}
	if spec.RunTimeout != 45*time.Second || spec.StaleTimeout != time.Minute || spec.HeartbeatInterval != 10*time.Second {
		t.Fatalf("timeouts = %v / %v / %v", spec.RunTimeout, spec.StaleTimeout, spec.HeartbeatInterval)
	}
	if spec.CatchUpMode != CatchUpLatestOnly || !spec.AllowStaleReentry || spec.MaxAttempts != 1 {
		t.Fatalf("catch-up %v, stale re-entry %v, attempts %d", spec.CatchUpMode, spec.AllowStaleReentry, spec.MaxAttempts)
	}
	if err := spec.validate(); err != nil {
		t.Fatalf("the spec is not valid: %v", err)
	}
	if _, err := spec.Handler(context.Background(), HandlerInput{}); err != nil || advancer.calls != 1 {
		t.Fatalf("the handler did not run one sweep: calls %d, err %v", advancer.calls, err)
	}
}
