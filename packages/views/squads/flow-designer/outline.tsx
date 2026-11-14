"use client";

import { useMemo, useState } from "react";
import type { FlowDoc, FlowDocStep, FlowIssue } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { AlertCircle, AlertTriangle, Plus, RotateCcw, Search } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { cn } from "@multica/ui/lib/utils";
import { STEP_TEMPLATE_IDS, type StepTemplateId } from "./step-templates";

export interface OutlineProps {
  doc: FlowDoc;
  selectedStepKey: string | null;
  onSelectStep: (stepKey: string) => void;
  /** "blank" adds an empty step of tasks; a template id adds a step with its standard tasks and edges. */
  onAddStep?: (template: StepTemplateId | "blank") => void;
  issues?: FlowIssue[];
  stepLimit?: number;
  /** Fills the width it is given, as on a phone, instead of the side column of a laptop. */
  fullWidth?: boolean;
}

export function Outline({
  fullWidth = false,
  doc,
  selectedStepKey,
  onSelectStep,
  onAddStep,
  issues = [],
  stepLimit = 40,
}: OutlineProps) {
  const { t } = useT("squads");
  const [filter, setFilter] = useState("");

  const filteredSteps = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return doc.steps;
    return doc.steps.filter(
      (s) => s.title.toLowerCase().includes(q) || s.key.toLowerCase().includes(q),
    );
  }, [doc.steps, filter]);

  // Map step key to loop info (header)
  const loopMap = useMemo(() => {
    const m = new Map<string, string>();
    for (const loop of doc.loops ?? []) {
      const maxText = typeof loop.max === "number" ? loop.max : (loop.max?.cap ?? "?");
      m.set(loop.header, `↺ max ${maxText}`);
    }
    return m;
  }, [doc.loops]);

  // Map step key or index to errors/warnings
  const stepIssuesMap = useMemo(() => {
    const m = new Map<string, { errors: number; warnings: number }>();
    for (let i = 0; i < doc.steps.length; i++) {
      const s = doc.steps[i];
      if (!s) continue;
      const stepPathPrefix = `steps[${i}]`;
      let errors = 0;
      let warnings = 0;
      for (const iss of issues) {
        if (iss.path?.startsWith(stepPathPrefix) || iss.params?.step === s.key) {
          if (iss.severity === "error") errors++;
          else warnings++;
        }
      }
      if (errors > 0 || warnings > 0) {
        m.set(s.key, { errors, warnings });
      }
    }
    return m;
  }, [doc.steps, issues]);

  const atLimit = (doc.steps?.length ?? 0) >= stepLimit;

  const getKindLabel = (kind: FlowDocStep["kind"]) => {
    switch (kind) {
      case "tasks":
        return t(($) => $.flow_designer.step.kind.tasks);
      case "lead":
        return t(($) => $.flow_designer.step.kind.lead);
      case "manual":
        return t(($) => $.flow_designer.step.kind.manual);
      default:
        return String(kind);
    }
  };

  return (
    <div className={cn("flex flex-col h-full bg-card shrink-0", fullWidth ? "w-full" : "border-r border-border w-64 md:w-72")}>
      {/* Header & Search */}
      <div className="p-3 border-b border-border space-y-2">
        <div className="flex items-center justify-between">
          <span className="text-caption font-semibold text-foreground">{t(($) => $.flow_designer.outline.title)}</span>
          <span className="text-micro text-muted-foreground tabular-nums">
            {doc.steps.length} / {stepLimit}
          </span>
        </div>
        <div className="relative">
          <Search className="size-3.5 absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground" aria-hidden />
          <Input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder={t(($) => $.flow_designer.outline.search)}
            className="h-8 pl-8 text-caption"
          />
        </div>
      </div>

      {/* Step List */}
      <div className="flex-1 overflow-y-auto p-2 space-y-1">
        {filteredSteps.map((step) => {
          const isSelected = step.key === selectedStepKey;
          const loopInfo = loopMap.get(step.key);
          const issueInfo = stepIssuesMap.get(step.key);

          return (
            <button
              key={step.key}
              type="button"
              onClick={() => onSelectStep(step.key)}
              className={cn(
                "w-full text-left p-2 rounded-lg transition-colors flex flex-col gap-1 border",
                isSelected
                  ? "bg-accent border-accent-foreground/20 text-accent-foreground"
                  : "border-transparent hover:bg-muted text-foreground",
              )}
            >
              <div className="flex items-center justify-between gap-1 w-full">
                <span className="text-caption font-medium truncate flex-1">{step.title || step.key}</span>
                {issueInfo && (
                  <span className="flex items-center gap-1 shrink-0">
                    {issueInfo.errors > 0 ? (
                      <Badge variant="destructive" className="h-4 px-1 text-micro gap-0.5">
                        <AlertCircle className="size-2.5" aria-hidden />
                        <span>{issueInfo.errors}</span>
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="h-4 px-1 text-micro text-amber-600 border-amber-500/40 gap-0.5">
                        <AlertTriangle className="size-2.5" aria-hidden />
                        <span>{issueInfo.warnings}</span>
                      </Badge>
                    )}
                  </span>
                )}
              </div>

              <div className="flex items-center gap-1.5 flex-wrap text-micro text-muted-foreground">
                <Badge variant="secondary" className="h-4 px-1 text-micro font-normal">
                  {getKindLabel(step.kind)}
                </Badge>
                {step.kind === "tasks" && (
                  <span>
                    {t(($) => $.workflows.tasks_count, { count: step.tasks?.length ?? 0 })}
                  </span>
                )}
                {loopInfo && (
                  <Badge variant="outline" className="h-4 px-1 text-micro gap-0.5 text-blue-600 border-blue-500/30">
                    <RotateCcw className="size-2.5" aria-hidden />
                    <span>{loopInfo}</span>
                  </Badge>
                )}
              </div>
            </button>
          );
        })}

        {filteredSteps.length === 0 && (
          <div className="p-4 text-center text-caption text-muted-foreground">
            {t(($) => $.flow_designer.outline.empty)}
          </div>
        )}
      </div>

      {/* Add step: a blank step, or one of the templates */}
      {onAddStep && (
        <div className="p-2 border-t border-border">
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={atLimit}
                  className="w-full justify-center gap-1.5 text-caption h-8"
                >
                  <Plus className="size-3.5" aria-hidden />
                  <span>{t(($) => $.flow_designer.step.add)}</span>
                </Button>
              }
            />
            <DropdownMenuContent align="start" className="min-w-48">
              <DropdownMenuItem onClick={() => onAddStep("blank")}>
                {t(($) => $.flow_designer.templates.blank)}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              {STEP_TEMPLATE_IDS.map((id) => (
                <DropdownMenuItem key={id} onClick={() => onAddStep(id)}>
                  {t(($) => $.flow_designer.templates[id])}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      )}
    </div>
  );
}
