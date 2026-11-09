package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Project docs ingest. For every project of a workspace whose local_directory
// resource is pinned to this daemon, the daemon reads the project's Markdown and
// sends it to the server, which keeps the project's knowledge base in step.
// The server owns what to keep: the daemon sends a manifest first and uploads
// only the files the server asks for.

const (
	projectDocsStartDelay  = 90 * time.Second
	projectDocsInterval    = 30 * time.Minute
	projectDocsMaxFiles    = 500
	projectDocsMaxFileSize = 1 << 20
	// projectDocsBatchBytes keeps one upload well under the server's body limit.
	projectDocsBatchBytes = 8 << 20
	projectDocsBatchFiles = 50
	// projectDocsDisableEnv turns the ingest off on a machine.
	projectDocsDisableEnv = "MULTICA_PROJECT_DOCS_SYNC"
)

// projectDocsRootFiles are the Markdown files read from the project root.
var projectDocsRootFiles = []string{"README.md", "AGENTS.md", "CLAUDE.md"}

// projectDocsRootDirs are the directories whose Markdown is read, recursively.
var projectDocsRootDirs = []string{"docs"}

// projectDocsSkipDirs are never entered: dependencies, build output and the
// tooling folders of agents, which are not the project's own knowledge.
var projectDocsSkipDirs = map[string]bool{
	"node_modules": true, "dist": true, "build": true, "vendor": true, "target": true, "coverage": true,
}

// ProjectDocFile mirrors the server's file shape.
type ProjectDocFile struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Content string `json:"content,omitempty"`
}

type projectDocSource struct {
	ProjectID    string `json:"project_id"`
	ProjectTitle string `json:"project_title"`
	ResourceID   string `json:"resource_id"`
	LocalPath    string `json:"local_path"`
	DaemonID     string `json:"daemon_id"`
}

type projectDocSourcesResponse struct {
	Sources []projectDocSource `json:"sources"`
}

type projectDocsRequest struct {
	Files []ProjectDocFile `json:"files"`
}

type projectDocsManifestResponse struct {
	Need []string `json:"need"`
}

type projectDocsResultResponse struct {
	Written int      `json:"written"`
	Failed  []string `json:"failed"`
}

func (c *Client) ListProjectDocSources(ctx context.Context, workspaceID string) ([]projectDocSource, error) {
	var resp projectDocSourcesResponse
	if err := c.getJSON(ctx, fmt.Sprintf("/api/daemon/workspaces/%s/knowledge/project-sources", workspaceID), &resp); err != nil {
		return nil, err
	}
	return resp.Sources, nil
}

func (c *Client) PostProjectDocsManifest(ctx context.Context, workspaceID, projectID string, files []ProjectDocFile) ([]string, error) {
	var resp projectDocsManifestResponse
	path := fmt.Sprintf("/api/daemon/workspaces/%s/projects/%s/knowledge/manifest", workspaceID, projectID)
	if err := c.postJSON(ctx, path, projectDocsRequest{Files: files}, &resp); err != nil {
		return nil, err
	}
	return resp.Need, nil
}

func (c *Client) PostProjectDocs(ctx context.Context, workspaceID, projectID string, files []ProjectDocFile) (projectDocsResultResponse, error) {
	var resp projectDocsResultResponse
	path := fmt.Sprintf("/api/daemon/workspaces/%s/projects/%s/knowledge/docs", workspaceID, projectID)
	err := c.postJSON(ctx, path, projectDocsRequest{Files: files}, &resp)
	return resp, err
}

// collectProjectDocs reads the Markdown a project owns: the root files and the
// docs directory. Symlinks, hidden directories, tooling directories, empty and
// oversized files are skipped. A scan with no file is an error, so the server
// is never told a project has no documents because a directory was unreadable.
func collectProjectDocs(root string) ([]ProjectDocFile, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}
	seen := map[string]bool{}
	var files []ProjectDocFile
	add := func(abs string) error {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if seen[rel] {
			return nil
		}
		fi, err := os.Lstat(abs)
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > projectDocsMaxFileSize {
			return nil
		}
		body, err := os.ReadFile(abs)
		if err != nil || strings.TrimSpace(string(body)) == "" {
			return nil
		}
		if len(files) >= projectDocsMaxFiles {
			return fmt.Errorf("more than %d Markdown files", projectDocsMaxFiles)
		}
		sum := sha256.Sum256(body)
		seen[rel] = true
		files = append(files, ProjectDocFile{Path: rel, SHA256: hex.EncodeToString(sum[:]), Content: string(body)})
		return nil
	}
	for _, name := range projectDocsRootFiles {
		if err := add(filepath.Join(root, name)); err != nil {
			return nil, err
		}
	}
	for _, dir := range projectDocsRootDirs {
		base := filepath.Join(root, dir)
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if p != base && (strings.HasPrefix(name, ".") || projectDocsSkipDirs[name]) {
					return fs.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || !strings.EqualFold(filepath.Ext(p), ".md") {
				return nil
			}
			return add(p)
		})
		if err != nil {
			return nil, err
		}
	}
	if len(files) == 0 {
		return nil, errors.New("no Markdown files found")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// syncProjectDocs sends one project directory to the server.
func (d *Daemon) syncProjectDocs(ctx context.Context, workspaceID string, src projectDocSource) error {
	files, err := collectProjectDocs(src.LocalPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", src.LocalPath, err)
	}
	manifest := make([]ProjectDocFile, len(files))
	byPath := make(map[string]ProjectDocFile, len(files))
	for i, f := range files {
		manifest[i] = ProjectDocFile{Path: f.Path, SHA256: f.SHA256}
		byPath[f.Path] = f
	}
	need, err := d.client.PostProjectDocsManifest(ctx, workspaceID, src.ProjectID, manifest)
	if err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	var batch []ProjectDocFile
	size := 0
	written := 0
	var failed []string
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		res, err := d.client.PostProjectDocs(ctx, workspaceID, src.ProjectID, batch)
		if err != nil {
			return fmt.Errorf("upload: %w", err)
		}
		written += res.Written
		failed = append(failed, res.Failed...)
		batch, size = nil, 0
		return nil
	}
	for _, p := range need {
		f, ok := byPath[p]
		if !ok {
			continue
		}
		if len(batch) >= projectDocsBatchFiles || size+len(f.Content) > projectDocsBatchBytes {
			if err := flush(); err != nil {
				return err
			}
		}
		batch = append(batch, f)
		size += len(f.Content)
	}
	if err := flush(); err != nil {
		return err
	}
	if written > 0 || len(failed) > 0 {
		d.logger.Info("project docs: synced", "project", src.ProjectTitle, "files", len(files), "written", written, "failed", len(failed))
	}
	for _, f := range failed {
		d.logger.Warn("project docs: file not ingested", "project", src.ProjectTitle, "detail", f)
	}
	return nil
}

// ownsProjectSource reports whether a local_directory resource is pinned to
// this daemon, under its current id or a historical one.
func (d *Daemon) ownsProjectSource(src projectDocSource) bool {
	if src.DaemonID == "" || strings.TrimSpace(src.LocalPath) == "" {
		return false
	}
	if src.DaemonID == d.cfg.DaemonID {
		return true
	}
	for _, id := range d.cfg.LegacyDaemonIDs {
		if id == src.DaemonID {
			return true
		}
	}
	return false
}

func (d *Daemon) runProjectDocsSync(ctx context.Context) {
	d.mu.Lock()
	ids := make([]string, 0, len(d.workspaces))
	for id := range d.workspaces {
		ids = append(ids, id)
	}
	d.mu.Unlock()
	for _, wsID := range ids {
		callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		sources, err := d.client.ListProjectDocSources(callCtx, wsID)
		cancel()
		if err != nil {
			d.logger.Debug("project docs: list sources failed", "workspace_id", wsID, "error", err)
			continue
		}
		for _, src := range sources {
			if !d.ownsProjectSource(src) {
				continue
			}
			runCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			err := d.syncProjectDocs(runCtx, wsID, src)
			cancel()
			if err != nil {
				d.logger.Warn("project docs: sync failed", "project", src.ProjectTitle, "error", err)
			}
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// projectDocsLoop keeps project knowledge bases in step with their directories.
func (d *Daemon) projectDocsLoop(ctx context.Context) {
	if v := strings.ToLower(strings.TrimSpace(os.Getenv(projectDocsDisableEnv))); v == "false" || v == "0" || v == "off" {
		d.logger.Info("project docs: disabled")
		return
	}
	if err := sleepWithContext(ctx, projectDocsStartDelay); err != nil {
		return
	}
	d.runProjectDocsSync(ctx)
	ticker := time.NewTicker(projectDocsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.runProjectDocsSync(ctx)
		}
	}
}
