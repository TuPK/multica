package hub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestParseGitHubURL(t *testing.T) {
	cases := []struct {
		name                     string
		raw                      string
		owner, repo, ref, subdir string
		wantErr                  error
	}{
		{name: "repo root", raw: "https://github.com/acme/skills", owner: "acme", repo: "skills"},
		{name: "trailing slash and .git", raw: "https://github.com/acme/skills.git/", owner: "acme", repo: "skills"},
		{name: "tree with ref", raw: "https://github.com/acme/skills/tree/v2", owner: "acme", repo: "skills", ref: "v2"},
		{name: "tree with ref and subdir", raw: "https://github.com/acme/skills/tree/main/skills/deploy", owner: "acme", repo: "skills", ref: "main", subdir: "skills/deploy"},
		{name: "http scheme accepted", raw: "http://github.com/acme/skills", owner: "acme", repo: "skills"},
		{name: "uppercase host", raw: "https://GitHub.com/acme/skills", owner: "acme", repo: "skills"},
		{name: "other host", raw: "https://gitlab.com/acme/skills", wantErr: ErrBadURL},
		{name: "suffix host trick", raw: "https://github.com.evil.example/acme/skills", wantErr: ErrBadURL},
		{name: "userinfo", raw: "https://user@github.com/acme/skills", wantErr: ErrBadURL},
		{name: "ftp scheme", raw: "ftp://github.com/acme/skills", wantErr: ErrBadURL},
		{name: "owner only", raw: "https://github.com/acme", wantErr: ErrBadURL},
		{name: "owner with leading dash", raw: "https://github.com/-acme/skills", wantErr: ErrBadURL},
		{name: "repo dot-dot", raw: "https://github.com/acme/..", wantErr: ErrBadURL},
		{name: "repo with slash-escape", raw: "https://github.com/acme/a%2Fb", wantErr: ErrBadURL},
		{name: "unsupported path form", raw: "https://github.com/acme/skills/issues/3", wantErr: ErrBadURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, ref, subdir, err := parseGitHubURL(tc.raw)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if owner != tc.owner || repo != tc.repo || ref != tc.ref || subdir != tc.subdir {
				t.Fatalf("got %q %q %q %q, want %q %q %q %q", owner, repo, ref, subdir, tc.owner, tc.repo, tc.ref, tc.subdir)
			}
		})
	}
}

func TestValidateGitRef(t *testing.T) {
	ok := []string{"main", "feature/login", "v1.2.3", "release-2026_05", "HEAD"}
	bad := []string{"../etc", "a/../b", "-rf", "a//b", "a/./b", "a b", "a?x=1", "a#frag", "", "a/-b", "a\\b"}
	for _, r := range ok {
		if err := validateGitRef(r); err != nil {
			t.Errorf("validateGitRef(%q) = %v, want nil", r, err)
		}
	}
	for _, r := range bad {
		if err := validateGitRef(r); !errors.Is(err, ErrBadURL) {
			t.Errorf("validateGitRef(%q) = %v, want ErrBadURL", r, err)
		}
	}
}

// repoZip is a GitHub-style archive: one top-level directory named repo-<sha>.
func repoZip(t *testing.T) []byte {
	t.Helper()
	return buildZip(t,
		zipSpec{name: "skills-abc123/", body: nil},
		zipSpec{name: "skills-abc123/skills/deploy/SKILL.md", body: []byte("---\nname: deploy\n---\nbody")},
		zipSpec{name: "skills-abc123/skills/deploy/scripts/run.sh", body: []byte("echo deploy")},
		zipSpec{name: "skills-abc123/README.md", body: []byte("root readme")},
	)
}

// codeloadServer serves zip archives at the paths codeload uses, and records
// every requested path so tests can check which candidates were tried.
type codeloadServer struct {
	*httptest.Server
	mu    sync.Mutex
	paths []string
}

func newCodeloadServer(t *testing.T, routes map[string][]byte) *codeloadServer {
	t.Helper()
	cs := &codeloadServer{}
	cs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.mu.Lock()
		cs.paths = append(cs.paths, r.URL.Path)
		cs.mu.Unlock()
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(body)
	}))
	t.Cleanup(cs.Close)
	return cs
}

func (cs *codeloadServer) requested() []string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return append([]string(nil), cs.paths...)
}

func githubImporterFor(base string) *GitHubImporter {
	return &GitHubImporter{codeloadBase: base, lim: DefaultLimits}
}

func TestGitHubImporter(t *testing.T) {
	archive := repoZip(t)
	t.Run("default ref falls through HEAD to main", func(t *testing.T) {
		cs := newCodeloadServer(t, map[string][]byte{
			"/acme/skills/zip/refs/heads/main": archive,
		})
		f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
		b, meta, err := githubImporterFor(cs.URL).Fetch(context.Background(), f,
			SourceRef{Type: SourceGitHub, URL: "https://github.com/acme/skills", Subdir: "skills/deploy"})
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if got := fileMap(b); got["SKILL.md"] == "" || got["scripts/run.sh"] != "echo deploy" || len(got) != 2 {
			t.Fatalf("subdir files = %v", got)
		}
		if meta.SourceRef != "refs/heads/main" || meta.SourceURL != "https://github.com/acme/skills" {
			t.Fatalf("meta = %+v", meta)
		}
		if meta.Digest != Digest(b) {
			t.Fatal("meta digest does not match bundle")
		}
		if reqs := cs.requested(); len(reqs) != 2 || reqs[0] != "/acme/skills/zip/HEAD" {
			t.Fatalf("candidate order = %v, want HEAD then main", reqs)
		}
	})

	t.Run("falls through to master", func(t *testing.T) {
		cs := newCodeloadServer(t, map[string][]byte{
			"/acme/skills/zip/refs/heads/master": archive,
		})
		f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
		_, meta, err := githubImporterFor(cs.URL).Fetch(context.Background(), f,
			SourceRef{Type: SourceGitHub, URL: "https://github.com/acme/skills"})
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if meta.SourceRef != "refs/heads/master" {
			t.Fatalf("SourceRef = %q", meta.SourceRef)
		}
	})

	t.Run("tree URL supplies ref and subdir", func(t *testing.T) {
		cs := newCodeloadServer(t, map[string][]byte{"/acme/skills/zip/v2": archive})
		f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
		b, _, err := githubImporterFor(cs.URL).Fetch(context.Background(), f,
			SourceRef{Type: SourceGitHub, URL: "https://github.com/acme/skills/tree/v2/skills/deploy"})
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if len(b.Files) != 2 {
			t.Fatalf("files = %v", fileMap(b))
		}
	})

	t.Run("explicit ref does not fall back", func(t *testing.T) {
		cs := newCodeloadServer(t, map[string][]byte{"/acme/skills/zip/refs/heads/main": archive})
		f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
		_, _, err := githubImporterFor(cs.URL).Fetch(context.Background(), f,
			SourceRef{Type: SourceGitHub, URL: "https://github.com/acme/skills", Ref: "nope"})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
		if reqs := cs.requested(); len(reqs) != 1 {
			t.Fatalf("explicit ref made %d requests, want 1: %v", len(reqs), reqs)
		}
	})

	t.Run("no candidate exists", func(t *testing.T) {
		cs := newCodeloadServer(t, map[string][]byte{})
		f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
		_, _, err := githubImporterFor(cs.URL).Fetch(context.Background(), f,
			SourceRef{Type: SourceGitHub, URL: "https://github.com/acme/skills"})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("missing subdir", func(t *testing.T) {
		cs := newCodeloadServer(t, map[string][]byte{"/acme/skills/zip/refs/heads/main": archive})
		f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
		_, _, err := githubImporterFor(cs.URL).Fetch(context.Background(), f,
			SourceRef{Type: SourceGitHub, URL: "https://github.com/acme/skills", Subdir: "skills/absent"})
		if !errors.Is(err, ErrSubdirNotFound) {
			t.Fatalf("err = %v, want ErrSubdirNotFound", err)
		}
	})

	t.Run("subdir traversal refused", func(t *testing.T) {
		cs := newCodeloadServer(t, map[string][]byte{"/acme/skills/zip/refs/heads/main": archive})
		f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
		_, _, err := githubImporterFor(cs.URL).Fetch(context.Background(), f,
			SourceRef{Type: SourceGitHub, URL: "https://github.com/acme/skills", Subdir: "../skills"})
		if !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("err = %v, want ErrUnsafePath", err)
		}
	})

	t.Run("checksum", func(t *testing.T) {
		sum := sha256.Sum256(archive)
		good := hex.EncodeToString(sum[:])
		cs := newCodeloadServer(t, map[string][]byte{"/acme/skills/zip/refs/heads/main": archive})
		f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
		imp := githubImporterFor(cs.URL)
		if _, _, err := imp.Fetch(context.Background(), f, SourceRef{URL: "https://github.com/acme/skills", SHA256: good}); err != nil {
			t.Fatalf("matching sha: %v", err)
		}
		_, _, err := imp.Fetch(context.Background(), f, SourceRef{URL: "https://github.com/acme/skills", SHA256: strings.Repeat("0", 64)})
		if !errors.Is(err, ErrChecksumMismatch) {
			t.Fatalf("mismatch err = %v, want ErrChecksumMismatch", err)
		}
	})

	t.Run("non-github host never fetches", func(t *testing.T) {
		cs := newCodeloadServer(t, map[string][]byte{"/acme/skills/zip/refs/heads/main": archive})
		f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})
		_, _, err := githubImporterFor(cs.URL).Fetch(context.Background(), f,
			SourceRef{Type: SourceGitHub, URL: "https://evil.example/acme/skills"})
		if !errors.Is(err, ErrBadURL) {
			t.Fatalf("err = %v, want ErrBadURL", err)
		}
		if len(cs.requested()) != 0 {
			t.Fatal("request made for a non-github host")
		}
	})
}

func TestURLZipImporter(t *testing.T) {
	archive := repoZip(t)
	sum := sha256.Sum256(archive)
	good := hex.EncodeToString(sum[:])
	cs := newCodeloadServer(t, map[string][]byte{
		"/pkg.zip":  archive,
		"/junk.zip": []byte("not a zip"),
	})
	imp := NewURLZipImporter()
	f := NewSafeFetcher(FetcherOptions{AllowPrivate: true})

	b, meta, err := imp.Fetch(context.Background(), f, SourceRef{Type: SourceURLZip, URL: cs.URL + "/pkg.zip", SHA256: strings.ToUpper(good)})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if meta.SourceURL != cs.URL+"/pkg.zip" || meta.Digest != Digest(b) {
		t.Fatalf("meta = %+v", meta)
	}

	cases := []struct {
		name    string
		ref     SourceRef
		wantErr error
	}{
		{"subdir", SourceRef{URL: cs.URL + "/pkg.zip", Subdir: "skills/deploy"}, nil},
		{"checksum mismatch", SourceRef{URL: cs.URL + "/pkg.zip", SHA256: strings.Repeat("f", 64)}, ErrChecksumMismatch},
		{"not a zip", SourceRef{URL: cs.URL + "/junk.zip"}, ErrInvalidArchive},
		{"404", SourceRef{URL: cs.URL + "/absent.zip"}, ErrNotFound},
		{"file scheme", SourceRef{URL: "file:///etc/passwd"}, ErrBadURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := imp.Fetch(context.Background(), f, tc.ref)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestURLZipImporterBlocksPrivateHost(t *testing.T) {
	f := NewSafeFetcher(FetcherOptions{})
	_, _, err := NewURLZipImporter().Fetch(context.Background(), f, SourceRef{URL: "http://169.254.169.254/latest/meta-data/x.zip"})
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v, want ErrBlockedAddress", err)
	}
}

func TestLegacyImporters(t *testing.T) {
	want := bundleOf(map[string]string{"SKILL.md": "---\nname: clawed\n---\n"})
	var gotURL string
	fn := func(_ context.Context, raw string) (*Bundle, error) {
		gotURL = raw
		return want, nil
	}
	for _, imp := range []Importer{NewClawHubImporter(fn), NewSkillsShImporter(fn)} {
		b, meta, err := imp.Fetch(context.Background(), nil, SourceRef{URL: "https://clawhub.ai/x/y"})
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if b != want || gotURL != "https://clawhub.ai/x/y" {
			t.Fatalf("legacy fetch not delegated: url=%q", gotURL)
		}
		if meta.Digest != Digest(want) || meta.SourceURL != "https://clawhub.ai/x/y" {
			t.Fatalf("meta = %+v", meta)
		}
	}

	t.Run("nil fetcher", func(t *testing.T) {
		_, _, err := NewClawHubImporter(nil).Fetch(context.Background(), nil, SourceRef{URL: "https://x"})
		if !errors.Is(err, ErrNoLegacyFetcher) {
			t.Fatalf("err = %v, want ErrNoLegacyFetcher", err)
		}
	})
	t.Run("empty URL", func(t *testing.T) {
		_, _, err := NewSkillsShImporter(fn).Fetch(context.Background(), nil, SourceRef{})
		if !errors.Is(err, ErrBadURL) {
			t.Fatalf("err = %v, want ErrBadURL", err)
		}
	})
	t.Run("nil bundle", func(t *testing.T) {
		_, _, err := NewSkillsShImporter(func(context.Context, string) (*Bundle, error) { return nil, nil }).
			Fetch(context.Background(), nil, SourceRef{URL: "https://x"})
		if !errors.Is(err, ErrInvalidArchive) {
			t.Fatalf("err = %v, want ErrInvalidArchive", err)
		}
	})
	t.Run("error propagates", func(t *testing.T) {
		sentinel := errors.New("upstream down")
		_, _, err := NewClawHubImporter(func(context.Context, string) (*Bundle, error) { return nil, sentinel }).
			Fetch(context.Background(), nil, SourceRef{URL: "https://x"})
		if !errors.Is(err, sentinel) {
			t.Fatalf("err = %v, want upstream error", err)
		}
	})
}
