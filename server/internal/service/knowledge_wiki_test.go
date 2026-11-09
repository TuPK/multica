package service

import (
	"context"
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/internal/knowledge"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestGenerateWikiTurnsWikiOnAndQueuesDocuments(t *testing.T) {
	env := newKnowledgeEnv(t)
	env.svc.LLM = &KnowledgeLLM{BaseURL: "https://llm.example.test/v1", ChatModel: "summary-chat"}
	ctx := context.Background()
	if _, err := env.svc.Enable(ctx, env.ws, env.user); err != nil {
		t.Fatalf("enable: %v", err)
	}
	project := mustUUID(t, testutil.New(env.pool, env.wsID, env.userID).Project(t, "Shop"))
	if _, err := env.svc.ProjectDocsManifest(ctx, env.ws, project, manifestOf(docFile("docs/prd.md", "# PRD\n\nBody"))); err != nil {
		t.Fatal(err)
	}
	if res, err := env.svc.IngestProjectDocs(ctx, env.ws, project, []ProjectDocFile{docFile("docs/prd.md", "# PRD\n\nBody")}); err != nil || res.Written != 1 {
		t.Fatalf("ingest: %+v %v", res, err)
	}
	key := ProjectBaseKey(project)
	actor := KnowledgeActor{Type: KnowledgeActorSystem}

	before, err := env.svc.WikiOverviewFor(ctx, env.ws, actor, key)
	if err != nil || before.Enabled || before.Documents != 1 || !before.ChatConfigured {
		t.Fatalf("before: %+v err=%v", before, err)
	}

	res, err := env.svc.GenerateWiki(ctx, env.ws, key)
	if err != nil || !res.Enabled || res.Queued != 1 {
		t.Fatalf("generate: %+v err=%v", res, err)
	}
	if n := env.fake.Calls(knowledge.OpReparseDocuments); n != 1 {
		t.Fatalf("reparse calls = %d, want 1", n)
	}
	// A second request does not enable again but queues the documents again.
	res, err = env.svc.GenerateWiki(ctx, env.ws, key)
	if err != nil || res.Enabled || res.Queued != 1 {
		t.Fatalf("second generate: %+v err=%v", res, err)
	}

	after, err := env.svc.WikiOverviewFor(ctx, env.ws, actor, key)
	if err != nil || !after.Enabled {
		t.Fatalf("after: %+v err=%v", after, err)
	}
}

func TestWikiPagesAreEmptyWhileWikiIsOff(t *testing.T) {
	env := newKnowledgeEnv(t)
	ctx := context.Background()
	if _, err := env.svc.Enable(ctx, env.ws, env.user); err != nil {
		t.Fatal(err)
	}
	list, err := env.svc.WikiPagesFor(ctx, env.ws, KnowledgeActor{Type: KnowledgeActorSystem}, "market", 1, 50, "")
	if err != nil || len(list.Pages) != 0 {
		t.Fatalf("list: %+v err=%v", list, err)
	}
	if _, err := env.svc.WikiPageFor(ctx, env.ws, KnowledgeActor{Type: KnowledgeActorSystem}, "nope", "x"); !errors.Is(err, ErrKnowledgeBaseNotFound) {
		t.Fatalf("unknown base: %v", err)
	}
}

func TestGenerateWikiNeedsAChatModel(t *testing.T) {
	env := newKnowledgeEnv(t)
	ctx := context.Background()
	if _, err := env.svc.Enable(ctx, env.ws, env.user); err != nil {
		t.Fatal(err)
	}
	if _, err := env.svc.GenerateWiki(ctx, env.ws, "market"); !errors.Is(err, ErrKnowledgeLLMNotConfigured) {
		t.Fatalf("err = %v, want ErrKnowledgeLLMNotConfigured", err)
	}
}
