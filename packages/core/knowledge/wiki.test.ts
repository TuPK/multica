// @vitest-environment node
import { describe, expect, it } from "vitest";
import { parseWithFallback } from "../api/schema";
import {
  FALLBACK_KNOWLEDGE_WIKI_OVERVIEW,
  FALLBACK_KNOWLEDGE_WIKI_PAGE_LIST,
  KnowledgeWikiOverviewSchema,
  KnowledgeWikiPageListSchema,
} from "./wiki-schemas";

describe("wiki schemas", () => {
  it("fills what an older server leaves out", () => {
    const parsed = parseWithFallback({ key: "k", enabled: true }, KnowledgeWikiOverviewSchema, FALLBACK_KNOWLEDGE_WIKI_OVERVIEW, {
      endpoint: "test",
    });
    expect(parsed).toMatchObject({ enabled: true, pages: 0, documents: 0, chat_configured: false, pages_by_type: {} });
  });

  it("falls back to an empty overview on a malformed body", () => {
    const parsed = parseWithFallback({ enabled: "yes", pages: "many" }, KnowledgeWikiOverviewSchema, FALLBACK_KNOWLEDGE_WIKI_OVERVIEW, {
      endpoint: "test",
    });
    expect(parsed).toEqual(FALLBACK_KNOWLEDGE_WIKI_OVERVIEW);
  });

  it("reads null lists as empty and maps an unknown page type to unknown", () => {
    const parsed = parseWithFallback(
      { total: 1, pages: [{ slug: "a/b", title: "T", page_type: "newtype", aliases: null, category_path: null }] },
      KnowledgeWikiPageListSchema,
      FALLBACK_KNOWLEDGE_WIKI_PAGE_LIST,
      { endpoint: "test" },
    );
    expect(parsed.pages[0]).toMatchObject({ page_type: "unknown", aliases: [], category_path: [] });
  });

  it("falls back to no pages when pages is not a list", () => {
    const parsed = parseWithFallback({ pages: "none" }, KnowledgeWikiPageListSchema, FALLBACK_KNOWLEDGE_WIKI_PAGE_LIST, {
      endpoint: "test",
    });
    expect(parsed).toEqual(FALLBACK_KNOWLEDGE_WIKI_PAGE_LIST);
  });
});
