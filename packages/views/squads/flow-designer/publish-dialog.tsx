"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { clientErrorMessage } from "@multica/core/api";
import {
  flowErrorBody,
  flowErrorCode,
  flowRequiredAcks,
  useCreateFlowDraft,
  usePublishFlowDraft,
  useValidateFlowDraft,
} from "@multica/core/flow-drafts";
import { useSetSquadActiveWorkflow } from "@multica/core/issue-workflows";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import type { FlowDoc, FlowIssue } from "@multica/core/types";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { AlertCircle, AlertTriangle, Check, Loader2, RotateCcw } from "@multica/ui/icons";
import { useNavigation } from "../../navigation";
import { useT } from "../../i18n";

export interface PublishDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  squadId: string;
  draftId: string;
  doc: FlowDoc;
  isAdmin: boolean;
  /** Saves the draft and resolves to the revision the server has; null when it could not be saved. */
  saveDraft: () => Promise<number | null>;
}

// The acknowledgements a publish can need, by the kind the server names them. An admin gives the two that
// let work leave the workspace without a person looking at it (spec 10).
const ACKS: Record<string, { adminOnly: boolean }> = {
  unattended_release: { adminOnly: true },
  lead_may_work: { adminOnly: true },
  no_gate: { adminOnly: false },
};

/** The acknowledgements a check asks for: warnings that name one, and a policy flag set without it. */
function requiredAcks(issues: FlowIssue[]): string[] {
  const kinds = new Set<string>();
  for (const issue of issues) {
    const kind = issue.code === "ack_required" ? issue.params?.ack : issue.ack;
    if (typeof kind === "string" && kind) kinds.add(kind);
  }
  return [...kinds];
}

export function PublishDialog({ open, onOpenChange, squadId, draftId, doc, isAdmin, saveDraft }: PublishDialogProps) {
  const { t } = useT("squads");
  const wsId = useWorkspaceId();
  const nav = useNavigation();
  const paths = useWorkspacePaths();
  const validateMut = useValidateFlowDraft();
  const publishMut = usePublishFlowDraft();
  const createDraftMut = useCreateFlowDraft();
  const setActiveMut = useSetSquadActiveWorkflow(wsId, squadId);

  const [issues, setIssues] = useState<FlowIssue[]>([]);
  const [checking, setChecking] = useState(false);
  const [givenAcks, setGivenAcks] = useState<Set<string>>(new Set());
  const [makeActive, setMakeActive] = useState(false);
  const [staleLatestId, setStaleLatestId] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  // Opening the dialog checks the document as it is now: what must be acknowledged comes from the
  // server's answer, not from a guess made here.
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setChecking(true);
    setErrorMessage(null);
    setStaleLatestId(null);
    setGivenAcks(new Set());
    validateMut
      .mutateAsync({ squadId, draftId, doc })
      .then((res) => {
        if (!cancelled) setIssues(res.issues);
      })
      .catch((err) => {
        if (!cancelled) setErrorMessage(clientErrorMessage(err) ?? t(($) => $.flow_designer.editor.check_failed));
      })
      .finally(() => {
        if (!cancelled) setChecking(false);
      });
    return () => {
      cancelled = true;
    };
    // The check runs when the dialog opens, not on every edit made behind it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const blocking = issues.filter((i) => i.severity === "error" && i.code !== "ack_required");
  const needed = requiredAcks(issues);
  const ackLabel = (kind: string) => {
    switch (kind) {
      case "unattended_release":
        return t(($) => $.flow_designer.ack.no_human_exit);
      case "no_gate":
        return t(($) => $.flow_designer.ack.no_gate);
      case "lead_may_work":
        return t(($) => $.flow_designer.ack.lead_may_work);
      default:
        return kind;
    }
  };
  const missingAck = needed.some((kind) => !givenAcks.has(kind) || (ACKS[kind]?.adminOnly && !isAdmin));
  const canPublish = !checking && blocking.length === 0 && !missingAck && !publishMut.isPending;

  const toggleAck = (kind: string, on: boolean) => {
    setGivenAcks((prev) => {
      const next = new Set(prev);
      if (on) next.add(kind);
      else next.delete(kind);
      return next;
    });
  };

  const handlePublish = async () => {
    if (!canPublish) return;
    setErrorMessage(null);
    setStaleLatestId(null);
    try {
      // The revision is the one the server holds after this save, never one remembered from loading.
      const revision = await saveDraft();
      if (revision === null) {
        setErrorMessage(t(($) => $.flow_designer.editor.not_saved));
        return;
      }
      const published = await publishMut.mutateAsync({
        squadId,
        draftId,
        data: { revision, ack: needed.filter((kind) => givenAcks.has(kind)) },
      });
      // The server never makes a version active on its own: the squad's active workflow is set through
      // its own route, as the list of versions does it.
      if (makeActive) {
        try {
          await setActiveMut.mutateAsync({ workflow_id: published.id });
        } catch (err) {
          toast.error(clientErrorMessage(err) ?? t(($) => $.flow_designer.publish.active_failed));
        }
      }
      toast.success(t(($) => $.flow_designer.publish.done));
      onOpenChange(false);
      nav.push(`${paths.squads()}/${squadId}`);
    } catch (err) {
      const code = flowErrorCode(err);
      if (code === "flow_draft_stale") {
        const latest = flowErrorBody(err).latest_workflow_id;
        setStaleLatestId(typeof latest === "string" ? latest : null);
        return;
      }
      if (code === "flow_ack_required") {
        // The server names what it still needs; show those boxes even if the check did not.
        const required = flowRequiredAcks(err);
        setIssues((prev) => [...prev, ...required.map((ack): FlowIssue => ({ code: "ack_required", severity: "error", message: ack, params: { ack } }))]);
        return;
      }
      if (code === "flow_draft_invalid") {
        const body = flowErrorBody(err).issues;
        if (Array.isArray(body)) setIssues(body as FlowIssue[]);
      }
      setErrorMessage(clientErrorMessage(err) ?? t(($) => $.flow_designer.publish.failed));
    }
  };

  // A newer version exists: a draft made from it carries the same lineage and publishes as the next one.
  const handleEditLatest = async () => {
    if (!staleLatestId) return;
    try {
      const draft = await createDraftMut.mutateAsync({
        squadId,
        data: { from: { kind: "workflow", workflow_id: staleLatestId } },
      });
      onOpenChange(false);
      nav.push(`${paths.squads()}/${squadId}/workflows/${draft.id}`);
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow_designer.start.failed));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg space-y-4 p-6">
        <DialogHeader className="space-y-1">
          <DialogTitle>{t(($) => $.flow_designer.actions.publish)}</DialogTitle>
          <DialogDescription className="text-body-sm text-muted-foreground">
            {t(($) => $.flow_designer.publish.note)}
          </DialogDescription>
        </DialogHeader>

        {checking && (
          <div className="flex items-center gap-2 text-caption text-muted-foreground">
            <Loader2 className="size-4 animate-spin" aria-hidden />
            <span>{t(($) => $.flow_designer.publish.checking)}</span>
          </div>
        )}

        {blocking.length > 0 && (
          <div className="space-y-1 rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-caption text-destructive">
            <div className="flex items-center gap-2 font-medium">
              <AlertCircle className="size-4 shrink-0" aria-hidden />
              <span>{t(($) => $.flow_designer.publish.blocked)}</span>
            </div>
            <ul className="list-disc space-y-0.5 pl-5 text-micro">
              {blocking.slice(0, 3).map((issue, i) => (
                <li key={i}>{issue.message || issue.code}</li>
              ))}
            </ul>
          </div>
        )}

        {errorMessage && (
          <div role="alert" className="rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-caption text-destructive">
            {errorMessage}
          </div>
        )}

        {staleLatestId && (
          <div className="space-y-2 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-caption text-amber-700 dark:text-amber-400">
            <div className="flex items-center gap-2 font-medium">
              <RotateCcw className="size-4 shrink-0" aria-hidden />
              <span>{t(($) => $.flow_designer.publish.stale)}</span>
            </div>
            <Button
              type="button"
              variant="outline"
              size="xs"
              className="h-7 text-caption"
              disabled={createDraftMut.isPending}
              onClick={() => void handleEditLatest()}
            >
              {t(($) => $.flow_designer.publish.edit_latest)}
            </Button>
          </div>
        )}

        {needed.map((kind) => {
          const adminOnly = ACKS[kind]?.adminOnly ?? false;
          return (
            <div key={kind} className="space-y-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-caption">
              {kind === "unattended_release" && (
                <div className="flex items-center gap-1.5 font-medium text-amber-600 dark:text-amber-400">
                  <AlertTriangle className="size-4 shrink-0" aria-hidden />
                  <span>{t(($) => $.flow_designer.release.warning)}</span>
                </div>
              )}
              <label className="flex cursor-pointer items-start gap-2">
                <input
                  type="checkbox"
                  disabled={adminOnly && !isAdmin}
                  checked={givenAcks.has(kind)}
                  onChange={(e) => toggleAck(kind, e.target.checked)}
                  className="mt-0.5 size-4 rounded border-amber-500 text-amber-600"
                />
                <span className={adminOnly && !isAdmin ? "opacity-70" : undefined}>
                  {ackLabel(kind)}
                  {adminOnly && !isAdmin && (
                    <span className="ml-1 text-micro text-muted-foreground">
                      ({t(($) => $.flow_designer.publish.admin_only)})
                    </span>
                  )}
                </span>
              </label>
            </div>
          );
        })}

        <label className="flex cursor-pointer items-center gap-2 pt-2 text-caption">
          <input
            type="checkbox"
            checked={makeActive}
            onChange={(e) => setMakeActive(e.target.checked)}
            className="size-4 rounded border-input"
          />
          <span className="font-medium text-foreground">{t(($) => $.flow_designer.publish.make_active)}</span>
        </label>

        <DialogFooter className="gap-2 border-t border-border pt-4">
          <Button type="button" variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            {t(($) => $.flow_designer.actions.cancel)}
          </Button>
          <Button type="button" size="sm" disabled={!canPublish} onClick={() => void handlePublish()} className="gap-1.5">
            {publishMut.isPending ? (
              <Loader2 className="size-3.5 animate-spin" aria-hidden />
            ) : (
              <Check className="size-3.5" aria-hidden />
            )}
            <span>{t(($) => $.flow_designer.actions.publish)}</span>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
