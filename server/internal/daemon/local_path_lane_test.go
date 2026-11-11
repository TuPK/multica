package daemon

import (
	"context"
	"errors"
	"testing"
	"time"
)

// acquireWithin acquires or fails the test if the acquire does not return in time.
func acquireWithin(t *testing.T, locker *LocalPathLocker, path, lane, taskID string) (func(), error) {
	t.Helper()
	type result struct {
		release func()
		err     error
	}
	done := make(chan result, 1)
	go func() {
		rel, err := locker.Acquire(context.Background(), path, lane, taskID, nil)
		done <- result{rel, err}
	}()
	select {
	case r := <-done:
		return r.release, r.err
	case <-time.After(2 * time.Second):
		t.Fatalf("acquire(lane=%q, task=%s) blocked", lane, taskID)
		return nil, nil
	}
}

// waitsBehind reports whether an acquire is blocked (it does not return within a
// short window). The blocked acquire is cancelled so it leaves no state behind.
func waitsBehind(locker *LocalPathLocker, path, lane, taskID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := locker.Acquire(ctx, path, lane, taskID, nil)
	return errors.Is(err, context.DeadlineExceeded)
}

func TestLocalPathLockerLanesRunSideBySide(t *testing.T) {
	t.Parallel()
	locker := NewLocalPathLocker()
	const path = "/repo"

	relA, err := acquireWithin(t, locker, path, "squad-a", "task-a1")
	if err != nil {
		t.Fatalf("lane a: %v", err)
	}
	defer relA()
	relB, err := acquireWithin(t, locker, path, "squad-b", "task-b1")
	if err != nil {
		t.Fatalf("lane b must run beside lane a: %v", err)
	}
	defer relB()
}

func TestLocalPathLockerSameLaneStaysSerial(t *testing.T) {
	t.Parallel()
	locker := NewLocalPathLocker()
	const path = "/repo"

	rel, err := acquireWithin(t, locker, path, "squad-a", "task-a1")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !waitsBehind(locker, path, "squad-a", "task-a2") {
		t.Fatal("second run of the same room must wait for the first")
	}
	rel()
	rel2, err := acquireWithin(t, locker, path, "squad-a", "task-a2")
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	rel2()
}

func TestLocalPathLockerLaneless_ExcludesEveryLane(t *testing.T) {
	t.Parallel()
	locker := NewLocalPathLocker()
	const path = "/repo"

	relLane, err := acquireWithin(t, locker, path, "squad-a", "task-a1")
	if err != nil {
		t.Fatalf("lane: %v", err)
	}
	if !waitsBehind(locker, path, "", "task-free") {
		t.Fatal("a run with no room must wait while a room holds the path")
	}
	relLane()

	relFree, err := acquireWithin(t, locker, path, "", "task-free")
	if err != nil {
		t.Fatalf("laneless alone: %v", err)
	}
	if !waitsBehind(locker, path, "squad-b", "task-b1") {
		t.Fatal("a room must wait while a run with no room holds the path")
	}
	relFree()
}

func TestLocalPathLockerLanelessWaiterBlocksNewLanes(t *testing.T) {
	t.Parallel()
	locker := NewLocalPathLocker()
	const path = "/repo"

	relLane, err := acquireWithin(t, locker, path, "squad-a", "task-a1")
	if err != nil {
		t.Fatalf("lane: %v", err)
	}
	// A laneless run queues behind squad-a's run.
	laneless := make(chan func(), 1)
	go func() {
		rel, err := locker.Acquire(context.Background(), path, "", "task-free", nil)
		if err == nil {
			laneless <- rel
		}
	}()
	time.Sleep(30 * time.Millisecond)

	// A new room arriving now must not overtake the laneless run.
	if !waitsBehind(locker, path, "squad-b", "task-b1") {
		t.Fatal("a new room must not overtake a laneless run that is waiting")
	}
	relLane()
	select {
	case rel := <-laneless:
		rel()
	case <-time.After(2 * time.Second):
		t.Fatal("laneless run never acquired after the room released")
	}
}

func TestLocalPathLockerCancelledLanelessWaiterFreesLanes(t *testing.T) {
	t.Parallel()
	locker := NewLocalPathLocker()
	const path = "/repo"

	relLane, err := acquireWithin(t, locker, path, "squad-a", "task-a1")
	if err != nil {
		t.Fatalf("lane: %v", err)
	}
	defer relLane()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := locker.Acquire(ctx, path, "", "task-free", nil)
		errCh <- err
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}

	// The cancelled laneless waiter held nothing and blocks nobody new.
	relB, err := acquireWithin(t, locker, path, "squad-b", "task-b1")
	if err != nil {
		t.Fatalf("room after cancelled waiter: %v", err)
	}
	relB()
}

func TestLocalPathLockerReleaseIsIdempotent(t *testing.T) {
	t.Parallel()
	locker := NewLocalPathLocker()
	const path = "/repo"

	rel, err := acquireWithin(t, locker, path, "squad-a", "task-a1")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	rel()
	rel()
	if got := locker.Holder(path); got != "" {
		t.Fatalf("holder after double release = %q, want empty", got)
	}
}
