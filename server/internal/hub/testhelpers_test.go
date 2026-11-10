package hub

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"testing"
)

// zipSpec is one entry of a hand-built test archive. mode is optional and lets a
// test create symlinks and other special entries that zip.Writer itself never makes.
type zipSpec struct {
	name string
	body []byte
	mode fs.FileMode
}

// buildZip writes entries exactly as given, including names that archive/zip
// would never produce itself (traversal, absolute, backslash), since those are the
// inputs the extractor must refuse.
func buildZip(t *testing.T, specs ...zipSpec) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, s := range specs {
		hdr := &zip.FileHeader{Name: s.name, Method: zip.Deflate}
		if s.mode != 0 {
			hdr.SetMode(s.mode)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("create %q: %v", s.name, err)
		}
		if _, err := w.Write(s.body); err != nil {
			t.Fatalf("write %q: %v", s.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func extractBytes(data []byte, lim Limits) (*Bundle, error) {
	return ExtractZip(bytes.NewReader(data), int64(len(data)), lim)
}

// fileMap returns the bundle as path -> content for easy comparison.
func fileMap(b *Bundle) map[string]string {
	out := make(map[string]string, len(b.Files))
	for _, f := range b.Files {
		out[f.Path] = string(f.Content)
	}
	return out
}

func bundleOf(files map[string]string) *Bundle {
	b := &Bundle{}
	for p, c := range files {
		b.Files = append(b.Files, FileEntry{Path: p, Size: int64(len(c)), Content: []byte(c)})
		b.TotalSize += int64(len(c))
	}
	return b
}
