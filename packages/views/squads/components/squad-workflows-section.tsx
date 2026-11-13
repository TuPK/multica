"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import {
  AlertCircle,
  CheckCircle2,
  ChevronRight,
  Loader2,
  RefreshCw,
  Workflow,
  Zap,
} from "@multica/ui/icons";
import type { IssueWorkflow, Squad } from "@multica/core/types";
import { clientErrorMessage } from "@multica/core/api";
import { useT } from "../../i18n";
import {
  squadWorkflowsOptions,
  useSetFlowAuto,
  useSetSquadActiveWorkflow,
} from "@multica/core/issue-workflows";
import { WorkflowGraphSheet } from "./workflow-graph-sheet";

export interface SquadWorkflowsSectionProps {
  wsId: string;
  squad: Squad;
  canManage: boolean;
}

export function SquadWorkflowsSection({
  wsId,
  squad,
  canManage,
}: SquadWorkflowsSectionProps) {
  const { t } = useT("squads");
  const [selectedWorkflow, setSelectedWorkflow] = useState<IssueWorkflow | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);

  const query = useQuery(squadWorkflowsOptions(wsId, squad.id));
  const setActiveMut = useSetSquadActiveWorkflow(wsId, squad.id);
  const setAutoMut = useSetFlowAuto(wsId);

  const handleOpenGraph = (wf: IssueWorkflow) => {
    setSelectedWorkflow(wf);
    setSheetOpen(true);
  };

  const handleSetActive = async (e: React.MouseEvent, wf: IssueWorkflow) => {
    e.stopPropagation();
    try {
      await setActiveMut.mutateAsync({ workflow_id: wf.id });
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.workflows.error));
    }
  };

  const handleToggleAuto = async (e: React.MouseEvent, wf: IssueWorkflow) => {
    e.stopPropagation();
    try {
      await setAutoMut.mutateAsync({ workflowId: wf.id, auto: !wf.flow_auto });
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.workflows.error));
    }
  };

  const workflows = query.data?.workflows ?? [];
  const activeId = query.data?.active_workflow_id ?? squad.active_workflow_id;

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-title font-semibold text-foreground">
          <Workflow className="size-4 text-muted-foreground" aria-hidden />
          <span>{t(($) => $.workflows.section_title)}</span>
        </h2>
      </div>

      {query.isPending ? (
        <div className="flex items-center justify-center p-8 text-muted-foreground">
          <Loader2 className="size-5 animate-spin" aria-hidden />
        </div>
      ) : query.isError ? (
        <div className="flex items-center justify-between rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-body text-destructive">
          <div className="flex items-center gap-2">
            <AlertCircle className="size-4 shrink-0" aria-hidden />
            <span>{t(($) => $.workflows.error)}</span>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void query.refetch()}
            className="gap-1.5"
          >
            <RefreshCw className="size-3.5" aria-hidden />
            <span>{t(($) => $.workflows.retry)}</span>
          </Button>
        </div>
      ) : workflows.length === 0 ? (
        <div className="rounded-xl border border-border/60 bg-muted/20 p-6 text-center text-body text-muted-foreground">
          {t(($) => $.workflows.empty)}
        </div>
      ) : (
        <div className="space-y-2">
          {workflows.map((wf) => {
            const isActive = wf.id === activeId;
            const totalTasks = wf.steps.reduce(
              (acc, step) => acc + (step.tasks?.length ?? 0),
              0,
            );

            return (
              <div
                key={wf.id}
                tabIndex={0}
                onClick={() => handleOpenGraph(wf)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    handleOpenGraph(wf);
                  }
                }}
                className="group flex flex-col sm:flex-row sm:items-center justify-between gap-3 rounded-xl border border-border bg-card p-4 hover:border-sidebar-accent hover:shadow-xs transition-all cursor-pointer text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <div className="flex flex-col space-y-1.5 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="font-medium text-body text-foreground truncate">
                      {wf.name}
                    </span>
                    {isActive && (
                      <Badge variant="default" className="text-micro font-medium gap-1 shrink-0">
                        <CheckCircle2 className="size-3" aria-hidden />
                        <span>{t(($) => $.workflows.badge_active)}</span>
                      </Badge>
                    )}
                    {wf.template_ref && (
                      <Badge variant="secondary" className="text-micro shrink-0">
                        {wf.template_ref}
                      </Badge>
                    )}
                  </div>

                  <div className="flex items-center gap-2 text-caption text-muted-foreground flex-wrap">
                    <span>
                      {t(($) => $.workflows.steps_count, { count: wf.steps.length })}
                    </span>
                    <span>·</span>
                    <span>
                      {t(($) => $.workflows.tasks_count, { count: totalTasks })}
                    </span>
                    {typeof wf.open_flow_requests === "number" && wf.open_flow_requests > 0 && (
                      <>
                        <span>·</span>
                        <span className="text-amber-600 dark:text-amber-400 font-medium">
                          {t(($) => $.workflows.open_requests, { count: wf.open_flow_requests })}
                        </span>
                      </>
                    )}
                  </div>
                </div>

                {/* Right side: Admin actions & Chevron */}
                <div className="flex items-center gap-2 shrink-0 self-end sm:self-center">
                  {canManage && (
                    <>
                      {/* Set Active */}
                      {!isActive && (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={setActiveMut.isPending}
                          onClick={(e) => void handleSetActive(e, wf)}
                          className="text-caption h-8"
                        >
                          {t(($) => $.workflows.set_active)}
                        </Button>
                      )}

                      {/* Auto Toggle (for flow workflows) */}
                      {wf.flow_config != null && (
                        <Button
                          variant={wf.flow_auto ? "secondary" : "ghost"}
                          size="sm"
                          disabled={setAutoMut.isPending}
                          onClick={(e) => void handleToggleAuto(e, wf)}
                          className="gap-1.5 text-caption h-8"
                        >
                          <Zap className="size-3.5" aria-hidden />
                          <span>
                            {wf.flow_auto
                              ? t(($) => $.workflows.auto_on)
                              : t(($) => $.workflows.auto_off)}
                          </span>
                        </Button>
                      )}
                    </>
                  )}

                  <ChevronRight className="size-4 text-muted-foreground group-hover:translate-x-0.5 transition-transform" aria-hidden />
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Graph Sheet */}
      <WorkflowGraphSheet
        workflow={selectedWorkflow}
        open={sheetOpen}
        onOpenChange={setSheetOpen}
      />
    </section>
  );
}
