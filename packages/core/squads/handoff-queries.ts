import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { IssueHandoff, SetSquadHandoffRuleRequest, SquadHandoffRule } from "./handoff";

export const squadHandoffRuleKey = (wsId: string, squadId: string) =>
  ["workspaces", wsId, "squads", "handoff-rule", squadId] as const;

export const issueHandoffKey = (wsId: string, issueId: string) =>
  ["workspaces", wsId, "issues", "handoff", issueId] as const;

/** A squad's handoff rule. The squad panel and the room overlay share it. */
export function squadHandoffRuleOptions(wsId: string, squadId: string) {
  return queryOptions({
    queryKey: squadHandoffRuleKey(wsId, squadId),
    queryFn: (): Promise<SquadHandoffRule> => api.getSquadHandoffRule(squadId),
    enabled: !!wsId && !!squadId,
  });
}

/** Where a request stands in its squad's handoff. */
export function issueHandoffOptions(wsId: string, issueId: string) {
  return queryOptions({
    queryKey: issueHandoffKey(wsId, issueId),
    queryFn: (): Promise<IssueHandoff> => api.getIssueHandoff(issueId),
    enabled: !!wsId && !!issueId,
  });
}

/** Saves a squad's handoff rule. The rule shows from the server's answer. */
export function useSetSquadHandoffRule(wsId: string, squadId: string) {
  const qc = useQueryClient();
  return useMutation<SquadHandoffRule, Error, SetSquadHandoffRuleRequest>({
    mutationFn: (data) => api.setSquadHandoffRule(squadId, data),
    onSuccess: (rule) => {
      qc.setQueryData(squadHandoffRuleKey(wsId, squadId), rule);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: squadHandoffRuleKey(wsId, squadId) });
    },
  });
}

/**
 * The boss's decision on a request's handoff: hand it off now (approving a pending one,
 * or handing off by hand when the rule is off), or decline a pending one. The server
 * creates the other squad's request, so the request's handoff and the issue lists are
 * refetched when the write settles.
 */
export function useIssueHandoffDecision(wsId: string, issueId: string) {
  const qc = useQueryClient();
  return useMutation<IssueHandoff, Error, "hand_off" | "decline">({
    mutationFn: (decision) => (decision === "hand_off" ? api.handOffIssue(issueId) : api.declineIssueHandoff(issueId)),
    onSuccess: (handoff) => {
      qc.setQueryData(issueHandoffKey(wsId, issueId), handoff);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: issueHandoffKey(wsId, issueId) });
      qc.invalidateQueries({ queryKey: ["workspaces", wsId, "issues"] });
    },
  });
}
