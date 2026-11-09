import { z } from "zod";
import type {
  KnowledgeWikiGenerateResult,
  KnowledgeWikiOverview,
  KnowledgeWikiPage,
  KnowledgeWikiPageList,
} from "./wiki-types";

const PageTypeSchema = z
  .string()
  .default("unknown")
  .transform((v) => (v === "summary" || v === "entity" || v === "concept" ? v : "unknown") as
    "summary" | "entity" | "concept" | "unknown");

const stringList = z
  .array(z.string())
  .nullish()
  .transform((v) => v ?? []);

export const KnowledgeWikiOverviewSchema = z
  .object({
    key: z.string().default(""),
    enabled: z.boolean().default(false),
    pages: z.number().default(0),
    pages_by_type: z
      .record(z.string(), z.number())
      .nullish()
      .transform((v) => v ?? {}),
    documents: z.number().default(0),
    chat_configured: z.boolean().default(false),
  })
  .loose();

export const KnowledgeWikiPageSchema = z
  .object({
    slug: z.string().default(""),
    title: z.string().default(""),
    page_type: PageTypeSchema,
    status: z.string().default(""),
    summary: z.string().default(""),
    content: z.string().default(""),
    aliases: stringList,
    category_path: stringList,
    updated_at: z.string().default(""),
  })
  .loose();

export const KnowledgeWikiPageListSchema = z
  .object({
    pages: z
      .array(KnowledgeWikiPageSchema)
      .nullish()
      .transform((v) => v ?? []),
    total: z.number().default(0),
  })
  .loose();

export const KnowledgeWikiGenerateResultSchema = z
  .object({
    enabled: z.boolean().default(false),
    queued: z.number().default(0),
  })
  .loose();

export const FALLBACK_KNOWLEDGE_WIKI_OVERVIEW: KnowledgeWikiOverview = {
  key: "",
  enabled: false,
  pages: 0,
  pages_by_type: {},
  documents: 0,
  chat_configured: false,
};

export const FALLBACK_KNOWLEDGE_WIKI_PAGE_LIST: KnowledgeWikiPageList = { pages: [], total: 0 };

export const FALLBACK_KNOWLEDGE_WIKI_PAGE: KnowledgeWikiPage = {
  slug: "",
  title: "",
  page_type: "unknown",
  status: "",
  summary: "",
  content: "",
  aliases: [],
  category_path: [],
  updated_at: "",
};

export const FALLBACK_KNOWLEDGE_WIKI_GENERATE: KnowledgeWikiGenerateResult = { enabled: false, queued: 0 };
