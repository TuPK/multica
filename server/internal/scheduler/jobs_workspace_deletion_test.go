package scheduler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// fakeWorkspaceDeleter stands in for the handler: it holds workspaces and
// applies the same pending-and-due rule as the SQL.
type fakeWorkspaceDeleter struct {
	scheduled map[string]time.Time
	order     []string
	failFor   map[string]bool
	listLimit int
	calls     []string
}

func (f *fakeWorkspaceDeleter) DueWorkspaceDeletions(_ context.Context, now time.Time, limit int) ([]string, error) {
	f.listLimit = limit
	var out []string
	for _, id := range f.order {
		at, pending := f.scheduled[id]
		if pending && !at.After(now) {
			out = append(out, id)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeWorkspaceDeleter) DeleteDueWorkspace(_ context.Context, id string, _ time.Time) (bool, error) {
	f.calls = append(f.calls, id)
	if f.failFor[id] {
		return false, errors.New("boom")
	}
	if _, pending := f.scheduled[id]; !pending {
		return false, nil
	}
	delete(f.scheduled, id)
	return true, nil
}

// fakeInvalidator records the workspaces whose cached state was dropped.
type fakeInvalidator struct {
	invalidated []string
}

func (f *fakeInvalidator) Invalidate(_ context.Context, id string) error {
	f.invalidated = append(f.invalidated, id)
	return nil
}

// listedThenSkipped lists three workspaces. One is deleted, one is skipped
// because it is no longer pending by the time the delete runs, and one fails.
type listedThenSkipped struct{}

func (listedThenSkipped) DueWorkspaceDeletions(context.Context, time.Time, int) ([]string, error) {
	return []string{"skipped", "failed", "deleted"}, nil
}

func (listedThenSkipped) DeleteDueWorkspace(_ context.Context, id string, _ time.Time) (bool, error) {
	switch id {
	case "deleted":
		return true, nil
	case "failed":
		return false, errors.New("boom")
	}
	return false, nil
}

func TestRunWorkspaceDeletionsInvalidatesTheStateOnlyAfterADelete(t *testing.T) {
	inv := &fakeInvalidator{}
	n, err := runWorkspaceDeletions(context.Background(), listedThenSkipped{}, inv, time.Now())
	if err != nil || n != 1 {
		t.Fatalf("deleted = %d, %v; want 1, nil", n, err)
	}
	if len(inv.invalidated) != 1 || inv.invalidated[0] != "deleted" {
		t.Fatalf("invalidated = %v, want only [deleted]: a skipped or failed workspace keeps its cached state", inv.invalidated)
	}
}

func TestWorkspaceDeletionJobIdentity(t *testing.T) {
	spec := WorkspaceDeletionJob(&fakeWorkspaceDeleter{}, &fakeInvalidator{})
	if spec.Name != "workspace_deletion" {
		t.Fatalf("name = %q", spec.Name)
	}
	if spec.Cadence != time.Hour {
		t.Fatalf("cadence = %v, want 1h", spec.Cadence)
	}
	scopes, err := spec.Scopes(context.Background(), time.Now())
	if err != nil || len(scopes) != 1 || scopes[0] != ScopeGlobal {
		t.Fatalf("scopes = %v, %v", scopes, err)
	}
}

func TestRunWorkspaceDeletionsDeletesOnlyDueWorkspaces(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	f := &fakeWorkspaceDeleter{scheduled: map[string]time.Time{}}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("due%d", i)
		f.order = append(f.order, id)
		f.scheduled[id] = now.Add(-time.Hour)
	}
	f.order = append(f.order, "future")
	f.scheduled["future"] = now.Add(time.Hour)

	n, err := runWorkspaceDeletions(context.Background(), f, &fakeInvalidator{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("deleted = %d, want 3", n)
	}
	if _, ok := f.scheduled["future"]; !ok {
		t.Fatal("a workspace that is not due was deleted")
	}
	if f.listLimit != workspaceDeletionBatch {
		t.Fatalf("list limit = %d, want %d", f.listLimit, workspaceDeletionBatch)
	}
}

func TestRunWorkspaceDeletionsSkipsAFailedWorkspaceAndRetriesItNextRun(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	f := &fakeWorkspaceDeleter{
		scheduled: map[string]time.Time{"bad": now.Add(-time.Hour), "good": now.Add(-time.Hour)},
		order:     []string{"bad", "good"},
		failFor:   map[string]bool{"bad": true},
	}
	n, err := runWorkspaceDeletions(context.Background(), f, &fakeInvalidator{}, now)
	if err != nil {
		t.Fatalf("a failed workspace must not fail the run: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	if _, ok := f.scheduled["bad"]; !ok {
		t.Fatal("the failed workspace is no longer pending")
	}

	f.failFor = nil
	f.calls = nil
	n, err = runWorkspaceDeletions(context.Background(), f, &fakeInvalidator{}, now)
	if err != nil || n != 1 || len(f.calls) != 1 || f.calls[0] != "bad" {
		t.Fatalf("second run = %d, %v, calls %v; want it to retry only the failed workspace", n, err, f.calls)
	}
}

func TestRunWorkspaceDeletionsStopsWhenTheContextEnds(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	f := &fakeWorkspaceDeleter{
		scheduled: map[string]time.Time{"a": now, "b": now},
		order:     []string{"a", "b"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n, err := runWorkspaceDeletions(ctx, f, &fakeInvalidator{}, now)
	if err != nil || n != 0 || len(f.calls) != 0 {
		t.Fatalf("run on a cancelled context = %d, %v, calls %v", n, err, f.calls)
	}
}
