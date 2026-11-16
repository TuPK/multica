package workspaceexport

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeStore applies the same conditional updates as the SQL: a claim moves
// queued to running once, and a finish or failure applies only to a running row.
type fakeStore struct {
	mu   sync.Mutex
	rows []*Export
}

func (s *fakeStore) get(id string) *Export {
	for _, r := range s.rows {
		if r.ID == id {
			return r
		}
	}
	return nil
}

func (s *fakeStore) ClaimNext(_ context.Context, now time.Time) (Export, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rows {
		if r.Status == StatusQueued {
			r.Status = StatusRunning
			r.StartedAt = &now
			return *r, true, nil
		}
	}
	return Export{}, false, nil
}

func (s *fakeStore) MarkDone(_ context.Context, id string, finished, expires time.Time, key string, size int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.get(id)
	if r == nil || r.Status != StatusRunning {
		return false, nil
	}
	r.Status, r.StorageKey, r.SizeBytes, r.FinishedAt, r.ExpiresAt = StatusDone, key, size, &finished, &expires
	return true, nil
}

func (s *fakeStore) MarkFailed(_ context.Context, id string, finished time.Time, msg string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.get(id)
	if r == nil || r.Status != StatusRunning {
		return false, nil
	}
	r.Status, r.Error, r.FinishedAt = StatusFailed, msg, &finished
	return true, nil
}

func (s *fakeStore) FailStale(_ context.Context, startedBefore, now time.Time, msg string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.rows {
		if r.Status == StatusRunning && r.StartedAt != nil && r.StartedAt.Before(startedBefore) {
			r.Status, r.Error, r.FinishedAt = StatusFailed, msg, &now
			n++
		}
	}
	return n, nil
}

func (s *fakeStore) ListExpired(_ context.Context, now time.Time, limit int) ([]Export, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Export
	for _, r := range s.rows {
		if r.Status == StatusDone && r.ExpiresAt != nil && !r.ExpiresAt.After(now) && len(out) < limit {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (s *fakeStore) MarkExpired(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.get(id)
	if r == nil || r.Status != StatusDone {
		return false, nil
	}
	r.Status = StatusExpired
	return true, nil
}

type fakeObjects struct {
	uploaded  map[string][]byte
	deleted   []string
	uploadErr error
	deleteErr error
}

func (o *fakeObjects) Upload(_ context.Context, key string, data []byte, _, _ string) (string, error) {
	if o.uploadErr != nil {
		return "", o.uploadErr
	}
	if o.uploaded == nil {
		o.uploaded = map[string][]byte{}
	}
	o.uploaded[key] = data
	return "https://x/" + key, nil
}

func (o *fakeObjects) DeleteObject(_ context.Context, key string) error {
	o.deleted = append(o.deleted, key)
	return o.deleteErr
}

type failingSource struct{}

func (failingSource) Rows(context.Context, string, string, func(Row) error) error {
	return errors.New("db is down")
}

func newRunner(store *fakeStore, objects *fakeObjects, src Source, now time.Time) *Runner {
	return &Runner{Store: store, Source: src, Objects: objects, Now: func() time.Time { return now }}
}

func TestRunOnceMovesQueuedToRunningToDone(t *testing.T) {
	store := &fakeStore{rows: []*Export{{ID: "e1", WorkspaceID: wsA, Status: StatusQueued}}}
	objects := &fakeObjects{}
	now := t0
	n, err := newRunner(store, objects, fixture(), now).RunOnce(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("RunOnce = %d, %v; want 1, nil", n, err)
	}
	e := store.rows[0]
	if e.Status != StatusDone {
		t.Fatalf("status = %q, want done", e.Status)
	}
	if e.StartedAt == nil || e.FinishedAt == nil || e.ExpiresAt == nil {
		t.Fatalf("timestamps not set: %+v", e)
	}
	if !e.ExpiresAt.Equal(e.FinishedAt.Add(7 * 24 * time.Hour)) {
		t.Errorf("expires_at = %v, want finished_at + 7 days (%v)", e.ExpiresAt, e.FinishedAt)
	}
	if e.StorageKey == "" || e.SizeBytes == 0 {
		t.Errorf("storage_key=%q size=%d", e.StorageKey, e.SizeBytes)
	}
	if got := objects.uploaded[e.StorageKey]; int64(len(got)) != e.SizeBytes {
		t.Errorf("uploaded %d bytes, row says %d", len(got), e.SizeBytes)
	}
	if !strings.Contains(e.StorageKey, wsA) || !strings.Contains(e.StorageKey, "e1") {
		t.Errorf("storage key %q should name the workspace and export", e.StorageKey)
	}
}

func TestRunOnceFailureMarksFailedAndDoesNotRetry(t *testing.T) {
	store := &fakeStore{rows: []*Export{{ID: "e1", WorkspaceID: wsA, Status: StatusQueued}}}
	r := newRunner(store, &fakeObjects{}, failingSource{}, t0)
	if n, err := r.RunOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("RunOnce = %d, %v; want 1, nil (a failed export is not a failed run)", n, err)
	}
	e := store.rows[0]
	if e.Status != StatusFailed || !strings.Contains(e.Error, "db is down") {
		t.Fatalf("row = %+v, want failed with the error", e)
	}
	if n, _ := r.RunOnce(context.Background()); n != 0 {
		t.Fatalf("second run processed %d rows; a failed export is not retried", n)
	}
	if e.Status != StatusFailed {
		t.Fatalf("status after second run = %q", e.Status)
	}
}

func TestUploadFailureMarksFailed(t *testing.T) {
	store := &fakeStore{rows: []*Export{{ID: "e1", WorkspaceID: wsA, Status: StatusQueued}}}
	objects := &fakeObjects{uploadErr: errors.New("bucket gone")}
	if _, err := newRunner(store, objects, fixture(), t0).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e := store.rows[0]; e.Status != StatusFailed || !strings.Contains(e.Error, "bucket gone") || e.StorageKey != "" {
		t.Fatalf("row = %+v", e)
	}
}

func TestASecondClaimOfTheSameRowDoesNothing(t *testing.T) {
	store := &fakeStore{rows: []*Export{{ID: "e1", WorkspaceID: wsA, Status: StatusQueued}}}
	if _, ok, _ := store.ClaimNext(context.Background(), t0); !ok {
		t.Fatal("first claim failed")
	}
	if _, ok, _ := store.ClaimNext(context.Background(), t0); ok {
		t.Fatal("second claim of the same row succeeded")
	}
	n, err := newRunner(store, &fakeObjects{}, fixture(), t0).RunOnce(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("RunOnce over a running row = %d, %v; want 0, nil", n, err)
	}
	if store.rows[0].Status != StatusRunning {
		t.Fatalf("status = %q, want it left running", store.rows[0].Status)
	}
}

func TestRunOnceHandlesOneRowPerClaimUpToTheBatch(t *testing.T) {
	store := &fakeStore{}
	for i := 0; i < Batch+3; i++ {
		store.rows = append(store.rows, &Export{ID: string(rune('a' + i)), WorkspaceID: wsA, Status: StatusQueued})
	}
	n, err := newRunner(store, &fakeObjects{}, fixture(), t0).RunOnce(context.Background())
	if err != nil || n != Batch {
		t.Fatalf("RunOnce = %d, %v; want %d", n, err, Batch)
	}
}

func TestRunOnceExpiresDoneRowsAndDeletesTheirObjects(t *testing.T) {
	past := t0.Add(-time.Hour)
	future := t0.Add(time.Hour)
	store := &fakeStore{rows: []*Export{
		{ID: "old", Status: StatusDone, StorageKey: "k/old.zip", ExpiresAt: &past},
		{ID: "new", Status: StatusDone, StorageKey: "k/new.zip", ExpiresAt: &future},
	}}
	objects := &fakeObjects{}
	if _, err := newRunner(store, objects, fixture(), t0).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.rows[0].Status != StatusExpired || store.rows[1].Status != StatusDone {
		t.Fatalf("statuses = %q, %q", store.rows[0].Status, store.rows[1].Status)
	}
	if len(objects.deleted) != 1 || objects.deleted[0] != "k/old.zip" {
		t.Fatalf("deleted = %v", objects.deleted)
	}
}

func TestAMissingObjectAtExpiryIsNotAnError(t *testing.T) {
	past := t0.Add(-time.Hour)
	store := &fakeStore{rows: []*Export{{ID: "old", Status: StatusDone, StorageKey: "k/old.zip", ExpiresAt: &past}}}
	objects := &fakeObjects{deleteErr: fs.ErrNotExist}
	if _, err := newRunner(store, objects, fixture(), t0).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.rows[0].Status != StatusExpired {
		t.Fatalf("status = %q, want expired", store.rows[0].Status)
	}
}

func TestADeleteFailureLeavesTheRowDoneForTheNextRun(t *testing.T) {
	past := t0.Add(-time.Hour)
	store := &fakeStore{rows: []*Export{{ID: "old", Status: StatusDone, StorageKey: "k/old.zip", ExpiresAt: &past}}}
	objects := &fakeObjects{deleteErr: errors.New("timeout")}
	if _, err := newRunner(store, objects, fixture(), t0).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.rows[0].Status != StatusDone {
		t.Fatalf("status = %q; the object still exists, so the row must stay done", store.rows[0].Status)
	}
}

func TestARunningRowOlderThanTheStaleLimitIsFailed(t *testing.T) {
	started := t0.Add(-StaleAfter - time.Minute)
	recent := t0.Add(-time.Minute)
	store := &fakeStore{rows: []*Export{
		{ID: "stuck", Status: StatusRunning, StartedAt: &started},
		{ID: "live", Status: StatusRunning, StartedAt: &recent},
	}}
	if _, err := newRunner(store, &fakeObjects{}, fixture(), t0).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.rows[0].Status != StatusFailed || store.rows[1].Status != StatusRunning {
		t.Fatalf("statuses = %q, %q", store.rows[0].Status, store.rows[1].Status)
	}
}

func TestWithoutFileStorageTheExportFailsWithAClearError(t *testing.T) {
	store := &fakeStore{rows: []*Export{{ID: "e1", WorkspaceID: wsA, Status: StatusQueued}}}
	r := &Runner{Store: store, Source: fixture(), Now: func() time.Time { return t0 }}
	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e := store.rows[0]; e.Status != StatusFailed || !strings.Contains(e.Error, "file storage is not configured") {
		t.Fatalf("row = %+v", e)
	}
}
