"use client";

import { memo } from "react";
import { Handle, Position, type NodeProps, type Node } from "@xyflow/react";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@multica/ui/components/ui/badge";
import { memberListOptions, agentListOptions } from "@multica/core/workspace/queries";
import type { IssueStatusEntry } from "@multica/core/types";
import { StatusIcon } from "../../issues/components/status-icon";
import { useStatusLabel } from "../../issues/utils/status-label";
import { ActorAvatar } from "../../common/actor-avatar";
import { useT } from "../../i18n";
import type { IssueWorkflowStep } from "../types";

export interface StatusNodeData extends Record<string, unknown> {
  step: IssueWorkflowStep;
  wsId: string;
  statusDef?: IssueStatusEntry;
  isSelected?: boolean;
  onSelect?: () => void;
}

export type StatusNodeType = Node<StatusNodeData, "statusNode">;

function StatusNodeCardComponent({ data, selected }: NodeProps<StatusNodeType>) {
  const { step, wsId, statusDef, isSelected: dataSelected, onSelect } = data;
  const isSelected = selected || dataSelected;

  const { t } = useT("workflow");
  const getStatusLabel = useStatusLabel(wsId);

  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: agents = [] } = useQuery(agentListOptions(wsId));

  const isBacklog = step.status_key === "backlog";
  const isTodo = step.status_key === "todo";
  const isDone = step.status_key === "done";
  const isCancelled = step.status_key === "cancelled";
  const isBlocked = step.status_key === "blocked";

  const isNewStatus = Boolean(step.new_status);
  const statusName = step.new_status
    ? step.new_status.name
    : statusDef
      ? getStatusLabel(statusDef.key)
      : step.status_key;

  // Resolve handler name
  let handlerText = t(($) => $.designer.node.no_handler);
  let handlerAvatar: React.ReactNode = null;

  if (step.handler.type === "agent") {
    const ag = agents.find((a) => a.id === step.handler.id);
    handlerText = ag?.name ?? t(($) => $.designer.node.handler_agent);
    if (ag) {
      handlerAvatar = <ActorAvatar actorType="agent" actorId={ag.id} className="size-3.5 shrink-0" />;
    }
  } else if (step.handler.type === "member") {
    const mb = members.find((m) => m.id === step.handler.id);
    handlerText = mb?.name || mb?.email || t(($) => $.designer.node.handler_member);
    if (mb) {
      handlerAvatar = <ActorAvatar actorType="member" actorId={mb.id} className="size-3.5 shrink-0" />;
    }
  } else if (step.handler.type === "project_lead") {
    handlerText = t(($) => $.designer.node.handler_project_lead);
  }

  const isUnassigned = step.handler.type === "none";
  const roleHint = step.role_hint;
  const transitionsCount = step.transitions?.length ?? 0;

  return (
    <div
      onClick={onSelect}
      data-testid={`status-node-${step.status_key}`}
      className={`relative flex flex-col w-[260px] rounded-xl border bg-card text-card-foreground select-none transition-all shadow-xs ${
        isSelected
          ? "border-primary ring-2 ring-primary/20 shadow-md"
          : "border-border hover:border-sidebar-accent"
      }`}
    >
      {/* Target Handle on Left */}
      <Handle
        type="target"
        position={Position.Left}
        className="!size-3.5 !bg-muted-foreground/60 !border-2 !border-background shadow-xs hover:scale-125 transition-transform"
      />

      {/* Source Handle on Right */}
      <Handle
        type="source"
        position={Position.Right}
        className="!size-3.5 !bg-primary !border-2 !border-background shadow-xs cursor-crosshair hover:scale-125 transition-transform"
      />

      {/* Header */}
      <div className="flex items-center justify-between p-3 border-b border-border bg-muted/20 rounded-t-xl gap-2">
        <div className="flex items-center gap-1.5 min-w-0">
          <StatusIcon status={step.status_key} className="size-4 shrink-0" />
          <span className="text-body font-semibold text-foreground truncate">
            {statusName}
          </span>
        </div>

        {/* Badges */}
        <div className="flex items-center gap-1 shrink-0">
          {isNewStatus && (
            <Badge variant="outline" className="text-micro font-medium text-brand border-brand/40">
              {t(($) => $.designer.node.will_be_created)}
            </Badge>
          )}
          {isBacklog && (
            <Badge variant="secondary" className="text-micro">
              {t(($) => $.designer.node.fixed_head)}
            </Badge>
          )}
          {isTodo && (
            <Badge variant="outline" className="text-micro font-medium text-brand border-brand/40">
              {t(($) => $.designer.node.required)}
            </Badge>
          )}
          {isBlocked && (
            <Badge variant="secondary" className="text-micro">
              {t(($) => $.designer.node.fixed_tail)}
            </Badge>
          )}
          {(isDone || isCancelled) && (
            <Badge variant="outline" className="text-micro font-medium text-brand border-brand/40">
              {t(($) => $.designer.node.required)}
            </Badge>
          )}
        </div>
      </div>

      {/* Body: Handler, Role hint chip when unassigned, and transition count */}
      <div className="p-3 space-y-2">
        {/* Handler display */}
        <div className="flex items-center justify-between gap-2 text-caption">
          <span className="text-muted-foreground shrink-0">{t(($) => $.designer.node.handler)}:</span>
          <div className="flex items-center gap-1.5 min-w-0 truncate font-medium text-foreground">
            {handlerAvatar}
            <span className="truncate">{handlerText}</span>
          </div>
        </div>

        {/* Role hint chip when unassigned */}
        {isUnassigned && Boolean(roleHint) && (
          <div className="flex items-center gap-1.5">
            <span
              data-testid="role-hint-chip"
              title={t(($) => $.designer.node.role_hint, { role: roleHint! })}
              className="inline-flex items-center rounded-xs bg-muted/80 px-1.5 py-0.5 text-micro font-medium text-muted-foreground border border-border/50 truncate max-w-full"
            >
              {roleHint}
            </span>
          </div>
        )}

        {/* Transitions count */}
        <div className="flex items-center justify-between text-caption pt-1 border-t border-border/40 text-muted-foreground">
          <span>{t(($) => $.designer.node.transitions)}</span>
          <span className="font-medium text-foreground">{transitionsCount}</span>
        </div>
      </div>
    </div>
  );
}

export const StatusNodeCard = memo(StatusNodeCardComponent);
