package scheduler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/accountdeletion"
)

// fakeDeleter stands in for the handler: it holds accounts and applies the
// same pending-and-due rule as the SQL.
type fakeDeleter struct {
	pending   []string
	scheduled map[string]time.Time
	failFor   map[string]bool
	listLimit int
	calls     []string
}

func (f *fakeDeleter) DueAccountDeletions(_ context.Context, now time.Time, limit int) ([]string, error) {
	f.listLimit = limit
	var out []string
	for _, id := range f.pending {
		at := f.scheduled[id]
		if accountdeletion.Due(accountdeletion.StatusPending, &at, now) {
			out = append(out, id)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeDeleter) DeleteAccount(_ context.Context, id string, _ time.Time) (bool, error) {
	f.calls = append(f.calls, id)
	if f.failFor[id] {
		return false, errors.New("boom")
	}
	for i, p := range f.pending {
		if p == id {
			f.pending = append(f.pending[:i], f.pending[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func TestAccountDeletionJobIdentity(t *testing.T) {
	spec := AccountDeletionJob(&fakeDeleter{})
	if spec.Name != "account_deletion" {
		t.Fatalf("name = %q", spec.Name)
	}
	if spec.Cadence != 24*time.Hour {
		t.Fatalf("cadence = %v", spec.Cadence)
	}
	scopes, err := spec.Scopes(context.Background(), time.Now())
	if err != nil || len(scopes) != 1 || scopes[0] != ScopeGlobal {
		t.Fatalf("scopes = %v, %v", scopes, err)
	}
}

func TestRunAccountDeletionsSelectsOnlyDueAccountsInABatch(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	f := &fakeDeleter{scheduled: map[string]time.Time{}}
	for i := 0; i < 60; i++ {
		id := fmt.Sprintf("u%02d", i)
		f.pending = append(f.pending, id)
		f.scheduled[id] = now.Add(-time.Hour)
	}
	f.pending = append(f.pending, "future")
	f.scheduled["future"] = now.Add(time.Hour)

	n, err := runAccountDeletions(context.Background(), f, now)
	if err != nil {
		t.Fatal(err)
	}
	if f.listLimit != 50 || n != 50 || len(f.calls) != 50 {
		t.Fatalf("limit=%d deleted=%d calls=%d, want 50 each", f.listLimit, n, len(f.calls))
	}
	for _, id := range f.calls {
		if id == "future" {
			t.Fatal("an account that is not due was processed")
		}
	}

	// The remaining ten are picked up by the next run; the future one is not.
	n, err = runAccountDeletions(context.Background(), f, now)
	if err != nil || n != 10 {
		t.Fatalf("second run deleted %d (%v), want 10", n, err)
	}
	n, err = runAccountDeletions(context.Background(), f, now)
	if err != nil || n != 0 {
		t.Fatalf("third run deleted %d (%v), want 0: a repeat run must change nothing", n, err)
	}
}

func TestRunAccountDeletionsSkipsAFailureAndKeepsGoing(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	f := &fakeDeleter{
		pending:   []string{"a", "bad", "c"},
		scheduled: map[string]time.Time{"a": now, "bad": now, "c": now},
		failFor:   map[string]bool{"bad": true},
	}
	n, err := runAccountDeletions(context.Background(), f, now)
	if err != nil {
		t.Fatalf("a failed account must not fail the run: %v", err)
	}
	if n != 2 || len(f.pending) != 1 || f.pending[0] != "bad" {
		t.Fatalf("deleted=%d pending=%v, want 2 and [bad]", n, f.pending)
	}
}

func TestRunAccountDeletionsStopsWhenCanceled(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	f := &fakeDeleter{pending: []string{"a", "b"}, scheduled: map[string]time.Time{"a": now, "b": now}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n, err := runAccountDeletions(ctx, f, now)
	if err != nil || n != 0 || len(f.calls) != 0 {
		t.Fatalf("deleted=%d calls=%v err=%v", n, f.calls, err)
	}
}
