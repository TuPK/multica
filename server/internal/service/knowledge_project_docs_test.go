package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func docFile(path, content string) ProjectDocFile {
	sum := sha256.Sum256([]byte(content))
	return ProjectDocFile{Path: path, SHA256: hex.EncodeToString(sum[:]), Content: content}
}

func manifestOf(files ...ProjectDocFile) []ProjectDocFile {
	out := make([]ProjectDocFile, len(files))
	for i, f := range files {
		out[i] = ProjectDocFile{Path: f.Path, SHA256: f.SHA256}
	}
	return out
}

func TestProjectDocsIngestFollowsTheDirectory(t *testing.T) {
	env := newKnowledgeEnv(t)
	ctx := context.Background()
	if _, err := env.svc.Enable(ctx, env.ws, env.user); err != nil {
		t.Fatalf("enable: %v", err)
	}
	fx := testutil.New(env.pool, env.wsID, env.userID)
	project := mustUUID(t, fx.Project(t, "Shop"))

	prd := docFile("docs/prd.md", "# Product requirements\n\nBuyers pick a taste mission.")
	arch := docFile("docs/architecture.md", "# Architecture\n\nSupabase and Vite.")

	need, err := env.svc.ProjectDocsManifest(ctx, env.ws, project, manifestOf(prd, arch))
	if err != nil || len(need) != 2 {
		t.Fatalf("first manifest: need=%v err=%v", need, err)
	}
	res, err := env.svc.IngestProjectDocs(ctx, env.ws, project, []ProjectDocFile{prd, arch})
	if err != nil || res.Written != 2 || len(res.Failed) != 0 {
		t.Fatalf("first ingest: %+v err=%v", res, err)
	}
	if _, err := env.q.GetKnowledgeBaseByKey(ctx, db.GetKnowledgeBaseByKeyParams{WorkspaceID: env.ws, Key: ProjectBaseKey(project)}); err != nil {
		t.Fatalf("project base missing: %v", err)
	}

	// Nothing changed: nothing is requested.
	if need, err = env.svc.ProjectDocsManifest(ctx, env.ws, project, manifestOf(prd, arch)); err != nil || len(need) != 0 {
		t.Fatalf("unchanged manifest: need=%v err=%v", need, err)
	}

	// The PRD changes and the architecture file is deleted.
	prd2 := docFile("docs/prd.md", "# Product requirements\n\nBuyers pick a taste mission and a budget.")
	need, err = env.svc.ProjectDocsManifest(ctx, env.ws, project, manifestOf(prd2))
	if err != nil || len(need) != 1 || need[0] != "docs/prd.md" {
		t.Fatalf("changed manifest: need=%v err=%v", need, err)
	}
	files, err := env.q.ListKnowledgeProjectFiles(ctx, db.ListKnowledgeProjectFilesParams{WorkspaceID: env.ws, ProjectID: project})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path == "docs/architecture.md" && !f.ArchivedAt.Valid {
			t.Fatal("the deleted file was not archived")
		}
	}
	if res, err = env.svc.IngestProjectDocs(ctx, env.ws, project, []ProjectDocFile{prd2}); err != nil || res.Written != 1 {
		t.Fatalf("replace ingest: %+v err=%v", res, err)
	}
}

func TestProjectDocsRefuseBadInput(t *testing.T) {
	env := newKnowledgeEnv(t)
	ctx := context.Background()
	if _, err := env.svc.Enable(ctx, env.ws, env.user); err != nil {
		t.Fatal(err)
	}
	project := mustUUID(t, testutil.New(env.pool, env.wsID, env.userID).Project(t, "Shop"))
	if _, err := env.svc.ProjectDocsManifest(ctx, env.ws, project, nil); err == nil {
		t.Fatal("an empty manifest must be refused")
	}
	bad := docFile("../secrets.md", "x")
	if _, err := env.svc.ProjectDocsManifest(ctx, env.ws, project, manifestOf(bad)); err == nil {
		t.Fatal("a path that leaves the directory must be refused")
	}
	other := mustUUID(t, "ef025dc6-2555-4c4a-85c9-d9618e130fa6")
	if _, err := env.svc.ProjectDocsManifest(ctx, env.ws, other, manifestOf(docFile("a.md", "a"))); err == nil {
		t.Fatal("a project of another workspace must be refused")
	}
}
