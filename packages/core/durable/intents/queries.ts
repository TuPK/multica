import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../../api";
import { toQueryString } from "../query-string";
import { parseIntentList, type IntentState } from "./schemas";

// Every key starts with "durable" and the workspace id, so a workspace never reads another's
// intents. Server data stays in TanStack Query; the mutations invalidate it.
export const intentKeys = {
  all: (wsId: string) => ["durable", wsId, "intents"] as const,
  list: (wsId: string, state: IntentState | "") => [...intentKeys.all(wsId), "list", state] as const,
};

/** The workspace's intents in one state, newest first. The pending list is the inbox's decisions. */
export function intentListOptions(wsId: string, state: IntentState | "" = "pending") {
  return queryOptions({
    queryKey: intentKeys.list(wsId, state),
    queryFn: async () =>
      parseIntentList(await api.request(`/api/intents${toQueryString({ state })}`), "GET /api/intents"),
    enabled: !!wsId,
  });
}

export function useIntents(wsId: string, state: IntentState | "" = "pending") {
  return useQuery(intentListOptions(wsId, state));
}
