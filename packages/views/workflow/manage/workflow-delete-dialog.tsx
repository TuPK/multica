"use client";

import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { AlertTriangle } from "@multica/ui/icons";
import { useT } from "../../i18n";
import type { IssueWorkflow } from "@multica/core/types";
import type { DeleteBlocker } from "../workflow-data";

export interface WorkflowDeleteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workflow: IssueWorkflow | null;
  /** Why the workflow cannot be deleted. Any entry blocks the delete. */
  blockers?: DeleteBlocker[];
  onConfirmDelete: () => void;
  isDeleting?: boolean;
}

export function WorkflowDeleteDialog({
  open,
  onOpenChange,
  workflow,
  blockers = [],
  onConfirmDelete,
  isDeleting,
}: WorkflowDeleteDialogProps) {
  const { t } = useT("workflow");
  if (!workflow) return null;

  const isBlocked = blockers.length > 0;

  // Unknown codes fall back to the default message; the server's own text is
  // English and would not match the locale.
  const blockerMessage = (blocker: DeleteBlocker): string => {
    switch (blocker.code) {
      case "workflow_in_use":
        return t(($) => $.manage.delete_dialog.blocked_projects, { count: blocker.count });
      case "workflow_is_squad_active":
        return t(($) => $.manage.delete_dialog.blocked_squads, { count: blocker.count });
      default:
        return t(($) => $.manage.delete_dialog.blocked_default);
    }
  };

  if (isBlocked) {
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-2 text-destructive">
              <AlertTriangle className="size-5 shrink-0" />
              <DialogTitle>{t(($) => $.manage.delete_dialog.blocked_title)}</DialogTitle>
            </div>
            <DialogDescription className="text-body text-foreground pt-2">
              <span className="font-medium text-foreground">{workflow.name}</span>
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-2 py-2 text-body-sm text-muted-foreground">
            {blockers.map((blocker) => (
              <p key={blocker.code} className="rounded-md bg-muted/60 p-2.5 text-foreground">
                {blockerMessage(blocker)}
              </p>
            ))}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t(($) => $.manage.delete_dialog.cancel)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t(($) => $.manage.delete_dialog.title)}</DialogTitle>
          <DialogDescription>
            {t(($) => $.manage.delete_dialog.warning)}
          </DialogDescription>
        </DialogHeader>

        <div className="py-2 text-body font-medium text-foreground">
          {workflow.name}
        </div>

        <DialogFooter className="gap-2 sm:gap-0">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={isDeleting}
          >
            {t(($) => $.manage.delete_dialog.cancel)}
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={onConfirmDelete}
            disabled={isDeleting}
          >
            {t(($) => $.manage.delete_dialog.confirm)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
