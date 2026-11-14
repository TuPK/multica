"use client";

import { useState } from "react";
import { toast } from "sonner";
import { clientErrorMessage } from "@multica/core/api";
import { useCreateIssue } from "@multica/core/issues/mutations";
import { useWorkspacePaths } from "@multica/core/paths";
import type { IssueWorkflow, Squad } from "@multica/core/types";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Loader2 } from "@multica/ui/icons";
import { useNavigation } from "../../navigation";
import { useT } from "../../i18n";

export interface TrialRunDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  squad: Squad;
  /** The version to rehearse. It need not be the one the squad runs. */
  workflow: IssueWorkflow;
}

/** Whether a version can be rehearsed: a published workflow of schema 2. */
export function canTrialRun(workflow: IssueWorkflow): boolean {
  return (workflow.flow_config?.version ?? 0) >= 2 && workflow.published_at != null;
}

/**
 * Starts a trial run of one version (spec 16.5): a new request given to the squad, pinned to that
 * version and marked as a rehearsal. The user types the title of the sample request.
 */
export function TrialRunDialog({ open, onOpenChange, squad, workflow }: TrialRunDialogProps) {
  const { t } = useT("squads");
  const nav = useNavigation();
  const paths = useWorkspacePaths();
  const createMut = useCreateIssue();
  const [title, setTitle] = useState("");
  const trimmed = title.trim();

  const start = async () => {
    if (!trimmed) return;
    try {
      const issue = await createMut.mutateAsync({
        title: trimmed,
        assignee_type: "squad",
        assignee_id: squad.id,
        flow_workflow_id: workflow.id,
        flow_trial: true,
      });
      setTitle("");
      onOpenChange(false);
      nav.push(paths.issueDetail(issue.id));
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow_designer.trial.failed));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md space-y-4 p-6">
        <DialogHeader className="space-y-1">
          <DialogTitle>{t(($) => $.flow_designer.trial.dialog_title)}</DialogTitle>
          <DialogDescription>{t(($) => $.flow_designer.trial.description)}</DialogDescription>
        </DialogHeader>

        <label className="block space-y-2">
          <span className="block text-caption font-medium text-foreground">{t(($) => $.flow_designer.trial.title_label)}</span>
          <Input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                void start();
              }
            }}
            maxLength={200}
            className="h-8 text-caption"
            autoFocus
          />
        </label>

        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            {t(($) => $.flow_designer.actions.cancel)}
          </Button>
          <Button type="button" disabled={!trimmed || createMut.isPending} onClick={() => void start()} className="gap-1.5">
            {createMut.isPending && <Loader2 className="size-3.5 animate-spin" aria-hidden />}
            <span>{t(($) => $.flow_designer.trial.start)}</span>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
