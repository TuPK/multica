import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { pluginPackagesOptions } from "../plugins/queries";
import { skillListOptions } from "../workspace/queries";
import type { HubItemMeta } from "../types/hub";
import { buildHubItems, type HubItem, type HubItemKind } from "./items";
import { hubItemOptions } from "./item-meta";

function buildAgentHubItem(meta: HubItemMeta): HubItem {
  return {
    key: `agent:${meta.ref_id}`,
    kind: "agent",
    refId: meta.ref_id,
    name: meta.source_ref || meta.ref_id,
    description: "",
    source: "manual",
    sourceUrl: null,
    version: meta.latest_version || null,
    labels: [],
    attestation: meta.attestation,
    updatedAt: meta.updated_at,
    createdAt: meta.created_at,
  };
}

/**
 * One Hub item, found in the skill list, the plugin package list, or directly
 * via GET /api/hub/items/agent/{id}. Only the query for `kind` is fetched.
 */
export function useHubItem(
  wsId: string,
  kind: HubItemKind,
  refId: string,
): { item: HubItem | undefined; isLoading: boolean; isError: boolean } {
  const isSkill = kind === "skill";
  const isPlugin = kind === "plugin";
  const isAgent = kind === "agent";
  const skills = useQuery({ ...skillListOptions(wsId), enabled: isSkill && wsId.length > 0 });
  const packages = useQuery({ ...pluginPackagesOptions(wsId), enabled: isPlugin && wsId.length > 0 });
  const agentItem = useQuery({
    ...hubItemOptions(wsId, "agent", refId),
    enabled: isAgent && wsId.length > 0 && refId.length > 0,
  });

  const item = useMemo(() => {
    if (isAgent) {
      return agentItem.data ? buildAgentHubItem(agentItem.data) : undefined;
    }
    return buildHubItems(skills.data ?? [], packages.data?.packages ?? []).find(
      (candidate) => candidate.key === `${kind}:${refId}`,
    );
  }, [isAgent, agentItem.data, skills.data, packages.data, kind, refId]);

  const active = isSkill ? skills : isPlugin ? packages : agentItem;
  return { item, isLoading: active.isLoading, isError: active.isError };
}
