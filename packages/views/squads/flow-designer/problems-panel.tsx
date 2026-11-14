"use client";

import { useMemo } from "react";
import type { FlowIssue } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { AlertCircle, AlertTriangle, CheckCircle2 } from "@multica/ui/icons";
import { useT } from "../../i18n";

export interface ProblemsPanelProps {
  issues: FlowIssue[];
  onSelectPath?: (path?: string) => void;
}

export function ProblemsPanel({ issues, onSelectPath }: ProblemsPanelProps) {
  const { t } = useT("squads");

  const errors = useMemo(() => issues.filter((i) => i.severity === "error"), [issues]);
  const warnings = useMemo(() => issues.filter((i) => i.severity === "warning"), [issues]);

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

      <ul className="divide-y divide-border/60 max-h-60 overflow-y-auto">
        {issues.map((issue, idx) => {
          const isError = issue.severity === "error";
          return (
            <li
              key={idx}
              onClick={() => onSelectPath?.(issue.path)}
              className="py-2 flex items-start gap-2 cursor-pointer hover:bg-muted/40 px-1.5 rounded transition-colors"
            >
              {isError ? (
                <AlertCircle className="size-4 text-destructive shrink-0 mt-0.5" aria-hidden />
              ) : (
                <AlertTriangle className="size-4 text-amber-500 shrink-0 mt-0.5" aria-hidden />
              )}
              <div className="flex-1 space-y-0.5">
                <div className="flex items-center gap-1.5 flex-wrap">
                  <span className="font-mono text-micro font-medium text-foreground">
                    {issue.code}
                  </span>
                  {issue.path && (
                    <span className="font-mono text-micro text-muted-foreground">
                      ({issue.path})
                    </span>
                  )}
                  {issue.ack && (
                    <Badge variant="outline" className="text-micro text-amber-600 font-normal">
                      {t(($) => $.flow_designer.problems.requires_ack)}
                    </Badge>
                  )}
                </div>
                <p className="text-caption text-muted-foreground">{issue.message}</p>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
