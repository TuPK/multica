"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
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
  Play,
  Unlink,
  Workflow,
  XCircle,
} from "@multica/ui/icons";
import { clientErrorMessage } from "@multica/core/api";
import { useT } from "../i18n";
import { issueFlowKeys, useDetachFlow, useResumeFlow } from "@multica/core/issue-workflows";
import { useUpdateIssue } from "@multica/core/issues/mutations";
import type { IssueFlowState, IssueFlowTask } from "@multica/core/types";
import { issueUpdateErrorMessage } from "../issues/utils/issue-update-error";

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
  const [confirmDetachOpen, setConfirmDetachOpen] = useState(false);

  const resumeMut = useResumeFlow(wsId, issueId);
  const detachMut = useDetachFlow(wsId, issueId);
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

  const handleResume = async () => {
    try {
      await resumeMut.mutateAsync({});
      toast.success(t(($) => $.flow.resume));
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow.error));
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
    if (flow.stop === 1) {
      label = t(($) => $.flow.stop["1"]);
    } else if (flow.stop === 3) {
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

      {/* Tasks Table */}
      {flow.tasks.length > 0 ? (
        <div className="space-y-2">
          <div className="overflow-x-auto rounded-lg border border-border/60">
            <table className="w-full text-left text-caption">
              <thead className="bg-muted/40 text-muted-foreground border-b border-border/60">
                <tr>
                  <th className="py-2 px-2 font-medium whitespace-nowrap">{t(($) => $.flow.table_issue)}</th>
                  <th className="py-2 px-2 font-medium">{t(($) => $.flow.table_task)}</th>
                  <th className="py-2 px-2 font-medium whitespace-nowrap">{t(($) => $.flow.table_round)}</th>
                  <th className="py-2 px-2 font-medium whitespace-nowrap">{t(($) => $.flow.table_status)}</th>
                  <th className="py-2 px-2 font-medium whitespace-nowrap">{t(($) => $.flow.table_outcome)}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border/40">
                {flow.tasks.map((task) => (
                  <tr key={task.issue_id} className="hover:bg-muted/20">
                    <td className="py-2 px-2 font-medium text-foreground whitespace-nowrap">
                      {task.identifier}
                    </td>
                    <td className="py-2 px-2 text-foreground">
                      {task.task}
                    </td>
                    <td className="py-2 px-2 text-muted-foreground tabular-nums whitespace-nowrap">
                      {task.round}
                    </td>
                    <td className="py-2 px-2 text-muted-foreground whitespace-nowrap">
                      {task.status}
                    </td>
                    <td className="py-2 px-2 whitespace-nowrap">
                      {task.kind === "gate" && task.closed_at === null ? (
                        <div className="flex items-center gap-1">
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
                        </div>
                      ) : (
                        renderOutcomeBadge(task.outcome)
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}

      {/* Admin Actions */}
      {canManage && (
        <div className="flex items-center justify-end gap-2 pt-2 border-t border-border/40">
          <Button
            variant="outline"
            size="sm"
            disabled={resumeMut.isPending}
            onClick={() => void handleResume()}
            className="gap-1.5 text-caption h-8"
          >
            <Play className="size-3.5" aria-hidden />
            <span>{t(($) => $.flow.resume)}</span>
          </Button>

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
