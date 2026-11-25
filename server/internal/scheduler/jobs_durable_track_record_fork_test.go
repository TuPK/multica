package scheduler

import (
	"context"
	"testing"
	"time"
)

type fakeTrackRecordRunner struct{ calls int }

func (f *fakeTrackRecordRunner) RecomputeAll(context.Context) (int, error) {
	f.calls++
	return 0, nil
}

// TestDurableTrackRecordJobIsDaily: the track record job runs once a day and does one
// RecomputeAll per run.
func TestDurableTrackRecordJobIsDaily(t *testing.T) {
	runner := &fakeTrackRecordRunner{}
	spec := DurableTrackRecordJob(runner)
	if spec.Name != "durable_track_record" {
		t.Fatalf("job name = %q", spec.Name)
	}
	if spec.Cadence != 24*time.Hour {
		t.Fatalf("cadence = %s, want 24h", spec.Cadence)
	}
	if _, err := spec.Handler(context.Background(), HandlerInput{}); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("RecomputeAll calls = %d, want 1", runner.calls)
	}
}
