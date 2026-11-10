package storage

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// Hub object storage (HUB_S3_*) is a second store, separate from the attachment
// bucket (S3_*). Hub bundles and staged previews live here, so pointing the Hub
// at another S3-compatible endpoint is a configuration change, not a code change.
//
// Environment variables:
//   - HUB_S3_BUCKET (required to enable; unset falls back to the attachment store)
//   - HUB_S3_REGION (default: us-east-1)
//   - HUB_S3_ENDPOINT (optional; custom S3-compatible endpoint, e.g. SeaweedFS)
//   - HUB_S3_ACCESS_KEY / HUB_S3_SECRET_KEY (optional; default credential chain otherwise)
//   - HUB_S3_USE_PATH_STYLE (default: true when HUB_S3_ENDPOINT is set)
//   - HUB_S3_PUBLIC_ENDPOINT (optional; the address clients reach the store at).
//     Presigned links are signed against it. Unset, the Hub serves bundles through
//     the server instead, because a link to the internal endpoint is unusable by clients.

const hubBucketCreateTimeout = 10 * time.Second

// NewHubS3StorageFromEnv builds the Hub's store from HUB_S3_*. It returns nil when
// HUB_S3_BUCKET is unset. A custom endpoint gets its bucket created idempotently;
// a failure there is logged and does not stop startup.
func NewHubS3StorageFromEnv() *S3Storage {
	bucket := os.Getenv("HUB_S3_BUCKET")
	if bucket == "" {
		return nil
	}
	region := os.Getenv("HUB_S3_REGION")
	if region == "" {
		region = "us-east-1"
	}
	endpoint := os.Getenv("HUB_S3_ENDPOINT")
	publicEndpoint := os.Getenv("HUB_S3_PUBLIC_ENDPOINT")
	usePathStyle := s3UsePathStyleFromEnvVar("HUB_S3_USE_PATH_STYLE", endpoint != "")

	store, err := NewS3Storage(S3Options{
		Bucket:          bucket,
		Region:          region,
		Endpoint:        endpoint,
		AccessKey:       os.Getenv("HUB_S3_ACCESS_KEY"),
		SecretKey:       os.Getenv("HUB_S3_SECRET_KEY"),
		UsePathStyle:    usePathStyle,
		PresignEndpoint: publicEndpoint,
		NoPresign:       publicEndpoint == "",
	})
	if err != nil {
		slog.Error("hub storage: failed to load AWS config", "error", err)
		return nil
	}
	if endpoint != "" {
		ctx, cancel := context.WithTimeout(context.Background(), hubBucketCreateTimeout)
		defer cancel()
		if err := store.EnsureBucket(ctx); err != nil {
			slog.Warn("hub storage: could not ensure bucket exists; uploads will fail until it does", "bucket", bucket, "endpoint_url", endpoint, "error", err)
		}
	}
	slog.Info("hub storage initialized", "bucket", bucket, "region", region, "endpoint_url", endpoint, "use_path_style", usePathStyle, "presign", publicEndpoint != "")
	return store
}

// HubStorageFromEnv returns the store the Hub writes to: the HUB_S3_* bucket when
// it is configured, otherwise the fallback (the attachment store, which itself
// falls back to local disk). The result is nil only when the fallback is nil.
func HubStorageFromEnv(fallback Storage) Storage {
	if store := NewHubS3StorageFromEnv(); store != nil {
		return store
	}
	slog.Info("HUB_S3_BUCKET not set, Hub bundles use the attachment storage")
	return fallback
}
