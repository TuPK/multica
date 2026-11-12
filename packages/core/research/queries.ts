import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import { parseResearchSettings } from "./schema";

/** Query keys are workspace-scoped: each workspace has its own Research Lab settings. */
export const researchKeys = {
  all: (wsId: string) => ["research", wsId] as const,
  settings: (wsId: string) => [...researchKeys.all(wsId), "settings"] as const,
};

/** The workspace's Research Lab settings. They hold no API key. */
export function researchSettingsOptions(wsId: string) {
  return queryOptions({
    queryKey: researchKeys.settings(wsId),
    queryFn: async () => parseResearchSettings(await api.getResearchSettings()),
    staleTime: 30_000,
  });
}
