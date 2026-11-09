package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/knowledge"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The Wiki of a knowledge base: pages WeKnora writes from the base's documents
// (a summary page per document, and entity and concept pages across them).
// Multica shows them and starts the generation; WeKnora writes them in the
// background with the chat model.

// reparseBatch is how many documents one reparse request names.
const reparseBatch = 50

// WikiOverview is a base's Wiki at a glance.
type WikiOverview struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
	// Pages and PagesByType count the generated pages; zero while Wiki is off.
	Pages       int            `json:"pages"`
	PagesByType map[string]int `json:"pages_by_type"`
	// Documents is how many published documents the base holds, which is what
	// the Wiki is generated from.
	Documents int `json:"documents"`
	// ChatConfigured is false when the deployment has no chat model, so Wiki
	// cannot be generated.
	ChatConfigured bool `json:"chat_configured"`
}

// WikiGenerateResult reports a generation request.
type WikiGenerateResult struct {
	// Enabled is true when this request turned Wiki on.
	Enabled bool `json:"enabled"`
	// Queued is how many documents were sent for Wiki generation.
	Queued int `json:"queued"`
}

// wikiBase returns the workspace client and a base the actor may read.
func (s *KnowledgeService) wikiBase(ctx context.Context, workspaceID pgtype.UUID, actor KnowledgeActor, key string) (knowledge.Workspace, db.KnowledgeBase, error) {
	ws, err := s.workspaceClient(ctx, workspaceID)
	if err != nil {
		return nil, db.KnowledgeBase{}, err
	}
	base, err := s.baseByKey(ctx, workspaceID, key)
	if err != nil {
		return nil, db.KnowledgeBase{}, err
	}
	grants, err := s.actorKnowledgeGrants(ctx, workspaceID, actor)
	if err != nil {
		return nil, db.KnowledgeBase{}, err
	}
	if !knowledgeCanRead(actor, base.Visibility, util.UUIDToString(base.ID), grants) {
		return nil, db.KnowledgeBase{}, ErrKnowledgeBaseNotFound
	}
	return ws, base, nil
}

// WikiOverviewFor reports the Wiki of a base the actor may read.
func (s *KnowledgeService) WikiOverviewFor(ctx context.Context, workspaceID pgtype.UUID, actor KnowledgeActor, key string) (WikiOverview, error) {
	ws, base, err := s.wikiBase(ctx, workspaceID, actor, key)
	if err != nil {
		return WikiOverview{}, err
	}
	links, err := s.Queries.ListPublishedKnowledgeLinksByBase(ctx, db.ListPublishedKnowledgeLinksByBaseParams{WorkspaceID: workspaceID, KnowledgeBaseID: base.ID})
	if err != nil {
		return WikiOverview{}, fmt.Errorf("list base documents: %w", err)
	}
	out := WikiOverview{
		Key: key, Enabled: base.WikiEnabled, PagesByType: map[string]int{}, Documents: len(links), ChatConfigured: s.chatConfigured(),
	}
	if !base.WikiEnabled {
		return out, nil
	}
	stats, err := ws.WikiStats(ctx, base.WeknoraKbID)
	if errors.Is(err, knowledge.ErrInvalid) {
		// Multica says on, WeKnora says off: report what WeKnora will do.
		out.Enabled = false
		return out, nil
	}
	if err != nil {
		return WikiOverview{}, err
	}
	out.Pages, out.PagesByType = stats.TotalPages, stats.PagesByType
	return out, nil
}

// WikiPagesFor lists a page of a base's Wiki pages, without their text.
func (s *KnowledgeService) WikiPagesFor(ctx context.Context, workspaceID pgtype.UUID, actor KnowledgeActor, key string, page, pageSize int, pageType string) (knowledge.WikiPageList, error) {
	ws, base, err := s.wikiBase(ctx, workspaceID, actor, key)
	if err != nil {
		return knowledge.WikiPageList{}, err
	}
	if !base.WikiEnabled {
		return knowledge.WikiPageList{Pages: []knowledge.WikiPage{}}, nil
	}
	list, err := ws.ListWikiPages(ctx, base.WeknoraKbID, page, pageSize, pageType)
	if errors.Is(err, knowledge.ErrInvalid) {
		return knowledge.WikiPageList{Pages: []knowledge.WikiPage{}}, nil
	}
	return list, err
}

// WikiPageFor returns one Wiki page with its text.
func (s *KnowledgeService) WikiPageFor(ctx context.Context, workspaceID pgtype.UUID, actor KnowledgeActor, key, slug string) (knowledge.WikiPage, error) {
	ws, base, err := s.wikiBase(ctx, workspaceID, actor, key)
	if err != nil {
		return knowledge.WikiPage{}, err
	}
	if slug == "" {
		return knowledge.WikiPage{}, &KnowledgeInvalidError{"slug is required"}
	}
	return ws.GetWikiPage(ctx, base.WeknoraKbID, slug)
}

// GenerateWiki turns Wiki on for a base and has WeKnora write the pages of the
// documents it already holds. Documents added later get their pages as they are
// stored. Running it again rewrites the pages, which is how a base picks up a
// changed language or model.
func (s *KnowledgeService) GenerateWiki(ctx context.Context, workspaceID pgtype.UUID, key string) (WikiGenerateResult, error) {
	var out WikiGenerateResult
	ws, base, err := s.wikiBase(ctx, workspaceID, KnowledgeActor{Type: KnowledgeActorSystem}, key)
	if err != nil {
		return out, err
	}
	if !base.WikiEnabled {
		on := true
		if _, err := s.UpdateBaseSettings(ctx, workspaceID, key, KnowledgeBaseSettingsPatch{Wiki: &on}); err != nil {
			return out, err
		}
		out.Enabled = true
	}
	links, err := s.Queries.ListPublishedKnowledgeLinksByBase(ctx, db.ListPublishedKnowledgeLinksByBaseParams{WorkspaceID: workspaceID, KnowledgeBaseID: base.ID})
	if err != nil {
		return out, fmt.Errorf("list base documents: %w", err)
	}
	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.WeknoraKnowledgeID)
	}
	for start := 0; start < len(ids); start += reparseBatch {
		end := start + reparseBatch
		if end > len(ids) {
			end = len(ids)
		}
		if err := ws.ReparseDocuments(ctx, base.WeknoraKbID, ids[start:end]); err != nil {
			return out, fmt.Errorf("queue wiki generation: %w", err)
		}
		out.Queued += end - start
	}
	return out, nil
}
