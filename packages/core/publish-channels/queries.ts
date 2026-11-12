import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import { parsePublishChannels, parsePublishSettings } from "./schema";

/**
 * Query keys are workspace-scoped: the channels of one workspace are a different
 * list from another's, so the key carries the workspace id.
 */
export const publishChannelKeys = {
  all: (wsId: string) => ["publish-channels", wsId] as const,
  list: (wsId: string) => [...publishChannelKeys.all(wsId), "list"] as const,
  settings: (wsId: string) => [...publishChannelKeys.all(wsId), "settings"] as const,
};

/** The workspace's publishing channels. */
export function publishChannelsOptions(wsId: string) {
  return queryOptions({
    queryKey: publishChannelKeys.list(wsId),
    queryFn: async () => parsePublishChannels(await api.listPublishChannels()),
    staleTime: 30_000,
  });
}

/** Which Ayrshare keys the workspace has set. */
export function publishSettingsOptions(wsId: string) {
  return queryOptions({
    queryKey: publishChannelKeys.settings(wsId),
    queryFn: async () => parsePublishSettings(await api.getPublishSettings()),
    staleTime: 30_000,
  });
}
