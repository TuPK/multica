"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  Play,
  Unlink,
  Workflow,
  XCircle,
} from "@multica/ui/icons";
import { clientErrorMessage } from "@multica/core/api";
import { useT } from "../i18n";
import { issueFlowKeys, useDetachFlow, useExitIssueFlow, useResumeFlow } from "@multica/core/issue-workflows";
import { useUpdateIssue } from "@multica/core/issues/mutations";
import type { IssueFlowState, IssueFlowTask, ResumeFlowRequest } from "@multica/core/types";
import { issueUpdateErrorMessage } from "../issues/utils/issue-update-error";
import { cn } from "@multica/ui/lib/utils";

export interface IssueFlowStatePanelProps {
  wsId: string;
  issueId: string;
  flow: IssueFlowState;
  canManage: boolean;
}

export function IssueFlowStatePanel({
  wsId,
  issueId,
  flow,
  canManage,
}: IssueFlowStatePanelProps) {
  const { t } = useT("issues");
  const { t: tSquads } = useT("squads");
  const [confirmDetachOpen, setConfirmDetachOpen] = useState(false);

  const resumeMut = useResumeFlow(wsId, issueId);
  const detachMut = useDetachFlow(wsId, issueId);
  const exitMut = useExitIssueFlow(wsId, issueId);
  const updateIssue = useUpdateIssue();
  const queryClient = useQueryClient();

  // A verdict is the same status change a person makes on the task: done, with
  // the outcome. The server's gate guard decides whether it is accepted.
  const handleVerdict = async (task: IssueFlowTask, outcome: "approved" | "rejected") => {
    try {
      await updateIssue.mutateAsync({ id: task.issue_id, status: "done", outcome });
    } catch (err) {
      toast.error(issueUpdateErrorMessage(err, t, t(($) => $.flow.error)));
    } finally {
      queryClient.invalidateQueries({ queryKey: issueFlowKeys.detail(wsId, issueId) });
    }
  };

  const handleResume = async (data: ResumeFlowRequest = {}) => {
    try {
      await resumeMut.mutateAsync(data);
      toast.success(t(($) => $.flow.resume));
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow.error));
    }
  };

  const handleExit = async (edgeId: string) => {
    try {
      await exitMut.mutateAsync(edgeId);
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? tSquads(($) => $.flow_designer.request.exit_failed));
    }
  };

  const handleConfirmDetach = async () => {
    try {
      await detachMut.mutateAsync({ cancel_tasks: false });
      setConfirmDetachOpen(false);
      toast.success(t(($) => $.flow.detach));
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow.error));
    }
  };

  const draftStep = flow.rework_step;
  // A work task a person cancelled is reopened by a resume, because its files are part of the step,
  // unless the resume says to go on without it.
  const hasCancelledWork = flow.tasks.some((task) => task.kind === "work" && task.status === "cancelled");

  const renderOutcomeBadge = (outcome: string | null) => {
    switch (outcome) {
      case "approved":
        return (
          <Badge variant="secondary" className="gap-1 text-micro text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 border-emerald-500/20">
            <CheckCircle2 className="size-3" aria-hidden />
            <span>{t(($) => $.flow.outcome.approved)}</span>
          </Badge>
        );
      case "rejected":
        return (
          <Badge variant="destructive" className="gap-1 text-micro">
            <XCircle className="size-3" aria-hidden />
            <span>{t(($) => $.flow.outcome.rejected)}</span>
          </Badge>
        );
      case "blocked":
        return (
          <Badge variant="outline" className="gap-1 text-micro text-amber-600 dark:text-amber-400 border-amber-500/30">
            <AlertTriangle className="size-3" aria-hidden />
            <span>{t(($) => $.flow.outcome.blocked)}</span>
          </Badge>
        );
      case "done":
        return (
          <Badge variant="secondary" className="gap-1 text-micro">
            <CheckCircle2 className="size-3" aria-hidden />
            <span>{t(($) => $.flow.outcome.done)}</span>
          </Badge>
        );
      default:
        return (
          <Badge variant="outline" className="text-micro text-muted-foreground">
            {t(($) => $.flow.outcome.none)}
          </Badge>
        );
    }
  };

  const renderStopBanner = () => {
    if (flow.stop === null || flow.stop === undefined) return null;
    let label = "";
    if (flow.stop.legacy_no === 1) {
      label = t(($) => $.flow.stop["1"]);
    } else if (flow.stop.legacy_no === 3) {
      label = t(($) => $.flow.stop["3"]);
    } else {
      label = t(($) => $.flow.stop.stalled);
    }

    return (
      <div className="flex items-center gap-2 p-3 rounded-lg border border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-400 text-caption font-medium">
        <AlertCircle className="size-4 shrink-0" aria-hidden />
        <span className="flex-1">{label}</span>
      </div>
    );
  };

  return (
    <div className="rounded-xl border border-border bg-card p-4 space-y-4 shadow-xs">
      {/* Header: Title, Step, Round, Workflow Name */}
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <div className="flex items-center gap-2 flex-wrap">
          <Workflow className="size-4 text-muted-foreground shrink-0" aria-hidden />
          <span className="text-title font-semibold text-foreground">
            {t(($) => $.flow.panel_title)}
          </span>
          <span className="text-caption text-muted-foreground">·</span>
          <span className="text-caption font-medium text-foreground">
            {flow.workflow_name}
          </span>
        </div>

        <div className="flex items-center gap-2">
          {flow.loops && Object.keys(flow.loops).length > 0 && (
            <Badge variant="outline" className="text-caption text-blue-600 border-blue-500/30">
              {Object.entries(flow.loops)
                .map(([_, count]) => `↺ ${count}`)
                .join(", ")}
            </Badge>
          )}
          <Badge variant="secondary" className="text-caption font-medium">
            {flow.step}
          </Badge>
          <Badge variant="outline" className="text-caption">
            {t(($) => $.flow.round, { round: flow.rework })}
          </Badge>
        </div>
      </div>

      {/* Stop banner if stopped */}
      {renderStopBanner()}

      {/* One block per task: the sidebar is too narrow for a table with a verdict column. */}
      {flow.tasks.length > 0 ? (
        <ul className="divide-y divide-border/40 rounded-lg border border-border/60 text-caption">
          {flow.tasks.map((task) => (
            <li key={task.issue_id} className="space-y-1.5 px-2 py-2">
              <div className="flex items-baseline gap-2">
                <span className="font-medium text-foreground whitespace-nowrap">{task.identifier}</span>
                <span className="min-w-0 flex-1 truncate text-foreground">{task.task}</span>
                <span className="text-muted-foreground tabular-nums whitespace-nowrap">
                  {t(($) => $.flow.round, { round: task.round })}
                </span>
              </div>
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span className="text-muted-foreground">{task.status}</span>
                {task.kind === "gate" && task.closed_at === null ? (
                  (!flow.viewer || flow.viewer.closable_task_ids?.includes(task.issue_id) || canManage) ? (
                    <div className="flex items-center gap-1 flex-wrap">
                      <Button
                        type="button"
                        variant="outline"
                        size="xs"
                        disabled={updateIssue.isPending}
                        onClick={() => void handleVerdict(task, "approved")}
                      >
                        {t(($) => $.flow.verdict.approve)}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="xs"
                        disabled={updateIssue.isPending}
                        onClick={() => void handleVerdict(task, "rejected")}
                      >
                        {t(($) => $.flow.verdict.reject)}
                      </Button>
                      {canManage && (
                        <Button
                          type="button"
                          variant="ghost"
                          size="xs"
                          disabled={updateIssue.isPending}
                          onClick={() => void handleVerdict(task, "approved")}
                          className="text-micro text-muted-foreground hover:text-foreground"
                        >
                          {tSquads(($) => $.flow_designer.close_on_behalf)}
                        </Button>
                      )}
                    </div>
                  ) : null
                ) : (
                  renderOutcomeBadge(task.outcome ?? (task.closed_at !== null ? "done" : null))
                )}
                {task.closed_by && task.closed_by.on_behalf && (
                  <Badge variant="secondary" className="text-micro font-normal text-muted-foreground">
                    {tSquads(($) => $.flow_designer.on_behalf_badge, { name: task.closed_by?.name || task.closed_by?.id || "" })}
                  </Badge>
                )}
              </div>
            </li>
          ))}
        </ul>
      ) : null}

      {/* Manual Exits */}
      {flow.next && flow.next.some((n) => n.trigger === "manual") && (
        <div className="flex items-center gap-2 flex-wrap pt-2 border-t border-border/40">
          <span className="text-micro text-muted-foreground font-medium">{tSquads(($) => $.flow_designer.request.exit)}</span>
          {flow.next
            .filter((n) => n.trigger === "manual")
            .map((edge) => (
              <Button
                key={edge.id}
                type="button"
                variant="outline"
                size="xs"
                className="h-7 text-caption"
                disabled={exitMut.isPending}
                onClick={() => void handleExit(edge.id)}
              >
                {edge.label || edge.id}
              </Button>
            ))}
        </div>
      )}

      {/* Path trail */}
      {flow.trail && flow.trail.length > 0 && (
        <div className="space-y-1.5 pt-2 border-t border-border/40">
          <span className="text-caption font-semibold text-foreground">
            {tSquads(($) => $.flow_designer.request.path)}
          </span>
          <ul className="divide-y divide-border/40 rounded-lg border border-border/60 text-micro">
            {flow.trail.map((entry, idx) => (
              <li key={idx} className="p-1.5 flex items-center justify-between text-muted-foreground">
                <span className="font-medium text-foreground">{entry.step}</span>
                {entry.edge && <span className="font-mono">{entry.edge}</span>}
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Admin Actions */}
      {canManage && (
        <div className="flex flex-wrap items-center justify-end gap-2 pt-2 border-t border-border/40">
          <div className="inline-flex">
            <Button
              variant="outline"
              size="sm"
              disabled={resumeMut.isPending}
              onClick={() => void handleResume()}
              className={cn("gap-1.5 text-caption h-8", (draftStep || hasCancelledWork) && "rounded-r-none")}
            >
              <Play className="size-3.5" aria-hidden />
              <span>{t(($) => $.flow.resume)}</span>
            </Button>
            {draftStep || hasCancelledWork ? (
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={resumeMut.isPending}
                      aria-label={t(($) => $.flow.choose_step)}
                      className="h-8 -ml-px rounded-l-none px-2"
                    />
                  }
                >
                  <ChevronDown className="size-3.5" aria-hidden />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-auto min-w-52">
                  {draftStep ? (
                    <DropdownMenuItem onClick={() => void handleResume({ to_step: draftStep })}>
                      {t(($) => $.flow.resume_to_draft)}
                    </DropdownMenuItem>
                  ) : null}
                  {hasCancelledWork ? (
                    <DropdownMenuItem onClick={() => void handleResume({ skip_cancelled: true })}>
                      {t(($) => $.flow.resume_skip_cancelled)}
                    </DropdownMenuItem>
                  ) : null}
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}
          </div>

          <Button
            variant="destructive"
            size="sm"
            onClick={() => setConfirmDetachOpen(true)}
            className="gap-1.5 text-caption h-8"
          >
            <Unlink className="size-3.5" aria-hidden />
            <span>{t(($) => $.flow.detach)}</span>
          </Button>
        </div>
      )}

      {/* Detach Confirmation Dialog */}
      <Dialog open={confirmDetachOpen} onOpenChange={setConfirmDetachOpen}>
        <DialogContent className="max-w-md p-6">
          <DialogHeader className="space-y-2">
            <DialogTitle>{t(($) => $.flow.detach_confirm)}</DialogTitle>
            <DialogDescription className="text-body-sm text-muted-foreground">
              {t(($) => $.flow.detach_confirm_desc)}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 pt-4">
            <Button
              variant="outline"
              size="sm"
              disabled={detachMut.isPending}
              onClick={() => setConfirmDetachOpen(false)}
            >
              {t(($) => $.flow.cancel)}
            </Button>
            <Button
              variant="destructive"
              size="sm"
              disabled={detachMut.isPending}
              onClick={() => void handleConfirmDetach()}
            >
              {t(($) => $.flow.detach_confirm_button)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
