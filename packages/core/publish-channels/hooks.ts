import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { CreatePublishChannelInput, PublishChannel, PublishSettings, SavePublishSettingsInput } from "../types/publish";
import { publishChannelKeys, publishChannelsOptions, publishSettingsOptions } from "./queries";
import {
  parsePublishChannel,
  parsePublishConnect,
  parsePublishDryRunBody,
  parsePublishSettings,
} from "./schema";

/**
 * Hooks for the publishing channels of one workspace. Each takes `wsId` and
 * keys its cache by it. Mutations refresh the channel list when they settle.
 */

/** The workspace's channels. `data` is the list; it is undefined until loaded. */
export function usePublishChannels(wsId: string) {
  return useQuery({ ...publishChannelsOptions(wsId), enabled: Boolean(wsId) });
}

/** Adds a channel that is not connected yet. */
export function useCreatePublishChannel(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: CreatePublishChannelInput): Promise<PublishChannel> =>
      parsePublishChannel(await api.createPublishChannel(input)),
    onSettled: () => qc.invalidateQueries({ queryKey: publishChannelKeys.all(wsId) }),
  });
}

/** Removes a channel. The list refreshes only after the server confirms the delete. */
export function useDeletePublishChannel(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deletePublishChannel(id),
    onSettled: () => qc.invalidateQueries({ queryKey: publishChannelKeys.all(wsId) }),
  });
}

/** Starts the account link for a channel. Resolves to `{ url }` for the user to open. */
export function useConnectPublishChannel(_wsId: string) {
  return useMutation({
    mutationFn: async (id: string): Promise<{ url: string }> =>
      parsePublishConnect(await api.connectPublishChannel(id)),
  });
}

/** Asks Ayrshare whether a channel is linked. The returned channel replaces its row. */
export function useCheckPublishChannel(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string): Promise<PublishChannel> =>
      parsePublishChannel(await api.checkPublishChannel(id)),
    onSuccess: (channel) => {
      qc.setQueryData<PublishChannel[]>(publishChannelKeys.list(wsId), (prev) =>
        prev?.map((c) => (c.id === channel.id ? channel : c)),
      );
    },
    onSettled: () => qc.invalidateQueries({ queryKey: publishChannelKeys.all(wsId) }),
  });
}

/** The exact request body a post on a channel would send. Nothing is sent. */
export function useDryRunPublishChannel(_wsId: string) {
  return useMutation({
    mutationFn: async (input: {
      id: string;
      text: string;
      media_url?: string;
      hashtags?: string[];
    }): Promise<Record<string, unknown>> => {
      const { id, ...body } = input;
      return parsePublishDryRunBody(await api.dryRunPublishChannel(id, body));
    },
  });
}

/** Whether the workspace has its Ayrshare keys set. `data` is undefined until loaded. */
export function usePublishSettings(wsId: string) {
  return useQuery({ ...publishSettingsOptions(wsId), enabled: Boolean(wsId) });
}

/**
 * Saves the workspace's Ayrshare keys. A field left out keeps its stored key. The
 * settings and the channel list refresh when the save settles, because a channel's
 * status depends on whether a key is set.
 */
export function useSavePublishSettings(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: SavePublishSettingsInput): Promise<PublishSettings> =>
      parsePublishSettings(await api.savePublishSettings(input)),
    onSuccess: (settings) => {
      qc.setQueryData<PublishSettings>(publishChannelKeys.settings(wsId), settings);
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: publishChannelKeys.settings(wsId) });
      void qc.invalidateQueries({ queryKey: publishChannelKeys.list(wsId) });
    },
  });
}
