// Package hub holds the pure, DB-free logic of the Skill/Plugin Hub: archive
// extraction, bundle detection, SSRF-safe fetching, source importers, the
// registry index parser and the import security scanner. Handlers wire these
// pieces together; nothing here knows about HTTP handlers or storage.
package hub

import "errors"

// SourceType identifies where an imported item came from.
type SourceType string

const (
	SourceGitHub   SourceType = "github"
	SourceSkillsSh SourceType = "skills_sh"
	SourceClawHub  SourceType = "clawhub"
	SourceURLZip   SourceType = "url_zip"
	SourceRegistry SourceType = "registry"
	SourceUpload   SourceType = "upload"
)

// FileEntry is one normalized file of a bundle. Path uses forward slashes and
// has no leading slash. Size always equals len(Content).
type FileEntry struct {
	Path    string
	Size    int64
	Content []byte
}

// Bundle is an ordered-by-path collection of files with their total size.
type Bundle struct {
	Files     []FileEntry
	TotalSize int64
}

// Kind is what a bundle installs as.
type Kind string

const (
	KindSkill  Kind = "skill"
	KindPlugin Kind = "plugin"
)

// Detected describes a bundle after detection. Name is the skill's frontmatter
// name, or the plugin manifest key. Version is only set for plugins.
type Detected struct {
	Kind        Kind
	Name        string
	Description string
	Version     string
	// SkillMDPath is the bundle path of the SKILL.md file; empty for plugins.
	SkillMDPath string
}

// Limits bound what an archive or bundle may contain.
type Limits struct {
	MaxFiles            int
	MaxFileBytes        int64
	MaxTotalBytes       int64
	MaxArchiveBytes     int64
	MaxCompressionRatio int
}

// DefaultLimits mirror the handler's import caps (512 files, 1 MiB per file,
// 8 MiB of supporting files) and add an archive cap and a ratio cap.
var DefaultLimits = Limits{
	MaxFiles:            512,
	MaxFileBytes:        1 << 20,
	MaxTotalBytes:       8 << 20,
	MaxArchiveBytes:     16 << 20,
	MaxCompressionRatio: 100,
}

// Sentinel errors. Callers match them with errors.Is; the message carries the
// specific detail.
var (
	ErrTooLarge         = errors.New("hub: content exceeds size limit")
	ErrUnsafePath       = errors.New("hub: unsafe archive path")
	ErrTooManyFiles     = errors.New("hub: too many files")
	ErrBomb             = errors.New("hub: archive expands beyond the compression ratio limit")
	ErrDuplicatePath    = errors.New("hub: duplicate archive path")
	ErrInvalidArchive   = errors.New("hub: invalid archive")
	ErrNotDetected      = errors.New("hub: bundle is neither a skill nor a plugin")
	ErrAmbiguousKind    = errors.New("hub: bundle is both a skill and a plugin")
	ErrInvalidSkill     = errors.New("hub: invalid SKILL.md")
	ErrInvalidManifest  = errors.New("hub: invalid plugin manifest")
	ErrBlockedAddress   = errors.New("hub: address is not allowed")
	ErrBadURL           = errors.New("hub: unsupported or invalid URL")
	ErrChecksumMismatch = errors.New("hub: sha256 checksum mismatch")
	ErrSubdirNotFound   = errors.New("hub: subdirectory not found in archive")
	ErrInvalidRegistry  = errors.New("hub: invalid registry index")
	ErrNoLegacyFetcher  = errors.New("hub: legacy fetcher not configured")
	ErrNotFound         = errors.New("hub: remote resource not found")
)
