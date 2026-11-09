package service

import (
	"context"
	"testing"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestProjectBaseKeyIsStableAndShort(t *testing.T) {
	id := mustUUID(t, "25cbad1a-a0ee-457f-8424-55ac56cba436")
	if got := ProjectBaseKey(id); got != "project-25cbad1aa0ee" {
		t.Fatalf("ProjectBaseKey = %q", got)
	}
	if !repoKBKeyPattern.MatchString(ProjectBaseKey(id)) {
		t.Fatal("a project base key must be a valid knowledge base key")
	}
}

func TestProjectBaseNamesDifferForEqualTitles(t *testing.T) {
	a := projectBaseName(mustUUID(t, "25cbad1a-a0ee-457f-8424-55ac56cba436"), "Shop")
	b := projectBaseName(mustUUID(t, "ef025dc6-2555-4c4a-85c9-d9618e130fa6"), "Shop")
	if a == b {
		t.Fatalf("two projects titled alike share the base name %q", a)
	}
}

func TestEnsureProjectBaseCreatesOnceAndSkipsDisabledWorkspace(t *testing.T) {
	env := newKnowledgeEnv(t)
	ctx := context.Background()
	project := mustUUID(t, "25cbad1a-a0ee-457f-8424-55ac56cba436")

	// Knowledge not enabled yet: nothing is created and nothing fails.
	if err := env.svc.EnsureProjectBase(ctx, env.ws, project, "Mua Ngon"); err != nil {
		t.Fatalf("disabled workspace: %v", err)
	}
	if _, err := env.q.GetKnowledgeBaseByKey(ctx, db.GetKnowledgeBaseByKeyParams{WorkspaceID: env.ws, Key: ProjectBaseKey(project)}); err == nil {
		t.Fatal("a base was created for a workspace without knowledge")
	}

	if _, err := env.svc.Enable(ctx, env.ws, env.user); err != nil {
		t.Fatalf("enable: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := env.svc.EnsureProjectBase(ctx, env.ws, project, "Mua Ngon"); err != nil {
			t.Fatalf("ensure %d: %v", i, err)
		}
	}
	bases, err := env.q.ListKnowledgeBases(ctx, env.ws)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, b := range bases {
		if b.Key == ProjectBaseKey(project) {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want one project base, got %d", n)
	}
}
