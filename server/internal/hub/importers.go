package hub

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// SourceRef describes where to fetch an item from. Ref and Subdir are optional;
// SHA256 is an optional expected digest of the downloaded archive. Credential is
// the saved source's token, if it has one; its Host names the host it may go to.
type SourceRef struct {
	Type       SourceType
	URL        string
	Ref        string
	Subdir     string
	SHA256     string
	Credential Credential
}

// SourceMeta is provenance recorded with an imported item. SourceRef holds the
// ref that was actually fetched (for GitHub, the first candidate that existed).
// Digest is Digest of the resulting bundle.
type SourceMeta struct {
	SourceURL string
	SourceRef string
	Digest    string
}

// Importer turns a SourceRef into a bundle. Implementations fetch through the
// given SafeFetcher; the legacy importers ignore it because they bring their own
// client.
type Importer interface {
	Fetch(ctx context.Context, f *SafeFetcher, ref SourceRef) (*Bundle, SourceMeta, error)
}

var (
	githubOwnerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	githubRepoPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	gitRefPattern      = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,255}$`)
)

// GitHubCodeloadBase is where GitHub serves repository zips. It is a variable so
// handler tests can point the importer at an httptest origin. Production never
// changes it. NewGitHubImporter reads it when the importer is built.
var GitHubCodeloadBase = "https://codeload.github.com"

// GitHubImporter downloads a repository zip from codeload.github.com.
type GitHubImporter struct {
	codeloadBase string
	lim          Limits
}

// NewGitHubImporter returns an importer for github.com repository URLs of the
// form https://github.com/{owner}/{repo}[/tree/{ref}[/{subdir}]].
func NewGitHubImporter() Importer {
	return &GitHubImporter{codeloadBase: GitHubCodeloadBase, lim: DefaultLimits}
}

// Fetch downloads the repository archive, verifies SHA256 when given, extracts
// it and narrows it to the subdirectory if one is set.
//
// When no ref is given it tries HEAD, then refs/heads/main, then
// refs/heads/master, so a repository with an unusual default branch still works.
// Only a 404 moves on to the next candidate.
func (g *GitHubImporter) Fetch(ctx context.Context, f *SafeFetcher, ref SourceRef) (*Bundle, SourceMeta, error) {
	owner, repo, urlRef, urlSubdir, err := parseGitHubURL(ref.URL)
	if err != nil {
		return nil, SourceMeta{}, err
	}
	gitRef := firstNonEmpty(ref.Ref, urlRef)
	subdir := firstNonEmpty(ref.Subdir, urlSubdir)
	if gitRef != "" {
		if err := validateGitRef(gitRef); err != nil {
			return nil, SourceMeta{}, err
		}
	}
	candidates := []string{gitRef}
	if gitRef == "" {
		candidates = []string{"HEAD", "refs/heads/main", "refs/heads/master"}
	}

	var (
		data    []byte
		fetched string
		lastErr error
	)
	for _, candidate := range candidates {
		zipURL := g.codeloadBase + "/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/zip/" + escapeRefPath(candidate)
		body, _, err := f.GetAuthorized(ctx, zipURL, g.lim.MaxArchiveBytes, g.credentialFor(ref))
		if err == nil {
			data, fetched = body, candidate
			break
		}
		lastErr = err
		if !isNotFound(err) {
			return nil, SourceMeta{}, err
		}
	}
	if data == nil {
		return nil, SourceMeta{}, fmt.Errorf("hub: no fetchable ref for %s/%s: %w", owner, repo, lastErr)
	}

	if err := verifySHA256(data, ref.SHA256); err != nil {
		return nil, SourceMeta{}, err
	}
	bundle, err := ExtractZip(bytes.NewReader(data), int64(len(data)), g.lim)
	if err != nil {
		return nil, SourceMeta{}, err
	}
	bundle, err = applySubdir(bundle, subdir)
	if err != nil {
		return nil, SourceMeta{}, err
	}
	return bundle, SourceMeta{SourceURL: ref.URL, SourceRef: fetched, Digest: Digest(bundle)}, nil
}

// credentialFor is the credential a GitHub archive request carries. The archive
// is served by the codeload host, not by github.com, so that is the host the
// token is pinned to.
func (g *GitHubImporter) credentialFor(ref SourceRef) Credential {
	host := ""
	if u, err := url.Parse(g.codeloadBase); err == nil {
		host = u.Host
	}
	return Credential{Host: host, Token: ref.Credential.Token}
}

// parseGitHubURL accepts github.com URLs only. Any other host is refused, so
// this importer cannot be pointed at arbitrary servers.
func parseGitHubURL(raw string) (owner, repo, ref, subdir string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "", "", fmt.Errorf("%w: %v", ErrBadURL, err)
	}
	if err := checkScheme(u); err != nil {
		return "", "", "", "", err
	}
	if u.User != nil {
		return "", "", "", "", fmt.Errorf("%w: credentials in URL are not allowed", ErrBadURL)
	}
	if !strings.EqualFold(u.Hostname(), "github.com") {
		return "", "", "", "", fmt.Errorf("%w: host %q is not github.com", ErrBadURL, u.Hostname())
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 {
		return "", "", "", "", fmt.Errorf("%w: expected github.com/{owner}/{repo}", ErrBadURL)
	}
	owner = segs[0]
	repo = strings.TrimSuffix(segs[1], ".git")
	if !githubOwnerPattern.MatchString(owner) || !githubRepoPattern.MatchString(repo) || repo == "." || repo == ".." {
		return "", "", "", "", fmt.Errorf("%w: invalid owner or repository name", ErrBadURL)
	}
	switch {
	case len(segs) == 2:
		return owner, repo, "", "", nil
	case len(segs) >= 4 && segs[2] == "tree":
		ref = segs[3]
		if len(segs) > 4 {
			subdir = strings.Join(segs[4:], "/")
		}
		return owner, repo, ref, subdir, nil
	default:
		return "", "", "", "", fmt.Errorf("%w: unsupported GitHub URL form", ErrBadURL)
	}
}

// URLZipImporter downloads a zip from any http or https URL.
type URLZipImporter struct {
	lim Limits
}

// NewURLZipImporter returns an importer for direct zip URLs.
func NewURLZipImporter() Importer {
	return &URLZipImporter{lim: DefaultLimits}
}

// Fetch downloads the archive, verifies SHA256 when given, and extracts it.
func (u *URLZipImporter) Fetch(ctx context.Context, f *SafeFetcher, ref SourceRef) (*Bundle, SourceMeta, error) {
	data, _, err := f.GetAuthorized(ctx, ref.URL, u.lim.MaxArchiveBytes, ref.Credential)
	if err != nil {
		return nil, SourceMeta{}, err
	}
	if err := verifySHA256(data, ref.SHA256); err != nil {
		return nil, SourceMeta{}, err
	}
	bundle, err := ExtractZip(bytes.NewReader(data), int64(len(data)), u.lim)
	if err != nil {
		return nil, SourceMeta{}, err
	}
	bundle, err = applySubdir(bundle, ref.Subdir)
	if err != nil {
		return nil, SourceMeta{}, err
	}
	return bundle, SourceMeta{SourceURL: ref.URL, SourceRef: ref.Ref, Digest: Digest(bundle)}, nil
}

// LegacyFetch is a fetcher that already exists elsewhere in the handler layer
// (ClawHub, skills.sh). The hub package does not own those HTTP clients; the
// handler passes its function in so the existing behavior is kept.
type LegacyFetch func(ctx context.Context, rawURL string) (*Bundle, error)

type legacyImporter struct {
	kind SourceType
	fn   LegacyFetch
}

// NewClawHubImporter wraps the handler's ClawHub fetcher as an Importer.
func NewClawHubImporter(fn LegacyFetch) Importer {
	return &legacyImporter{kind: SourceClawHub, fn: fn}
}

// NewSkillsShImporter wraps the handler's skills.sh fetcher as an Importer.
func NewSkillsShImporter(fn LegacyFetch) Importer {
	return &legacyImporter{kind: SourceSkillsSh, fn: fn}
}

func (l *legacyImporter) Fetch(ctx context.Context, _ *SafeFetcher, ref SourceRef) (*Bundle, SourceMeta, error) {
	if l.fn == nil {
		return nil, SourceMeta{}, ErrNoLegacyFetcher
	}
	if ref.URL == "" {
		return nil, SourceMeta{}, fmt.Errorf("%w: empty URL", ErrBadURL)
	}
	bundle, err := l.fn(ctx, ref.URL)
	if err != nil {
		return nil, SourceMeta{}, err
	}
	if bundle == nil {
		return nil, SourceMeta{}, fmt.Errorf("%w: %s fetcher returned no bundle", ErrInvalidArchive, l.kind)
	}
	return bundle, SourceMeta{SourceURL: ref.URL, SourceRef: ref.Ref, Digest: Digest(bundle)}, nil
}

// applySubdir keeps only files under subdir and strips that prefix. An empty
// subdir returns the bundle unchanged. The subdir goes through the same path
// rules as archive entries, so ".." cannot escape.
func applySubdir(b *Bundle, subdir string) (*Bundle, error) {
	subdir = strings.Trim(subdir, "/")
	if subdir == "" {
		return b, nil
	}
	if _, err := normalizeEntryName(subdir); err != nil {
		return nil, err
	}
	prefix := subdir + "/"
	out := &Bundle{}
	for _, f := range b.Files {
		if !strings.HasPrefix(f.Path, prefix) {
			continue
		}
		f.Path = strings.TrimPrefix(f.Path, prefix)
		out.Files = append(out.Files, f)
		out.TotalSize += f.Size
	}
	if len(out.Files) == 0 {
		return nil, fmt.Errorf("%w: %q", ErrSubdirNotFound, subdir)
	}
	return out, nil
}

func verifySHA256(data []byte, expected string) error {
	if expected == "" {
		return nil
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), strings.TrimSpace(expected)) {
		return ErrChecksumMismatch
	}
	return nil
}

// validateGitRef allows the characters git branch and tag names use, with no
// empty, "." or ".." segments and no leading "-", so a ref cannot change the
// codeload path or be read as an option.
func validateGitRef(ref string) error {
	if !gitRefPattern.MatchString(ref) || strings.Contains(ref, "..") || strings.HasPrefix(ref, "-") {
		return fmt.Errorf("%w: invalid git ref %q", ErrBadURL, ref)
	}
	for _, seg := range strings.Split(ref, "/") {
		if seg == "" || seg == "." || strings.HasPrefix(seg, "-") {
			return fmt.Errorf("%w: invalid git ref %q", ErrBadURL, ref)
		}
	}
	return nil
}

func escapeRefPath(ref string) string {
	segs := strings.Split(ref, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

func isNotFound(err error) bool {
	return err != nil && errors.Is(err, ErrNotFound)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
