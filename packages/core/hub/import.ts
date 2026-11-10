import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type {
  HubConflictPolicy,
  HubImportSourceRequest,
} from "../types/hub";
import { invalidateHubCatalog } from "./sources";

export type HubImportPreviewVars =
  | { wsId: string; source: HubImportSourceRequest; file?: never }
  | { wsId: string; file: File; source?: never };

/**
 * Previews an import from a source or from an uploaded zip. Nothing is written
 * until the preview is committed, so this is a mutation rather than a cached query.
 */
export function usePreviewHubImport() {
  return useMutation({
    mutationFn: (vars: HubImportPreviewVars) =>
      vars.file
        ? api.previewHubImportUpload(vars.wsId, vars.file)
        : api.previewHubImport(vars.wsId, vars.source),
  });
}

export interface CommitHubImportVars {
  wsId: string;
  previewId: string;
  onConflict: HubConflictPolicy;
  name?: string;
  confirmWarnings: boolean;
  sourceId?: string;
}

export function useCommitHubImport() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: CommitHubImportVars) =>
      api.commitHubImport(vars.wsId, {
        preview_id: vars.previewId,
        on_conflict: vars.onConflict,
        name: vars.name,
        confirm_warnings: vars.confirmWarnings,
        source_id: vars.sourceId,
      }),
    onSuccess: (_result, vars) => invalidateHubCatalog(qc, vars.wsId),
  });
}
