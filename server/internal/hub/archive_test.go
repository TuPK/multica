package hub

import (
	"bytes"
	"errors"
	"io/fs"
	"math/rand"
	"strings"
	"testing"
)

func TestExtractZipRefusesUnsafeEntries(t *testing.T) {
	cases := []struct {
		name  string
		entry zipSpec
		want  error
	}{
		{"parent traversal", zipSpec{name: "../evil.md", body: []byte("x")}, ErrUnsafePath},
		{"nested traversal", zipSpec{name: "a/../../evil.md", body: []byte("x")}, ErrUnsafePath},
		{"traversal through valid dir", zipSpec{name: "a/../b.md", body: []byte("x")}, ErrUnsafePath},
		{"absolute unix path", zipSpec{name: "/etc/evil.md", body: []byte("x")}, ErrUnsafePath},
		{"backslash traversal", zipSpec{name: `..\evil.md`, body: []byte("x")}, ErrUnsafePath},
		{"backslash separator", zipSpec{name: `a\b.md`, body: []byte("x")}, ErrUnsafePath},
		{"windows drive letter", zipSpec{name: "C:/evil.md", body: []byte("x")}, ErrUnsafePath},
		{"drive-relative", zipSpec{name: "C:evil.md", body: []byte("x")}, ErrUnsafePath},
		{"alternate data stream", zipSpec{name: "SKILL.md:stream", body: []byte("x")}, ErrUnsafePath},
		{"dot segment", zipSpec{name: "a/./b.md", body: []byte("x")}, ErrUnsafePath},
		{"empty segment", zipSpec{name: "a//b.md", body: []byte("x")}, ErrUnsafePath},
		{"NUL in name", zipSpec{name: "evil\x00.md", body: []byte("x")}, ErrUnsafePath},
		{"control char in name", zipSpec{name: "a\x01b.md", body: []byte("x")}, ErrUnsafePath},
		{"traversal directory entry", zipSpec{name: "../", body: nil}, ErrUnsafePath},
		{"symlink", zipSpec{name: "link", body: []byte("/etc/passwd"), mode: fs.ModeSymlink | 0o777}, ErrUnsafePath},
		{"named pipe", zipSpec{name: "pipe", mode: fs.ModeNamedPipe | 0o644}, ErrUnsafePath},
		{"symlink inside junk dir", zipSpec{name: "__MACOSX/link", body: []byte("x"), mode: fs.ModeSymlink | 0o777}, ErrUnsafePath},
		{"traversal inside junk dir", zipSpec{name: "__MACOSX/../evil", body: []byte("x")}, ErrUnsafePath},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := buildZip(t, zipSpec{name: "SKILL.md", body: []byte("---\nname: ok\n---\n")}, tc.entry)
			_, err := extractBytes(data, DefaultLimits)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want errors.Is %v", err, tc.want)
			}
		})
	}
}

func TestExtractZipLimits(t *testing.T) {
	small := Limits{MaxFiles: 2, MaxFileBytes: 16, MaxTotalBytes: 40, MaxArchiveBytes: 1 << 20, MaxCompressionRatio: 100}
	cases := []struct {
		name  string
		specs []zipSpec
		lim   Limits
		want  error
	}{
		{
			name:  "file count over limit",
			specs: []zipSpec{{name: "a", body: []byte("1")}, {name: "b", body: []byte("2")}, {name: "c", body: []byte("3")}},
			lim:   small,
			want:  ErrTooManyFiles,
		},
		{
			name: "raw central directory over factor",
			specs: []zipSpec{
				{name: "a", body: []byte("1")}, {name: "b", body: []byte("2")}, {name: "c", body: []byte("3")},
				{name: "d", body: []byte("4")}, {name: "e", body: []byte("5")},
			},
			lim:  Limits{MaxFiles: 1, MaxFileBytes: 16, MaxTotalBytes: 100, MaxArchiveBytes: 1 << 20, MaxCompressionRatio: 100},
			want: ErrTooManyFiles,
		},
		{
			name:  "single file over per-file cap",
			specs: []zipSpec{{name: "big.md", body: bytes.Repeat([]byte("x"), 17)}},
			lim:   small,
			want:  ErrTooLarge,
		},
		{
			name:  "total over cap",
			specs: []zipSpec{{name: "a", body: bytes.Repeat([]byte("a"), 16)}, {name: "b", body: bytes.Repeat([]byte("b"), 16)}, {name: "c", body: bytes.Repeat([]byte("c"), 16)}},
			lim:   Limits{MaxFiles: 10, MaxFileBytes: 16, MaxTotalBytes: 40, MaxArchiveBytes: 1 << 20, MaxCompressionRatio: 100},
			want:  ErrTooLarge,
		},
		{
			name:  "archive bytes over cap",
			specs: []zipSpec{{name: "a", body: []byte("1")}},
			lim:   Limits{MaxFiles: 10, MaxFileBytes: 16, MaxTotalBytes: 40, MaxArchiveBytes: 10, MaxCompressionRatio: 100},
			want:  ErrTooLarge,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := extractBytes(buildZip(t, tc.specs...), tc.lim)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want errors.Is %v", err, tc.want)
			}
		})
	}
}

func TestExtractZipDuplicatePath(t *testing.T) {
	data := buildZip(t, zipSpec{name: "SKILL.md", body: []byte("a")}, zipSpec{name: "SKILL.md", body: []byte("b")})
	if _, err := extractBytes(data, DefaultLimits); !errors.Is(err, ErrDuplicatePath) {
		t.Fatalf("err = %v, want ErrDuplicatePath", err)
	}
}

func TestExtractZipBomb(t *testing.T) {
	// 900 KiB of zeros deflates to about 1 KiB, a ratio far above the default cap of 100.
	// Under the 1 MiB per-file cap, so only the ratio check can refuse it.
	bomb := buildZip(t, zipSpec{name: "SKILL.md", body: make([]byte, 900<<10)})
	if _, err := extractBytes(bomb, DefaultLimits); !errors.Is(err, ErrBomb) {
		t.Fatalf("bomb err = %v, want ErrBomb", err)
	}

	// Random bytes do not compress, so a large but honest file is accepted.
	noise := make([]byte, 200<<10)
	rand.New(rand.NewSource(1)).Read(noise)
	ok := buildZip(t, zipSpec{name: "data.bin", body: noise})
	if _, err := extractBytes(ok, DefaultLimits); err != nil {
		t.Fatalf("honest large file rejected: %v", err)
	}
}

func TestExtractZipInvalidArchive(t *testing.T) {
	if _, err := extractBytes([]byte("not a zip at all"), DefaultLimits); !errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("err = %v, want ErrInvalidArchive", err)
	}
}

func TestExtractZipStripsSingleTopDirAndDropsJunk(t *testing.T) {
	cases := []struct {
		name  string
		specs []zipSpec
		want  map[string]string
	}{
		{
			name: "single top directory is stripped",
			specs: []zipSpec{
				{name: "repo-abc/", body: nil},
				{name: "repo-abc/SKILL.md", body: []byte("skill")},
				{name: "repo-abc/scripts/run.sh", body: []byte("echo")},
			},
			want: map[string]string{"SKILL.md": "skill", "scripts/run.sh": "echo"},
		},
		{
			name: "root file prevents stripping",
			specs: []zipSpec{
				{name: "SKILL.md", body: []byte("root")},
				{name: "docs/guide.md", body: []byte("g")},
			},
			want: map[string]string{"SKILL.md": "root", "docs/guide.md": "g"},
		},
		{
			name: "two top directories are not stripped",
			specs: []zipSpec{
				{name: "a/SKILL.md", body: []byte("a")},
				{name: "b/SKILL.md", body: []byte("b")},
			},
			want: map[string]string{"a/SKILL.md": "a", "b/SKILL.md": "b"},
		},
		{
			name: "a zip whose only folder is agents/ keeps it: it is content, not a wrapper",
			specs: []zipSpec{
				{name: "agents/", body: nil},
				{name: "agents/writer.md", body: []byte("w")},
				{name: "agents/editor.md", body: []byte("e")},
			},
			want: map[string]string{"agents/writer.md": "w", "agents/editor.md": "e"},
		},
		{
			name: "skills/ alone is kept too",
			specs: []zipSpec{
				{name: "skills/style/SKILL.md", body: []byte("s")},
				{name: "skills/style/references/a.md", body: []byte("a")},
			},
			want: map[string]string{"skills/style/SKILL.md": "s", "skills/style/references/a.md": "a"},
		},
		{
			name: "docs/ alone is kept",
			specs: []zipSpec{
				{name: "docs/a.md", body: []byte("a")},
				{name: "docs/b/c.md", body: []byte("c")},
			},
			want: map[string]string{"docs/a.md": "a", "docs/b/c.md": "c"},
		},
		{
			name:  "workflows/ alone is kept",
			specs: []zipSpec{{name: "workflows/flow.md", body: []byte("f")}},
			want:  map[string]string{"workflows/flow.md": "f"},
		},
		{
			name:  "references/ alone is kept",
			specs: []zipSpec{{name: "references/r.md", body: []byte("r")}},
			want:  map[string]string{"references/r.md": "r"},
		},
		{
			name: "a wrapper of any other name is stripped once, and the agents/ inside it stays",
			specs: []zipSpec{
				{name: "writing-pack-main/agents/writer.md", body: []byte("w")},
				{name: "writing-pack-main/agents/editor.md", body: []byte("e")},
			},
			want: map[string]string{"agents/writer.md": "w", "agents/editor.md": "e"},
		},
		{
			name:  "a wrapper that is named like a content folder but differs in case is a wrapper",
			specs: []zipSpec{{name: "Agents/SKILL.md", body: []byte("s")}},
			want:  map[string]string{"SKILL.md": "s"},
		},
		{
			name: "macOS and finder junk is dropped before stripping",
			specs: []zipSpec{
				{name: "__MACOSX/repo/._SKILL.md", body: []byte("resource fork")},
				{name: "repo/.DS_Store", body: []byte("finder")},
				{name: "repo/Thumbs.db", body: []byte("thumbs")},
				{name: "repo/._hidden", body: []byte("appledouble")},
				{name: "repo/SKILL.md", body: []byte("skill")},
			},
			want: map[string]string{"SKILL.md": "skill"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := extractBytes(buildZip(t, tc.specs...), DefaultLimits)
			if err != nil {
				t.Fatalf("extract: %v", err)
			}
			got := fileMap(b)
			if len(got) != len(tc.want) {
				t.Fatalf("files = %v, want %v", got, tc.want)
			}
			for p, c := range tc.want {
				if got[p] != c {
					t.Fatalf("file %q = %q, want %q (all: %v)", p, got[p], c, got)
				}
			}
			var total int64
			for _, f := range b.Files {
				total += f.Size
				if f.Size != int64(len(f.Content)) {
					t.Fatalf("size %d != len %d for %q", f.Size, len(f.Content), f.Path)
				}
			}
			if total != b.TotalSize {
				t.Fatalf("TotalSize = %d, want %d", b.TotalSize, total)
			}
		})
	}
}

func TestBuildZipDeterministicAndRoundTrips(t *testing.T) {
	files := map[string]string{
		"SKILL.md":         "---\nname: deploy\ndescription: ship it\n---\nbody",
		"scripts/run.sh":   "echo hi\n",
		"docs/notes.md":    "notes",
		"assets/empty.txt": "",
	}
	first, err := BuildZip(bundleOf(files))
	if err != nil {
		t.Fatalf("BuildZip: %v", err)
	}
	// Same content, built again from a differently ordered bundle.
	reordered := &Bundle{}
	for _, p := range []string{"docs/notes.md", "SKILL.md", "assets/empty.txt", "scripts/run.sh"} {
		reordered.Files = append(reordered.Files, FileEntry{Path: p, Size: int64(len(files[p])), Content: []byte(files[p])})
	}
	second, err := BuildZip(reordered)
	if err != nil {
		t.Fatalf("BuildZip reordered: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("BuildZip output depends on input order")
	}

	back, err := extractBytes(first, DefaultLimits)
	if err != nil {
		t.Fatalf("round trip extract: %v", err)
	}
	if got := fileMap(back); len(got) != len(files) {
		t.Fatalf("round trip files = %v", got)
	}
	if Digest(back) != Digest(bundleOf(files)) {
		t.Fatal("digest changed across BuildZip/ExtractZip round trip")
	}
}

func TestBuildZipRejectsUnsafeAndDuplicatePaths(t *testing.T) {
	for _, p := range []string{"../x", "/x", "a\\b", "a/../b", "C:x"} {
		if _, err := BuildZip(bundleOf(map[string]string{p: "x"})); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("BuildZip(%q) err = %v, want ErrUnsafePath", p, err)
		}
	}
	dup := &Bundle{Files: []FileEntry{{Path: "a", Content: []byte("1")}, {Path: "a", Content: []byte("2")}}}
	if _, err := BuildZip(dup); !errors.Is(err, ErrDuplicatePath) {
		t.Fatalf("duplicate err = %v, want ErrDuplicatePath", err)
	}
}

func TestDigest(t *testing.T) {
	base := bundleOf(map[string]string{"SKILL.md": "a", "x.md": "b"})
	same := &Bundle{Files: []FileEntry{
		{Path: "x.md", Content: []byte("b")},
		{Path: "SKILL.md", Content: []byte("a")},
	}}
	if Digest(base) != Digest(same) {
		t.Fatal("digest depends on file order")
	}

	cases := []struct {
		name string
		b    *Bundle
	}{
		{"content changed", bundleOf(map[string]string{"SKILL.md": "A", "x.md": "b"})},
		{"path changed", bundleOf(map[string]string{"SKILL.md": "a", "y.md": "b"})},
		{"extra file", bundleOf(map[string]string{"SKILL.md": "a", "x.md": "b", "z": ""})},
	}
	for _, tc := range cases {
		if Digest(tc.b) == Digest(base) {
			t.Errorf("%s: digest unchanged", tc.name)
		}
	}

	// Length framing: moving a byte between path and content must not collide.
	a := bundleOf(map[string]string{"ab": "c"})
	b := bundleOf(map[string]string{"a": "bc"})
	if Digest(a) == Digest(b) {
		t.Fatal("digest framing is ambiguous across path/content boundary")
	}
	if len(Digest(base)) != 64 || strings.ToLower(Digest(base)) != Digest(base) {
		t.Fatalf("digest is not lowercase hex sha256: %q", Digest(base))
	}
}
