import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { pluginPackagesOptions } from "../plugins/queries";
import { skillListOptions } from "../workspace/queries";
import { buildHubItems, type HubItem, type HubItemKind } from "./items";

/**
 * One Hub item, found in the skill list or the plugin package list. Only the list
 * for `kind` is fetched.
 */
export function useHubItem(
  wsId: string,
  kind: HubItemKind,
  refId: string,
): { item: HubItem | undefined; isLoading: boolean; isError: boolean } {
  const isSkill = kind === "skill";
  const skills = useQuery({ ...skillListOptions(wsId), enabled: isSkill && wsId.length > 0 });
  const packages = useQuery({ ...pluginPackagesOptions(wsId), enabled: !isSkill && wsId.length > 0 });

  const item = useMemo(
    () =>
      buildHubItems(skills.data ?? [], packages.data?.packages ?? []).find(
        (candidate) => candidate.key === `${kind}:${refId}`,
      ),
    [skills.data, packages.data, kind, refId],
  );

  const active = isSkill ? skills : packages;
  return { item, isLoading: active.isLoading, isError: active.isError };
}
