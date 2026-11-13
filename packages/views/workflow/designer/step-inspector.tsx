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
  ChevronDown,
  ChevronUp,
  Pencil,
  Plus,
  Trash2,
  X,
} from "@multica/ui/icons";
import { agentListOptions, memberListOptions } from "@multica/core/workspace/queries";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import type { IssueWorkflowHandlerType } from "@multica/core/types";
import { StatusIcon } from "../../issues/components/status-icon";
import { useStatusLabel } from "../../issues/utils/status-label";
import { ActorAvatar } from "../../common/actor-avatar";
import { useT } from "../../i18n";
import type { IssueWorkflowStep, IssueWorkflowTransition } from "../types";

export interface StepInspectorProps {
  step: IssueWorkflowStep | null;
  steps: IssueWorkflowStep[];
  wsId: string;
  onUpdateStep: (updated: IssueWorkflowStep) => void;
  onMoveStepUp: (stepKey: string) => void;
  onMoveStepDown: (stepKey: string) => void;
  onRemoveStep: (stepKey: string) => void;
  onAddStep: () => void;
  onSelectStep: (stepKey: string) => void;
  onAddTransition: (sourceKey: string) => void;
  onEditTransition: (sourceKey: string, transition: IssueWorkflowTransition) => void;
  onDeleteTransition: (sourceKey: string, transitionId: string) => void;
  onClose: () => void;
  /** A template-managed workflow is read-only: its steps and transitions cannot change here. */
  readOnly?: boolean;
}

export function StepInspector({
  step,
  steps,
  wsId,
  onUpdateStep,
  onMoveStepUp,
  onMoveStepDown,
  onRemoveStep,
  onAddStep,
  onSelectStep,
  onAddTransition,
  onEditTransition,
  onDeleteTransition,
  onClose,
  readOnly = false,
}: StepInspectorProps) {
  const { t } = useT("workflow");
  useIssueStatuses(wsId);
  const getStatusLabel = useStatusLabel(wsId);

  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: agents = [] } = useQuery(agentListOptions(wsId));

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

  const stepIndex = step ? steps.findIndex((s) => s.status_key === step.status_key) : -1;
  const canMoveUp = stepIndex > 0;
  const canMoveDown = stepIndex >= 0 && stepIndex < steps.length - 1;

  const isTodo = step?.status_key === "todo";
  const isDone = step?.status_key === "done";
  const isCancelled = step?.status_key === "cancelled";
  const canRemove = Boolean(step && !isTodo && !isDone && !isCancelled);

  const isNewStatus = Boolean(step?.new_status);
  const statusName = step
    ? step.new_status
      ? step.new_status.name
      : getStatusLabel(step.status_key)
    : "";

  const handleHandlerTypeChange = (type: IssueWorkflowHandlerType) => {
    if (!step) return;
    let id: string | undefined = undefined;
    if (type === "member") id = members[0]?.id;
    if (type === "agent") id = agents[0]?.id;

    onUpdateStep({
      ...step,
      handler: { type, id },
    });
  };

  const handleHandlerIdChange = (id: string | null) => {
    if (!step || !id) return;
    onUpdateStep({
      ...step,
      handler: { ...step.handler, id },
    });
  };

  const handleInstructionsChange = (instructions: string) => {
    if (!step) return;
    onUpdateStep({
      ...step,
      instructions: instructions.slice(0, 400),
    });
  };

  return (
    <aside
      aria-label={t(($) => $.designer.inspector.title)}
      className="w-80 sm:w-88 border-l border-border bg-card p-4 overflow-y-auto flex flex-col gap-5 shrink-0"
    >
      {step ? (
        <>
          {/* Header of selected node */}
          <div className="flex flex-col gap-2 pb-3 border-b border-border">
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2 min-w-0">
                <StatusIcon status={step.status_key} className="size-4 shrink-0" />
                <span className="text-body font-semibold text-foreground truncate">
                  {statusName}
                </span>
                {isNewStatus && (
                  <Badge variant="outline" className="text-micro font-medium text-brand border-brand/40">
                    {t(($) => $.designer.node.will_be_created)}
                  </Badge>
                )}
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                onClick={onClose}
                aria-label={t(($) => $.designer.inspector.close)}
              >
                <X className="size-3.5" />
              </Button>
            </div>

            {/* Step Position & Order controls */}
            <div className="flex items-center justify-between text-caption text-muted-foreground pt-1">
              <span>
                {t(($) => $.designer.inspector.step_count, {
                  current: stepIndex + 1,
                  total: steps.length,
                })}
              </span>
              <div className="flex items-center gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  disabled={readOnly || !canMoveUp}
                  onClick={() => onMoveStepUp(step.status_key)}
                  aria-label={t(($) => $.designer.node.move_up)}
                >
                  <ChevronUp className="size-3.5" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  disabled={readOnly || !canMoveDown}
                  onClick={() => onMoveStepDown(step.status_key)}
                  aria-label={t(($) => $.designer.node.move_down)}
                >
                  <ChevronDown className="size-3.5" />
                </Button>
                {canRemove && !readOnly && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    onClick={() => onRemoveStep(step.status_key)}
                    className="text-muted-foreground hover:text-destructive"
                    aria-label={t(($) => $.designer.node.remove_step)}
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                )}
              </div>
            </div>
          </div>

          {/* Handler Section */}
          <div className="space-y-2">
            <Label className="text-caption font-medium text-foreground">
              {t(($) => $.designer.node.handler)}
            </Label>
            <div className="flex flex-col gap-2">
              <Select
              disabled={readOnly}
                items={handlerItems}
                value={step.handler.type}
                onValueChange={(val) => handleHandlerTypeChange(val as IssueWorkflowHandlerType)}
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

              {/* Agent selector */}
              {step.handler.type === "agent" && agents.length > 0 && (
                <Select
                disabled={readOnly}
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

              {/* Member selector */}
              {step.handler.type === "member" && members.length > 0 && (
                <Select
                disabled={readOnly}
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

              {/* Role Hint Chip when present */}
              {Boolean(step.role_hint) && (
                <div className="flex items-center gap-1.5 pt-1">
                  <span className="text-micro text-muted-foreground">
                    {t(($) => $.designer.node.role_hint, { role: step.role_hint! })}
                  </span>
                </div>
              )}
            </div>
          </div>

          {/* Instructions Section */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label className="text-caption font-medium text-foreground">
                {t(($) => $.designer.node.instructions)}
              </Label>
              <span className="text-micro text-muted-foreground">
                {(step.instructions || "").length}/400
              </span>
            </div>
            <Textarea
            disabled={readOnly}
              value={step.instructions || ""}
              onChange={(e) => handleInstructionsChange(e.target.value)}
              placeholder={t(($) => $.designer.node.instructions_placeholder)}
              rows={3}
              className="text-caption resize-none min-h-[72px]"
            />
          </div>

          {/* Transitions Section */}
          <div className="space-y-2 pt-2 border-t border-border">
            <div className="flex items-center justify-between">
              <Label className="text-caption font-medium text-foreground">
                {t(($) => $.designer.node.transitions)} ({step.transitions?.length ?? 0})
              </Label>
              <Button
                type="button"
                variant="ghost"
                size="xs"
                onClick={() => onAddTransition(step.status_key)}
                disabled={readOnly}
                className="h-6 text-micro gap-1 text-primary hover:text-primary"
              >
                <Plus className="size-3" />
                <span>{t(($) => $.designer.node.add_transition)}</span>
              </Button>
            </div>

            <div className="flex flex-col gap-1.5">
              {(!step.transitions || step.transitions.length === 0) ? (
                <span className="text-micro text-muted-foreground italic py-1">
                  {t(($) => $.designer.inspector.no_transitions)}
                </span>
              ) : (
                step.transitions.map((trans) => (
                  <div
                    key={trans.id}
                    className="flex items-center justify-between p-2 rounded-md border border-border bg-muted/30 text-micro gap-2"
                  >
                    <div className="flex items-center gap-1.5 min-w-0">
                      <span className="font-medium text-foreground truncate max-w-[100px]">
                        {trans.label || (trans.when === null ? "Default" : "Branch")}
                      </span>
                      <ArrowRight className="size-3 text-muted-foreground shrink-0" />
                      <span className="font-mono text-muted-foreground truncate max-w-[80px]">
                        {getStatusLabel(trans.to_status_key)}
                      </span>
                    </div>

                    <div className="flex items-center gap-0.5 shrink-0">
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => onEditTransition(step.status_key, trans)}
                        disabled={readOnly}
                        aria-label={t(($) => $.designer.edge.edit_transition)}
                      >
                        <Pencil className="size-3" />
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => onDeleteTransition(step.status_key, trans.id)}
                        disabled={readOnly}
                        className="text-muted-foreground hover:text-destructive"
                        aria-label={t(($) => $.designer.edge.delete_transition)}
                      >
                        <Trash2 className="size-3" />
                      </Button>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        </>
      ) : (
        <div className="flex flex-col gap-2 pb-3 border-b border-border">
          <span className="text-body font-semibold text-foreground">
            {t(($) => $.designer.inspector.title)}
          </span>
          <span className="text-caption text-muted-foreground">
            {t(($) => $.designer.inspector.select_step_hint)}
          </span>
        </div>
      )}

      {/* Keyboard Path: All steps list with Move up / Move down and Add step */}
      <div className="space-y-2 pt-2 border-t border-border mt-auto">
        <div className="flex items-center justify-between">
          <Label className="text-caption font-medium text-foreground">
            {t(($) => $.designer.inspector.board_order)}
          </Label>
          <Button
            type="button"
            variant="outline"
            size="xs"
            onClick={onAddStep}
            disabled={readOnly}
            className="h-6 text-micro gap-1"
          >
            <Plus className="size-3" />
            <span>{t(($) => $.designer.inspector.add_step)}</span>
          </Button>
        </div>

        <div className="flex flex-col gap-1 max-h-56 overflow-y-auto">
          {steps.map((s, idx) => {
            const isCurrentSelected = step?.status_key === s.status_key;
            const sName = s.new_status
              ? s.new_status.name
              : getStatusLabel(s.status_key);

            return (
              <div
                key={s.status_key}
                role="button"
                tabIndex={0}
                onClick={() => onSelectStep(s.status_key)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    onSelectStep(s.status_key);
                  }
                }}
                className={`flex items-center justify-between px-2 py-1.5 rounded-md border text-caption transition-colors cursor-pointer ${
                  isCurrentSelected
                    ? "border-primary bg-primary/10 font-medium"
                    : "border-border/60 hover:bg-muted/40"
                }`}
              >
                <div className="flex items-center gap-1.5 min-w-0">
                  <span className="text-micro text-muted-foreground w-4 shrink-0">
                    {idx + 1}.
                  </span>
                  <StatusIcon status={s.status_key} className="size-3.5 shrink-0" />
                  <span className="truncate">{sName}</span>
                </div>

                <div className="flex items-center gap-0.5 shrink-0" onClick={(e) => e.stopPropagation()}>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    disabled={readOnly || idx === 0}
                    onClick={() => onMoveStepUp(s.status_key)}
                    aria-label={t(($) => $.designer.node.move_up)}
                  >
                    <ChevronUp className="size-3" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    disabled={readOnly || idx === steps.length - 1}
                    onClick={() => onMoveStepDown(s.status_key)}
                    aria-label={t(($) => $.designer.node.move_down)}
                  >
                    <ChevronDown className="size-3" />
                  </Button>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </aside>
  );
}
