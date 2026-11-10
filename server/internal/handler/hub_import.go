package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/logger"
	skillpkg "github.com/multica-ai/multica/server/internal/skill"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/plugincontract"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Skill / plugin hub import (spec 5.3, 6, 8, 9).
//
// Preview fetches a bundle through the SSRF-safe fetcher, detects and scans it,
// and stages the normalized zip plus a small metadata object in the Hub store
// under staging/{workspace}/{preview}. The top-level staging/ prefix is the one
// the bucket's expiry rule targets (docs/engineering/runbooks/hub-object-storage.md).
// Commit reloads the staged bytes, re-runs detection and the scan (the staged
// metadata is never trusted for the decision), writes the skill or plugin through
// the same paths the existing import routes use, then records a hub_source and
// hub_item and keeps the bundle.

const (
	hubImportBodyLimit          = 64 << 10
	hubImportMultipartOverhead  = 64 << 10
	hubImportFetchTimeout       = 45 * time.Second
	hubImportMaxURLLength       = 2048
	hubImportLegacyClientWindow = 30 * time.Second
	hubImportStagingContentType = "application/json"
)

// hubPreviewTTL is how long a staged preview may be committed. A variable so a
// test can make previews expire immediately.
var hubPreviewTTL = 30 * time.Minute

// hubImportFetcher is the fetcher used for network sources. Production uses the
// zero options: loopback and private addresses are refused at dial time. Tests
// swap in a fetcher that allows an httptest server.
var hubImportFetcher = hub.NewSafeFetcher(hub.FetcherOptions{})

type HubImportSource struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	Ref    string `json:"ref,omitempty"`
	Subdir string `json:"subdir,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type HubImportPreviewRequest struct {
	Source HubImportSource `json:"source"`
}

type HubImportFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

type HubImportFinding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

type HubImportScan struct {
	Status   string             `json:"status"`
	Findings []HubImportFinding `json:"findings"`
}

type HubImportConflict struct {
	Exists bool   `json:"exists"`
	ItemID string `json:"item_id,omitempty"`
	Kind   string `json:"kind,omitempty"`
}

type HubImportRegistryItem struct {
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	URL         string   `json:"url"`
	SHA256      string   `json:"sha256"`
	Tags        []string `json:"tags"`
}

type HubImportRegistry struct {
	Name  string                  `json:"name"`
	Items []HubImportRegistryItem `json:"items"`
}

// HubImportPreview is the preview response. For a registry listing only Kind and
// Registry carry data; the other fields are empty so every field is present.
type HubImportPreview struct {
	PreviewID   string             `json:"preview_id"`
	Kind        string             `json:"kind"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Version     string             `json:"version"`
	Files       []HubImportFile    `json:"files"`
	Scan        HubImportScan      `json:"scan"`
	Conflict    HubImportConflict  `json:"conflict"`
	Registry    *HubImportRegistry `json:"registry,omitempty"`
}

type HubImportCommitRequest struct {
	PreviewID       string `json:"preview_id"`
	OnConflict      string `json:"on_conflict"`
	Name            string `json:"name"`
	ConfirmWarnings bool   `json:"confirm_warnings"`
	SourceID        string `json:"source_id"`
}

type HubImportCommitResponse struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	HubItemID string `json:"hub_item_id"`
	Skipped   bool   `json:"skipped"`
}

// hubSourceMeta is where a bundle came from, as recorded at preview time.
type hubSourceMeta struct {
	Type   string
	URL    string
	Ref    string
	Digest string
}

// hubStagedMeta is the sibling object of a staged zip. It names what was
// previewed so commit can check the zip still matches; it does not decide anything.
type hubStagedMeta struct {
	WorkspaceID  string    `json:"workspace_id"`
	UserID       string    `json:"user_id"`
	Kind         string    `json:"kind"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Version      string    `json:"version"`
	SourceType   string    `json:"source_type"`
	SourceURL    string    `json:"source_url"`
	SourceRef    string    `json:"source_ref"`
	BundleDigest string    `json:"bundle_digest"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// hubScanReport is what hub_item.scan_report stores.
type hubScanReport struct {
	Status   string             `json:"status"`
	Findings []HubImportFinding `json:"findings"`
}

func hubStagingZipKey(workspaceID, previewID string) string {
	return "staging/" + workspaceID + "/" + previewID + ".zip"
}

func hubStagingMetaKey(workspaceID, previewID string) string {
	return "staging/" + workspaceID + "/" + previewID + ".json"
}

// hubBundleKey is the immutable copy of the bundle a hub item was created from.
func hubBundleKey(workspaceID, kind, refID, digest string) string {
	return fmt.Sprintf("hub/%s/%s/%s/%s/bundle.zip", workspaceID, kind, refID, digest)
}

// hubItemVersion names the version a hub item's bundle is recorded under: the
// manifest version of a plugin, or the bundle digest for a skill, which has none.
func hubItemVersion(latestVersion, digest string) string {
	if latestVersion != "" {
		return latestVersion
	}
	return digest
}

// writeHubVersionBlob records the bundle a commit stored as one immutable version
// row. A repeat of the same version and digest is a no-op.
func writeHubVersionBlob(ctx context.Context, q *db.Queries, item db.HubItem, createdBy pgtype.UUID) error {
	return q.InsertHubItemVersionBlob(ctx, db.InsertHubItemVersionBlobParams{
		WorkspaceID: item.WorkspaceID,
		ItemID:      item.ID,
		Version:     hubItemVersion(item.LatestVersion, item.BundleDigest),
		BundleKey:   item.BundleKey,
		Digest:      item.BundleDigest,
		SizeBytes:   item.SizeBytes,
		CreatedBy:   createdBy,
	})
}

// hubImportCaller resolves the workspace and the acting user and checks that the
// user is a member. Any member may preview; commit uses the same gate skill and
// plugin creation use.
func (h *Handler) hubImportCaller(w http.ResponseWriter, r *http.Request) (string, pgtype.UUID, string, pgtype.UUID, bool) {
	workspaceID := h.resolveWorkspaceID(r)
	ws, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return "", pgtype.UUID{}, "", pgtype.UUID{}, false
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return "", pgtype.UUID{}, "", pgtype.UUID{}, false
	}
	user, ok := parseUUIDOrBadRequest(w, userID, "user_id")
	if !ok {
		return "", pgtype.UUID{}, "", pgtype.UUID{}, false
	}
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return "", pgtype.UUID{}, "", pgtype.UUID{}, false
	}
	return workspaceID, ws, userID, user, true
}

// hubImportFailure maps an import error onto a status and a message. Hub
// sentinels get their own messages. Anything unrecognized becomes a generic 502
// so transport details and SSRF resolution data never reach the client.
func hubImportFailure(ctx context.Context, err error) (int, string) {
	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr), errors.Is(err, hub.ErrTooLarge), isCapError(err):
		return http.StatusRequestEntityTooLarge, "the source is too large to import"
	case errors.Is(err, hub.ErrBlockedAddress):
		return http.StatusBadRequest, "address is not allowed"
	case errors.Is(err, context.DeadlineExceeded), ctx.Err() == context.DeadlineExceeded:
		return http.StatusGatewayTimeout, "timed out fetching the source"
	case errors.Is(err, hub.ErrNotFound):
		return http.StatusBadGateway, "the source was not found upstream"
	case errors.Is(err, hub.ErrInvalidRegistry):
		return http.StatusBadGateway, "the registry index is invalid"
	case errors.Is(err, hub.ErrBadURL),
		errors.Is(err, hub.ErrUnsafePath),
		errors.Is(err, hub.ErrTooManyFiles),
		errors.Is(err, hub.ErrBomb),
		errors.Is(err, hub.ErrDuplicatePath),
		errors.Is(err, hub.ErrInvalidArchive),
		errors.Is(err, hub.ErrNotDetected),
		errors.Is(err, hub.ErrAmbiguousKind),
		errors.Is(err, hub.ErrInvalidSkill),
		errors.Is(err, hub.ErrInvalidManifest),
		errors.Is(err, hub.ErrChecksumMismatch),
		errors.Is(err, hub.ErrSubdirNotFound):
		return http.StatusBadRequest, err.Error()
	}
	return http.StatusBadGateway, "failed to fetch the source"
}

// hubLegacyFetch adapts the existing ClawHub and skills.sh fetchers. They use
// their own HTTP clients and do not go through the SafeFetcher; their hosts are
// fixed and the request URLs are normalized by detectImportSource first.
func hubLegacyFetch(source importSource) hub.LegacyFetch {
	return func(ctx context.Context, rawURL string) (*hub.Bundle, error) {
		client := &http.Client{Timeout: hubImportLegacyClientWindow}
		var (
			imported *importedSkill
			err      error
		)
		if source == sourceClawHub {
			imported, err = fetchFromClawHub(ctx, client, rawURL)
		} else {
			imported, err = fetchFromSkillsSh(ctx, client, rawURL)
		}
		if err != nil {
			return nil, err
		}
		return importedSkillToHubBundle(imported), nil
	}
}

// errHubURLMismatch means a legacy source type was given a URL of the other
// legacy host. errHubUnsupportedSource means the type is not fetched by URL.
var (
	errHubURLMismatch       = errors.New("hub: url does not match the source type")
	errHubUnsupportedSource = errors.New("hub: unsupported source type")
)

// hubImporterFor picks the importer for a fetched source type. The ref is returned
// with the URL normalized for the legacy hosts, which is the URL a source records.
// Preview and sync share it, so both fetch an item the same way.
func hubImporterFor(ref hub.SourceRef) (hub.Importer, hub.SourceRef, error) {
	switch ref.Type {
	case hub.SourceGitHub:
		return hub.NewGitHubImporter(), ref, nil
	case hub.SourceURLZip:
		return hub.NewURLZipImporter(), ref, nil
	case hub.SourceClawHub, hub.SourceSkillsSh:
		want := sourceClawHub
		if ref.Type == hub.SourceSkillsSh {
			want = sourceSkillsSh
		}
		detected, normalized, err := detectImportSource(ref.URL)
		if err != nil || detected != want {
			return nil, ref, errHubURLMismatch
		}
		ref.URL = normalized
		if want == sourceSkillsSh {
			return hub.NewSkillsShImporter(hubLegacyFetch(want)), ref, nil
		}
		return hub.NewClawHubImporter(hubLegacyFetch(want)), ref, nil
	}
	return nil, ref, errHubUnsupportedSource
}

func importedSkillToHubBundle(imported *importedSkill) *hub.Bundle {
	bundle := &hub.Bundle{}
	add := func(p, content string) {
		bundle.Files = append(bundle.Files, hub.FileEntry{Path: p, Size: int64(len(content)), Content: []byte(content)})
		bundle.TotalSize += int64(len(content))
	}
	add(skillpkg.ContentFilename, imported.content)
	for _, f := range imported.files {
		add(f.path, f.content)
	}
	return bundle
}

// PreviewHubImport handles POST /api/hub/import/preview: a JSON source to fetch,
// or a multipart zip upload in field "file".
func (h *Handler) PreviewHubImport(w http.ResponseWriter, r *http.Request) {
	workspaceID, ws, userID, _, ok := h.hubImportCaller(w, r)
	if !ok {
		return
	}

	if isMultipartForm(r) {
		bundle, ok := h.hubUploadBundle(w, r)
		if !ok {
			return
		}
		h.stageHubPreview(w, r, workspaceID, ws, userID, bundle, hubSourceMeta{Type: string(hub.SourceUpload), Digest: hub.Digest(bundle)})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, hubImportBodyLimit)
	var req HubImportPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	src := req.Source
	src.URL = strings.TrimSpace(src.URL)
	if len(src.URL) > hubImportMaxURLLength {
		writeError(w, http.StatusBadRequest, "url must be 2048 characters or fewer")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), hubImportFetchTimeout)
	defer cancel()

	if hub.SourceType(src.Type) == hub.SourceRegistry {
		h.previewHubRegistry(ctx, w, src.URL)
		return
	}

	ref := hub.SourceRef{Type: hub.SourceType(src.Type), URL: src.URL, Ref: src.Ref, Subdir: src.Subdir, SHA256: src.SHA256}
	importer, ref, err := hubImporterFor(ref)
	if errors.Is(err, errHubURLMismatch) {
		writeError(w, http.StatusBadRequest, "url does not match the source type")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "unsupported source type")
		return
	}

	bundle, meta, err := importer.Fetch(ctx, hubImportFetcher, ref)
	if err != nil {
		status, msg := hubImportFailure(ctx, err)
		writeError(w, status, msg)
		return
	}
	h.stageHubPreview(w, r, workspaceID, ws, userID, bundle, hubSourceMeta{
		Type:   src.Type,
		URL:    ref.URL,
		Ref:    meta.SourceRef,
		Digest: meta.Digest,
	})
}

// hubUploadBundle reads a zip upload. The body is bounded before the multipart
// parser sees it, so an oversized upload is refused without being spooled.
func (h *Handler) hubUploadBundle(w http.ResponseWriter, r *http.Request) (*hub.Bundle, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, hub.DefaultLimits.MaxArchiveBytes+hubImportMultipartOverhead)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "the upload is too large to import")
			return nil, false
		}
		writeError(w, http.StatusBadRequest, "invalid multipart upload")
		return nil, false
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, `a zip file is required (form field "file")`)
		return nil, false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, hub.DefaultLimits.MaxArchiveBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read the uploaded file")
		return nil, false
	}
	if int64(len(data)) > hub.DefaultLimits.MaxArchiveBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "the upload is too large to import")
		return nil, false
	}
	bundle, err := hub.ExtractZip(bytes.NewReader(data), int64(len(data)), hub.DefaultLimits)
	if err != nil {
		status, msg := hubImportFailure(r.Context(), err)
		writeError(w, status, msg)
		return nil, false
	}
	return bundle, true
}

// previewHubRegistry lists the items of a registry index. Nothing is staged:
// the user picks an item and previews that item's archive as its own source.
func (h *Handler) previewHubRegistry(ctx context.Context, w http.ResponseWriter, rawURL string) {
	if rawURL == "" {
		writeError(w, http.StatusBadRequest, "url is required for a registry")
		return
	}
	registry, err := hub.FetchRegistry(ctx, hubImportFetcher, hub.RegistryIndexURL(rawURL))
	if err != nil {
		status, msg := hubImportFailure(ctx, err)
		writeError(w, status, msg)
		return
	}
	items := make([]HubImportRegistryItem, 0, len(registry.Items))
	for _, it := range registry.Items {
		tags := it.Tags
		if tags == nil {
			tags = []string{}
		}
		items = append(items, HubImportRegistryItem{
			Kind:        string(it.Kind),
			Name:        it.Name,
			Description: it.Description,
			Version:     it.Version,
			URL:         it.URL,
			SHA256:      it.SHA256,
			Tags:        tags,
		})
	}
	writeJSON(w, http.StatusOK, HubImportPreview{
		Kind:     "registry",
		Files:    []HubImportFile{},
		Scan:     HubImportScan{Status: hub.ScanClean, Findings: []HubImportFinding{}},
		Conflict: HubImportConflict{Exists: false},
		Registry: &HubImportRegistry{Name: registry.Name, Items: items},
	})
}

// stageHubPreview detects, scans and stages a fetched bundle, then answers with
// the preview. A blocked bundle is still staged so the user can see why.
func (h *Handler) stageHubPreview(w http.ResponseWriter, r *http.Request, workspaceID string, ws pgtype.UUID, userID string, bundle *hub.Bundle, meta hubSourceMeta) {
	ctx := r.Context()
	detected, err := hub.Detect(bundle)
	if err != nil {
		status, msg := hubImportFailure(ctx, err)
		writeError(w, status, msg)
		return
	}
	zipBytes, err := hub.BuildZip(bundle)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to stage the bundle")
		return
	}
	if detected.Kind == hub.KindPlugin {
		if _, err := plugincontract.ParseBundle(zipBytes); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("plugin package is invalid: %v", err))
			return
		}
	}
	scan := hub.Scan(bundle)

	conflict, err := h.hubImportConflict(ctx, ws, detected)
	if err != nil {
		slog.Warn("hub import conflict lookup failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to check for an existing item")
		return
	}
	if h.HubStorage == nil {
		writeError(w, http.StatusServiceUnavailable, "file storage is not configured")
		return
	}

	previewID := uuid.NewString()
	expiresAt := time.Now().Add(hubPreviewTTL)
	stagedMeta := hubStagedMeta{
		WorkspaceID:  workspaceID,
		UserID:       userID,
		Kind:         string(detected.Kind),
		Name:         detected.Name,
		Description:  detected.Description,
		Version:      detected.Version,
		SourceType:   meta.Type,
		SourceURL:    meta.URL,
		SourceRef:    meta.Ref,
		BundleDigest: meta.Digest,
		ExpiresAt:    expiresAt,
	}
	metaBytes, err := json.Marshal(stagedMeta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to stage the bundle")
		return
	}
	if _, err := h.HubStorage.Upload(ctx, hubStagingZipKey(workspaceID, previewID), zipBytes, "application/zip", ""); err != nil {
		slog.Warn("hub staging upload failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to stage the bundle")
		return
	}
	if _, err := h.HubStorage.Upload(ctx, hubStagingMetaKey(workspaceID, previewID), metaBytes, hubImportStagingContentType, ""); err != nil {
		slog.Warn("hub staging metadata upload failed", append(logger.RequestAttrs(r), "error", err)...)
		h.deleteHubStaging(ctx, workspaceID, previewID)
		writeError(w, http.StatusInternalServerError, "failed to stage the bundle")
		return
	}

	files := make([]HubImportFile, 0, len(bundle.Files))
	for _, f := range bundle.Files {
		files = append(files, HubImportFile{Path: f.Path, SizeBytes: f.Size})
	}
	findings := make([]HubImportFinding, 0, len(scan.Findings))
	for _, f := range scan.Findings {
		findings = append(findings, HubImportFinding{Rule: f.Rule, Severity: string(f.Severity), Path: f.Path, Message: f.Message})
	}
	writeJSON(w, http.StatusOK, HubImportPreview{
		PreviewID:   previewID,
		Kind:        string(detected.Kind),
		Name:        detected.Name,
		Description: detected.Description,
		Version:     detected.Version,
		Files:       files,
		Scan:        HubImportScan{Status: scan.Status, Findings: findings},
		Conflict:    conflict,
	})
}

// hubImportConflict checks for an existing skill with the same name or an
// existing plugin with the same key.
func (h *Handler) hubImportConflict(ctx context.Context, ws pgtype.UUID, detected hub.Detected) (HubImportConflict, error) {
	switch detected.Kind {
	case hub.KindSkill:
		existing, err := h.Queries.GetSkillByWorkspaceAndName(ctx, db.GetSkillByWorkspaceAndNameParams{WorkspaceID: ws, Name: detected.Name})
		if errors.Is(err, pgx.ErrNoRows) {
			return HubImportConflict{Exists: false}, nil
		}
		if err != nil {
			return HubImportConflict{}, err
		}
		return HubImportConflict{Exists: true, ItemID: uuidToString(existing.ID), Kind: string(hub.KindSkill)}, nil
	case hub.KindPlugin:
		existing, err := h.Queries.GetWorkspacePluginPackageByKey(ctx, db.GetWorkspacePluginPackageByKeyParams{WorkspaceID: ws, PluginKey: detected.Name})
		if errors.Is(err, pgx.ErrNoRows) {
			return HubImportConflict{Exists: false}, nil
		}
		if err != nil {
			return HubImportConflict{}, err
		}
		return HubImportConflict{Exists: true, ItemID: uuidToString(existing.ID), Kind: string(hub.KindPlugin)}, nil
	}
	return HubImportConflict{Exists: false}, nil
}

// loadHubPreview reads a staged preview's metadata and checks that it belongs to
// this workspace and user. Another caller's or another workspace's preview is
// reported as not found.
func (h *Handler) loadHubPreview(w http.ResponseWriter, r *http.Request, workspaceID, userID, previewID string) (hubStagedMeta, bool) {
	rc, err := h.HubStorage.GetReader(r.Context(), hubStagingMetaKey(workspaceID, previewID))
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.Is(err, os.ErrNotExist) || errors.As(err, &noSuchKey) {
			writeError(w, http.StatusNotFound, "preview not found")
			return hubStagedMeta{}, false
		}
		slog.Warn("hub preview read failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to load the preview")
		return hubStagedMeta{}, false
	}
	defer rc.Close()
	var meta hubStagedMeta
	if err := json.NewDecoder(io.LimitReader(rc, hubImportBodyLimit)).Decode(&meta); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the preview")
		return hubStagedMeta{}, false
	}
	if meta.WorkspaceID != workspaceID || meta.UserID != userID {
		writeError(w, http.StatusNotFound, "preview not found")
		return hubStagedMeta{}, false
	}
	if time.Now().After(meta.ExpiresAt) {
		writeError(w, http.StatusGone, "preview expired")
		return hubStagedMeta{}, false
	}
	return meta, true
}

// readHubStagedZip loads the staged zip bytes.
func (h *Handler) readHubStagedZip(ctx context.Context, workspaceID, previewID string) ([]byte, error) {
	rc, err := h.HubStorage.GetReader(ctx, hubStagingZipKey(workspaceID, previewID))
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, hub.DefaultLimits.MaxArchiveBytes+1))
}

// deleteHubStaging removes a preview's staged objects. Best effort: a leftover
// object only costs storage, and it is never read after its expiry.
func (h *Handler) deleteHubStaging(ctx context.Context, workspaceID, previewID string) {
	for _, key := range []string{hubStagingZipKey(workspaceID, previewID), hubStagingMetaKey(workspaceID, previewID)} {
		if err := h.HubStorage.DeleteObject(ctx, key); err != nil {
			slog.Warn("hub staging cleanup failed", "key", key, "error", err)
		}
	}
}

// hubCommit carries one commit's inputs through the kind-specific paths.
type hubCommit struct {
	workspaceID string
	ws          pgtype.UUID
	userID      string
	user        pgtype.UUID
	req         HubImportCommitRequest
	previewID   string
	meta        hubStagedMeta
	detected    hub.Detected
	bundle      *hub.Bundle
	zipBytes    []byte
	scan        hub.ScanResult
	// sourceID is set when the request named an existing source.
	sourceID pgtype.UUID
	// name is the trimmed request name, used by rename.
	name string
}

// CommitHubImport handles POST /api/hub/import/commit.
func (h *Handler) CommitHubImport(w http.ResponseWriter, r *http.Request) {
	workspaceID, ws, userID, user, ok := h.hubImportCaller(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, hubImportBodyLimit)
	var req HubImportCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	previewUUID, ok := parseUUIDOrBadRequest(w, req.PreviewID, "preview_id")
	if !ok {
		return
	}
	switch req.OnConflict {
	case "", "rename", "replace", "skip":
	default:
		writeError(w, http.StatusBadRequest, "on_conflict must be one of: rename, replace, skip")
		return
	}
	if h.HubStorage == nil {
		writeError(w, http.StatusServiceUnavailable, "file storage is not configured")
		return
	}
	previewID := uuidToString(previewUUID)

	meta, ok := h.loadHubPreview(w, r, workspaceID, userID, previewID)
	if !ok {
		return
	}
	zipBytes, err := h.readHubStagedZip(r.Context(), workspaceID, previewID)
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.Is(err, os.ErrNotExist) || errors.As(err, &noSuchKey) {
			writeError(w, http.StatusNotFound, "preview not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load the preview")
		return
	}

	// Re-derive everything from the staged bytes. The metadata only names what
	// was previewed; a mismatch means the preview no longer describes the zip.
	bundle, err := hub.ExtractZip(bytes.NewReader(zipBytes), int64(len(zipBytes)), hub.DefaultLimits)
	if err != nil {
		status, msg := hubImportFailure(r.Context(), err)
		writeError(w, status, msg)
		return
	}
	detected, err := hub.Detect(bundle)
	if err != nil {
		status, msg := hubImportFailure(r.Context(), err)
		writeError(w, status, msg)
		return
	}
	digest := hub.Digest(bundle)
	if string(detected.Kind) != meta.Kind || detected.Name != meta.Name || digest != meta.BundleDigest {
		writeError(w, http.StatusConflict, "the preview no longer matches its bundle; start a new preview")
		return
	}
	scan := hub.Scan(bundle)
	if scan.Status == hub.ScanBlocked {
		writeErrorCode(w, http.StatusConflict, "scan_blocked", "the bundle failed the security scan and cannot be imported")
		return
	}
	if scan.Status == hub.ScanWarn && !req.ConfirmWarnings {
		writeErrorCode(w, http.StatusConflict, "warnings_unconfirmed", "the bundle has warnings; confirm them to import")
		return
	}

	c := &hubCommit{
		workspaceID: workspaceID,
		ws:          ws,
		userID:      userID,
		user:        user,
		req:         req,
		previewID:   previewID,
		meta:        meta,
		detected:    detected,
		bundle:      bundle,
		zipBytes:    zipBytes,
		scan:        scan,
		name:        strings.TrimSpace(req.Name),
	}
	if req.SourceID != "" {
		sourceUUID, ok := parseUUIDOrBadRequest(w, req.SourceID, "source_id")
		if !ok {
			return
		}
		if _, err := h.Queries.GetHubSourceInWorkspace(r.Context(), db.GetHubSourceInWorkspaceParams{ID: sourceUUID, WorkspaceID: ws}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "source not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to load the source")
			return
		}
		c.sourceID = sourceUUID
	}

	switch detected.Kind {
	case hub.KindSkill:
		h.commitHubSkill(w, r, c)
	case hub.KindPlugin:
		h.commitHubPlugin(w, r, c)
	default:
		writeError(w, http.StatusBadRequest, "unsupported bundle kind")
	}
}

// commitHubSkill applies the on_conflict strategy for a skill and writes it.
func (h *Handler) commitHubSkill(w http.ResponseWriter, r *http.Request, c *hubCommit) {
	ctx := r.Context()
	target := c.detected.Name
	if c.req.OnConflict == "rename" && c.name != "" {
		target = c.name
	}
	existing, err := h.Queries.GetSkillByWorkspaceAndName(ctx, db.GetSkillByWorkspaceAndNameParams{WorkspaceID: c.ws, Name: target})
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to check for an existing skill")
		return
	}
	if found {
		switch c.req.OnConflict {
		case "":
			writeErrorCode(w, http.StatusConflict, "name_conflict", "a skill with this name already exists")
		case "skip":
			h.writeHubSkipped(w, r, c, "skill", existing.ID)
		case "rename":
			if c.name == "" {
				writeError(w, http.StatusBadRequest, "name is required to rename")
				return
			}
			writeError(w, http.StatusBadRequest, "a skill with this name already exists; choose another name")
		case "replace":
			h.replaceHubSkill(w, r, c, existing, target)
		}
		return
	}
	h.createHubSkill(w, r, c, target)
}

// hubSkillInputs builds the skill fields from the bundle. Config carries the
// provenance that the skill import path records in config.origin.
func hubSkillInputs(c *hubCommit) (content string, config map[string]any, files []CreateSkillFileRequest) {
	content, files = hubSkillParts(c.bundle, c.detected.SkillMDPath)
	config = map[string]any{"origin": map[string]any{
		"type":       c.meta.SourceType,
		"source_url": c.meta.SourceURL,
	}}
	return content, config, files
}

// hubSkillParts lays a bundle out the way a skill stores it: the SKILL.md content,
// and the text supporting files beside it, with paths relative to SKILL.md's
// directory. Sync compares a remote bundle and a local skill through this same
// layout, so both are judged by what the skill would hold.
func hubSkillParts(bundle *hub.Bundle, skillPath string) (string, []CreateSkillFileRequest) {
	var content string
	var files []CreateSkillFileRequest
	prefix := ""
	if dir := path.Dir(skillPath); dir != "." {
		prefix = dir + "/"
	}
	for _, f := range bundle.Files {
		if f.Path == skillPath {
			content = string(f.Content)
			continue
		}
		if !strings.HasPrefix(f.Path, prefix) {
			continue
		}
		rel := strings.TrimPrefix(f.Path, prefix)
		if rel == "" || strings.EqualFold(path.Base(rel), skillpkg.ContentFilename) || isIgnoredArchiveEntry(rel) || !validateFilePath(rel) {
			continue
		}
		// Skill files are stored as text. Binary assets are skipped, as the
		// archive import does, because their bytes cannot live in a TEXT column.
		if skillpkg.IsLikelyBinaryFilePath(rel) || !utf8.Valid(f.Content) || bytes.IndexByte(f.Content, 0) >= 0 {
			continue
		}
		files = append(files, CreateSkillFileRequest{Path: rel, Content: string(f.Content)})
	}
	return content, files
}

func (h *Handler) createHubSkill(w http.ResponseWriter, r *http.Request, c *hubCommit, name string) {
	ctx := r.Context()
	content, config, files := hubSkillInputs(c)

	if !h.checkHubCommitQuota(ctx, w, c.ws, 1, c.bundle.TotalSize) {
		return
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save the import")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)

	resp, err := createSkillWithFilesInTx(ctx, qtx, skillCreateInput{
		VersionSource: skillVersionSourceImport,
		WorkspaceID:   c.ws,
		CreatorID:     c.user,
		Name:          name,
		Description:   c.detected.Description,
		Content:       content,
		Config:        config,
		Files:         files,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeErrorCode(w, http.StatusConflict, "name_conflict", "a skill with this name already exists")
			return
		}
		slog.Warn("hub import: create skill failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to save the import")
		return
	}

	// The bundle is stored before the commit, so a committed hub item never
	// points at a missing object. If the commit fails, the object is orphaned.
	bundleKey, err := h.hubStoreBundle(ctx, c, "skill", resp.ID)
	if err != nil {
		slog.Warn("hub import: bundle upload failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to store the bundle")
		return
	}
	item, err := h.recordHubCommitItem(ctx, qtx, c, parseUUID(resp.ID), "", bundleKey)
	if err != nil {
		writeHubCommitFailure(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeHubCommitFailure(w, r, err)
		return
	}

	actorType, actorID := h.resolveActor(r, c.userID, c.workspaceID)
	h.publish(protocol.EventSkillCreated, c.workspaceID, actorType, actorID, map[string]any{"skill": resp})
	h.recordHubImportActivity(r, c, name, item)
	h.deleteHubStaging(ctx, c.workspaceID, c.previewID)
	writeJSON(w, http.StatusOK, HubImportCommitResponse{Kind: "skill", ID: resp.ID, HubItemID: uuidToString(item.ID)})
}

// replaceHubSkill overwrites an existing skill with the bundle. The overwrite
// keeps the existing creator-only permission check. It runs in its own
// transaction, so the hub rows are written in a second step: a retry with the
// same preview is safe because the overwrite and the hub upsert are idempotent.
func (h *Handler) replaceHubSkill(w http.ResponseWriter, r *http.Request, c *hubCommit, existing db.Skill, target string) {
	ctx := r.Context()
	content, config, files := hubSkillInputs(c)
	refID := uuidToString(existing.ID)

	exists, oldSize, err := h.hubItemFootprint(ctx, c.ws, "skill", existing.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save the import")
		return
	}
	if !h.checkHubCommitQuota(ctx, w, c.ws, hubNewItemCount(exists), c.bundle.TotalSize-oldSize) {
		return
	}
	bundleKey, err := h.hubStoreBundle(ctx, c, "skill", refID)
	if err != nil {
		slog.Warn("hub import: bundle upload failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to store the bundle")
		return
	}
	resp, err := h.overwriteSkillWithFiles(ctx, skillOverwriteInput{
		WorkspaceID:   c.ws,
		TargetSkillID: existing.ID,
		UserID:        c.userID,
		ExpectedName:  target,
		Description:   c.detected.Description,
		Content:       content,
		Config:        config,
		Files:         files,
	})
	if err != nil {
		status, reason := skillImportOverwriteFailure(err)
		if status == http.StatusInternalServerError {
			slog.Warn("hub import: overwrite skill failed", append(logger.RequestAttrs(r), "error", err)...)
		}
		writeError(w, status, reason)
		return
	}

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeHubCommitFailure(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	item, err := h.recordHubCommitItem(ctx, h.Queries.WithTx(tx), c, existing.ID, "", bundleKey)
	if err != nil {
		writeHubCommitFailure(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeHubCommitFailure(w, r, err)
		return
	}

	actorType, actorID := h.resolveActor(r, c.userID, c.workspaceID)
	h.publish(protocol.EventSkillUpdated, c.workspaceID, actorType, actorID, map[string]any{"skill": resp})
	h.recordHubImportActivity(r, c, target, item)
	h.deleteHubStaging(ctx, c.workspaceID, c.previewID)
	writeJSON(w, http.StatusOK, HubImportCommitResponse{Kind: "skill", ID: resp.ID, HubItemID: uuidToString(item.ID)})
}

// commitHubPlugin publishes the bundle as a new immutable version of its plugin
// package. The package's own transaction does the publish. The bundle is then
// stored and the hub rows are written, so a failure after publish leaves the
// version published without a hub item. A retry then reports a version conflict;
// that is the immutability guarantee, and it is reported as a 409.
func (h *Handler) commitHubPlugin(w http.ResponseWriter, r *http.Request, c *hubCommit) {
	if !h.requirePluginsV1(w, r) {
		return
	}
	ctx := r.Context()
	existing, err := h.Queries.GetWorkspacePluginPackageByKey(ctx, db.GetWorkspacePluginPackageByKeyParams{WorkspaceID: c.ws, PluginKey: c.detected.Name})
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to check for an existing plugin")
		return
	}
	if found {
		switch c.req.OnConflict {
		case "":
			writeErrorCode(w, http.StatusConflict, "name_conflict", "a plugin with this key already exists")
			return
		case "skip":
			h.writeHubSkipped(w, r, c, "plugin", existing.ID)
			return
		case "rename":
			// A plugin's identity is its manifest key, and installations are
			// bound to it. Renaming means changing the key in the manifest.
			writeError(w, http.StatusBadRequest, "a plugin is identified by its manifest key; change the key in multica.plugin.json and import again")
			return
		}
		// replace: publish the bundle as a new version of the existing package.
	}

	addItems, oldSize := 1, int64(0)
	if found {
		exists, size, err := h.hubItemFootprint(ctx, c.ws, "plugin", existing.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save the import")
			return
		}
		addItems, oldSize = hubNewItemCount(exists), size
	}
	if !h.checkHubCommitQuota(ctx, w, c.ws, addItems, c.bundle.TotalSize-oldSize) {
		return
	}

	published, err := h.PluginService.PublishBundle(ctx, c.ws, c.user, c.zipBytes)
	if err != nil {
		writePluginError(w, err, "failed to publish the Plugin")
		return
	}
	pkgID := parseUUID(published.ID)
	bundleKey, err := h.hubStoreBundle(ctx, c, "plugin", published.ID)
	if err != nil {
		slog.Warn("hub import: plugin bundle upload failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "the plugin was published but its bundle could not be stored")
		return
	}

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeHubCommitFailure(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	item, err := h.recordHubCommitItem(ctx, h.Queries.WithTx(tx), c, pkgID, c.detected.Version, bundleKey)
	if err != nil {
		writeHubCommitFailure(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeHubCommitFailure(w, r, err)
		return
	}

	h.recordHubImportActivity(r, c, published.Name, item)
	h.deleteHubStaging(ctx, c.workspaceID, c.previewID)
	writeJSON(w, http.StatusOK, HubImportCommitResponse{Kind: "plugin", ID: published.ID, HubItemID: uuidToString(item.ID)})
}

// writeHubSkipped answers a skip without writing anything. The existing item's
// hub row is reported when there is one.
func (h *Handler) writeHubSkipped(w http.ResponseWriter, r *http.Request, c *hubCommit, kind string, refID pgtype.UUID) {
	hubItemID := ""
	if item, err := h.Queries.GetHubItemByRef(r.Context(), db.GetHubItemByRefParams{WorkspaceID: c.ws, Kind: kind, RefID: refID}); err == nil {
		hubItemID = uuidToString(item.ID)
	}
	// The preview stays staged: a skip writes nothing, so the user may still
	// commit the same preview with another strategy until it expires.
	writeJSON(w, http.StatusOK, HubImportCommitResponse{Kind: kind, ID: uuidToString(refID), HubItemID: hubItemID, Skipped: true})
}

// hubSourceForCommit returns the source a commit records against: the one the
// request named, or the existing source for this type and URL, or a new one.
// An existing source is reused as it is, so a user's name and settings survive.
func (h *Handler) hubSourceForCommit(ctx context.Context, q *db.Queries, c *hubCommit) (pgtype.UUID, error) {
	if c.sourceID.Valid {
		return c.sourceID, nil
	}
	sourceType := c.meta.SourceType
	sourceURL := c.meta.SourceURL
	if sourceType == string(hub.SourceUpload) {
		sourceURL = ""
	}
	rows, err := q.ListHubSourcesWithItemCount(ctx, c.ws)
	if err != nil {
		return pgtype.UUID{}, err
	}
	for _, row := range rows {
		if row.Type == sourceType && row.Url == sourceURL {
			return row.ID, nil
		}
	}
	src, err := q.UpsertHubSourceByTypeURL(ctx, db.UpsertHubSourceByTypeURLParams{
		WorkspaceID:         c.ws,
		Name:                hubSourceName(sourceType, sourceURL),
		Type:                sourceType,
		Url:                 sourceURL,
		Config:              []byte("{}"),
		Enabled:             true,
		AutoSyncIntervalSec: 0,
		CreatedBy:           c.user,
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return src.ID, nil
}

var hubSourceLabels = map[string]string{
	"github":    "GitHub",
	"skills_sh": "skills.sh",
	"clawhub":   "ClawHub",
	"url_zip":   "Zip URL",
	"registry":  "Registry",
	"upload":    "Uploads",
}

// hubSourceName is the default name for a new source. Names are unique per
// workspace and capped at the source name limit.
func hubSourceName(sourceType, sourceURL string) string {
	label := hubSourceLabels[sourceType]
	if label == "" {
		label = sourceType
	}
	name := label
	if sourceURL != "" {
		name = label + ": " + sourceURL
	}
	if utf8.RuneCountInString(name) > hubSourceNameMaxRunes {
		runes := []rune(name)
		name = string(runes[:hubSourceNameMaxRunes])
	}
	return name
}

// hubItemParams builds the hub_item write for a committed skill or plugin.
func hubItemParams(c *hubCommit, sourceID, refID pgtype.UUID, latestVersion, bundleKey string) db.UpsertHubItemParams {
	tracking := "tracking"
	if c.meta.SourceType == string(hub.SourceUpload) {
		tracking = "manual"
	}
	report, _ := json.Marshal(hubScanReport{Status: c.scan.Status, Findings: scanFindings(c.scan)})
	return db.UpsertHubItemParams{
		WorkspaceID:   c.ws,
		Kind:          string(c.detected.Kind),
		RefID:         refID,
		SourceID:      sourceID,
		SourceRef:     c.meta.SourceRef,
		SourceDigest:  c.meta.BundleDigest,
		Tracking:      tracking,
		LatestVersion: latestVersion,
		BundleKey:     bundleKey,
		BundleDigest:  c.meta.BundleDigest,
		SizeBytes:     c.bundle.TotalSize,
		FileCount:     int32(len(c.bundle.Files)),
		ScanStatus:    c.scan.Status,
		ScanReport:    report,
	}
}

func scanFindings(scan hub.ScanResult) []HubImportFinding {
	findings := make([]HubImportFinding, 0, len(scan.Findings))
	for _, f := range scan.Findings {
		findings = append(findings, HubImportFinding{Rule: f.Rule, Severity: string(f.Severity), Path: f.Path, Message: f.Message})
	}
	return findings
}

// writeHubCommitFailure reports a failed hub write. A unique violation means a
// name collided with a concurrent write, so it is a conflict.
func writeHubCommitFailure(w http.ResponseWriter, r *http.Request, err error) {
	if isUniqueViolation(err) {
		writeErrorCode(w, http.StatusConflict, "name_conflict", "the import conflicts with an existing record; retry with another name")
		return
	}
	slog.Warn("hub import: save failed", append(logger.RequestAttrs(r), "error", err)...)
	writeError(w, http.StatusInternalServerError, "failed to save the import")
}

// hubStoreBundle writes a commit's zip under its immutable key and returns the key.
// It runs before the hub rows are written, so a committed row never points at a
// missing object.
func (h *Handler) hubStoreBundle(ctx context.Context, c *hubCommit, kind, refID string) (string, error) {
	key := hubBundleKey(c.workspaceID, kind, refID, c.meta.BundleDigest)
	if _, err := h.HubStorage.Upload(ctx, key, c.zipBytes, "application/zip", ""); err != nil {
		return "", err
	}
	return key, nil
}

// recordHubCommitItem writes the hub row of a committed skill or plugin and its
// version blob, on the caller's transaction, so the row commits with the write
// that produced the item.
func (h *Handler) recordHubCommitItem(ctx context.Context, q *db.Queries, c *hubCommit, refID pgtype.UUID, latestVersion, bundleKey string) (db.HubItem, error) {
	sourceID, err := h.hubSourceForCommit(ctx, q, c)
	if err != nil {
		return db.HubItem{}, err
	}
	item, err := q.UpsertHubItem(ctx, hubItemParams(c, sourceID, refID, latestVersion, bundleKey))
	if err != nil {
		return db.HubItem{}, err
	}
	if err := writeHubVersionBlob(ctx, q, item, c.user); err != nil {
		return db.HubItem{}, err
	}
	return item, nil
}

// hubItemFootprint reports whether a skill or plugin already has a hub row, and the
// bytes that row counts toward the storage quota. A commit that replaces the row
// adds only the difference, and a commit that creates one adds an item.
func (h *Handler) hubItemFootprint(ctx context.Context, ws pgtype.UUID, kind string, refID pgtype.UUID) (bool, int64, error) {
	item, err := h.Queries.GetHubItemByRef(ctx, db.GetHubItemByRefParams{WorkspaceID: ws, Kind: kind, RefID: refID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	return true, item.SizeBytes, nil
}

// hubNewItemCount is the item count a write adds: one when it creates the row.
func hubNewItemCount(exists bool) int {
	if exists {
		return 0
	}
	return 1
}

// checkHubCommitQuota refuses a write that would put the workspace over a hub
// quota. It writes the response and returns false when the write must stop. A
// write that adds nothing is never refused, so an over-quota workspace can still
// replace an item with a smaller one.
func (h *Handler) checkHubCommitQuota(ctx context.Context, w http.ResponseWriter, ws pgtype.UUID, addItems int, addBytes int64) bool {
	if addItems <= 0 && addBytes <= 0 {
		return true
	}
	if err := h.checkHubQuota(ctx, ws, addItems, addBytes); err != nil {
		if !writeHubQuotaError(w, err) {
			slog.Warn("hub quota check failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to check the hub quota")
		}
		return false
	}
	return true
}

// recordHubImportActivity writes the import_committed entry for a commit.
// The commit has already happened, so the entry is best effort.
func (h *Handler) recordHubImportActivity(r *http.Request, c *hubCommit, name string, item db.HubItem) {
	h.recordHubActivity(r.Context(), r, hubActivityEntry{
		WorkspaceID: c.ws,
		Action:      hubActionImportCommit,
		Kind:        string(c.detected.Kind),
		RefID:       item.RefID,
		ItemName:    name,
		SourceID:    item.SourceID,
		Detail: map[string]any{
			"version":     hubItemVersion(item.LatestVersion, item.BundleDigest),
			"source_type": c.meta.SourceType,
			"scan_status": c.scan.Status,
			"on_conflict": c.req.OnConflict,
		},
	})
}
