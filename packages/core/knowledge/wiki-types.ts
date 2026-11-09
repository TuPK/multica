/**
 * The Wiki of a knowledge base: pages written from its documents. The server
 * starts the generation and reads the pages; nothing here is the engine's own
 * shape.
 */

export type KnowledgeWikiPageType = "summary" | "entity" | "concept" | "unknown";

/** A base's Wiki at a glance. */
export interface KnowledgeWikiOverview {
  key: string;
  enabled: boolean;
  pages: number;
  pages_by_type: Record<string, number>;
  /** Published documents the Wiki is generated from. */
  documents: number;
  /** False when the deployment has no chat model, so Wiki cannot be generated. */
  chat_configured: boolean;
}

export interface KnowledgeWikiPage {
  slug: string;
  title: string;
  page_type: KnowledgeWikiPageType;
  status: string;
  summary: string;
  /** The page text in Markdown. Empty in a list. */
  content: string;
  aliases: string[];
  category_path: string[];
  updated_at: string;
}

export interface KnowledgeWikiPageList {
  pages: KnowledgeWikiPage[];
  total: number;
}

export interface KnowledgeWikiGenerateResult {
  /** True when this request turned Wiki on. */
  enabled: boolean;
  /** Documents sent for Wiki generation. */
  queued: number;
}

export interface ListKnowledgeWikiPagesParams {
  page?: number;
  page_size?: number;
  type?: Exclude<KnowledgeWikiPageType, "unknown">;
}
