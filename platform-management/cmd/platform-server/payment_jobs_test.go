package main

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/invoice"
	"github.com/multica-ai/multica/platform-management/internal/jobs"
)

// recordingLocker records each name it is asked for and never grants the lock,
// so no job body runs.
type recordingLocker struct {
	mu    sync.Mutex
	names []string
}

func (l *recordingLocker) TryLock(_ context.Context, name string) (func(), bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.names = append(l.names, name)
	return nil, false, nil
}

func registerForTest(runner *jobs.Runner) error {
	return registerPaymentJobs(runner, billing.NewReconciler(nil, nil), invoice.NewService(nil, nil, invoice.Config{}))
}

func TestPaymentJobsAreRegistered(t *testing.T) {
	locker := &recordingLocker{}
	runner := jobs.New(locker)
	if err := registerForTest(runner); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = runner.Run(ctx); close(done) }()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		locker.mu.Lock()
		n := len(locker.names)
		locker.mu.Unlock()
		if n >= 3 {
			break
		}
	}
	cancel()
	<-done

	locker.mu.Lock()
	got := append([]string(nil), locker.names...)
	locker.mu.Unlock()
	sort.Strings(got)
	want := []string{"einvoice_enqueue", "einvoice_issue", "payment_notice_redrive"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs asked for the lock = %v, want %v", got, want)
	}
}

func TestPaymentJobsRejectADuplicateName(t *testing.T) {
	runner := jobs.New(&recordingLocker{})
	if err := registerForTest(runner); err != nil {
		t.Fatal(err)
	}
	if err := registerForTest(runner); err == nil {
		t.Fatal("second registration succeeded")
	}
}

func TestPaymentJobIntervals(t *testing.T) {
	if paymentNoticeRedriveJob != "payment_notice_redrive" {
		t.Errorf("redrive job name = %q", paymentNoticeRedriveJob)
	}
	if paymentNoticeRedriveEvery != 5*time.Minute {
		t.Errorf("redrive interval = %v, want 5m", paymentNoticeRedriveEvery)
	}
	if jobs.EinvoiceEvery != time.Minute {
		t.Errorf("einvoice interval = %v", jobs.EinvoiceEvery)
	}
}
