package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/plugincontract"
)

// Read-only access to the files of a published version.
//
// The files are the ones publish stored in plugin_package_file, plus one virtual
// file: the manifest, served as multica.plugin.json. The manifest lives in
// plugin_package_version.manifest, so it is rendered from there on read. A real
// file row with the same path always wins. Nothing here rewrites a row. Ownership is checked in two steps: the
// package must belong to the workspace, and the version must belong to that
// package. Either failing is a plain not-found, so a caller cannot tell another
// workspace's ids from ids that do not exist.

// pluginPackageFileTextLimit bounds what the content endpoint returns as text.
// Larger files are still listed and their size is still reported; only the
// bytes are withheld.
const pluginPackageFileTextLimit = 1 << 20

// maxPluginPackageFilePathLength matches the plugin_package_file.path check.
const maxPluginPackageFilePathLength = 1024

// PluginPackageFileEntry is one file of a published version, without its bytes.
type PluginPackageFileEntry struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// PluginPackageFileContent is one file of a published version with its text.
// Content is nil when the file is binary, too large to return as text, or not
// valid UTF-8; SizeBytes is always the real size.
type PluginPackageFileContent struct {
	Path      string  `json:"path"`
	Content   *string `json:"content"`
	IsBinary  bool    `json:"is_binary"`
	SizeBytes int64   `json:"size_bytes"`
	SHA256    string  `json:"sha256"`
}

// ListPackageVersionFiles returns the paths, sizes and digests of every file in
// a published version, including the virtual manifest, sorted by path in byte
// order.
func (s *PluginService) ListPackageVersionFiles(ctx context.Context, workspaceID pgtype.UUID, packageID, versionID string) ([]PluginPackageFileEntry, error) {
	version, err := s.packageVersionInWorkspace(ctx, workspaceID, packageID, versionID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Queries.ListPluginPackageFilePaths(ctx, version.ID)
	if err != nil {
		return nil, &PluginError{Kind: PluginErrorUnavailable, Message: "list plugin package files", Err: err}
	}
	files := make([]PluginPackageFileEntry, 0, len(rows)+1)
	hasRealManifest := false
	for _, row := range rows {
		if row.Path == plugincontract.ManifestFilename {
			hasRealManifest = true
		}
		files = append(files, PluginPackageFileEntry{Path: row.Path, SizeBytes: row.SizeBytes, SHA256: row.Sha256})
	}
	if !hasRealManifest {
		manifest, err := manifestVirtualFile(version)
		if err != nil {
			return nil, err
		}
		files = append(files, PluginPackageFileEntry{Path: manifest.Path, SizeBytes: manifest.SizeBytes, SHA256: manifest.Sha256})
	}
	slices.SortFunc(files, func(a, b PluginPackageFileEntry) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}

// manifestVirtualFile renders the version's manifest as the file the draft
// builder reads at multica.plugin.json: indented JSON with a trailing newline.
// The list and the content endpoint both call this, so the size and digest the
// list reports are the digest of the bytes the content endpoint returns.
func manifestVirtualFile(version db.PluginPackageVersion) (db.PluginPackageFile, error) {
	var text bytes.Buffer
	if err := json.Indent(&text, version.Manifest, "", "  "); err != nil {
		return db.PluginPackageFile{}, &PluginError{Kind: PluginErrorUnavailable, Message: "render plugin manifest", Err: err}
	}
	text.WriteByte('\n')
	sum := sha256.Sum256(text.Bytes())
	return db.PluginPackageFile{
		Path:      plugincontract.ManifestFilename,
		Content:   text.Bytes(),
		SizeBytes: int64(text.Len()),
		Sha256:    hex.EncodeToString(sum[:]),
	}, nil
}

// PackageVersionFile returns one file of a published version. The path is an
// exact, workspace-relative path as the file list reports it.
func (s *PluginService) PackageVersionFile(ctx context.Context, workspaceID pgtype.UUID, packageID, versionID, path string) (PluginPackageFileContent, error) {
	if err := validatePluginPackageFilePath(path); err != nil {
		return PluginPackageFileContent{}, err
	}
	version, err := s.packageVersionInWorkspace(ctx, workspaceID, packageID, versionID)
	if err != nil {
		return PluginPackageFileContent{}, err
	}
	file, err := s.Queries.GetPluginPackageFile(ctx, db.GetPluginPackageFileParams{
		VersionID: version.ID,
		Path:      path,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if path != plugincontract.ManifestFilename {
			return PluginPackageFileContent{}, pluginErrf(PluginErrorNotFound, "plugin package file not found")
		}
		// No real row claims this path, so the manifest is served from the version.
		file, err = manifestVirtualFile(version)
		if err != nil {
			return PluginPackageFileContent{}, err
		}
	case err != nil:
		return PluginPackageFileContent{}, &PluginError{Kind: PluginErrorUnavailable, Message: "load plugin package file", Err: err}
	}
	return classifyPluginPackageFile(file), nil
}

// classifyPluginPackageFile decides whether the bytes can be shown as text. A
// NUL byte, invalid UTF-8 or an over-limit size all mean "binary" to the reader:
// the file is still listed with its size and digest, but its text is withheld.
func classifyPluginPackageFile(file db.PluginPackageFile) PluginPackageFileContent {
	result := PluginPackageFileContent{
		Path:      file.Path,
		SizeBytes: file.SizeBytes,
		SHA256:    file.Sha256,
	}
	if len(file.Content) > pluginPackageFileTextLimit ||
		bytes.IndexByte(file.Content, 0) >= 0 ||
		!utf8.Valid(file.Content) {
		result.IsBinary = true
		return result
	}
	text := string(file.Content)
	result.Content = &text
	return result
}

// packageVersionInWorkspace resolves a version the caller may read: the package
// is in the workspace, and the version is one of that package's versions.
func (s *PluginService) packageVersionInWorkspace(ctx context.Context, workspaceID pgtype.UUID, packageID, versionID string) (db.PluginPackageVersion, error) {
	pkgUUID, err := util.ParseUUID(strings.TrimSpace(packageID))
	if err != nil {
		return db.PluginPackageVersion{}, pluginErrf(PluginErrorNotFound, "plugin package not found")
	}
	pkg, err := s.Queries.GetWorkspacePluginPackage(ctx, db.GetWorkspacePluginPackageParams{
		WorkspaceID: workspaceID,
		ID:          pkgUUID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PluginPackageVersion{}, pluginErrf(PluginErrorNotFound, "plugin package not found")
	}
	if err != nil {
		return db.PluginPackageVersion{}, &PluginError{Kind: PluginErrorUnavailable, Message: "load plugin package", Err: err}
	}
	version, err := s.VersionForWorkspace(ctx, workspaceID, versionID)
	if err != nil {
		return db.PluginPackageVersion{}, err
	}
	if uuidString(version.PackageID) != uuidString(pkg.ID) {
		return db.PluginPackageVersion{}, pluginErrf(PluginErrorNotFound, "published plugin version not found")
	}
	return version, nil
}

// validatePluginPackageFilePath accepts only the relative paths a bundle can
// contain. The lookup is an exact match, so this is about giving the caller a
// 400 for a malformed request rather than a misleading 404.
func validatePluginPackageFilePath(path string) error {
	switch {
	case path == "":
		return pluginErrf(PluginErrorInvalid, "path is required")
	case len(path) > maxPluginPackageFilePathLength:
		return pluginErrf(PluginErrorInvalid, "path is too long")
	case strings.HasPrefix(path, "/"), strings.Contains(path, `\`), strings.ContainsRune(path, 0):
		return pluginErrf(PluginErrorInvalid, "path must be a relative path inside the plugin package")
	}
	for segment := range strings.SplitSeq(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return pluginErrf(PluginErrorInvalid, "path must be a relative path inside the plugin package")
		}
	}
	return nil
}
