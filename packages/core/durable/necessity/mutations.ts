import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { necessityKeys } from "./queries";
import { parseNecessityTest, type NecessityTest } from "./schemas";

export type StartNecessityTestRequest = {
  skill_id: string;
};

/**
 * Starts a necessity test on one skill of a seat (POST /api/agents/{id}/necessity).
 * Refreshes the seat's necessity test list on settle, whether or not the caller is still mounted.
 */
export function useStartNecessityTest(wsId: string, agentId: string) {
  const qc = useQueryClient();
  return useMutation<NecessityTest | null, Error, StartNecessityTestRequest>({
    mutationFn: async (body) =>
      parseNecessityTest(
        await api.request(`/api/agents/${encodeURIComponent(agentId)}/necessity`, {
          method: "POST",
          body: JSON.stringify(body),
        }),
        "POST /api/agents/:id/necessity",
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: necessityKeys.list(wsId, agentId) });
    },
  });
}
