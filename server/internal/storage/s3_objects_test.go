package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3 answers the handful of S3 calls the object helpers make. Each handler
// receives the request; a nil handler answers 200 with an empty body.
func fakeS3(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handle == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		handle(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newFakeS3Store(t *testing.T, endpoint string, mutate func(*S3Options)) *S3Storage {
	t.Helper()
	opts := S3Options{
		Bucket:       "hub-bucket",
		Region:       "us-east-1",
		Endpoint:     endpoint,
		AccessKey:    "AKID",
		SecretKey:    "SECRET",
		UsePathStyle: true,
	}
	if mutate != nil {
		mutate(&opts)
	}
	store, err := NewS3Storage(opts)
	if err != nil {
		t.Fatalf("NewS3Storage: %v", err)
	}
	return store
}

func TestS3StorageListPrefixFollowsContinuationTokens(t *testing.T) {
	var mu sync.Mutex
	var prefixes, tokens []string
	srv := fakeS3(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		mu.Lock()
		prefixes = append(prefixes, q.Get("prefix"))
		tokens = append(tokens, q.Get("continuation-token"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		if q.Get("continuation-token") == "" {
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult><Name>hub-bucket</Name><Prefix>hub/ws/staging/</Prefix><IsTruncated>true</IsTruncated><NextContinuationToken>page2</NextContinuationToken>
<Contents><Key>hub/ws/staging/a.zip</Key><Size>10</Size><LastModified>2026-11-01T00:00:00.000Z</LastModified></Contents>
</ListBucketResult>`)
			return
		}
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult><Name>hub-bucket</Name><Prefix>hub/ws/staging/</Prefix><IsTruncated>false</IsTruncated>
<Contents><Key>hub/ws/staging/b.json</Key><Size>4</Size><LastModified>2026-11-01T00:00:00.000Z</LastModified></Contents>
</ListBucketResult>`)
	})
	store := newFakeS3Store(t, srv.URL, nil)

	got, err := store.ListPrefix(context.Background(), "hub/ws/staging/")
	if err != nil {
		t.Fatalf("ListPrefix: %v", err)
	}
	if len(got) != 2 || got[0].Key != "hub/ws/staging/a.zip" || got[0].Size != 10 || got[1].Key != "hub/ws/staging/b.json" {
		t.Fatalf("ListPrefix = %+v, want both pages' objects", got)
	}
	if len(tokens) != 2 || tokens[0] != "" || tokens[1] != "page2" {
		t.Fatalf("continuation tokens sent = %q, want [\"\" page2]", tokens)
	}
	for _, p := range prefixes {
		if p != "hub/ws/staging/" {
			t.Fatalf("prefix sent = %q, want hub/ws/staging/", p)
		}
	}
}

func TestS3StorageCopyObjectEncodesCopySource(t *testing.T) {
	var gotSource, gotPath, gotMethod string
	srv := fakeS3(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotSource = r.Header.Get("X-Amz-Copy-Source")
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<CopyObjectResult><ETag>"abc"</ETag><LastModified>2026-11-01T00:00:00.000Z</LastModified></CopyObjectResult>`)
	})
	store := newFakeS3Store(t, srv.URL, nil)

	if err := store.CopyObject(context.Background(), "staging/src dir/a+b.zip", "bundles/final.zip"); err != nil {
		t.Fatalf("CopyObject: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/hub-bucket/bundles/final.zip" {
		t.Fatalf("request = %s %s, want PUT /hub-bucket/bundles/final.zip", gotMethod, gotPath)
	}
	if want := "hub-bucket/staging/src%20dir/a%2Bb.zip"; gotSource != want {
		t.Fatalf("x-amz-copy-source = %q, want %q", gotSource, want)
	}
}

func TestS3StorageCopyObjectRejectsEmptyKeys(t *testing.T) {
	store := newFakeS3Store(t, "", nil)
	if err := store.CopyObject(context.Background(), "", "dst"); err == nil {
		t.Fatal("CopyObject with empty source = nil error")
	}
	if err := store.CopyObject(context.Background(), "src", ""); err == nil {
		t.Fatal("CopyObject with empty destination = nil error")
	}
}

func TestS3StoragePresignPutSignsAgainstPresignEndpoint(t *testing.T) {
	// Requests go to the server's own endpoint; the URL handed to clients names
	// the public one. The signature covers the host, so it must be signed there.
	srv := fakeS3(t, nil)
	store := newFakeS3Store(t, srv.URL, func(o *S3Options) {
		o.PresignEndpoint = "https://files.example.com"
	})

	signed, err := store.PresignPut(context.Background(), "hub/ws/staging/x.zip", 5*time.Minute, "application/zip")
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse presigned URL: %v", err)
	}
	if u.Host != "files.example.com" || u.Path != "/hub-bucket/hub/ws/staging/x.zip" {
		t.Fatalf("presigned URL = %s, want host files.example.com and the bucket path", signed)
	}
	if u.Query().Get("X-Amz-Expires") != "300" || u.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("presigned URL missing expiry or signature: %s", signed)
	}
}

func TestS3StorageWithoutPublicEndpointDoesNotPresign(t *testing.T) {
	srv := fakeS3(t, nil)
	store := newFakeS3Store(t, srv.URL, func(o *S3Options) { o.NoPresign = true })

	if _, err := store.PresignGet(context.Background(), "k", time.Minute); !errors.Is(err, ErrPresignUnsupported) {
		t.Fatalf("PresignGet err = %v, want ErrPresignUnsupported", err)
	}
	if _, err := store.PresignPut(context.Background(), "k", time.Minute, ""); !errors.Is(err, ErrPresignUnsupported) {
		t.Fatalf("PresignPut err = %v, want ErrPresignUnsupported", err)
	}
	if _, err := store.PresignGetWithContentDisposition(context.Background(), "k", time.Minute, "attachment"); !errors.Is(err, ErrPresignUnsupported) {
		t.Fatalf("PresignGetWithContentDisposition err = %v, want ErrPresignUnsupported", err)
	}
}

func TestS3StoragePresignGetUsesPublicEndpointWhenSet(t *testing.T) {
	srv := fakeS3(t, nil)
	store := newFakeS3Store(t, srv.URL, func(o *S3Options) {
		o.NoPresign = true
		o.PresignEndpoint = "https://files.example.com"
	})
	got, err := store.PresignGet(context.Background(), "bundle.zip", time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	if !strings.HasPrefix(got, "https://files.example.com/hub-bucket/bundle.zip?") {
		t.Fatalf("PresignGet = %s, want the public endpoint", got)
	}
}

func TestS3StorageEnsureBucket(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{name: "created", status: http.StatusOK},
		{name: "owned by caller", status: http.StatusConflict, body: `<Error><Code>BucketAlreadyOwnedByYou</Code><Message>x</Message></Error>`},
		{name: "exists", status: http.StatusConflict, body: `<Error><Code>BucketAlreadyExists</Code><Message>x</Message></Error>`},
		{name: "access denied", status: http.StatusForbidden, body: `<Error><Code>AccessDenied</Code><Message>x</Message></Error>`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var method, path string
			srv := fakeS3(t, func(w http.ResponseWriter, r *http.Request) {
				method, path = r.Method, r.URL.Path
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			store := newFakeS3Store(t, srv.URL, nil)
			err := store.EnsureBucket(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("EnsureBucket err = %v, wantErr %v", err, tc.wantErr)
			}
			if method != http.MethodPut || path != "/hub-bucket" {
				t.Fatalf("request = %s %s, want PUT /hub-bucket", method, path)
			}
		})
	}
}

func TestHubStorageFromEnvFallsBackWhenBucketUnset(t *testing.T) {
	t.Setenv("HUB_S3_BUCKET", "")
	fallback := &LocalStorage{uploadDir: t.TempDir()}
	if got := HubStorageFromEnv(fallback); got != Storage(fallback) {
		t.Fatalf("HubStorageFromEnv = %T, want the fallback store", got)
	}
}

func TestNewHubS3StorageFromEnvReadsHubVariablesAndCreatesBucket(t *testing.T) {
	var creates int
	srv := fakeS3(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/multica-hub" {
			creates++
		}
		w.WriteHeader(http.StatusOK)
	})
	t.Setenv("HUB_S3_BUCKET", "multica-hub")
	t.Setenv("HUB_S3_REGION", "")
	t.Setenv("HUB_S3_ENDPOINT", srv.URL)
	t.Setenv("HUB_S3_ACCESS_KEY", "hub-key")
	t.Setenv("HUB_S3_SECRET_KEY", "hub-secret")
	t.Setenv("HUB_S3_USE_PATH_STYLE", "")
	t.Setenv("HUB_S3_PUBLIC_ENDPOINT", "")
	// The attachment store points elsewhere; the Hub must not inherit it.
	t.Setenv("AWS_ENDPOINT_URL", "https://attachments.example.com")
	t.Setenv("S3_BUCKET", "attachments")

	store := NewHubS3StorageFromEnv()
	if store == nil {
		t.Fatal("NewHubS3StorageFromEnv() = nil")
	}
	if store.bucket != "multica-hub" || store.region != "us-east-1" {
		t.Fatalf("bucket/region = %q/%q, want multica-hub/us-east-1", store.bucket, store.region)
	}
	if !store.usePathStyle {
		t.Fatal("usePathStyle = false, want true for a custom endpoint")
	}
	if !store.presignOff {
		t.Fatal("presignOff = false, want true when HUB_S3_PUBLIC_ENDPOINT is unset")
	}
	if creates != 1 {
		t.Fatalf("CreateBucket calls = %d, want 1", creates)
	}
	if got := store.client.Options().BaseEndpoint; got == nil || *got != srv.URL {
		t.Fatalf("client endpoint = %v, want the HUB_S3_ENDPOINT", got)
	}
}

func TestNewHubS3StorageFromEnvDoesNotInheritAttachmentEndpoint(t *testing.T) {
	t.Setenv("HUB_S3_BUCKET", "multica-hub")
	t.Setenv("HUB_S3_ENDPOINT", "")
	t.Setenv("HUB_S3_ACCESS_KEY", "hub-key")
	t.Setenv("HUB_S3_SECRET_KEY", "hub-secret")
	t.Setenv("AWS_ENDPOINT_URL", "https://attachments.example.com")

	store := NewHubS3StorageFromEnv()
	if store == nil {
		t.Fatal("NewHubS3StorageFromEnv() = nil")
	}
	if got := store.client.Options().BaseEndpoint; got != nil {
		t.Fatalf("hub client endpoint = %q, want AWS default (nil): AWS_ENDPOINT_URL must not leak in", *got)
	}
}

func TestNewHubS3StorageFromEnvNilWithoutBucket(t *testing.T) {
	t.Setenv("HUB_S3_BUCKET", "")
	if store := NewHubS3StorageFromEnv(); store != nil {
		t.Fatalf("NewHubS3StorageFromEnv() = %+v, want nil", store)
	}
}
