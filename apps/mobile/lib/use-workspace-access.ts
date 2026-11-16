import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@/data/auth-store";
import { memberListOptions } from "@/data/queries/members";
import { useWorkspaceAccessStore } from "@/data/stores/workspace-access-store";
import { useWorkspaceStore } from "@/data/workspace-store";
import {
  canWriteInWorkspace,
  type WorkspaceBlockReason,
} from "./workspace-access";

export function useWorkspaceBlock(slug?: string | null): WorkspaceBlockReason | null {
  const currentSlug = useWorkspaceStore((s) => s.currentWorkspaceSlug);
  const resolvedSlug = slug === undefined ? currentSlug : slug;
  return useWorkspaceAccessStore((s) =>
    resolvedSlug ? (s.blocks[resolvedSlug] ?? null) : null,
  );
}

/**
 * Full access status for write-route guards so cold-start deep links can wait
 * for membership to resolve before deciding to block or allow a write route.
 */
export function useWorkspaceAccessStatus(wsId?: string | null): {
  canWrite: boolean;
  isLoading: boolean;
  block: WorkspaceBlockReason | null;
} {
  const currentWsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const resolvedWsId = wsId === undefined ? currentWsId : wsId;
  const block = useWorkspaceBlock();
  const userId = useAuthStore((s) => s.user?.id ?? null);
  const isBlocked = block === "suspended" || block === "pending_deletion";

  const { data: members, isLoading } = useQuery({
    ...memberListOptions(resolvedWsId ?? ""),
    enabled: Boolean(resolvedWsId) && !isBlocked,
  });

  if (block != null) {
    return { canWrite: false, isLoading: false, block };
  }
  if (!resolvedWsId || !userId || !members) {
    return { canWrite: false, isLoading: Boolean(resolvedWsId) && isLoading, block: null };
  }
  const currentMember = members.find((m) => m.user_id === userId);
  return {
    canWrite: canWriteInWorkspace({ role: currentMember?.role ?? null, block }),
    isLoading: false,
    block: null,
  };
}

/**
 * Returns whether the current signed-in user may perform non-personal writes in
 * the active (or specified) workspace.
 *
 * Fails closed (`false`) while members are still loading, when the user is
 * `"viewer"` (or an unknown role), or when a workspace block is active.
 */
export function useCanWrite(wsId?: string | null): boolean {
  return useWorkspaceAccessStatus(wsId).canWrite;
}
