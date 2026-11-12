"use client";

import { useQuery } from "@tanstack/react-query";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Label } from "@multica/ui/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  ArrowRight,
  ChevronLeft,
  ChevronRight,
  GripVertical,
  Plus,
  Trash2,
} from "@multica/ui/icons";
import {
  agentListOptions,
  memberListOptions,
} from "@multica/core/workspace/queries";
import { StatusIcon } from "../../issues/components/status-icon";
import { useStatusLabel } from "../../issues/utils/status-label";
import { ActorAvatar } from "../../common/actor-avatar";
import { useT } from "../../i18n";
import type {
  IssueWorkflowStep,
  IssueWorkflowTransition,
} from "../types";
import type { IssueStatusEntry, IssueWorkflowHandlerType } from "@multica/core/types";

export interface StatusNodeCardProps {
  step: IssueWorkflowStep;
  wsId: string;
  statusDef?: IssueStatusEntry;
  canMoveLeft: boolean;
  canMoveRight: boolean;
  canRemove: boolean;
  onMoveLeft: () => void;
  onMoveRight: () => void;
  onRemove: () => void;
  onUpdateStep: (updated: IssueWorkflowStep) => void;
  onAddTransition: () => void;
  onEditTransition: (transition: IssueWorkflowTransition) => void;
  isSelected?: boolean;
  onSelect?: () => void;
  dragHandleProps?: Record<string, unknown>;
}

export function StatusNodeCard({
  step,
  wsId,
  statusDef,
  canMoveLeft,
  canMoveRight,
  canRemove,
  onMoveLeft,
  onMoveRight,
  onRemove,
  onUpdateStep,
  onAddTransition,
  onEditTransition,
  isSelected,
  onSelect,
  dragHandleProps,
}: StatusNodeCardProps) {
  const { t } = useT("workflow");
  const getStatusLabel = useStatusLabel(wsId);

  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: agents = [] } = useQuery(agentListOptions(wsId));

  const isBacklog = step.status_key === "backlog";
  const isTodo = step.status_key === "todo";
  const isDone = step.status_key === "done";
  const isCancelled = step.status_key === "cancelled";
  const isBlocked = step.status_key === "blocked";

  const handleHandlerTypeChange = (type: IssueWorkflowHandlerType) => {
    let id: string | undefined = undefined;
    if (type === "member") id = members[0]?.id;
    if (type === "agent") id = agents[0]?.id;

    onUpdateStep({
      ...step,
      handler: { type, id },
    });
  };

  const handleHandlerIdChange = (id: string | null) => {
    if (!id) return;
    onUpdateStep({
      ...step,
      handler: { ...step.handler, id },
    });
  };

  const handleInstructionsChange = (instructions: string) => {
    onUpdateStep({
      ...step,
      instructions,
    });
  };

  const handlerItems = [
    { value: "none", label: t(($) => $.designer.node.no_handler) },
    { value: "agent", label: t(($) => $.designer.node.handler_agent) },
    { value: "member", label: t(($) => $.designer.node.handler_member) },
    { value: "project_lead", label: t(($) => $.designer.node.handler_project_lead) },
  ];

  const agentItems = agents.map((ag) => ({ value: ag.id, label: ag.name }));
  const memberItems = members.map((m) => ({
    value: m.id,
    label: m.name || m.email || m.id,
  }));

  return (
    <div
      id={`node-${step.status_key}`}
      onClick={onSelect}
      className={`relative flex flex-col w-[300px] shrink-0 rounded-xl border bg-card transition-all ${
        isSelected
          ? "border-primary ring-2 ring-primary/20 shadow-md"
          : "border-border shadow-xs hover:border-sidebar-accent"
      }`}
    >
      {/* Input Port (Target anchor on the left) */}
      <div
        data-port-input={step.status_key}
        className="absolute -left-2 top-10 size-4 rounded-full border-2 border-background bg-muted-foreground/60 shadow-xs"
        aria-hidden="true"
      />

      {/* Output Port (Source anchor on the right) */}
      <div
        data-port-output={step.status_key}
        className="absolute -right-2 top-10 size-4 rounded-full border-2 border-background bg-primary shadow-xs cursor-crosshair hover:scale-125 transition-transform"
        aria-hidden="true"
      />

      {/* Header */}
      <div className="flex items-center justify-between p-3 border-b border-border bg-muted/20 rounded-t-xl gap-2">
        <div className="flex items-center gap-1.5 min-w-0">
          <button
            type="button"
            className="cursor-grab active:cursor-grabbing text-muted-foreground hover:text-foreground p-0.5"
            {...dragHandleProps}
            aria-label={t(($) => $.designer.node.drag_handle)}
          >
            <GripVertical className="size-4" />
          </button>
          <StatusIcon status={step.status_key} className="size-4 shrink-0" />
          <span className="text-body font-semibold text-foreground truncate">
            {statusDef ? getStatusLabel(statusDef.key) : step.status_key}
          </span>
        </div>

        {/* Badges / Controls */}
        <div className="flex items-center gap-1 shrink-0">
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

          {/* Keyboard reordering buttons */}
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            disabled={!canMoveLeft}
            onClick={(e) => {
              e.stopPropagation();
              onMoveLeft();
            }}
            aria-label={t(($) => $.designer.node.move_left)}
          >
            <ChevronLeft className="size-3.5" />
          </Button>

          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            disabled={!canMoveRight}
            onClick={(e) => {
              e.stopPropagation();
              onMoveRight();
            }}
            aria-label={t(($) => $.designer.node.move_right)}
          >
            <ChevronRight className="size-3.5" />
          </Button>

          {canRemove && (
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              onClick={(e) => {
                e.stopPropagation();
                onRemove();
              }}
              className="text-muted-foreground hover:text-destructive"
              aria-label={t(($) => $.designer.node.remove_step)}
            >
              <Trash2 className="size-3.5" />
            </Button>
          )}
        </div>
      </div>

      {/* Body: Handler picker and Instructions */}
      <div className="p-3 space-y-3">
        {/* Handler picker */}
        <div className="space-y-1.5">
          <Label className="text-caption font-medium text-muted-foreground">
            {t(($) => $.designer.node.handler)}
          </Label>
          <div className="grid grid-cols-1 gap-1.5">
            <Select
              items={handlerItems}
              value={step.handler.type}
              onValueChange={(val) =>
                handleHandlerTypeChange(val as IssueWorkflowHandlerType)
              }
            >
              <SelectTrigger className="h-8 text-caption">
                <SelectValue>
                  {handlerItems.find((h) => h.value === step.handler.type)?.label}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {handlerItems.map((h) => (
                  <SelectItem key={h.value} value={h.value}>
                    {h.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {/* Actor choice dropdown when handler type needs an ID */}
            {step.handler.type === "agent" && agents.length > 0 && (
              <Select
                items={agentItems}
                value={step.handler.id || agents[0]?.id || ""}
                onValueChange={handleHandlerIdChange}
              >
                <SelectTrigger className="h-8 text-caption">
                  <SelectValue>
                    {agentItems.find((a) => a.value === (step.handler.id || agents[0]?.id))?.label}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {agents.map((ag) => (
                    <SelectItem key={ag.id} value={ag.id}>
                      <div className="flex items-center gap-1.5">
                        <ActorAvatar actorType="agent" actorId={ag.id} className="size-3.5" />
                        <span>{ag.name}</span>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}

            {step.handler.type === "member" && members.length > 0 && (
              <Select
                items={memberItems}
                value={step.handler.id || members[0]?.id || ""}
                onValueChange={handleHandlerIdChange}
              >
                <SelectTrigger className="h-8 text-caption">
                  <SelectValue>
                    {memberItems.find((m) => m.value === (step.handler.id || members[0]?.id))?.label}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {members.map((m) => (
                    <SelectItem key={m.id} value={m.id}>
                      <div className="flex items-center gap-1.5">
                        <ActorAvatar actorType="member" actorId={m.id} className="size-3.5" />
                        <span>{m.name || m.email || m.id}</span>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>
        </div>

        {/* Instructions */}
        <div className="space-y-1.5">
          <Label className="text-caption font-medium text-muted-foreground">
            {t(($) => $.designer.node.instructions)}
          </Label>
          <Textarea
            value={step.instructions || ""}
            onChange={(e) => handleInstructionsChange(e.target.value)}
            placeholder={t(($) => $.designer.node.instructions_placeholder)}
            rows={2}
            className="text-caption resize-none min-h-[56px]"
          />
        </div>
      </div>

      {/* Transitions / Outgoing branches */}
      <div className="p-3 border-t border-border bg-muted/10 rounded-b-xl space-y-2">
        <div className="flex items-center justify-between">
          <span className="text-caption font-medium text-muted-foreground">
            {t(($) => $.designer.node.transitions)} ({step.transitions?.length ?? 0})
          </span>
          <Button
            type="button"
            variant="ghost"
            size="xs"
            onClick={(e) => {
              e.stopPropagation();
              onAddTransition();
            }}
            className="h-6 text-micro gap-1 text-primary hover:text-primary"
          >
            <Plus className="size-3" />
            <span>{t(($) => $.designer.node.add_transition)}</span>
          </Button>
        </div>

        <div className="flex flex-col gap-1.5">
          {step.transitions?.map((trans) => (
            <button
              key={trans.id}
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onEditTransition(trans);
              }}
              className="flex items-center justify-between p-1.5 rounded-md border border-border/70 bg-card hover:bg-accent/40 text-left text-micro transition-colors group/item"
            >
              <span className="font-medium text-foreground truncate max-w-[120px]">
                {trans.label || (trans.when === null ? "Default" : "Branch")}
              </span>
              <div className="flex items-center gap-1 text-muted-foreground group-hover/item:text-foreground">
                <ArrowRight className="size-3" />
                <span className="font-mono">{trans.to_status_key}</span>
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
