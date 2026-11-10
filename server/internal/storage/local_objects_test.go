package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func newObjectTestLocal(t *testing.T) *LocalStorage {
	t.Helper()
	t.Setenv("LOCAL_UPLOAD_DIR", t.TempDir())
	store := NewLocalStorageFromEnv()
	if store == nil {
		t.Fatal("NewLocalStorageFromEnv() = nil")
	}
	return store
}

func TestLocalStorageListPrefixReturnsObjectsUnderPrefixSorted(t *testing.T) {
	store := newObjectTestLocal(t)
	ctx := context.Background()
	for key, body := range map[string]string{
		"hub/ws1/staging/a.zip":      "aa",
		"hub/ws1/staging/b.json":     "bbb",
		"hub/ws1/skill/x/bundle.zip": "cccc",
		"hub/ws2/staging/c.zip":      "d",
	} {
		if _, err := store.Upload(ctx, key, []byte(body), "application/octet-stream", ""); err != nil {
			t.Fatalf("Upload(%s): %v", key, err)
		}
	}

	got, err := store.ListPrefix(ctx, "hub/ws1/staging/")
	if err != nil {
		t.Fatalf("ListPrefix: %v", err)
	}
	var keys []string
	sizes := map[string]int64{}
	for _, obj := range got {
		keys = append(keys, obj.Key)
		sizes[obj.Key] = obj.Size
		if obj.LastModified.IsZero() || obj.LastModified.After(time.Now().Add(time.Minute)) {
			t.Errorf("LastModified(%s) = %v, want a recent time", obj.Key, obj.LastModified)
		}
	}
	want := []string{"hub/ws1/staging/a.zip", "hub/ws1/staging/b.json"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("ListPrefix keys = %v, want %v", keys, want)
	}
	if sizes["hub/ws1/staging/b.json"] != 3 {
		t.Fatalf("size of b.json = %d, want 3", sizes["hub/ws1/staging/b.json"])
	}
}

func TestLocalStorageListPrefixSkipsInternalFilesAndMissingDirs(t *testing.T) {
	store := newObjectTestLocal(t)
	ctx := context.Background()
	if _, err := store.Upload(ctx, "p/visible.txt", []byte("v"), "text/plain", "visible.txt"); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	// A staging file left by a crash must not show up as an object.
	if err := os.WriteFile(filepath.Join(store.uploadDir, "p", ".half.txt.tmp"), []byte("partial"), 0644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}
	got, err := store.ListPrefix(ctx, "p/")
	if err != nil {
		t.Fatalf("ListPrefix: %v", err)
	}
	if len(got) != 1 || got[0].Key != "p/visible.txt" {
		t.Fatalf("ListPrefix = %+v, want only p/visible.txt", got)
	}

	missing, err := store.ListPrefix(ctx, "nowhere/deep/")
	if err != nil {
		t.Fatalf("ListPrefix on missing dir: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("ListPrefix on missing dir = %+v, want empty", missing)
	}
}

func TestLocalStorageListPrefixRefusesEscape(t *testing.T) {
	store := newObjectTestLocal(t)
	if _, err := store.ListPrefix(context.Background(), "../outside/"); err == nil {
		t.Fatal("ListPrefix with ../ = nil error, want refusal")
	}
}

func TestLocalStorageCopyObjectCopiesBodyAndSidecar(t *testing.T) {
	store := newObjectTestLocal(t)
	ctx := context.Background()
	if _, err := store.Upload(ctx, "src/report.pdf", []byte("pdf-bytes"), "application/pdf", "Quarterly report.pdf"); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if err := store.CopyObject(ctx, "src/report.pdf", "dst/report.pdf"); err != nil {
		t.Fatalf("CopyObject: %v", err)
	}

	rc, err := store.GetReader(ctx, "dst/report.pdf")
	if err != nil {
		t.Fatalf("GetReader(copy): %v", err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if string(body) != "pdf-bytes" {
		t.Fatalf("copied body = %q, want pdf-bytes", body)
	}
	meta, ok := readLocalMeta(filepath.Join(store.uploadDir, "dst/report.pdf"))
	if !ok || meta.Filename != "Quarterly report.pdf" {
		t.Fatalf("copied sidecar = %+v, %v; want the source filename", meta, ok)
	}
}

func TestLocalStorageCopyObjectReplacesDestinationAndDropsStaleSidecar(t *testing.T) {
	store := newObjectTestLocal(t)
	ctx := context.Background()
	if _, err := store.Upload(ctx, "dst/a.txt", []byte("old"), "text/plain", "old name.txt"); err != nil {
		t.Fatalf("Upload dst: %v", err)
	}
	if _, err := store.Upload(ctx, "src/a.txt", []byte("new"), "text/plain", ""); err != nil {
		t.Fatalf("Upload src: %v", err)
	}
	if err := store.CopyObject(ctx, "src/a.txt", "dst/a.txt"); err != nil {
		t.Fatalf("CopyObject: %v", err)
	}
	rc, err := store.GetReader(ctx, "dst/a.txt")
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if string(body) != "new" {
		t.Fatalf("body after copy = %q, want new", body)
	}
	if _, ok := readLocalMeta(filepath.Join(store.uploadDir, "dst/a.txt")); ok {
		t.Fatal("destination kept the old sidecar; the source had none")
	}
}

func TestLocalStorageCopyObjectErrors(t *testing.T) {
	store := newObjectTestLocal(t)
	ctx := context.Background()
	if err := store.CopyObject(ctx, "missing/key", "dst/key"); err == nil {
		t.Fatal("CopyObject from missing source = nil error")
	}
	if _, err := store.Upload(ctx, "src/x", []byte("x"), "text/plain", ""); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if err := store.CopyObject(ctx, "src/x", "dst/x.meta.json"); err == nil {
		t.Fatal("CopyObject onto an internal sidecar key = nil error")
	}
	if err := store.CopyObject(ctx, "src/x", "../escape"); err == nil {
		t.Fatal("CopyObject escaping the upload dir = nil error")
	}
}

func TestLocalStoragePresignPutIsUnsupported(t *testing.T) {
	store := newObjectTestLocal(t)
	_, err := store.PresignPut(context.Background(), "k", time.Minute, "application/zip")
	if !errors.Is(err, ErrPresignUnsupported) {
		t.Fatalf("PresignPut err = %v, want ErrPresignUnsupported", err)
	}
}
