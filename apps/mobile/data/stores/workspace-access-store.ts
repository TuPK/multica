/**
 * In-memory workspace access blocks discovered from 403 refusal codes.
 *
 * Never persisted: switching workspace, signing out, or restarting the app
 * clears the recorded state so workspace access is re-evaluated cleanly.
 */
import { create } from "zustand";
import type { WorkspaceBlockReason } from "@/lib/workspace-access";

interface WorkspaceAccessState {
  blocks: Record<string, WorkspaceBlockReason>;
  setBlock: (slug: string, reason: WorkspaceBlockReason) => void;
  clearWorkspace: (slug: string | null | undefined) => void;
  clearAll: () => void;
}

export const useWorkspaceAccessStore = create<WorkspaceAccessState>(
  (set, get) => ({
    blocks: {},

    setBlock: (slug, reason) => {
      if (!slug) return;
      const current = get().blocks;
      if (current[slug] === reason) return;
      set({ blocks: { ...current, [slug]: reason } });
    },

    clearWorkspace: (slug) => {
      if (!slug) return;
      const current = get().blocks;
      if (!(slug in current)) return;
      const next = { ...current };
      delete next[slug];
      set({ blocks: next });
    },

    clearAll: () => {
      if (Object.keys(get().blocks).length === 0) return;
      set({ blocks: {} });
    },
  }),
);
