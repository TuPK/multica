import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { issueWorkflowKeys } from "../issue-workflows/queries";
import { knowledgeKeys } from "../knowledge/queries";
import { workspaceKeys } from "../workspace/queries";
import type { SalesIntake } from "./sales-intake";
import {
  runSquadQuickstart,
  type QuickstartOptions,
  type QuickstartOutcome,
} from "./quickstart";
import { SQUAD_TEMPLATES, squadTemplateCategory, type SquadTemplate } from "./templates";
import { upgradeKitvietWorkspace, type KitvietWorkspaceUpgradeEntry } from "./upgrade";

export type { KitvietWorkspaceUpgradeEntry };
export { hasKitvietSquad } from "./upgrade";

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

/**
 * Brings every Kit Viet squad in the workspace up to the current pack: its agents' and squad's
 * instructions and its skills. Texts the boss edited are kept and reported in the result.
 */
export function useUpgradeKitvietSquads(wsId: string) {
  const qc = useQueryClient();
  return useMutation<KitvietWorkspaceUpgradeEntry[], Error, void>({
    mutationFn: () =>
      upgradeKitvietWorkspace(
        api,
        SQUAD_TEMPLATES.filter((tpl) => squadTemplateCategory(tpl) === "kitviet"),
      ),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: workspaceKeys.agents(wsId) });
      qc.invalidateQueries({ queryKey: workspaceKeys.squads(wsId) });
      qc.invalidateQueries({ queryKey: workspaceKeys.skills(wsId) });
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) });
    },
  });
}

/**
 * Adds a source to a Sales Loop squad. The server writes the source document and a request issue for
 * the squad; nothing runs until the squad's leader routes it. Awaits the server before the lists refetch.
 */
export function useCreateSquadSource(wsId: string, squadId: string) {
  const qc = useQueryClient();
  return useMutation<SalesIntake, Error, { input: string; lane?: string }>({
    mutationFn: (body) => api.createSquadSource(squadId, body),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: knowledgeKeys.all(wsId) });
      qc.invalidateQueries({ queryKey: workspaceKeys.squads(wsId) });
    },
  });
}

/** Adds a shared post to a Sales Loop squad, the same way as a source. */
export function useCreateSquadShare(wsId: string, squadId: string) {
  const qc = useQueryClient();
  return useMutation<SalesIntake, Error, { input: string; text?: string; lane?: string }>({
    mutationFn: (body) => api.createSquadShare(squadId, body),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: knowledgeKeys.all(wsId) });
    },
  });
}
