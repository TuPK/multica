package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The project docs ingest. A daemon reads the Markdown of a project's local
// directory and sends it here, so the project's own knowledge base follows the
// project without anyone uploading a file. The daemon first sends a manifest
// (path and hash of every file) and receives the paths whose content the server
// lacks, then uploads only those. A file that left the directory has its
// document removed when the manifest arrives.

const (
	// ProjectDocsMaxFiles bounds one project's files, in a manifest and an upload.
	ProjectDocsMaxFiles = 500
	// ProjectDocsMaxFileBytes bounds one file's text.
	ProjectDocsMaxFileBytes = 1 << 20
	// projectDocsKeyPrefix names the idempotency key of a project document.
	projectDocsKeyPrefix = "project-docs:"
)

// ProjectDocFile is one Markdown file of a project directory: its path from the
// directory root, the hash of its text, and the text (empty in a manifest).
type ProjectDocFile struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Content string `json:"content,omitempty"`
}

// ProjectDocsResult reports one upload: how many documents were written and
// which files failed, with the reason.
type ProjectDocsResult struct {
	Written int      `json:"written"`
	Failed  []string `json:"failed"`
}

// ProjectDocSource is a local directory attached to a live project.
type ProjectDocSource struct {
	ProjectID    string `json:"project_id"`
	ProjectTitle string `json:"project_title"`
	ResourceID   string `json:"resource_id"`
	LocalPath    string `json:"local_path"`
	DaemonID     string `json:"daemon_id"`
}

var projectDocHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// validProjectDocPath accepts a relative, clean, slash-separated Markdown path.
func validProjectDocPath(p string) bool {
	if p == "" || len(p) > 1024 || strings.Contains(p, `\`) || path.IsAbs(p) || path.Clean(p) != p {
		return false
	}
	if p == ".." || strings.HasPrefix(p, "../") {
		return false
	}
	return strings.EqualFold(path.Ext(p), ".md")
}

// ProjectDocSources lists the local directories of the workspace's live
// projects. It is empty while knowledge is not ready, so a daemon reads nothing
// it could not deliver.
func (s *KnowledgeService) ProjectDocSources(ctx context.Context, workspaceID pgtype.UUID) ([]ProjectDocSource, error) {
	if _, err := s.workspaceClient(ctx, workspaceID); err != nil {
		var notReady *KnowledgeNotReadyError
		if errors.As(err, &notReady) {
			return nil, nil
		}
		return nil, err
	}
	rows, err := s.Queries.ListProjectDocSources(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list project doc sources: %w", err)
	}
	out := make([]ProjectDocSource, 0, len(rows))
	for _, r := range rows {
		var ref struct {
			LocalPath string `json:"local_path"`
			DaemonID  string `json:"daemon_id"`
		}
		if err := json.Unmarshal(r.ResourceRef, &ref); err != nil || ref.LocalPath == "" {
			continue
		}
		out = append(out, ProjectDocSource{
			ProjectID: util.UUIDToString(r.ProjectID), ProjectTitle: r.ProjectTitle, ResourceID: util.UUIDToString(r.ResourceID),
			LocalPath: ref.LocalPath, DaemonID: ref.DaemonID,
		})
	}
	return out, nil
}

// projectForDocs checks the project is a live project of the workspace and that
// its base exists, creating the base when it is missing.
func (s *KnowledgeService) projectForDocs(ctx context.Context, workspaceID, projectID pgtype.UUID) error {
	project, err := s.Queries.GetProjectInWorkspace(ctx, db.GetProjectInWorkspaceParams{ID: projectID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && project.Status == "cancelled") {
		return &KnowledgeInvalidError{"project is not a live project of this workspace"}
	}
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	if _, err := s.workspaceClient(ctx, workspaceID); err != nil {
		return err
	}
	return s.EnsureProjectBase(ctx, workspaceID, projectID, project.Title)
}

// ProjectDocsManifest takes the complete list of a project directory's files.
// It removes the documents of files no longer listed and returns the paths whose
// text must be uploaded: new files and files whose hash changed. An empty
// manifest is refused, so a failed scan cannot empty a project's base.
func (s *KnowledgeService) ProjectDocsManifest(ctx context.Context, workspaceID, projectID pgtype.UUID, files []ProjectDocFile) ([]string, error) {
	if len(files) == 0 {
		return nil, &KnowledgeInvalidError{"the manifest lists no files"}
	}
	if len(files) > ProjectDocsMaxFiles {
		return nil, &KnowledgeInvalidError{fmt.Sprintf("more than %d files", ProjectDocsMaxFiles)}
	}
	listed := make(map[string]string, len(files))
	for _, f := range files {
		if !validProjectDocPath(f.Path) {
			return nil, &KnowledgeInvalidError{"invalid file path " + f.Path}
		}
		if !projectDocHash.MatchString(f.SHA256) {
			return nil, &KnowledgeInvalidError{"invalid hash for " + f.Path}
		}
		listed[f.Path] = f.SHA256
	}
	if err := s.projectForDocs(ctx, workspaceID, projectID); err != nil {
		return nil, err
	}
	recorded, err := s.Queries.ListKnowledgeProjectFiles(ctx, db.ListKnowledgeProjectFilesParams{WorkspaceID: workspaceID, ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("list project files: %w", err)
	}
	have := make(map[string]db.KnowledgeProjectFile, len(recorded))
	for _, r := range recorded {
		have[r.Path] = r
	}
	for p, prev := range have {
		if _, ok := listed[p]; ok || prev.ArchivedAt.Valid {
			continue
		}
		if prev.KnowledgeLinkID.Valid {
			if err := s.removeSyncedDocument(ctx, workspaceID, prev.KnowledgeLinkID); err != nil {
				return nil, fmt.Errorf("remove %s: %w", p, err)
			}
		}
		if err := s.Queries.ArchiveKnowledgeProjectFile(ctx, db.ArchiveKnowledgeProjectFileParams{WorkspaceID: workspaceID, ProjectID: projectID, Path: p}); err != nil {
			return nil, fmt.Errorf("archive %s: %w", p, err)
		}
	}
	var need []string
	for _, f := range files {
		prev, ok := have[f.Path]
		if ok && !prev.ArchivedAt.Valid && prev.ContentHash == f.SHA256 {
			continue
		}
		need = append(need, f.Path)
	}
	return need, nil
}

// IngestProjectDocs writes uploaded files into the project's base. A changed
// file replaces its old document. One file's failure is reported and does not
// stop the others.
func (s *KnowledgeService) IngestProjectDocs(ctx context.Context, workspaceID, projectID pgtype.UUID, files []ProjectDocFile) (ProjectDocsResult, error) {
	res := ProjectDocsResult{Failed: []string{}}
	if len(files) == 0 || len(files) > ProjectDocsMaxFiles {
		return res, &KnowledgeInvalidError{fmt.Sprintf("send between 1 and %d files", ProjectDocsMaxFiles)}
	}
	if err := s.projectForDocs(ctx, workspaceID, projectID); err != nil {
		return res, err
	}
	recorded, err := s.Queries.ListKnowledgeProjectFiles(ctx, db.ListKnowledgeProjectFilesParams{WorkspaceID: workspaceID, ProjectID: projectID})
	if err != nil {
		return res, fmt.Errorf("list project files: %w", err)
	}
	have := make(map[string]db.KnowledgeProjectFile, len(recorded))
	for _, r := range recorded {
		have[r.Path] = r
	}
	for _, f := range files {
		if !validProjectDocPath(f.Path) {
			res.Failed = append(res.Failed, f.Path+": invalid path")
			continue
		}
		if len(f.Content) > ProjectDocsMaxFileBytes || strings.TrimSpace(f.Content) == "" {
			res.Failed = append(res.Failed, f.Path+": empty or over the size limit")
			continue
		}
		sum := sha256.Sum256([]byte(f.Content))
		hash := hex.EncodeToString(sum[:])
		prev, ok := have[f.Path]
		if ok && !prev.ArchivedAt.Valid && prev.ContentHash == hash {
			continue
		}
		if ok && !prev.ArchivedAt.Valid && prev.KnowledgeLinkID.Valid {
			if err := s.removeSyncedDocument(ctx, workspaceID, prev.KnowledgeLinkID); err != nil {
				res.Failed = append(res.Failed, f.Path+": "+err.Error())
				continue
			}
		}
		view, _, err := s.CreateDocument(ctx, workspaceID, KnowledgeActor{Type: KnowledgeActorSystem}, KnowledgeDocumentInput{
			KB: ProjectBaseKey(projectID), Title: projectDocTitle(f.Path, f.Content), Content: f.Content,
			SourceType: "sync", IdempotencyKey: projectDocsKeyPrefix + util.UUIDToString(projectID) + ":" + f.Path + ":" + hash, Publish: true,
		})
		if err != nil {
			res.Failed = append(res.Failed, f.Path+": "+err.Error())
			continue
		}
		linkID, err := util.ParseUUID(view.ID)
		if err != nil {
			return res, fmt.Errorf("project docs: document id: %w", err)
		}
		if dir := path.Dir(f.Path); dir != "." {
			if err := s.Queries.SetKnowledgeLinkFolder(ctx, db.SetKnowledgeLinkFolderParams{ID: linkID, WorkspaceID: workspaceID, Folder: dir}); err != nil {
				return res, fmt.Errorf("set project file folder: %w", err)
			}
		}
		if err := s.Queries.UpsertKnowledgeProjectFile(ctx, db.UpsertKnowledgeProjectFileParams{
			WorkspaceID: workspaceID, ProjectID: projectID, Path: f.Path, KnowledgeLinkID: linkID, ContentHash: hash,
		}); err != nil {
			return res, fmt.Errorf("record project file: %w", err)
		}
		res.Written++
	}
	return res, nil
}

// projectDocTitle is the first level-one heading with the path in brackets, or
// the path alone. The path keeps two files with one heading apart.
func projectDocTitle(p, content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			if h := strings.TrimSpace(strings.TrimPrefix(line, "# ")); h != "" {
				return truncateRunes(h+" ("+p+")", knowledgeMaxTitle)
			}
		}
	}
	return truncateRunes(p, knowledgeMaxTitle)
}
