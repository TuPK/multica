package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/legalhold"
)

// heldWorkspaceDeleter refuses the held workspaces with the hold error, as the
// handler does; the others delete as usual.
type heldWorkspaceDeleter struct {
	fakeWorkspaceDeleter
	held map[string]bool
}

func (h *heldWorkspaceDeleter) DeleteDueWorkspace(ctx context.Context, id string, now time.Time) (bool, error) {
	if h.held[id] {
		h.calls = append(h.calls, id)
		return false, &legalhold.Blocked{HoldID: "hold-" + id}
	}
	return h.fakeWorkspaceDeleter.DeleteDueWorkspace(ctx, id, now)
}

// The workspace deletion job skips a held workspace without an error, leaves it
// pending, and still deletes the others.
func TestWorkspaceDeletionJobSkipsHeldWorkspace(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	f := &heldWorkspaceDeleter{
		fakeWorkspaceDeleter: fakeWorkspaceDeleter{scheduled: map[string]time.Time{}},
		held:                 map[string]bool{"held": true},
	}
	for _, id := range []string{"held", "free"} {
		f.order = append(f.order, id)
		f.scheduled[id] = now.Add(-time.Hour)
	}

	n, err := runWorkspaceDeletions(context.Background(), f, &fakeInvalidator{}, now)
	if err != nil || n != 1 {
		t.Fatalf("runWorkspaceDeletions = %d, %v; want 1, nil", n, err)
	}
	if _, pending := f.scheduled["held"]; !pending {
		t.Fatal("the held workspace left the schedule")
	}
	if _, pending := f.scheduled["free"]; pending {
		t.Fatal("the free workspace was not deleted")
	}
}
