package hub

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"time"
)

// bombFloor is the decompressed size below which the ratio check is skipped.
// Small highly compressible files are common and harmless; the ratio only
// matters once an entry has expanded enough to be worth refusing.
const bombFloor = 64 << 10

// maxEntryNameLen bounds one archive path. Real skill paths are far shorter.
const maxEntryNameLen = 1024

// rawEntryFactor bounds the central directory before any junk is dropped, so a
// huge directory of junk entries is refused without walking it.
const rawEntryFactor = 4

// fixedZipTime is the timestamp every entry gets in BuildZip. 1980-01-01 is the
// DOS epoch, the earliest time zip can store, so it is a safe constant.
var fixedZipTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// ExtractZip reads a zip archive into a Bundle, enforcing lim.
//
// It refuses (wrapped sentinel errors, matchable with errors.Is):
//   - symlinks, devices, named pipes and encrypted entries (ErrUnsafePath / ErrInvalidArchive)
//   - absolute paths, "..", ".", empty segments, backslashes, drive letters, colons,
//     control characters and NUL (ErrUnsafePath)
//   - two entries with the same path (ErrDuplicatePath)
//   - more entries or bytes than the limits allow (ErrTooManyFiles, ErrTooLarge)
//   - entries whose declared or actual decompressed size exceeds the compression
//     ratio (ErrBomb)
//
// Junk such as __MACOSX/, .DS_Store and AppleDouble "._" files is dropped after
// its path is validated. When every remaining file sits under one top-level
// directory, that directory is stripped.
func ExtractZip(r io.ReaderAt, size int64, lim Limits) (*Bundle, error) {
	if size > lim.MaxArchiveBytes {
		return nil, fmt.Errorf("%w: archive is %d bytes, limit %d", ErrTooLarge, size, lim.MaxArchiveBytes)
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
	}
	if len(zr.File) > lim.MaxFiles*rawEntryFactor {
		return nil, fmt.Errorf("%w: archive has %d entries", ErrTooManyFiles, len(zr.File))
	}

	type candidate struct {
		path string
		file *zip.File
	}
	var candidates []candidate
	seen := make(map[string]bool, len(zr.File))
	for _, f := range zr.File {
		if f.Flags&0x1 != 0 {
			return nil, fmt.Errorf("%w: encrypted entry %q", ErrInvalidArchive, f.Name)
		}
		mode := f.Mode()
		if mode&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: symlink entry %q", ErrUnsafePath, f.Name)
		}
		if mode&(fs.ModeNamedPipe|fs.ModeSocket|fs.ModeDevice|fs.ModeCharDevice|fs.ModeIrregular) != 0 {
			return nil, fmt.Errorf("%w: special file entry %q", ErrUnsafePath, f.Name)
		}
		isDir := mode.IsDir() || strings.HasSuffix(f.Name, "/")
		p, err := normalizeEntryName(strings.TrimSuffix(f.Name, "/"))
		if err != nil {
			return nil, err
		}
		if isJunk(p) || isDir {
			continue
		}
		if seen[p] {
			return nil, fmt.Errorf("%w: %q", ErrDuplicatePath, p)
		}
		seen[p] = true
		candidates = append(candidates, candidate{path: p, file: f})
	}

	if len(candidates) > lim.MaxFiles {
		return nil, fmt.Errorf("%w: %d files, limit %d", ErrTooManyFiles, len(candidates), lim.MaxFiles)
	}

	// Declared sizes are checked before any decompression so an archive that
	// announces an oversized entry is refused without being read.
	var declaredTotal uint64
	for _, c := range candidates {
		u := c.file.UncompressedSize64
		if u > uint64(lim.MaxFileBytes) {
			return nil, fmt.Errorf("%w: %q declares %d bytes, per-file limit %d", ErrTooLarge, c.path, u, lim.MaxFileBytes)
		}
		declaredTotal += u
		if declaredTotal > uint64(lim.MaxTotalBytes) {
			return nil, fmt.Errorf("%w: declared total exceeds %d bytes", ErrTooLarge, lim.MaxTotalBytes)
		}
		if u > bombFloor && (c.file.CompressedSize64 == 0 || u/c.file.CompressedSize64 > uint64(lim.MaxCompressionRatio)) {
			return nil, fmt.Errorf("%w: %q declares %d bytes from %d compressed", ErrBomb, c.path, u, c.file.CompressedSize64)
		}
	}

	paths := make([]string, len(candidates))
	for i, c := range candidates {
		paths[i] = c.path
	}
	top := commonTopDir(paths)

	bundle := &Bundle{Files: make([]FileEntry, 0, len(candidates))}
	for _, c := range candidates {
		content, err := readEntry(c.file, c.path, lim)
		if err != nil {
			return nil, err
		}
		bundle.TotalSize += int64(len(content))
		if bundle.TotalSize > lim.MaxTotalBytes {
			return nil, fmt.Errorf("%w: archive expands beyond %d bytes", ErrTooLarge, lim.MaxTotalBytes)
		}
		name := c.path
		if top != "" {
			name = strings.TrimPrefix(name, top+"/")
		}
		bundle.Files = append(bundle.Files, FileEntry{Path: name, Size: int64(len(content)), Content: content})
	}
	sort.Slice(bundle.Files, func(i, j int) bool { return bundle.Files[i].Path < bundle.Files[j].Path })
	return bundle, nil
}

// readEntry decompresses one entry, stopping as soon as it exceeds the
// per-file limit or the compression ratio. The ratio is judged on the bytes
// actually produced, so a lying central directory cannot hide a bomb.
func readEntry(f *zip.File, name string, lim Limits) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %v", ErrInvalidArchive, name, err)
	}
	defer rc.Close()

	cr := &countingReader{
		r:          io.LimitReader(rc, lim.MaxFileBytes+1),
		compressed: f.CompressedSize64,
		maxRatio:   int64(lim.MaxCompressionRatio),
		name:       name,
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, cr); err != nil {
		return nil, err
	}
	if int64(buf.Len()) > lim.MaxFileBytes {
		return nil, fmt.Errorf("%w: %q exceeds %d bytes", ErrTooLarge, name, lim.MaxFileBytes)
	}
	return buf.Bytes(), nil
}

// countingReader counts decompressed bytes and fails once they exceed the
// compression ratio. A zero compressed size with more than bombFloor bytes is
// treated as an unbounded ratio.
type countingReader struct {
	r          io.Reader
	n          int64
	compressed uint64
	maxRatio   int64
	name       string
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.n > bombFloor && (c.compressed == 0 || c.n/int64(c.compressed) > c.maxRatio) {
		return n, fmt.Errorf("%w: %q expanded to %d+ bytes from %d compressed", ErrBomb, c.name, c.n, c.compressed)
	}
	return n, err
}

// normalizeEntryName validates one archive path and returns it unchanged.
// Callers strip a trailing slash for directory entries first.
func normalizeEntryName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: empty path", ErrUnsafePath)
	}
	if len(name) > maxEntryNameLen {
		return "", fmt.Errorf("%w: path longer than %d bytes", ErrUnsafePath, maxEntryNameLen)
	}
	if strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("%w: absolute path %q", ErrUnsafePath, name)
	}
	if strings.ContainsAny(name, "\\:") {
		return "", fmt.Errorf("%w: %q contains a backslash or colon", ErrUnsafePath, name)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%w: %q contains a control character", ErrUnsafePath, name)
		}
	}
	for _, seg := range strings.Split(name, "/") {
		switch seg {
		case "", ".", "..":
			return "", fmt.Errorf("%w: %q has an empty, \".\" or \"..\" segment", ErrUnsafePath, name)
		}
	}
	return name, nil
}

// isJunk reports archive entries that tools add but a bundle never needs.
func isJunk(p string) bool {
	segs := strings.Split(p, "/")
	for _, seg := range segs {
		if seg == "__MACOSX" {
			return true
		}
	}
	base := segs[len(segs)-1]
	return base == ".DS_Store" || base == "Thumbs.db" || strings.HasPrefix(base, "._")
}

// commonTopDir returns the single directory every path lives under, or "" when
// any path is at the root or the directories differ.
func commonTopDir(paths []string) string {
	top := ""
	for _, p := range paths {
		i := strings.IndexByte(p, '/')
		if i < 0 {
			return ""
		}
		dir := p[:i]
		if top == "" {
			top = dir
		} else if dir != top {
			return ""
		}
	}
	return top
}

// BuildZip writes a bundle as a zip archive. Output is deterministic: entries
// are sorted by path, carry a fixed timestamp and mode, and use one compression
// level, so the same bundle always produces the same bytes.
func BuildZip(b *Bundle) ([]byte, error) {
	order := make([]int, len(b.Files))
	for i := range order {
		order[i] = i
	}
	seen := make(map[string]bool, len(b.Files))
	for _, f := range b.Files {
		if _, err := normalizeEntryName(f.Path); err != nil {
			return nil, err
		}
		if seen[f.Path] {
			return nil, fmt.Errorf("%w: %q", ErrDuplicatePath, f.Path)
		}
		seen[f.Path] = true
	}
	sort.Slice(order, func(i, j int) bool { return b.Files[order[i]].Path < b.Files[order[j]].Path })

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, i := range order {
		f := b.Files[i]
		hdr := &zip.FileHeader{Name: f.Path, Method: zip.Deflate, Modified: fixedZipTime}
		hdr.SetMode(0o644)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return nil, fmt.Errorf("hub: write zip entry %q: %w", f.Path, err)
		}
		if _, err := w.Write(f.Content); err != nil {
			return nil, fmt.Errorf("hub: write zip entry %q: %w", f.Path, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("hub: close zip: %w", err)
	}
	return buf.Bytes(), nil
}

// Digest returns the hex SHA-256 of a bundle. Each file contributes its path
// and content, length-prefixed so that no two distinct bundles frame to the
// same byte stream. Files are sorted by path, so input order does not matter.
func Digest(b *Bundle) string {
	order := make([]int, len(b.Files))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return b.Files[order[i]].Path < b.Files[order[j]].Path })

	h := sha256.New()
	for _, i := range order {
		f := b.Files[i]
		fmt.Fprintf(h, "%d:%s", len(f.Path), f.Path)
		fmt.Fprintf(h, "%d:", len(f.Content))
		h.Write(f.Content)
	}
	return hex.EncodeToString(h.Sum(nil))
}
