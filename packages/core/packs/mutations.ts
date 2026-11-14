import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";
import { flowDraftKeys } from "../flow-drafts/queries";
import { workspaceKeys } from "../workspace/queries";
import type { FlowDraft } from "../types/flow-draft";
import type { PackCommitRequest, PackPreview, PackSourceRequest } from "../types/pack";

/** A preview takes a source to fetch or a zip the person chose; it writes nothing. */
export type PreviewPackVariables = { source: PackSourceRequest; file?: undefined } | { file: File; source?: undefined };

export function usePreviewPack() {
  return useMutation<PackPreview, Error, PreviewPackVariables>({
    mutationFn: (vars) => (vars.file ? api.previewPackUpload(vars.file) : api.previewPack(vars.source)),
  });
}

/**
 * Installs a pack. Per AGENTS.md state rules the server decides first: nothing is written to the cache
 * before it answers, and a refusal leaves the cache as it was.
 */
export function useCommitPack() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: (data: PackCommitRequest) => api.commitPack(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: workspaceKeys.squads(wsId) });
      qc.invalidateQueries({ queryKey: workspaceKeys.agents(wsId) });
      qc.invalidateQueries({ queryKey: flowDraftKeys.all(wsId) });
    },
  });
}

export interface ImportFlowProposalVariables {
  squadId: string;
  draftId: string;
  revision: number;
}

/** Reads the file the agent attached into the draft; the answer is the draft as the server holds it. */
export function useImportFlowProposal() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({ squadId, draftId, revision }: ImportFlowProposalVariables) =>
      api.importFlowProposal(squadId, draftId, { revision }),
    onSuccess: (draft, { squadId, draftId }) => {
      qc.setQueryData<FlowDraft>(flowDraftKeys.detail(wsId, squadId, draftId), draft);
      qc.invalidateQueries({ queryKey: flowDraftKeys.list(wsId, squadId) });
    },
  });
}
