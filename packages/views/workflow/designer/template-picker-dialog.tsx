"use client";

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { AlertCircle, ArrowRight, LayoutTemplate, Loader2 } from "@multica/ui/icons";
import { useIssueWorkflowTemplates } from "@multica/core/issue-workflows";
import type {
  IssueWorkflowTemplate,
  IssueWorkflowTemplateId,
} from "@multica/core/types";
import { useStatusLabel } from "../../issues/utils/status-label";
import { useT } from "../../i18n";

export interface TemplatePickerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  wsId: string;
  onSelectTemplate: (template: IssueWorkflowTemplate, name: string, description: string) => void;
}

function resolveTemplateI18n(
  t: ReturnType<typeof useT<"workflow">>["t"],
  id: IssueWorkflowTemplateId,
): { name: string; description: string } {
  switch (id) {
    case "delivery":
      return {
        name: t(($) => $.templates.delivery.name),
        description: t(($) => $.templates.delivery.description),
      };
    case "content":
      return {
        name: t(($) => $.templates.content.name),
        description: t(($) => $.templates.content.description),
      };
    case "community":
      return {
        name: t(($) => $.templates.community.name),
        description: t(($) => $.templates.community.description),
      };
    case "sdlc_auto":
      return {
        name: t(($) => $.templates.sdlc_auto.name),
        description: t(($) => $.templates.sdlc_auto.description),
      };
    case "bmad_planning_auto":
      return {
        name: t(($) => $.templates.bmad_planning_auto.name),
        description: t(($) => $.templates.bmad_planning_auto.description),
      };
    case "sales_loop":
      return {
        name: t(($) => $.templates.sales_loop.name),
        description: t(($) => $.templates.sales_loop.description),
      };
    case "quick_fix":
      return {
        name: t(($) => $.templates.quick_fix.name),
        description: t(($) => $.templates.quick_fix.description),
      };
    default: {
      const exhaustiveCheck: never = id;
      return { name: String(exhaustiveCheck), description: "" };
    }
  }
}

export function TemplatePickerDialog({
  open,
  onOpenChange,
  wsId,
  onSelectTemplate,
}: TemplatePickerDialogProps) {
  const { t } = useT("workflow");
  // Unknown template ids are already dropped by the core response schema.
  const { templates, isLoading, isError, refetch } = useIssueWorkflowTemplates(wsId);
  const getStatusLabel = useStatusLabel(wsId);

  const handlePickTemplate = (template: IssueWorkflowTemplate) => {
    const { name, description } = resolveTemplateI18n(t, template.id);
    onSelectTemplate(template, name, description);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl p-6">
        <DialogHeader className="pb-2">
          <DialogTitle>{t(($) => $.designer.template_dialog.title)}</DialogTitle>
        </DialogHeader>

        {/* Template List / States */}
        <div className="flex flex-col gap-3 max-h-[70vh] overflow-y-auto py-1 pr-1">
          {isLoading ? (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground gap-2">
              <Loader2 className="size-5 animate-spin" />
              <span className="text-caption">
                {t(($) => $.designer.template_dialog.loading)}
              </span>
            </div>
          ) : isError ? (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground gap-3">
              <AlertCircle className="size-5 text-destructive" />
              <span className="text-caption text-center">
                {t(($) => $.designer.template_dialog.load_failed)}
              </span>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => refetch()}
              >
                {t(($) => $.manage.action_failed)}
              </Button>
            </div>
          ) : templates.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
              <span className="text-caption">
                {t(($) => $.designer.template_dialog.empty_title)}
              </span>
            </div>
          ) : (
            templates.map((tpl) => {
              const { name, description } = resolveTemplateI18n(t, tpl.id);
              const stepCount = tpl.steps?.length ?? 0;

              return (
                <div
                  key={tpl.id}
                  onClick={() => handlePickTemplate(tpl)}
                  className="flex flex-col p-4 rounded-lg border border-border bg-card hover:border-primary/50 transition-colors gap-3 cursor-pointer group"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex items-start gap-3 min-w-0">
                      <div className="mt-0.5 p-2 rounded-md bg-muted text-foreground shrink-0">
                        <LayoutTemplate className="size-4" />
                      </div>
                      <div className="min-w-0 flex flex-col">
                        <span className="text-body font-semibold text-foreground">
                          {name}
                        </span>
                        {description && (
                          <span className="text-caption text-muted-foreground mt-0.5">
                            {description}
                          </span>
                        )}
                      </div>
                    </div>

                    <Button
                      type="button"
                      size="sm"
                      onClick={() => handlePickTemplate(tpl)}
                      className="shrink-0"
                    >
                      {t(($) => $.designer.start_points.template)}
                    </Button>
                  </div>

                  {/* Steps preview list, with "will be created on save" indicator */}
                  <div className="flex flex-wrap items-center gap-1.5 pt-2 border-t border-border/60">
                    <span className="text-micro font-medium text-muted-foreground mr-1">
                      {t(($) => $.manage.steps_count, { count: stepCount })}:
                    </span>
                    {tpl.steps.map((step, idx) => {
                      const isNew = "new_status" in step && Boolean(step.new_status);
                      const statusName =
                        "new_status" in step && step.new_status
                          ? step.new_status.name
                          : "status_key" in step
                            ? getStatusLabel(step.status_key)
                            : "";

                      return (
                        <div key={idx} className="flex items-center gap-1.5">
                          <span className="inline-flex items-center gap-1.5 rounded-md border border-border bg-muted/40 px-2 py-0.5 text-caption">
                            <span className="font-medium text-foreground">{statusName}</span>
                            {isNew && (
                              <span
                                data-testid="new-status-badge"
                                className="rounded-xs bg-brand/10 border border-brand/20 px-1 py-0.5 text-micro text-brand font-medium"
                              >
                                {t(($) => $.designer.template_dialog.will_be_created)}
                              </span>
                            )}
                            {step.role_hint && (
                              <span className="rounded-xs bg-muted px-1 text-micro text-muted-foreground">
                                {step.role_hint}
                              </span>
                            )}
                          </span>
                          {idx < tpl.steps.length - 1 && (
                            <ArrowRight className="size-3 text-faint-foreground shrink-0" />
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>
              );
            })
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
