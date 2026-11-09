import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import { knowledgeKeys } from "./queries";
import type { ListKnowledgeWikiPagesParams } from "./wiki-types";

export const knowledgeWikiKeys = {
  all: (wsId: string, key: string) => [...knowledgeKeys.all(wsId), "wiki", key] as const,
  overview: (wsId: string, key: string) => [...knowledgeWikiKeys.all(wsId, key), "overview"] as const,
  pages: (wsId: string, key: string, params?: ListKnowledgeWikiPagesParams) =>
    [...knowledgeWikiKeys.all(wsId, key), "pages", params ?? {}] as const,
  page: (wsId: string, key: string, slug: string) =>
    [...knowledgeWikiKeys.all(wsId, key), "page", slug] as const,
};

/** A base's Wiki at a glance. Polls while pages are still being written. */
export function knowledgeWikiOverviewOptions(wsId: string, key: string, poll = false) {
  return queryOptions({
    queryKey: knowledgeWikiKeys.overview(wsId, key),
    queryFn: () => api.getKnowledgeWiki(key),
    enabled: key.length > 0,
    refetchInterval: poll ? 15_000 : false,
  });
}

export function knowledgeWikiPagesOptions(
  wsId: string,
  key: string,
  params?: ListKnowledgeWikiPagesParams,
  poll = false,
) {
  return queryOptions({
    queryKey: knowledgeWikiKeys.pages(wsId, key, params),
    queryFn: () => api.listKnowledgeWikiPages(key, params),
    enabled: key.length > 0,
    refetchInterval: poll ? 15_000 : false,
  });
}

export function knowledgeWikiPageOptions(wsId: string, key: string, slug: string) {
  return queryOptions({
    queryKey: knowledgeWikiKeys.page(wsId, key, slug),
    queryFn: () => api.getKnowledgeWikiPage(key, slug),
    enabled: key.length > 0 && slug.length > 0,
  });
}
