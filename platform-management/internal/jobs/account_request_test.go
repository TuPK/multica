package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// Spec 7b.5, rule 7, step 3: the finalize job runs every minute under its own
// name, so the advisory lock keeps two platform instances from running it at once.
func TestAccountRequestFinalizeJobIsRegisteredEveryMinute(t *testing.T) {
	if AccountRequestFinalizeJob != "account_request_finalize" {
		t.Fatalf("job name = %q", AccountRequestFinalizeJob)
	}
	r := New(newFakeLocker())
	var ran atomic.Int32
	if err := r.RegisterAccountRequestFinalize(func(context.Context) error { ran.Add(1); return nil }); err != nil {
		t.Fatalf("register: %v", err)
	}
	if len(r.jobs) != 1 || r.jobs[0].name != "account_request_finalize" || r.jobs[0].every != time.Minute {
		t.Fatalf("jobs = %+v, want account_request_finalize every 1m", r.jobs)
	}
	if err := r.RegisterAccountRequestFinalize(noop); err == nil {
		t.Error("a second registration succeeded")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = r.Run(ctx); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for ran.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if ran.Load() == 0 {
		t.Error("the job did not run at start")
	}
}

func TestAccountRequestFinalizeJobNeedsAFunction(t *testing.T) {
	if err := New(newFakeLocker()).RegisterAccountRequestFinalize(nil); err == nil {
		t.Error("a nil function was registered")
	}
}
