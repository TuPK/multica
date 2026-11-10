package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Compile-time checks that each backend provides the optional capabilities.
var (
	_ Lister            = (*LocalStorage)(nil)
	_ Copier            = (*LocalStorage)(nil)
	_ PresignPutter     = (*LocalStorage)(nil)
	_ Lister            = (*S3Storage)(nil)
	_ Copier            = (*S3Storage)(nil)
	_ PresignPutter     = (*S3Storage)(nil)
	_ Presigner         = (*S3Storage)(nil)
	_ DownloadPresigner = (*S3Storage)(nil)
)

// ListPrefix walks the directory that holds prefix and returns the objects whose
// keys start with it, sorted by key. Internal files (sidecars and staging files)
// are skipped, as GetReader skips them.
func (s *LocalStorage) ListPrefix(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	dirPart := ""
	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		dirPart = prefix[:i+1]
	}
	root := filepath.Join(s.uploadDir, filepath.FromSlash(dirPart))
	if !isUnder(s.uploadDir, root) {
		return nil, fmt.Errorf("local ListPrefix: prefix escapes upload dir: %q", prefix)
	}
	var out []ObjectInfo
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.uploadDir, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !strings.HasPrefix(key, prefix) || isInternalLocalPath(key) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, ObjectInfo{Key: key, Size: info.Size(), LastModified: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("local ListPrefix: %w", err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// CopyObject copies srcKey to dstKey, replacing dstKey. The filename sidecar
// follows the object, as S3's copy carries the object's metadata; a source with
// no sidecar leaves no sidecar on the destination.
func (s *LocalStorage) CopyObject(ctx context.Context, srcKey, dstKey string) error {
	if srcKey == "" || dstKey == "" {
		return fmt.Errorf("local CopyObject: empty key")
	}
	if isInternalLocalPath(dstKey) {
		return fmt.Errorf("local CopyObject: refusing to write internal key %q", dstKey)
	}
	dst := filepath.Join(s.uploadDir, dstKey)
	if !isUnder(s.uploadDir, dst) {
		return fmt.Errorf("local CopyObject: key escapes upload dir: %q", dstKey)
	}
	src, err := s.GetReader(ctx, srcKey)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := writeAtomic(dst, src); err != nil {
		return err
	}

	srcMeta, err := os.ReadFile(filepath.Join(s.uploadDir, srcKey) + metaSuffix)
	switch {
	case err == nil:
		if err := os.WriteFile(dst+metaSuffix, srcMeta, 0644); err != nil {
			return fmt.Errorf("local CopyObject meta: %w", err)
		}
	case errors.Is(err, fs.ErrNotExist):
		if err := os.Remove(dst + metaSuffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("local CopyObject meta: %w", err)
		}
	default:
		return fmt.Errorf("local CopyObject meta: %w", err)
	}
	return nil
}

// PresignPut is not available on local disk: there is no URL a client can PUT to.
func (s *LocalStorage) PresignPut(_ context.Context, _ string, _ time.Duration, _ string) (string, error) {
	return "", ErrPresignUnsupported
}
