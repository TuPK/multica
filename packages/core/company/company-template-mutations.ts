import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { autopilotKeys } from "../autopilots/queries";
import { createLogger } from "../logger";
import { issueWorkflowKeys } from "../issue-workflows/queries";
import type { CompanyStart, CompanySquadKey } from "../squads/company-template";
import { attachKitvietExtensions } from "../squads/kitviet-extension-attach";
import { workspaceKeys } from "../workspace/queries";
import { companyKeys } from "./queries";
import { companySetupKeys } from "./setup-query";
import type {
  CompanyTemplateApplyRequest,
  CompanyTemplateApplyResult,
} from "./company-template-types";

const logger = createLogger("company.template");

/**
 * Attaches the Kit Viet extension skills (spec 7) to the company squads after the apply. The apply has
 * already succeeded, so a failure here is logged and does not fail the apply. A squad that was not
 * attached is named in the log, and running the apply again attaches it.
 */
async function attachExtensionSkillsAfterApply(): Promise<void> {
  try {
    const outcomes = await attachKitvietExtensions(api, await api.listSquads());
    const incomplete = outcomes.filter((outcome) => outcome.status !== "ready");
    if (incomplete.length > 0) logger.warn("kitviet extension skills partly attached", { incomplete });
  } catch (err) {
    logger.warn("kitviet extension skills not attached", { err });
  }
}

/**
 * The request the wizard sends. A blank model is left out, so the runtime's
 * default applies rather than an empty string.
 */
export function buildCompanyTemplateApplyRequest(input: {
  runtimeId: string;
  model: string;
  squads: readonly CompanySquadKey[];
  start: CompanyStart;
}): CompanyTemplateApplyRequest {
  const model = input.model.trim();
  return {
    runtime_id: input.runtimeId,
    ...(model ? { model } : {}),
    squads: [...input.squads],
    start: input.start,
  };
}

/**
 * Runs the starter company's apply. The server answers with each step, so the
 * caller reports a partial result instead of treating it as an error. Nothing
 * is patched optimistically: the caches the apply touches are refetched once
 * it settles.
 */
export function useApplyCompanyTemplate(wsId: string) {
  const qc = useQueryClient();
  return useMutation<CompanyTemplateApplyResult, Error, CompanyTemplateApplyRequest>({
    mutationFn: async (data) => {
      const result = await api.applyCompanyTemplate(data);
      await attachExtensionSkillsAfterApply();
      return result;
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: workspaceKeys.agents(wsId) });
      qc.invalidateQueries({ queryKey: workspaceKeys.squads(wsId) });
      qc.invalidateQueries({ queryKey: workspaceKeys.skills(wsId) });
      qc.invalidateQueries({ queryKey: companyKeys.floors(wsId) });
      qc.invalidateQueries({ queryKey: companySetupKeys.all(wsId) });
      qc.invalidateQueries({ queryKey: autopilotKeys.list(wsId) });
      qc.invalidateQueries({ queryKey: issueWorkflowKeys.all(wsId) });
    },
  });
}
