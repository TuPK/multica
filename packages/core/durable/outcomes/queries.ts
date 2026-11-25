import { queryOptions, useQuery } from "@tanstack/react-query";
import { api, ApiError } from "../../api";
import { toQueryString } from "../query-string";
import {
  parseCriteriaList,
  parseOutcome,
  parseOutcomeList,
  parseOutcomeSummary,
  type OutcomeVerdict,
} from "./schemas";

// Every key starts with "durable" and the workspace id, so a workspace never reads another's
// cache. Server data stays here in TanStack Query; the realtime events only invalidate it.
export const outcomeKeys = {
  all: (wsId: string) => ["durable", wsId, "outcomes"] as const,
  issue: (wsId: string, issueId: string) => [...outcomeKeys.all(wsId), "issue", issueId] as const,
  list: (wsId: string, filters: OutcomeFilters) => [...outcomeKeys.all(wsId), "list", filters] as const,
  detail: (wsId: string, id: string) => [...outcomeKeys.all(wsId), "detail", id] as const,
  summary: (wsId: string, params: OutcomeSummaryParams) => [...outcomeKeys.all(wsId), "summary", params] as const,
  criteria: (wsId: string, issueId: string) => ["durable", wsId, "criteria", issueId] as const,
};

export type OutcomeFilters = {
  agent_id?: string;
  squad_id?: string;
  work_class?: string;
  verdict?: OutcomeVerdict;
  since?: string;
  cursor?: string;
  limit?: number;
};

export type OutcomeSummaryParams = {
  group_by?: "agent" | "squad" | "work_class";
  period?: "week" | "month";
};

/** The outcome of an issue, or null while the issue has none (the route answers 404). */
export function issueOutcomeOptions(wsId: string, issueId: string) {
  return queryOptions({
    queryKey: outcomeKeys.issue(wsId, issueId),
    queryFn: async () => {
      try {
        const raw = await api.request(`/api/issues/${encodeURIComponent(issueId)}/outcome`);
        return parseOutcome(raw, "GET /api/issues/:id/outcome");
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) return null;
        throw err;
      }
    },
    enabled: !!wsId && !!issueId,
  });
}

/** An issue's criteria, archived ones included. */
export function issueCriteriaOptions(wsId: string, issueId: string) {
  return queryOptions({
    queryKey: outcomeKeys.criteria(wsId, issueId),
    queryFn: async () =>
      parseCriteriaList(await api.request(`/api/issues/${encodeURIComponent(issueId)}/criteria`), "GET /api/issues/:id/criteria"),
    enabled: !!wsId && !!issueId,
  });
}

/** One page of outcomes, newest first. The next page is asked with the cursor the last one returned. */
export function outcomeListOptions(wsId: string, filters: OutcomeFilters = {}) {
  return queryOptions({
    queryKey: outcomeKeys.list(wsId, filters),
    queryFn: async () =>
      parseOutcomeList(await api.request(`/api/outcomes${toQueryString(filters)}`), "GET /api/outcomes"),
    enabled: !!wsId,
  });
}

/** One outcome with its checks. */
export function outcomeOptions(wsId: string, id: string) {
  return queryOptions({
    queryKey: outcomeKeys.detail(wsId, id),
    queryFn: async () =>
      parseOutcome(await api.request(`/api/outcomes/${encodeURIComponent(id)}`), "GET /api/outcomes/:id"),
    enabled: !!wsId && !!id,
  });
}

/** Outcomes grouped for the summary view. */
export function outcomeSummaryOptions(wsId: string, params: OutcomeSummaryParams = {}) {
  return queryOptions({
    queryKey: outcomeKeys.summary(wsId, params),
    queryFn: async () =>
      parseOutcomeSummary(
        await api.request(`/api/outcomes/summary${toQueryString(params)}`),
        "GET /api/outcomes/summary",
      ),
    enabled: !!wsId,
  });
}

export function useIssueOutcome(wsId: string, issueId: string) {
  return useQuery(issueOutcomeOptions(wsId, issueId));
}

export function useIssueCriteria(wsId: string, issueId: string) {
  return useQuery(issueCriteriaOptions(wsId, issueId));
}

export function useOutcomes(wsId: string, filters: OutcomeFilters = {}) {
  return useQuery(outcomeListOptions(wsId, filters));
}

export function useOutcome(wsId: string, id: string) {
  return useQuery(outcomeOptions(wsId, id));
}

export function useOutcomeSummary(wsId: string, params: OutcomeSummaryParams = {}) {
  return useQuery(outcomeSummaryOptions(wsId, params));
}
