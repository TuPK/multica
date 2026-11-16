"use client";

import { useMemo } from "react";
import type { FlowDoc, FlowIssue } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { AlertCircle, AlertTriangle, CheckCircle2 } from "@multica/ui/icons";
import { useT } from "../../i18n";

export interface ProblemsPanelProps {
  issues: FlowIssue[];
  /** The document the issues were found in: it tells which step an issue belongs to. */
  doc?: FlowDoc;
  /** An issue was clicked; `stepKey` is the step it belongs to, or null for one that belongs to none. */
  onSelectIssue?: (issue: FlowIssue, stepKey: string | null) => void;
}

/**
 * The step an issue is about: the one its `params.step` names, else the one its path points into. The server writes
 * `steps[<key>]`, and `steps[<index>]` for a step whose key is not valid.
 */
export function stepKeyOfIssue(issue: FlowIssue, doc?: FlowDoc): string | null {
  if (!doc) return null;
  const named = issue.params?.step;
  if (typeof named === "string" && doc.steps.some((s) => s.key === named)) return named;
  const token = /^steps\[([^\]]+)\]/.exec(issue.path ?? "")?.[1];
  if (token === undefined) return null;
  if (doc.steps.some((s) => s.key === token)) return token;
  return /^\d+$/.test(token) ? (doc.steps[Number(token)]?.key ?? null) : null;
}

export function ProblemsPanel({ issues, doc, onSelectIssue }: ProblemsPanelProps) {
  const { t } = useT("squads");

  const errors = useMemo(() => issues.filter((i) => i.severity === "error"), [issues]);
  const warnings = useMemo(() => issues.filter((i) => i.severity === "warning"), [issues]);

  // The issues of no step first, then one group for each step that has some, in the order of the steps.
  const { loose, groups } = useMemo(() => {
    const byStep = new Map<string, FlowIssue[]>();
    const rest: FlowIssue[] = [];
    for (const issue of issues) {
      const key = stepKeyOfIssue(issue, doc);
      if (key === null) rest.push(issue);
      else byStep.set(key, [...(byStep.get(key) ?? []), issue]);
    }
    return {
      loose: rest,
      groups: (doc?.steps ?? []).filter((s) => byStep.has(s.key)).map((s) => ({ step: s, issues: byStep.get(s.key)! })),
    };
  }, [issues, doc]);

  const renderIssue = (issue: FlowIssue, idx: number) => {
    const isError = issue.severity === "error";
    return (
      <li
        key={idx}
        onClick={() => onSelectIssue?.(issue, stepKeyOfIssue(issue, doc))}
        className="py-2 flex items-start gap-2 cursor-pointer hover:bg-muted/40 px-1.5 rounded transition-colors"
      >
        {isError ? (
          <AlertCircle className="size-4 text-destructive shrink-0 mt-0.5" aria-hidden />
        ) : (
          <AlertTriangle className="size-4 text-amber-500 shrink-0 mt-0.5" aria-hidden />
        )}
        <div className="min-w-0 flex-1 space-y-0.5">
          <div className="flex items-center gap-1.5 flex-wrap">
            <span className="font-mono text-micro font-medium text-foreground">{issue.code}</span>
            {issue.path && <span className="font-mono text-micro text-muted-foreground [overflow-wrap:anywhere]">({issue.path})</span>}
            {issue.ack && (
              <Badge variant="outline" className="text-micro text-amber-600 font-normal">
                {t(($) => $.flow_designer.problems.requires_ack)}
              </Badge>
            )}
          </div>
          <p className="text-caption text-muted-foreground [overflow-wrap:anywhere]">{issue.message}</p>
        </div>
      </li>
    );
  };

  if (issues.length === 0) {
    return (
      <div className="flex items-center gap-2 p-3 rounded-lg border border-border bg-card text-caption text-muted-foreground">
        <CheckCircle2 className="size-4 text-emerald-500 shrink-0" aria-hidden />
        <span>{t(($) => $.flow_designer.problems.none)}</span>
      </div>
    );
  }

  return (
    <div className="space-y-3 p-3 rounded-lg border border-border bg-card text-caption">
      <div className="flex items-center justify-between">
        <span className="font-semibold text-foreground">{t(($) => $.flow_designer.problems.title)}</span>
        <div className="flex gap-1.5">
          {errors.length > 0 && (
            <Badge variant="destructive" className="text-micro gap-1">
              <AlertCircle className="size-3" aria-hidden />
              <span>{t(($) => $.flow_designer.problems.errors, { count: errors.length })}</span>
            </Badge>
          )}
          {warnings.length > 0 && (
            <Badge variant="outline" className="text-micro text-amber-600 border-amber-500/40 gap-1">
              <AlertTriangle className="size-3" aria-hidden />
              <span>{t(($) => $.flow_designer.problems.warnings, { count: warnings.length })}</span>
            </Badge>
          )}
        </div>
      </div>

      <div className="space-y-2">
        {loose.length > 0 && <ul className="divide-y divide-border/60">{loose.map(renderIssue)}</ul>}
        {groups.map(({ step, issues: stepIssues }) => (
          <div key={step.key} role="group" aria-labelledby={`problems-${step.key}`} className="space-y-0.5">
            <h4 id={`problems-${step.key}`} className="px-1.5 text-micro font-semibold text-muted-foreground">
              {step.title || step.key}
            </h4>
            <ul className="divide-y divide-border/60">{stepIssues.map(renderIssue)}</ul>
          </div>
        ))}
      </div>
    </div>
  );
}
