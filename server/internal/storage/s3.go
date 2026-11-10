package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type S3Storage struct {
	client       *s3.Client
	bucket       string
	region       string // used to construct virtual-hosted-style public URLs when no CDN/endpoint is set
	cdnDomain    string // if set, returned URLs use this instead of bucket name
	endpointURL  string // custom S3-compatible endpoint (e.g. MinIO)
	usePathStyle bool   // controls path-style S3 addressing
	// presign signs presigned URLs against a client-reachable endpoint. Nil
	// means sign with client (the attachment default).
	presign *s3.PresignClient
	// presignOff makes presign methods return ErrPresignUnsupported.
	presignOff bool
}

// S3Options configures NewS3Storage. Attachment and Hub stores differ only in
// where these values come from.
type S3Options struct {
	Bucket string
	// Region is the signing region.
	Region string
	// Endpoint is a custom S3-compatible endpoint (MinIO, SeaweedFS, ...). Empty means AWS.
	Endpoint string
	// AccessKey and SecretKey are used together as static credentials. When either
	// is empty the default AWS credential chain is used.
	AccessKey string
	SecretKey string
	// UsePathStyle selects path-style addressing.
	UsePathStyle bool
	// CDNDomain, when set, makes uploaded URLs point at this domain.
	CDNDomain string
	// PresignEndpoint, when set, signs presigned URLs against this URL instead of
	// Endpoint. Use it when clients reach the store at a different address than
	// the server does; the signature covers the host, so it cannot be rewritten later.
	PresignEndpoint string
	// NoPresign makes presign methods return ErrPresignUnsupported unless
	// PresignEndpoint is set. Set it when the server's endpoint is not reachable
	// by clients.
	NoPresign bool
}

// NewS3Storage builds an S3-compatible store from explicit options.
func NewS3Storage(opts S3Options) (*S3Storage, error) {
	cfgOpts := []func(*config.LoadOptions) error{
		config.WithRegion(opts.Region),
	}
	if opts.AccessKey != "" && opts.SecretKey != "" {
		cfgOpts = append(cfgOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(opts.AccessKey, opts.SecretKey, ""),
		))
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), cfgOpts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	s := &S3Storage{
		client:       s3.NewFromConfig(cfg, s3ClientOptions(opts.Endpoint, opts.UsePathStyle)...),
		bucket:       opts.Bucket,
		region:       opts.Region,
		cdnDomain:    opts.CDNDomain,
		endpointURL:  opts.Endpoint,
		usePathStyle: opts.UsePathStyle,
		presignOff:   opts.NoPresign && opts.PresignEndpoint == "",
	}
	if opts.PresignEndpoint != "" {
		s.presign = s3.NewPresignClient(s3.NewFromConfig(cfg, s3ClientOptions(opts.PresignEndpoint, opts.UsePathStyle)...))
	}
	return s, nil
}

// s3ClientOptions pins the endpoint and addressing style explicitly. An empty
// endpoint clears any AWS_ENDPOINT_URL that the SDK config picked up from the
// environment, so a store built for AWS never talks to another store's endpoint.
func s3ClientOptions(endpoint string, usePathStyle bool) []func(*s3.Options) {
	opts := []func(*s3.Options){func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		} else {
			o.BaseEndpoint = nil
		}
		o.UsePathStyle = usePathStyle
	}}
	if isGCSEndpoint(endpoint) {
		opts = append(opts, func(o *s3.Options) {
			o.APIOptions = append(o.APIOptions, excludeGCSAcceptEncoding)
		})
	}
	return opts
}

// NewS3StorageFromEnv creates an S3Storage from environment variables.
// Returns nil if S3_BUCKET is not set.
//
// Environment variables:
//   - S3_BUCKET (required)
//   - S3_REGION (default: us-west-2)
//   - AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY (optional; falls back to default credential chain)
//   - AWS_ENDPOINT_URL (optional S3-compatible endpoint)
//   - S3_USE_PATH_STYLE (optional; defaults to true when AWS_ENDPOINT_URL is set)
//   - CLOUDFRONT_DOMAIN (optional; public URL domain for uploads)
func NewS3StorageFromEnv() *S3Storage {
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		slog.Info("S3_BUCKET not set, cloud upload disabled")
		return nil
	}
	if looksLikeS3Hostname(bucket) {
		slog.Warn(
			"S3_BUCKET looks like a hostname rather than a bucket name — uploads and public URLs will likely both fail. Use only the bucket name (e.g. \"my-bucket\"), not \"<bucket>.s3.<region>.amazonaws.com\".",
			"value", bucket,
		)
	}

	region := os.Getenv("S3_REGION")
	if region == "" {
		region = "us-west-2"
	}
	cdnDomain := os.Getenv("CLOUDFRONT_DOMAIN")
	endpointURL := os.Getenv("AWS_ENDPOINT_URL")
	usePathStyle := s3UsePathStyleFromEnv(endpointURL)

	store, err := NewS3Storage(S3Options{
		Bucket:       bucket,
		Region:       region,
		Endpoint:     endpointURL,
		AccessKey:    os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretKey:    os.Getenv("AWS_SECRET_ACCESS_KEY"),
		UsePathStyle: usePathStyle,
		CDNDomain:    cdnDomain,
	})
	if err != nil {
		slog.Error("failed to load AWS config", "error", err)
		return nil
	}

	slog.Info("S3 storage initialized", "bucket", bucket, "region", region, "cdn_domain", cdnDomain, "endpoint_url", endpointURL, "use_path_style", usePathStyle)
	return store
}

func (s *S3Storage) CdnDomain() string {
	return s.cdnDomain
}

// looksLikeS3Hostname returns true when the configured S3_BUCKET value looks
// like an S3 endpoint hostname rather than a bucket name. Real bucket names
// can never legitimately contain "amazonaws.com", so this is an unambiguous
// misconfiguration signal — the most common form being users pasting
// "<bucket>.s3.<region>.amazonaws.com" into S3_BUCKET.
func looksLikeS3Hostname(bucket string) bool {
	return strings.Contains(bucket, "amazonaws.com")
}

func s3UsePathStyleFromEnv(endpointURL string) bool {
	return s3UsePathStyleFromEnvVar("S3_USE_PATH_STYLE", endpointURL != "")
}

// s3UsePathStyleFromEnvVar reads a boolean addressing-style variable, falling
// back to defaultValue when it is unset or malformed.
func s3UsePathStyleFromEnvVar(name string, defaultValue bool) bool {
	raw, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(raw) == "" {
		return defaultValue
	}
	parsed, err := parseBoolEnv(raw)
	if err != nil {
		slog.Warn("invalid "+name+" value, using default", "value", raw, "default", defaultValue)
		return defaultValue
	}
	return parsed
}

func parseBoolEnv(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "t", "true", "y", "yes", "on":
		return true, nil
	case "0", "f", "false", "n", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q", raw)
	}
}

// isGCSEndpoint reports whether endpointURL points to Google Cloud Storage XML API.
func isGCSEndpoint(endpointURL string) bool {
	if endpointURL == "" {
		return false
	}
	host := endpointHostname(endpointURL)
	return host == "storage.googleapis.com" || strings.HasSuffix(host, ".storage.googleapis.com")
}

// excludeGCSAcceptEncoding keeps the header GCS rewrites out of the signature,
// while retaining the SDK's value on the wire to prevent automatic decompression.
func excludeGCSAcceptEncoding(stack *middleware.Stack) error {
	// Presigning replaces Signing and removes the SDK's Accept-Encoding middleware.
	if _, ok := stack.Finalize.Get("Signing"); !ok {
		return nil
	}

	// Values belong to this operation's stack; retry attempts run sequentially.
	var values []string
	if err := stack.Finalize.Insert(middleware.FinalizeMiddlewareFunc("RemoveGCSAcceptEncoding",
		func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
			req, ok := in.Request.(*smithyhttp.Request)
			if !ok {
				return middleware.FinalizeOutput{}, middleware.Metadata{}, fmt.Errorf("GCS signing: unexpected request type %T", in.Request)
			}
			values = req.Header.Values("Accept-Encoding")
			req.Header.Del("Accept-Encoding")
			return next.HandleFinalize(ctx, in)
		},
	), "Signing", middleware.Before); err != nil {
		return err
	}
	return stack.Finalize.Insert(middleware.FinalizeMiddlewareFunc("RestoreGCSAcceptEncoding",
		func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
			req, ok := in.Request.(*smithyhttp.Request)
			if !ok {
				return middleware.FinalizeOutput{}, middleware.Metadata{}, fmt.Errorf("GCS signing: unexpected request type %T", in.Request)
			}
			if len(values) > 0 {
				req.Header["Accept-Encoding"] = values
			}
			return next.HandleFinalize(ctx, in)
		},
	), "Signing", middleware.After)
}

// awsEndpointDomains are the parent domains AWS serves S3 from. VPC, dualstack
// and FIPS hostnames all live under them, as does the China partition.
var awsEndpointDomains = []string{"amazonaws.com", "amazonaws.com.cn"}

// usesAWSEndpoint reports whether uploads go to real AWS S3 — either the
// default endpoint or an explicitly configured AWS one. The match is on the
// host label boundary, so "notamazonaws.com" and "s3.amazonaws.com.evil.net"
// are correctly treated as third-party endpoints.
func (s *S3Storage) usesAWSEndpoint() bool {
	if s.endpointURL == "" {
		return true
	}
	host := endpointHostname(s.endpointURL)
	for _, domain := range awsEndpointDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// endpointHostname extracts the comparable hostname from a configured
// endpoint: lowercased, without the port, and without a trailing root dot.
// A value written without a scheme is retried as an https URL, since
// url.Parse otherwise reads the whole thing as a path.
func endpointHostname(endpointURL string) string {
	parsed, err := url.Parse(endpointURL)
	if err != nil {
		return ""
	}
	if parsed.Hostname() == "" {
		parsed, err = url.Parse("https://" + endpointURL)
		if err != nil {
			return ""
		}
	}
	return strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
}

// uploadChecksumOptions returns the per-request options for the buffered
// upload path. The SDK defaults RequestChecksumCalculation to WhenSupported,
// which sends the body as aws-chunked with a CRC32 trailer; Aliyun OSS and
// Tencent COS do not implement that encoding and reject the request outright.
//
// Downgrading to WhenRequired drops the trailer, but it also drops the
// client-side checksum entirely, so it is applied only to non-AWS endpoints.
// Real AWS S3 accepts the trailer today, and buckets carrying a default Object
// Lock retention actually require a checksum, so those requests are left
// exactly as the SDK built them — including any AWS_REQUEST_CHECKSUM_CALCULATION
// the operator configured.
func (s *S3Storage) uploadChecksumOptions() []func(*s3.Options) {
	if s.usesAWSEndpoint() {
		return nil
	}
	return []func(*s3.Options){func(opts *s3.Options) {
		opts.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	}}
}

// storageClass returns the appropriate S3 storage class.
// Custom endpoints (e.g. MinIO) only support STANDARD; real AWS defaults to INTELLIGENT_TIERING.
func (s *S3Storage) storageClass() types.StorageClass {
	if s.endpointURL != "" {
		return types.StorageClassStandard
	}
	return types.StorageClassIntelligentTiering
}

// KeyFromURL extracts the S3 object key from a CDN or bucket URL.
// e.g. "https://multica-static.copilothub.ai/abc123.png" → "abc123.png"
//
//	"https://my-bucket.s3.us-east-1.amazonaws.com/uploads/x/y.png" → "uploads/x/y.png"
func (s *S3Storage) KeyFromURL(rawURL string) string {
	if s.endpointURL != "" {
		for _, prefix := range []string{
			customEndpointObjectPrefix(s.endpointURL, s.bucket, true),
			customEndpointObjectPrefix(s.endpointURL, s.bucket, false),
		} {
			if strings.HasPrefix(rawURL, prefix) {
				return strings.TrimPrefix(rawURL, prefix)
			}
		}
	}

	// Strip known "https://host/" prefixes. Order matters: the more specific
	// region-qualified hosts come first so they win over the legacy bucket-only
	// prefix that we used to write before the suffix bug was fixed.
	prefixes := make([]string, 0, 5)
	if s.cdnDomain != "" {
		prefixes = append(prefixes, "https://"+s.cdnDomain+"/")
	}
	if s.region != "" {
		// virtual-hosted-style: https://<bucket>.s3.<region>.amazonaws.com/<key>
		prefixes = append(prefixes,
			"https://"+s.bucket+".s3."+s.region+".amazonaws.com/",
			// path-style: https://s3.<region>.amazonaws.com/<bucket>/<key>
			"https://s3."+s.region+".amazonaws.com/"+s.bucket+"/",
		)
	}
	// Legacy / fallback: the buggy "https://<bucket>/<key>" form that older
	// records may still hold, plus a generic bucket-host prefix.
	prefixes = append(prefixes, "https://"+s.bucket+"/")

	for _, prefix := range prefixes {
		if strings.HasPrefix(rawURL, prefix) {
			return strings.TrimPrefix(rawURL, prefix)
		}
	}
	// Fallback: take everything after the last "/".
	if i := strings.LastIndex(rawURL, "/"); i >= 0 {
		return rawURL[i+1:]
	}
	return rawURL
}

// GetReader streams the object body back to the caller. The returned
// ReadCloser must be closed; closing it terminates the underlying HTTP
// connection to S3. A missing key surfaces as an *types.NoSuchKey error
// wrapped in the SDK's smithy wrapper — callers can use errors.As to
// distinguish "not found" from a transport failure.
func (s *S3Storage) GetReader(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, fmt.Errorf("s3 GetReader: empty key")
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 GetObject: %w", err)
	}
	return out.Body, nil
}

func (s *S3Storage) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return s.PresignGetWithContentDisposition(ctx, key, ttl, "")
}

func (s *S3Storage) PresignGetWithContentDisposition(ctx context.Context, key string, ttl time.Duration, contentDisposition string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("s3 PresignGet: empty key")
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	if contentDisposition != "" {
		input.ResponseContentDisposition = aws.String(contentDisposition)
	}
	presign, err := s.presignClient()
	if err != nil {
		return "", err
	}
	out, err := presign.PresignGetObject(ctx, input, func(opts *s3.PresignOptions) {
		opts.Expires = ttl
	})
	if err != nil {
		return "", fmt.Errorf("s3 PresignGetObject: %w", err)
	}
	return out.URL, nil
}

// PresignPut returns a URL that accepts one PUT of key within ttl. A client
// that uses it must send the same Content-Type. Returns ErrPresignUnsupported
// when the store has no client-reachable endpoint.
func (s *S3Storage) PresignPut(ctx context.Context, key string, ttl time.Duration, contentType string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("s3 PresignPut: empty key")
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	presign, err := s.presignClient()
	if err != nil {
		return "", err
	}
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	out, err := presign.PresignPutObject(ctx, input, func(opts *s3.PresignOptions) {
		opts.Expires = ttl
	})
	if err != nil {
		return "", fmt.Errorf("s3 PresignPutObject: %w", err)
	}
	return out.URL, nil
}

// presignClient returns the client that signs presigned URLs.
func (s *S3Storage) presignClient() (*s3.PresignClient, error) {
	if s.presignOff {
		return nil, ErrPresignUnsupported
	}
	if s.presign != nil {
		return s.presign, nil
	}
	return s3.NewPresignClient(s.client), nil
}

// ListPrefix returns every object whose key starts with prefix. It follows
// continuation tokens, so the result covers the whole prefix; keep prefixes
// narrow when the store is large.
func (s *S3Storage) ListPrefix(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var out []ObjectInfo
	pages := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 ListObjectsV2: %w", err)
		}
		for _, obj := range page.Contents {
			out = append(out, ObjectInfo{
				Key:          aws.ToString(obj.Key),
				Size:         aws.ToInt64(obj.Size),
				LastModified: aws.ToTime(obj.LastModified),
			})
		}
	}
	return out, nil
}

// CopyObject copies srcKey to dstKey inside the bucket, replacing dstKey. S3's
// single-request copy is limited to 5 GiB; the Hub's bundles are far below that.
func (s *S3Storage) CopyObject(ctx context.Context, srcKey, dstKey string) error {
	if srcKey == "" || dstKey == "" {
		return fmt.Errorf("s3 CopyObject: empty key")
	}
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(dstKey),
		CopySource:   aws.String(copySourceHeader(s.bucket, srcKey)),
		StorageClass: s.storageClass(),
	})
	if err != nil {
		return fmt.Errorf("s3 CopyObject: %w", err)
	}
	return nil
}

// copySourceHeader is the x-amz-copy-source value: bucket/key with each path
// segment percent-encoded. The SDK does not encode this header itself.
func copySourceHeader(bucket, key string) string {
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		segments[i] = escapeCopySourceSegment(segment)
	}
	return escapeCopySourceSegment(bucket) + "/" + strings.Join(segments, "/")
}

// escapeCopySourceSegment encodes a space as %20, not +: a literal + must stay
// %2B, because S3 decodes + in this header as a space.
func escapeCopySourceSegment(segment string) string {
	return strings.ReplaceAll(url.QueryEscape(segment), "+", "%20")
}

// EnsureBucket creates the bucket when it does not exist. S3-compatible servers
// answer "already exists" in more than one way, and both mean the bucket is
// there, so both count as success. Only meaningful for custom endpoints; AWS
// buckets are provisioned out of band.
func (s *S3Storage) EnsureBucket(ctx context.Context) error {
	_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}
	var owned *types.BucketAlreadyOwnedByYou
	var exists *types.BucketAlreadyExists
	if errors.As(err, &owned) || errors.As(err, &exists) {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "BucketAlreadyOwnedByYou", "BucketAlreadyExists":
			return nil
		}
	}
	return fmt.Errorf("s3 CreateBucket %q: %w", s.bucket, err)
}

// Delete removes an object from S3. Errors are logged but not fatal.
func (s *S3Storage) Delete(ctx context.Context, key string) {
	if err := s.DeleteObject(ctx, key); err != nil {
		slog.Error("s3 DeleteObject failed", "key", key, "error", err)
	}
}

// DeleteObject is Delete with the error surfaced — the media reconciler needs
// it to keep the ledger row and schedule a retry instead of assuming success.
func (s *S3Storage) DeleteObject(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

// ObjectURL returns the URL a successful Upload/UploadStream of key would
// return. It is a pure function of configuration, so the media intent ledger
// can persist the URL BEFORE the upload for the durable reference check.
func (s *S3Storage) ObjectURL(key string) string {
	return s.uploadedURL(key)
}

// DeleteKeys removes multiple objects from S3. Best-effort, errors are logged.
func (s *S3Storage) DeleteKeys(ctx context.Context, keys []string) {
	for _, key := range keys {
		s.Delete(ctx, key)
	}
}

func (s *S3Storage) Upload(ctx context.Context, key string, data []byte, contentType string, filename string) (string, error) {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:             aws.String(s.bucket),
		Key:                aws.String(key),
		Body:               bytes.NewReader(data),
		ContentType:        aws.String(contentType),
		ContentDisposition: aws.String(ContentDisposition(contentType, filename)),
		CacheControl:       aws.String("max-age=432000,public"),
		StorageClass:       s.storageClass(),
	}, s.uploadChecksumOptions()...)
	if err != nil {
		return "", fmt.Errorf("s3 PutObject: %w", err)
	}
	return s.uploadedURL(key), nil
}

func (s *S3Storage) UploadStream(ctx context.Context, key string, data io.Reader, sizeBytes int64, contentType string, filename string) (string, error) {
	if sizeBytes < 0 {
		return "", fmt.Errorf("s3 PutObject: content length is required for streaming upload")
	}
	input := &s3.PutObjectInput{
		Bucket:             aws.String(s.bucket),
		Key:                aws.String(key),
		Body:               data,
		ContentLength:      aws.Int64(sizeBytes),
		ContentType:        aws.String(contentType),
		ContentDisposition: aws.String(ContentDisposition(contentType, filename)),
		CacheControl:       aws.String("max-age=432000,public"),
		StorageClass:       s.storageClass(),
	}
	_, err := s.client.PutObject(ctx, input, func(opts *s3.Options) {
		// A non-seekable stream cannot be rewound for SigV4 payload hashing.
		// S3 supports UNSIGNED-PAYLOAD for authenticated requests. Avoid the
		// optional trailing-checksum path as well: it requires another rewind
		// or aws-chunked framing that S3-compatible backends handle unevenly.
		opts.APIOptions = append(opts.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
		opts.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	})
	if err != nil {
		return "", fmt.Errorf("s3 PutObject: %w", err)
	}
	return s.uploadedURL(key), nil
}

// uploadedURL returns the URL stored for client consumption after an upload.
// Priority: CDN domain > custom endpoint > AWS S3 region-qualified host. The CDN
// domain wins even when a custom endpoint is set so S3-compatible backends
// (MinIO, R2, B2, Wasabi, etc.) can be paired with a separate public-read
// domain — writes still go through the SDK with the custom endpoint; only the
// reader-facing URL changes.
//
// For the default AWS S3 case, virtual-hosted-style is preferred:
// https://<bucket>.s3.<region>.amazonaws.com/<key>. When the bucket name
// contains dots, the AWS-issued wildcard TLS certificate (`*.s3.amazonaws.com`)
// fails to validate the host, so we fall back to path-style:
// https://s3.<region>.amazonaws.com/<bucket>/<key>.
func (s *S3Storage) uploadedURL(key string) string {
	if s.cdnDomain != "" {
		return fmt.Sprintf("https://%s/%s", s.cdnDomain, key)
	}
	if s.endpointURL != "" {
		return customEndpointObjectURL(s.endpointURL, s.bucket, key, s.usePathStyle)
	}
	if s.usePathStyle || strings.Contains(s.bucket, ".") {
		return fmt.Sprintf("https://s3.%s.amazonaws.com/%s/%s", s.region, s.bucket, key)
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, key)
}

func customEndpointObjectURL(endpointURL, bucket, key string, usePathStyle bool) string {
	return customEndpointObjectPrefix(endpointURL, bucket, usePathStyle) + key
}

func customEndpointObjectPrefix(endpointURL, bucket string, usePathStyle bool) string {
	trimmed := strings.TrimRight(endpointURL, "/")
	if usePathStyle {
		return trimmed + "/" + bucket + "/"
	}

	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return trimmed + "/"
	}
	u.Host = bucket + "." + u.Host
	u.Path = strings.TrimRight(u.Path, "/") + "/"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
