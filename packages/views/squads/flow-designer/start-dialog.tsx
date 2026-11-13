"use client";

import { useState } from "react";
import { toast } from "sonner";
import { clientErrorMessage } from "@multica/core/api";
import { useCreateFlowDraft } from "@multica/core/flow-drafts";
import { useWorkspacePaths } from "@multica/core/paths";
import type { IssueWorkflow, Squad } from "@multica/core/types";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Copy, FileText, Loader2, Upload } from "@multica/ui/icons";
import { useNavigation } from "../../navigation";
import { useT } from "../../i18n";
import { cutDraftName } from "./draft-name";

export interface StartDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  squad: Squad;
  /** The workflows of the squad: a Kit Việt workflow among them can be copied. */
  workflows: IssueWorkflow[];
  onOpenImport?: () => void;
}

/**
 * The Kit Việt workflow a draft can be copied from: the one the squad runs if a catalog flow made
 * it, else the newest catalog flow of the squad. A squad with none has nothing to copy.
 */
export function kitVietSource(workflows: IssueWorkflow[], activeId: string | null | undefined): IssueWorkflow | null {
  const catalog = workflows.filter((w) => w.template_ref && w.flow_config != null);
  return catalog.find((w) => w.id === activeId) ?? catalog[0] ?? null;
}

export function StartDialog({ open, onOpenChange, squad, workflows, onOpenImport }: StartDialogProps) {
  const { t } = useT("squads");
  const nav = useNavigation();
  const paths = useWorkspacePaths();
  const createMut = useCreateFlowDraft();
  const [name, setName] = useState("");

  const source = kitVietSource(workflows, squad.active_workflow_id);

  const create = async (from: { kind: "blank" } | { kind: "workflow"; workflow_id: string }) => {
    try {
      const typed = name.trim();
      // A blank draft needs a name. A copy takes its name from the workflow it copies unless one was typed.
      const data =
        from.kind === "blank"
          ? { name: cutDraftName(typed || squad.name), from }
          : { ...(typed ? { name: cutDraftName(typed) } : {}), from };
      const draft = await createMut.mutateAsync({ squadId: squad.id, data });
      onOpenChange(false);
      nav.push(`${paths.squads()}/${squad.id}/workflows/${draft.id}`);
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow_designer.start.failed));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md space-y-4 p-6">
        <DialogHeader className="space-y-1">
          <DialogTitle>{t(($) => $.flow_designer.new)}</DialogTitle>
        </DialogHeader>

        <label className="block space-y-2">
          <span className="block text-caption font-medium text-foreground">{t(($) => $.flow_designer.start.name)}</span>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t(($) => $.flow_designer.editor.name_placeholder)}
            maxLength={56}
            className="h-8 text-caption"
          />
        </label>

        <div className="grid grid-cols-1 gap-2 pt-2">
          <Button
            type="button"
            variant="outline"
            disabled={createMut.isPending}
            onClick={() => void create({ kind: "blank" })}
            className="h-10 justify-start gap-3 px-3 text-caption"
          >
            <FileText className="size-4 shrink-0 text-primary" aria-hidden />
            <span className="font-semibold">{t(($) => $.flow_designer.start.blank)}</span>
          </Button>

          {source && (
            <Button
              type="button"
              variant="outline"
              disabled={createMut.isPending}
              onClick={() => void create({ kind: "workflow", workflow_id: source.id })}
              className="h-10 justify-start gap-3 px-3 text-caption"
            >
              <Copy className="size-4 shrink-0 text-emerald-500" aria-hidden />
              <span className="font-semibold">{t(($) => $.flow_designer.start.kitviet)}</span>
            </Button>
          )}

          <Button
            type="button"
            variant="outline"
            disabled={createMut.isPending}
            onClick={() => {
              onOpenChange(false);
              onOpenImport?.();
            }}
            className="h-10 justify-start gap-3 px-3 text-caption"
          >
            <Upload className="size-4 shrink-0 text-blue-500" aria-hidden />
            <span className="font-semibold">{t(($) => $.flow_designer.start.import)}</span>
          </Button>
        </div>

        {createMut.isPending && (
          <div className="flex items-center justify-center gap-2 py-2 text-caption text-muted-foreground">
            <Loader2 className="size-4 animate-spin" aria-hidden />
            <span>{t(($) => $.flow_designer.start.creating)}</span>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
