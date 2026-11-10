package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

type Storage interface {
	Upload(ctx context.Context, key string, data []byte, contentType string, filename string) (string, error)
	Delete(ctx context.Context, key string)
	// DeleteObject is Delete with the error surfaced — the channel-media
	// reconciler schedules retries on failure instead of assuming success.
	DeleteObject(ctx context.Context, key string) error
	DeleteKeys(ctx context.Context, keys []string)
	KeyFromURL(rawURL string) string
	// ObjectURL is the URL a successful Upload of key would return — a pure
	// function of configuration, so the media intent ledger can persist it
	// BEFORE the upload.
	ObjectURL(key string) string
	CdnDomain() string
	// GetReader streams an object back to the caller. Used by the attachment
	// preview proxy (GET /api/attachments/{id}/content) to bypass CloudFront
	// CORS and the inline/attachment Content-Disposition decision. Caller
	// must Close the returned reader.
	GetReader(ctx context.Context, key string) (io.ReadCloser, error)
}

// ErrPresignUnsupported is returned by presign methods on a store that cannot
// issue a URL a client can use: local disk has no URLs, and an S3 store with no
// client-reachable endpoint must not hand out a link the client cannot open.
// Callers fall back to serving the bytes through the server.
var ErrPresignUnsupported = errors.New("storage: presigned URLs are not available for this store")

type Presigner interface {
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

type DownloadPresigner interface {
	PresignGetWithContentDisposition(ctx context.Context, key string, ttl time.Duration, contentDisposition string) (string, error)
}

// PresignPutter issues a time-limited URL a client can PUT one object body to.
// Local storage returns ErrPresignUnsupported.
type PresignPutter interface {
	PresignPut(ctx context.Context, key string, ttl time.Duration, contentType string) (string, error)
}

// ObjectInfo describes one stored object.
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// Lister lists the objects whose keys start with prefix, sorted by key.
// An empty prefix lists the whole store.
type Lister interface {
	ListPrefix(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

// Copier copies an object within the same store, replacing dstKey if it exists.
type Copier interface {
	CopyObject(ctx context.Context, srcKey, dstKey string) error
}
