"use client";

import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import { AlertCircle, AlertTriangle, CheckCircle2 } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { useValidationMessage } from "../use-validation-message";
import type { ValidationIssue } from "../types";

export interface ValidationListProps {
  issues: ValidationIssue[];
  onSelectStep?: (stepKey: string) => void;
}

export function ValidationList({ issues, onSelectStep }: ValidationListProps) {
  const { t } = useT("workflow");
  const renderMessage = useValidationMessage();

  const errors = issues.filter((i) => i.severity === "error");
  const warnings = issues.filter((i) => i.severity === "warning");
  const isValid = issues.length === 0;

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant={errors.length > 0 ? "destructive" : warnings.length > 0 ? "outline" : "ghost"}
            size="sm"
            className="gap-1.5 h-8 text-caption font-medium"
          >
            {isValid ? (
              <>
                <CheckCircle2 className="size-4 text-brand" />
                <span className="hidden sm:inline">{t(($) => $.designer.validation.all_valid)}</span>
              </>
            ) : errors.length > 0 ? (
              <>
                <AlertCircle className="size-4 shrink-0" />
                <span>
                  {errors.length} {t(($) => $.designer.validation.errors)}
                </span>
              </>
            ) : (
              <>
                <AlertTriangle className="size-4 text-warning shrink-0" />
                <span>
                  {warnings.length} {t(($) => $.designer.validation.warnings)}
                </span>
              </>
            )}
          </Button>
        }
      />
      <PopoverContent align="end" className="w-80 sm:w-96 p-4 max-h-96 overflow-y-auto space-y-3">
        <div className="flex items-center justify-between border-b border-border pb-2">
          <span className="text-body font-semibold text-foreground">
            {t(($) => $.designer.validation.title)}
          </span>
          <div className="flex items-center gap-1.5">
            {errors.length > 0 && (
              <Badge variant="destructive" className="text-micro">
                {errors.length} {t(($) => $.designer.validation.errors)}
              </Badge>
            )}
            {warnings.length > 0 && (
              <Badge variant="outline" className="text-micro text-warning border-warning/40">
                {warnings.length} {t(($) => $.designer.validation.warnings)}
              </Badge>
            )}
          </div>
        </div>

        {isValid ? (
          <div className="flex items-center gap-2 py-4 text-brand text-body-sm font-medium">
            <CheckCircle2 className="size-5 shrink-0" />
            <span>{t(($) => $.designer.validation.all_valid)}</span>
          </div>
        ) : (
          <div className="space-y-2">
            {issues.map((issue) => (
              <div
                key={issue.id}
                role={issue.step_key ? "button" : undefined}
                tabIndex={issue.step_key ? 0 : undefined}
                onClick={() => issue.step_key && onSelectStep?.(issue.step_key)}
                onKeyDown={(e) => {
                  if (issue.step_key && (e.key === "Enter" || e.key === " ")) {
                    e.preventDefault();
                    onSelectStep?.(issue.step_key);
                  }
                }}
                className={`flex items-start gap-2.5 p-2 rounded-lg border text-caption text-left transition-colors ${
                  issue.severity === "error"
                    ? "border-destructive/30 bg-destructive/5 text-destructive"
                    : "border-warning/30 bg-warning/5 text-foreground"
                } ${issue.step_key ? "cursor-pointer hover:bg-accent/40" : ""}`}
              >
                {issue.severity === "error" ? (
                  <AlertCircle className="size-4 shrink-0 mt-0.5" />
                ) : (
                  <AlertTriangle className="size-4 text-warning shrink-0 mt-0.5" />
                )}
                <div className="space-y-0.5 flex-1 min-w-0">
                  <p className="leading-snug">{renderMessage(issue.message)}</p>
                  {issue.step_key && (
                    <span className="text-micro font-mono text-muted-foreground">
                      {t(($) => $.designer.validation.step_label, { key: issue.step_key })}
                    </span>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </PopoverContent>
    </Popover>
  );
}
