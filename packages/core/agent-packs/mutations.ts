import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { workspaceKeys } from "../workspace/queries";
import { runAgentPackInstall, type AgentPackOutcome } from "./install";
import type { AgentPack } from "./schema";

/**
 * Installs a pack into the workspace. The outcome says what was created, what
 * was already there, and what failed, so the caller reports a partial result
 * instead of treating it as an error.
 */
export function useInstallAgentPack(wsId: string) {
  const qc = useQueryClient();
  return useMutation<AgentPackOutcome, Error, { pack: AgentPack; runtimeId: string | null }>({
    mutationFn: ({ pack, runtimeId }) => runAgentPackInstall(api, pack, { runtimeId }),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: workspaceKeys.agents(wsId) });
      qc.invalidateQueries({ queryKey: workspaceKeys.skills(wsId) });
    },
  });
}
