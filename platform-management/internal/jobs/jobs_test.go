package jobs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/testdb"
)

// fakeLocker grants a name to one holder at a time, like advisory locks.
type fakeLocker struct {
	mu       sync.Mutex
	held     map[string]bool
	err      error
	released atomic.Int32
}

func newFakeLocker() *fakeLocker { return &fakeLocker{held: map[string]bool{}} }

func (l *fakeLocker) TryLock(_ context.Context, name string) (func(), bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return nil, false, l.err
	}
	if l.held[name] {
		return nil, false, nil
	}
	l.held[name] = true
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		delete(l.held, name)
		l.released.Add(1)
	}, true, nil
}

func noop(context.Context) error { return nil }

func TestRegisterChecksTheJobName(t *testing.T) {
	valid := []string{"a", "usage-rollup", "billing.notice_sweep", "job2"}
	invalid := []string{"", " ", "Upper", "2job", "has space", "-lead", strings.Repeat("a", 64), "semi;colon"}
	for _, name := range valid {
		if err := New(newFakeLocker()).Register(name, time.Minute, noop); err != nil {
			t.Errorf("Register(%q): %v", name, err)
		}
	}
	for _, name := range invalid {
		if err := New(newFakeLocker()).Register(name, time.Minute, noop); err == nil {
			t.Errorf("Register(%q) succeeded", name)
		}
	}
}

func TestRegisterRejectsBadArguments(t *testing.T) {
	r := New(newFakeLocker())
	if err := r.Register("a", 0, noop); err == nil {
		t.Error("zero interval accepted")
	}
	if err := r.Register("a", -time.Second, noop); err == nil {
		t.Error("negative interval accepted")
	}
	if err := r.Register("a", time.Minute, nil); err == nil {
		t.Error("nil function accepted")
	}
	if err := r.Register("a", time.Minute, noop); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("a", time.Minute, noop); err == nil {
		t.Error("duplicate name accepted")
	}
}

func TestRunOnceRunsUnderTheLockAndReleasesIt(t *testing.T) {
	l := newFakeLocker()
	r := New(l)
	var ran int
	var heldDuring bool
	if err := r.Register("a", time.Minute, func(context.Context) error {
		ran++
		heldDuring = l.held["a"]
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r.runOnce(context.Background(), r.jobs[0])
	if ran != 1 || !heldDuring || l.held["a"] || l.released.Load() != 1 {
		t.Fatalf("ran = %d, heldDuring = %v, stillHeld = %v, released = %d", ran, heldDuring, l.held["a"], l.released.Load())
	}
}

func TestRunOnceSkipsWhenAnotherRunnerHoldsTheLock(t *testing.T) {
	l := newFakeLocker()
	a, b := New(l), New(l)
	entered := make(chan struct{})
	finish := make(chan struct{})
	var aRan, bRan atomic.Int32
	_ = a.Register("same", time.Minute, func(context.Context) error {
		aRan.Add(1)
		close(entered)
		<-finish
		return nil
	})
	_ = b.Register("same", time.Minute, func(context.Context) error { bRan.Add(1); return nil })

	done := make(chan struct{})
	go func() { a.runOnce(context.Background(), a.jobs[0]); close(done) }()
	<-entered
	b.runOnce(context.Background(), b.jobs[0])
	if bRan.Load() != 0 {
		t.Fatal("second runner ran the job while the first held the lock")
	}
	close(finish)
	<-done
	b.runOnce(context.Background(), b.jobs[0])
	if aRan.Load() != 1 || bRan.Load() != 1 {
		t.Fatalf("runs: a = %d, b = %d", aRan.Load(), bRan.Load())
	}
}

func TestRunOnceKeepsGoingAfterAFailure(t *testing.T) {
	for name, fn := range map[string]func(context.Context) error{
		"error": func(context.Context) error { return errors.New("boom") },
		"panic": func(context.Context) error { panic("boom") },
	} {
		l := newFakeLocker()
		r := New(l)
		_ = r.Register("a", time.Minute, fn)
		r.runOnce(context.Background(), r.jobs[0])
		if l.held["a"] || l.released.Load() != 1 {
			t.Errorf("%s: lock not released", name)
		}
	}
}

func TestRunOnceSkipsWhenTheLockCannotBeTaken(t *testing.T) {
	l := newFakeLocker()
	l.err = errors.New("db down")
	r := New(l)
	var ran atomic.Int32
	_ = r.Register("a", time.Minute, func(context.Context) error { ran.Add(1); return nil })
	r.runOnce(context.Background(), r.jobs[0])
	if ran.Load() != 0 {
		t.Fatal("job ran without the lock")
	}
}

func TestRunSchedulesEveryJobUntilTheContextEnds(t *testing.T) {
	r := New(newFakeLocker())
	var fast, slow atomic.Int32
	_ = r.Register("fast", 5*time.Millisecond, func(context.Context) error { fast.Add(1); return nil })
	_ = r.Register("slow", time.Hour, func(context.Context) error { slow.Add(1); return nil })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for fast.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the context ended")
	}
	if fast.Load() < 3 {
		t.Fatalf("fast job ran %d times", fast.Load())
	}
	if slow.Load() != 1 {
		t.Fatalf("slow job ran %d times, want the one start-up run", slow.Load())
	}
}

func TestRunWithNoJobsWaitsForTheContext(t *testing.T) {
	r := New(newFakeLocker())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	select {
	case <-done:
		t.Fatal("Run returned before the context ended")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRegisterAfterRunStartsIsRejected(t *testing.T) {
	r := New(newFakeLocker())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = r.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := r.Register("late", time.Minute, noop); err != nil {
			return
		}
		r.mu.Lock()
		r.jobs = nil // undo, so the next attempt tests the same thing
		r.names = map[string]bool{}
		r.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Register succeeded after Run started")
}

// Database test: skipped unless PLATFORM_TEST_DATABASE_URL names a *_test
// database and DATABASE_URL equals it.
func TestAdvisoryLockAllowsOneRunnerPerName(t *testing.T) {
	pool := testdb.Open(t)
	a, b := New(NewPoolLocker(pool)), New(NewPoolLocker(pool))
	name := "platform-lock-test"

	entered := make(chan struct{})
	finish := make(chan struct{})
	var aRan, bRan, otherRan atomic.Int32
	if err := a.Register(name, time.Minute, func(context.Context) error {
		aRan.Add(1)
		close(entered)
		<-finish
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = b.Register(name, time.Minute, func(context.Context) error { bRan.Add(1); return nil })
	_ = b.Register(name+"-other", time.Minute, func(context.Context) error { otherRan.Add(1); return nil })

	done := make(chan struct{})
	go func() { a.runOnce(context.Background(), a.jobs[0]); close(done) }()
	<-entered

	b.runOnce(context.Background(), b.jobs[0])
	if bRan.Load() != 0 {
		t.Fatal("two runners ran the same job at once")
	}
	b.runOnce(context.Background(), b.jobs[1])
	if otherRan.Load() != 1 {
		t.Fatal("a different job name was blocked")
	}

	close(finish)
	<-done
	b.runOnce(context.Background(), b.jobs[0])
	if aRan.Load() != 1 || bRan.Load() != 1 {
		t.Fatalf("after release: a = %d, b = %d", aRan.Load(), bRan.Load())
	}
}
