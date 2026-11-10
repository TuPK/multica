package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestS3StorageAgainstEndpoint exercises the Hub's object operations against a
// real S3-compatible server (SeaweedFS in the self-host compose file). It is
// skipped unless HUB_S3_TEST_ENDPOINT is set, so the default suite never needs
// a server. Run it with:
//
//	HUB_S3_TEST_ENDPOINT=http://127.0.0.1:8333 \
//	HUB_S3_TEST_ACCESS_KEY=... HUB_S3_TEST_SECRET_KEY=... \
//	go test ./internal/storage -run TestS3StorageAgainstEndpoint -count=1 -v
//
// The bucket is HUB_S3_TEST_BUCKET (default: hub-it). Objects under the test's
// own prefix are removed at the end; the bucket is left in place.
func TestS3StorageAgainstEndpoint(t *testing.T) {
	endpoint := os.Getenv("HUB_S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("HUB_S3_TEST_ENDPOINT not set; skipping the object-store integration test")
	}
	bucket := os.Getenv("HUB_S3_TEST_BUCKET")
	if bucket == "" {
		bucket = "hub-it"
	}
	store, err := NewS3Storage(S3Options{
		Bucket:       bucket,
		Region:       "us-east-1",
		Endpoint:     endpoint,
		AccessKey:    os.Getenv("HUB_S3_TEST_ACCESS_KEY"),
		SecretKey:    os.Getenv("HUB_S3_TEST_SECRET_KEY"),
		UsePathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewS3Storage: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	// A second call must succeed when the bucket already exists.
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket (second call): %v", err)
	}

	prefix := fmt.Sprintf("it-%d/", time.Now().UnixNano())
	t.Cleanup(func() {
		objects, err := store.ListPrefix(context.Background(), prefix)
		if err != nil {
			t.Logf("cleanup list: %v", err)
			return
		}
		for _, obj := range objects {
			if err := store.DeleteObject(context.Background(), obj.Key); err != nil {
				t.Logf("cleanup delete %s: %v", obj.Key, err)
			}
		}
	})

	body := []byte("PK\x03\x04 integration bundle bytes")
	srcKey := prefix + "staging/one.zip"
	dstKey := prefix + "bundles/two.zip"
	if _, err := store.Upload(ctx, srcKey, body, "application/zip", ""); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	listed, err := store.ListPrefix(ctx, prefix+"staging/")
	if err != nil {
		t.Fatalf("ListPrefix: %v", err)
	}
	if len(listed) != 1 || listed[0].Key != srcKey || listed[0].Size != int64(len(body)) {
		t.Fatalf("ListPrefix = %+v, want the one uploaded object", listed)
	}

	if err := store.CopyObject(ctx, srcKey, dstKey); err != nil {
		t.Fatalf("CopyObject: %v", err)
	}
	if got := readStoreObject(t, ctx, store, dstKey); !bytes.Equal(got, body) {
		t.Fatalf("copied body = %q, want %q", got, body)
	}

	signedGet, err := store.PresignGet(ctx, dstKey, time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	if got := fetchURL(t, ctx, http.MethodGet, signedGet, nil); !bytes.Equal(got, body) {
		t.Fatalf("presigned GET body = %q, want %q", got, body)
	}

	putKey := prefix + "uploads/three.zip"
	signedPut, err := store.PresignPut(ctx, putKey, time.Minute, "application/zip")
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	fetchURL(t, ctx, http.MethodPut, signedPut, body, "Content-Type", "application/zip")
	if got := readStoreObject(t, ctx, store, putKey); !bytes.Equal(got, body) {
		t.Fatalf("presigned PUT body = %q, want %q", got, body)
	}

	if err := store.DeleteObject(ctx, dstKey); err != nil {
		t.Fatalf("DeleteObject: %v", err)
	}
	if err := store.DeleteObject(ctx, dstKey); err != nil {
		t.Fatalf("DeleteObject of a missing key: %v", err)
	}
}

func readStoreObject(t *testing.T, ctx context.Context, store *S3Storage, key string) []byte {
	t.Helper()
	rc, err := store.GetReader(ctx, key)
	if err != nil {
		t.Fatalf("GetReader(%s): %v", key, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	return data
}

// fetchURL performs one request against a presigned URL and returns the body.
// headerPairs are extra request headers, given as name, value.
func fetchURL(t *testing.T, ctx context.Context, method, rawURL string, body []byte, headerPairs ...string) []byte {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for i := 0; i+1 < len(headerPairs); i += 2 {
		req.Header.Set(headerPairs[i], headerPairs[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s presigned URL: %v", method, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("%s presigned URL status %d: %s", method, resp.StatusCode, out)
	}
	return out
}
