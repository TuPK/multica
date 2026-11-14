"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, clientErrorMessage } from "@multica/core/api";
import { useCreateFlowDraft, useFlowCapabilities } from "@multica/core/flow-drafts";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { agentListOptions } from "@multica/core/workspace/queries";
import type { IssueWorkflow, Squad } from "@multica/core/types";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Bot, Copy, FileText, Loader2, Paperclip, Upload, X } from "@multica/ui/icons";
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

// The most files a request to an agent carries; the server takes the same number.
const MAX_REFERENCES = 10;

export function StartDialog({ open, onOpenChange, squad, workflows, onOpenImport }: StartDialogProps) {
  const { t } = useT("squads");
  const nav = useNavigation();
  const paths = useWorkspacePaths();
  const wsId = useWorkspaceId();
  const createMut = useCreateFlowDraft();
  const [name, setName] = useState("");
  const [asking, setAsking] = useState(false);
  const [agentId, setAgentId] = useState("");
  const [note, setNote] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [sending, setSending] = useState(false);

  // A server that has not said is taken to allow it, as the squad section does for authoring.
  const capabilitiesQuery = useFlowCapabilities(wsId);
  const proposalsEnabled = capabilitiesQuery.data?.features?.proposals ?? true;
  const agentsQuery = useQuery({ ...agentListOptions(wsId), enabled: Boolean(wsId) && asking });
  const agents = useMemo(
    () => (agentsQuery.data ?? []).filter((a) => !a.archived_at).sort((a, b) => a.name.localeCompare(b.name)),
    [agentsQuery.data],
  );
  const chosenAgent = agentId || agents[0]?.id || "";

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

  // Asks an agent to write the first version: the files go up first, and the draft keeps their ids as references.
  const ask = async () => {
    if (!chosenAgent) return;
    setSending(true);
    try {
      const references: string[] = [];
      for (const file of files) references.push((await api.uploadFile(file)).id);
      const draft = await createMut.mutateAsync({
        squadId: squad.id,
        data: { name: cutDraftName(name.trim() || squad.name), from: { kind: "proposal", agent_id: chosenAgent, references, note: note.trim() } },
      });
      onOpenChange(false);
      nav.push(`${paths.squads()}/${squad.id}/workflows/${draft.id}`);
    } catch (err) {
      toast.error(clientErrorMessage(err) ?? t(($) => $.flow_designer.proposal.ask.failed));
    } finally {
      setSending(false);
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

        {asking ? (
          <div className="space-y-3 pt-2">
            <label className="block space-y-1">
              <span className="block text-caption font-medium text-foreground">{t(($) => $.flow_designer.proposal.ask.agent)}</span>
              {agents.length === 0 ? (
                !agentsQuery.isLoading && <p className="text-caption text-muted-foreground">{t(($) => $.flow_designer.proposal.ask.no_agent)}</p>
              ) : (
                <select
                  value={chosenAgent}
                  onChange={(e) => setAgentId(e.target.value)}
                  className="h-8 w-full rounded-md border border-input bg-background px-2 text-caption"
                >
                  {agents.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name}
                    </option>
                  ))}
                </select>
              )}
            </label>
            <label className="block space-y-1">
              <span className="block text-caption font-medium text-foreground">{t(($) => $.flow_designer.proposal.ask.note)}</span>
              <Textarea value={note} onChange={(e) => setNote(e.target.value)} maxLength={4000} rows={3} className="text-caption" />
            </label>
            <div className="space-y-1">
              <span className="block text-caption font-medium text-foreground">{t(($) => $.flow_designer.proposal.ask.files)}</span>
              <ul className="space-y-1">
                {files.map((file, i) => (
                  <li key={`${file.name}-${i}`} className="flex items-center justify-between gap-2 text-caption">
                    <span className="min-w-0 truncate">{file.name}</span>
                    <Button type="button" variant="ghost" size="icon-xs" aria-label={t(($) => $.packs.remove)} onClick={() => setFiles(files.filter((_, j) => j !== i))}>
                      <X className="size-3.5" aria-hidden />
                    </Button>
                  </li>
                ))}
              </ul>
              {files.length < MAX_REFERENCES && (
                <label className="inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md border border-input bg-background px-3 text-caption font-medium hover:bg-muted">
                  <Paperclip className="size-3.5" aria-hidden />
                  <span>{t(($) => $.flow_designer.proposal.ask.attach)}</span>
                  <input
                    type="file"
                    className="sr-only"
                    onChange={(e) => {
                      const picked = e.target.files?.[0];
                      if (picked) setFiles([...files, picked]);
                      e.target.value = "";
                    }}
                  />
                </label>
              )}
            </div>
            <div className="flex justify-end gap-2 pt-1">
              <Button type="button" variant="outline" size="sm" disabled={sending} onClick={() => setAsking(false)}>
                {t(($) => $.flow_designer.actions.cancel)}
              </Button>
              <Button type="button" size="sm" disabled={sending || !chosenAgent} onClick={() => void ask()} className="gap-1.5">
                {sending && <Loader2 className="size-3.5 animate-spin" aria-hidden />}
                <span>{t(($) => $.flow_designer.proposal.ask.send)}</span>
              </Button>
            </div>
          </div>
        ) : (
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

          {proposalsEnabled && (
            <Button
              type="button"
              variant="outline"
              disabled={createMut.isPending}
              onClick={() => setAsking(true)}
              className="h-10 justify-start gap-3 px-3 text-caption"
            >
              <Bot className="size-4 shrink-0 text-violet-500" aria-hidden />
              <span className="font-semibold">{t(($) => $.flow_designer.start.proposal)}</span>
            </Button>
          )}
        </div>
        )}

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
