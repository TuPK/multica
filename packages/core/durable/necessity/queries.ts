import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../../api";
import { parseNecessityTests, parseSeatSkills } from "./schemas";

// Every key starts with "durable" and the workspace id, then scopes to the seat (agentId),
// so neither workspaces nor seats share cache entries.
export const necessityKeys = {
  all: (wsId: string) => ["durable", wsId, "necessity"] as const,
  agent: (wsId: string, agentId: string) => [...necessityKeys.all(wsId), agentId] as const,
  list: (wsId: string, agentId: string) => [...necessityKeys.agent(wsId, agentId), "list"] as const,
  skills: (wsId: string, agentId: string) => [...necessityKeys.agent(wsId, agentId), "skills"] as const,
};

/** Recent necessity tests for a seat, newest first (GET /api/agents/{id}/necessity). */
export function necessityTestsOptions(wsId: string, agentId: string, enabled = true) {
  return queryOptions({
    queryKey: necessityKeys.list(wsId, agentId),
    queryFn: async () =>
      parseNecessityTests(
        await api.request(`/api/agents/${encodeURIComponent(agentId)}/necessity`),
        "GET /api/agents/:id/necessity",
      ),
    enabled: enabled && !!wsId && !!agentId,
  });
}

export function useSeatNecessityTests(wsId: string, agentId: string, enabled = true) {
  return useQuery(necessityTestsOptions(wsId, agentId, enabled));
}

/** Skills currently held by a seat (GET /api/agents/{id}/skills). */
export function seatSkillsOptions(wsId: string, agentId: string, enabled = true) {
  return queryOptions({
    queryKey: necessityKeys.skills(wsId, agentId),
    queryFn: async () =>
      parseSeatSkills(await api.listAgentSkills(agentId), "GET /api/agents/:id/skills"),
    enabled: enabled && !!wsId && !!agentId,
  });
}

export function useSeatSkills(wsId: string, agentId: string, enabled = true) {
  return useQuery(seatSkillsOptions(wsId, agentId, enabled));
}
