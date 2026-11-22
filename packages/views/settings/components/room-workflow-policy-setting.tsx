"use client";

import { useMemo, useState } from "react";
import { toast } from "sonner";
import {
  useIssueWorkflows,
  useRoomWorkflowPolicy,
} from "@multica/core/issue-workflows/hooks";
import { useSetRoomWorkflowPolicy } from "@multica/core/issue-workflows/mutations";
import type { RoomNonRequestKind, RoomWorkflowPolicy } from "@multica/core/types";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { useT } from "../../i18n";
import { SettingsCard, SettingsRow } from "./settings-layout";

const DEFAULT_POLICY: RoomWorkflowPolicy = {
  kind: "no_action",
  workflowId: null,
};

/**
 * Workspace-level workflow choice for issues that are not requests of a flow
 * squad (squad-room-workflow spec, sections 4 and 6). Only workspace admins
 * can change it; members see the stored choice read-only.
 */
export function RoomWorkflowPolicySetting({
  wsId,
  canManage,
}: {
  wsId: string;
  canManage: boolean;
}) {
  const { t } = useT("settings");
  const { policy } = useRoomWorkflowPolicy(wsId);
  const { workflows } = useIssueWorkflows(wsId);
  const setPolicy = useSetRoomWorkflowPolicy();

  // Tracks an in-progress switch to "workflow" before the admin picks one from
  // the workflow dropdown. Cleared as soon as a save succeeds or another kind is chosen.
  const [pickingWorkflow, setPickingWorkflow] = useState(false);

  const stored = policy ?? DEFAULT_POLICY;
  const nonDefaultWorkflows = useMemo(
    () => workflows.filter((w) => !w.is_default),
    [workflows],
  );
  const workflowItems = useMemo(
    () => nonDefaultWorkflows.map((w) => ({ value: w.id, label: w.name })),
    [nonDefaultWorkflows],
  );

  const activeKind: RoomNonRequestKind = pickingWorkflow
    ? "workflow"
    : stored.kind;
  const selectedWorkflowId = stored.kind === "workflow" ? stored.workflowId ?? "" : "";

  const modeItems = useMemo(
    () => [
      { value: "no_action", label: t(($) => $.workspace.room_workflow_no_action) },
      { value: "default", label: t(($) => $.workspace.room_workflow_default) },
      { value: "workflow", label: t(($) => $.workspace.room_workflow_existing) },
    ],
    [t],
  );

  const readOnlyLabel = useMemo(() => {
    if (stored.kind === "default") {
      return t(($) => $.workspace.room_workflow_default);
    }
    if (stored.kind === "workflow") {
      const matched = workflows.find((w) => w.id === stored.workflowId);
      return matched?.name ?? stored.workflowId ?? t(($) => $.workspace.room_workflow_default);
    }
    return t(($) => $.workspace.room_workflow_no_action);
  }, [stored.kind, stored.workflowId, workflows, t]);

  async function persist(next: RoomWorkflowPolicy) {
    try {
      await setPolicy.mutateAsync(next);
      setPickingWorkflow(false);
      toast.success(t(($) => $.workspace.room_workflow_saved));
    } catch (err) {
      const status =
        err && typeof err === "object" && "status" in err
          ? (err as { status?: unknown }).status
          : undefined;
      if (status === 403) {
        toast.error(t(($) => $.workspace.room_workflow_admin_only));
      } else {
        toast.error(
          err instanceof Error && err.message
            ? err.message
            : t(($) => $.workspace.room_workflow_failed),
        );
      }
    }
  }

  return (
    <SettingsCard>
      <SettingsRow
        anchor="room-non-request-workflow"
        label={t(($) => $.workspace.room_workflow_label)}
        size={canManage ? "select-wide" : undefined}
      >
        {canManage ? (
          <Select
            items={modeItems}
            value={activeKind}
            disabled={setPolicy.isPending}
            onValueChange={(value) => {
              if (value === null) return;
              if (value === "workflow") {
                setPickingWorkflow(true);
                return;
              }
              setPickingWorkflow(false);
              if (value === "no_action" || value === "default") {
                void persist({ kind: value, workflowId: null });
              }
            }}
          >
            <SelectTrigger aria-label={t(($) => $.workspace.room_workflow_label)}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {modeItems.map((item) => (
                <SelectItem
                  key={item.value}
                  value={item.value}
                  disabled={
                    item.value === "workflow" && workflowItems.length === 0
                  }
                >
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <p className="text-body text-foreground sm:text-right">
            {readOnlyLabel}
          </p>
        )}
      </SettingsRow>

      {canManage && activeKind === "workflow" && (
        <SettingsRow
          anchor="room-non-request-workflow-picker"
          label={t(($) => $.workspace.room_workflow_workflow_label)}
          size="select-wide"
        >
          <Select
            items={workflowItems}
            value={selectedWorkflowId}
            disabled={setPolicy.isPending || workflowItems.length === 0}
            onValueChange={(value) => {
              if (value !== null) {
                void persist({ kind: "workflow", workflowId: value });
              }
            }}
          >
            <SelectTrigger aria-label={t(($) => $.workspace.room_workflow_workflow_label)}>
              <SelectValue
                placeholder={t(($) => $.workspace.room_workflow_select_placeholder)}
              />
            </SelectTrigger>
            <SelectContent>
              {workflowItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingsRow>
      )}
    </SettingsCard>
  );
}
