"use client";

import { Badge } from "@multica/ui/components/ui/badge";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@multica/ui/components/ui/sheet";
import { ArrowRight, Lock, Paperclip, ShieldCheck, Workflow } from "@multica/ui/icons";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import type { IssueWorkflow, IssueWorkflowTask, WorkflowCondition } from "@multica/core/types";

export interface WorkflowGraphSheetProps {
  workflow: IssueWorkflow | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// A condition in words: a switch that must be on reads as its key, other checks as "field op value".
function formatWhenCondition(when: WorkflowCondition | null | undefined): string {
  if (!when) return "";
  if ("all" in when) return when.all.map(formatWhenCondition).filter(Boolean).join(" & ");
  if ("any" in when) return when.any.map(formatWhenCondition).filter(Boolean).join(" | ");
  if ("not" in when) return `not ${formatWhenCondition(when.not)}`;
  const field = when.field.replace(/^switch\./, "");
  if (when.op === "eq" && when.value === true) return field;
  return `${field} ${when.op} ${String(when.value ?? "")}`;
}

// N of a rework branch's `rework_count lt N` check, alone or inside an all group.
function extractReworkLimit(when: WorkflowCondition | null | undefined): number | null {
  if (!when) return null;
  if ("all" in when) {
    for (const child of when.all) {
      const limit = extractReworkLimit(child);
      if (limit !== null) return limit;
    }
    return null;
  }
  if ("field" in when && when.field === "rework_count" && when.op === "lt" && typeof when.value === "number") return when.value;
  return null;
}

export function WorkflowGraphSheet({
  workflow,
  open,
  onOpenChange,
}: WorkflowGraphSheetProps) {
  const { t } = useT("squads");

  if (!workflow) return null;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="w-full sm:max-w-2xl overflow-y-auto p-4 sm:p-6"
      >
        <SheetHeader className="space-y-2 border-b border-border pb-4">
          <div className="flex items-center gap-2 flex-wrap">
            <Workflow className="size-5 text-muted-foreground shrink-0" aria-hidden />
            <SheetTitle className="text-title font-semibold text-foreground">
              {workflow.name}
            </SheetTitle>
            {workflow.version !== undefined && workflow.version !== null && (
              <Badge variant="secondary" className="text-micro">
                {t(($) => $.flow_designer.version, { n: workflow.version })}
              </Badge>
            )}
            {(workflow.published_at || workflow.template_ref) && (
              <Badge variant="outline" className="text-micro gap-1">
                <Lock className="size-2.5" aria-hidden />
                <span>{t(($) => $.flow_designer.locked)}</span>
              </Badge>
            )}
            {workflow.template_ref && (
              <Badge variant="outline" className="text-micro">
                {workflow.template_ref}
              </Badge>
            )}
          </div>
          {workflow.description ? (
            <SheetDescription className="text-body-sm text-muted-foreground">
              {workflow.description}
            </SheetDescription>
          ) : null}
        </SheetHeader>

        <div className="py-6 space-y-6">
          {workflow.steps.map((step, idx) => {
            const tasks: IssueWorkflowTask[] = step.tasks ?? [];
            return (
              <div
                key={step.status_key || idx}
                className="rounded-xl border border-border bg-card p-4 space-y-3.5 shadow-xs"
              >
                {/* Step header */}
                <div className="flex items-center justify-between gap-2 flex-wrap">
                  <div className="flex items-center gap-2">
                    <span className="flex size-6 items-center justify-center rounded-full bg-muted text-caption font-semibold text-muted-foreground">
                      {idx + 1}
                    </span>
                    <Badge variant="secondary" className="text-body-sm font-medium">
                      {step.status_key}
                    </Badge>
                  </div>
                  {step.human_only && (
                    <Badge variant="outline" className="gap-1 text-micro text-amber-600 dark:text-amber-400 border-amber-500/30">
                      <Lock className="size-3" aria-hidden />
                      <span>{t(($) => $.workflows.sheet.human_only)}</span>
                    </Badge>
                  )}
                </div>

                {/* Instructions if any */}
                {step.instructions ? (
                  <p className="text-caption text-muted-foreground bg-muted/40 p-2.5 rounded-lg border border-border/40">
                    {step.instructions}
                  </p>
                ) : null}

                {/* Step tasks */}
                {tasks.length > 0 && (
                  <div className="space-y-2 pt-1">
                    <div className="text-caption font-medium text-muted-foreground">
                      {t(($) => $.workflows.tasks_count, { count: tasks.length })}
                    </div>
                    <div className="grid gap-2 sm:grid-cols-2">
                      {tasks.map((task, taskIdx) => {
                        const isGate = task.kind === "gate";
                        const requiresOutcome =
                          isGate || task.requires.includes("outcome");
                        const requiresAttachment =
                          task.requires.includes("attachment");
                        const whenStr = formatWhenCondition(task.when);

                        return (
                          <div
                            key={task.key || taskIdx}
                            className={cn(
                              "flex flex-col gap-1.5 p-3 rounded-lg border text-caption bg-background/50",
                              isGate ? "border-amber-500/40 bg-amber-500/5" : "border-border/60",
                            )}
                          >
                            <div className="flex items-center justify-between gap-1.5">
                              <span className="font-medium text-foreground truncate">
                                {task.key}
                              </span>
                              <Badge
                                variant={isGate ? "default" : "secondary"}
                                className="text-micro shrink-0"
                              >
                                {isGate
                                  ? t(($) => $.workflows.sheet.kind_gate)
                                  : t(($) => $.workflows.sheet.kind_work)}
                              </Badge>
                            </div>

                            <div className="text-micro text-muted-foreground truncate">
                              {task.title}
                            </div>

                            {/* Badges for requires & conditions */}
                            <div className="flex flex-wrap gap-1 pt-1">
                              {requiresOutcome && (
                                <Badge variant="outline" className="text-micro gap-1 text-muted-foreground">
                                  <ShieldCheck className="size-3" aria-hidden />
                                  <span>{t(($) => $.workflows.sheet.requires_outcome)}</span>
                                </Badge>
                              )}
                              {requiresAttachment && (
                                <Badge variant="outline" className="text-micro gap-1 text-muted-foreground">
                                  <Paperclip className="size-3" aria-hidden />
                                  <span>{t(($) => $.workflows.sheet.requires_attachment)}</span>
                                </Badge>
                              )}
                              {whenStr && (
                                <Badge variant="ghost" className="text-micro bg-muted/60 text-muted-foreground">
                                  {t(($) => $.workflows.sheet.optional_when, { condition: whenStr })}
                                </Badge>
                              )}
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}

                {/* Step transitions */}
                {step.transitions && step.transitions.length > 0 && (
                  <div className="space-y-1.5 pt-2 border-t border-border/40">
                    {step.transitions.map((tr) => {
                      const reworkLimit = tr.rework ? extractReworkLimit(tr.when) : null;
                      return (
                        <div
                          key={tr.id}
                          className="flex items-center gap-2 text-caption text-muted-foreground flex-wrap"
                        >
                          <ArrowRight className="size-3.5 shrink-0" aria-hidden />
                          <span className="font-medium text-foreground">{tr.to_status_key}</span>
                          {tr.label && <span className="text-micro truncate">({tr.label})</span>}
                          {reworkLimit !== null && (
                            <Badge variant="secondary" className="text-micro">
                              {t(($) => $.workflows.sheet.rework_limit, { count: reworkLimit })}
                            </Badge>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </SheetContent>
    </Sheet>
  );
}
