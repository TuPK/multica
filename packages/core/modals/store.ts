"use client";

import { create } from "zustand";

type ModalType =
  | "create-issue"
  | "quick-create-issue"
  | "create-project"
  | "create-squad"
  | "squad-quickstart"
  | "agent-pack-install"
  | "feedback"
  | "issue-set-parent"
  | "issue-mark-duplicate"
  | "issue-add-child"
  | "issue-add-blocker"
  | "issue-add-blocking"
  | "project-add-issue"
  | "project-rename"
  | "issue-rename"
  | "issue-delete-confirm"
  | "issue-run-confirm"
  | "issue-workflow-handoff"
  | "issue-save-to-knowledge"
  | null;

// Each reason is a Cloud-enforced limit whose recovery dialog offers billing
// actions. Add a reason here when a new limit gets its own recovery copy.
export type LimitRecoveryReason =
  | "autopilot_quota"
  | "issue_limit"
  | "agents"
  | "runtimes"
  | "storage_bytes"
  | "agent_limit"
  | "runtime_limit"
  | "storage_limit";

interface ModalStore {
  modal: ModalType;
  data: Record<string, unknown> | null;
  limitRecoveryWorkspaceId: string | null;
  limitRecoveryReason: LimitRecoveryReason;
  open: (modal: NonNullable<ModalType>, data?: Record<string, unknown> | null) => void;
  close: () => void;
  showLimitRecovery: (workspaceId: string, reason?: LimitRecoveryReason) => void;
  dismissLimitRecovery: () => void;
}

export const useModalStore = create<ModalStore>((set) => ({
  modal: null,
  data: null,
  limitRecoveryWorkspaceId: null,
  limitRecoveryReason: "issue_limit",
  open: (modal, data = null) => set({ modal, data }),
  close: () => set({ modal: null, data: null }),
  showLimitRecovery: (workspaceId, reason = "issue_limit") =>
    set({
      limitRecoveryWorkspaceId: workspaceId,
      limitRecoveryReason: reason,
    }),
  dismissLimitRecovery: () => set({ limitRecoveryWorkspaceId: null }),
}));
