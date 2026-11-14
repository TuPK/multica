import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";

/** Workspace-scoped keys of the numbers of a version. */
export const packKeys = {
  all: (wsId: string) => ["packs", wsId] as const,
  insights: (wsId: string, workflowId: string, includeTrial: boolean) =>
    [...packKeys.all(wsId), "insights", workflowId, includeTrial] as const,
  proposalFiles: (wsId: string, issueId: string) => [...packKeys.all(wsId), "proposal-files", issueId] as const,
};

/** The files attached to the issue an agent was given to draft a workflow; the banner looks for a flow.json. */
export function proposalFilesOptions(wsId: string, issueId: string, opts?: { poll?: boolean }) {
  return queryOptions({
    queryKey: packKeys.proposalFiles(wsId, issueId),
    queryFn: () => api.listAttachments(issueId),
    enabled: Boolean(wsId && issueId),
    staleTime: 10_000,
    // While the agent has not answered, look again now and then.
    refetchInterval: opts?.poll ? 15_000 : false,
  });
}

/** The operating numbers of one version of a flow (spec 16.6). */
export function flowInsightsOptions(wsId: string, workflowId: string, includeTrial: boolean) {
  return queryOptions({
    queryKey: packKeys.insights(wsId, workflowId, includeTrial),
    queryFn: () => api.getFlowInsights(workflowId, { includeTrial }),
    enabled: Boolean(wsId && workflowId),
    staleTime: 60_000,
  });
}

/**
 * The numbers of a version. `enabled` is false when the server does not offer them (a manager reads
 * them; a server without the feature answers 404), and the query waits for a version to ask about.
 */
export function useFlowInsights(
  workflowId: string | null | undefined,
  opts: { includeTrial: boolean; enabled?: boolean; wsId?: string },
) {
  const currentWsId = useWorkspaceId();
  const wsId = opts.wsId || currentWsId;
  return useQuery({
    ...flowInsightsOptions(wsId, workflowId ?? "", opts.includeTrial),
    enabled: Boolean(wsId && workflowId) && opts.enabled !== false,
    retry: false,
  });
}
