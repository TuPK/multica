import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { ResearchSettings, UpdateResearchSettingsInput } from "../types/research";
import { researchKeys, researchSettingsOptions } from "./queries";
import { parseResearchSettings } from "./schema";

/** The workspace's Research Lab settings. `data` is undefined until loaded. */
export function useResearchSettings(wsId: string) {
  return useQuery({ ...researchSettingsOptions(wsId), enabled: Boolean(wsId) });
}

/**
 * Saves the keys and the contact address. The cache is replaced with what the server answers once it
 * confirms, never before: a rejected key must not look saved.
 */
export function useUpdateResearchSettings(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: UpdateResearchSettingsInput): Promise<ResearchSettings> =>
      parseResearchSettings(await api.updateResearchSettings(input)),
    onSuccess: (settings) => qc.setQueryData(researchKeys.settings(wsId), settings),
    onSettled: () => qc.invalidateQueries({ queryKey: researchKeys.all(wsId) }),
  });
}
