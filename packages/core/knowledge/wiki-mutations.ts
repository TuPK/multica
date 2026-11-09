import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { knowledgeKeys } from "./queries";
import { knowledgeWikiKeys } from "./wiki-queries";
import type { KnowledgeWikiGenerateResult } from "./wiki-types";

/**
 * Turns Wiki on for a base and has the server write the pages of its documents.
 * Owners and admins only. Nothing is patched optimistically: the server answers
 * with how many documents it queued, and the pages appear as they are written.
 */
export function useGenerateKnowledgeWiki(wsId: string) {
  const qc = useQueryClient();
  return useMutation<KnowledgeWikiGenerateResult, Error, { key: string }>({
    mutationFn: ({ key }) => api.generateKnowledgeWiki(key),
    onSettled: (_data, _err, { key }) => {
      qc.invalidateQueries({ queryKey: knowledgeWikiKeys.all(wsId, key) });
      qc.invalidateQueries({ queryKey: knowledgeKeys.bases(wsId) });
    },
  });
}
