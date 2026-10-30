import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { issueWorkflowKeys } from "../issue-workflows/queries";
import { workspaceKeys } from "../workspace/queries";
import {
  runSquadQuickstart,
  type QuickstartOptions,
  type QuickstartOutcome,
} from "./quickstart";
import type { SquadTemplate } from "./templates";

/**
 * Runs a squad template's quickstart. The outcome says what was created, what
 * already existed, and what failed, so the caller reports it instead of
 * treating a partial result as an error.
 */
export function useSquadQuickstart(wsId: string) {
  const qc = useQueryClient();
  return useMutation<
    QuickstartOutcome,
    Error,
    {
      template: SquadTemplate;
      options: QuickstartOptions;
      onProgress?: (outcome: QuickstartOutcome) => void;
    }
  >({
    mutationFn: ({ template, options, onProgress }) =>
      runSquadQuickstart(api, template, options, onProgress),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: workspaceKeys.agents(wsId) });
      qc.invalidateQueries({ queryKey: workspaceKeys.squads(wsId) });
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) });
    },
  });
}
