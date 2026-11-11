package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/storage"
)

// noListStore is a store whose method set has no ListPrefix. The sweep must skip it.
type noListStore struct{ storage.Storage }

func TestHubStagingGCJobRegisters(t *testing.T) {
	m := NewManager(nil, Options{})
	if err := m.Register(HubStagingGCJob(nil)); err != nil {
		t.Fatalf("register hub staging gc: %v", err)
	}
}

func TestExpiredStagingKeysSelection(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-HubStagingMaxAge)
	objects := []storage.ObjectInfo{
		{Key: "staging/ws/old.zip", LastModified: cutoff.Add(-time.Second)},
		{Key: "staging/ws/old.json", LastModified: cutoff.Add(-time.Hour)},
		{Key: "staging/ws/at-cutoff.zip", LastModified: cutoff},
		{Key: "staging/ws/fresh.zip", LastModified: now.Add(-time.Minute)},
		// Outside the prefix: never swept, however old.
		{Key: "hub/ws/skill/x/digest/bundle.zip", LastModified: now.Add(-240 * time.Hour)},
		{Key: "attachments/ws/file.png", LastModified: now.Add(-240 * time.Hour)},
	}
	got := expiredStagingKeys(objects, now, HubStagingMaxAge)
	want := []string{"staging/ws/old.zip", "staging/ws/old.json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expired keys = %v, want %v", got, want)
	}
}

func TestSweepHubStagingDeletesOnlyExpiredPreviewsOnLocalDisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCAL_UPLOAD_DIR", dir)
	store := storage.NewLocalStorageFromEnv()
	if store == nil {
		t.Fatal("local storage did not initialize")
	}
	ctx := context.Background()
	now := time.Now()

	files := map[string]time.Duration{
		"staging/ws-1/old-preview.zip":        2 * time.Hour,
		"staging/ws-1/old-preview.json":       2 * time.Hour,
		"staging/ws-2/fresh-preview.zip":      10 * time.Minute,
		"hub/ws-1/skill/s1/digest/bundle.zip": 240 * time.Hour,
		"attachments/ws-1/keep.png":           240 * time.Hour,
	}
	for key, age := range files {
		path := filepath.Join(dir, filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mtime := now.Add(-age)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	deleted, err := SweepHubStaging(ctx, store, now, HubStagingMaxAge)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	for key := range files {
		_, statErr := os.Stat(filepath.Join(dir, filepath.FromSlash(key)))
		gone := os.IsNotExist(statErr)
		wantGone := key == "staging/ws-1/old-preview.zip" || key == "staging/ws-1/old-preview.json"
		if gone != wantGone {
			t.Errorf("%s: deleted = %v, want %v", key, gone, wantGone)
		}
	}

	// A second sweep finds nothing left to delete.
	if deleted, err := SweepHubStaging(ctx, store, now, HubStagingMaxAge); err != nil || deleted != 0 {
		t.Fatalf("second sweep = %d, %v; want 0, nil", deleted, err)
	}
}

func TestSweepHubStagingSkipsStoreWithoutListing(t *testing.T) {
	deleted, err := SweepHubStaging(context.Background(), noListStore{}, time.Now(), HubStagingMaxAge)
	if err != nil || deleted != 0 {
		t.Fatalf("sweep without listing = %d, %v; want 0, nil", deleted, err)
	}
}
