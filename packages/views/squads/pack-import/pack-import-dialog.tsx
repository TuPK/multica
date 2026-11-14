"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { clientErrorMessage } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { useCommitPack, usePreviewPack, packConflictPrevious, packErrorCode, packNameConflicts } from "@multica/core/packs";
import { useWorkspacePaths } from "@multica/core/paths";
import { runtimeListOptions } from "@multica/core/runtimes/queries";
import type { PackCommitResponse, PackPreview, PackPreviousInstall } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { AlertCircle, AlertTriangle, Loader2, Package } from "@multica/ui/icons";
import { useT } from "../../i18n";
import { buildCommitRequest, initialChoices, installBlocker, type PackChoices } from "./pack-choices";
import { PackLeaderPicker } from "./pack-leader-picker";
import { PackPreviewAgents, runtimeLabel } from "./pack-preview-agents";
import { PackPreviewSkills } from "./pack-preview-skills";
import { PackResult } from "./pack-result";

export interface PackImportDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** A link on github.com is read as a repository; any other is a link to a zip. */
function sourceOf(url: string) {
  const trimmed = url.trim();
  let host = "";
  try {
    host = new URL(trimmed).hostname;
  } catch {
    host = "";
  }
  return { kind: host === "github.com" || host === "www.github.com" ? ("github" as const) : ("url_zip" as const), url: trimmed };
}

/**
 * Importing a pack in three steps (spec 11.9): read it from a zip or a link, look at what it holds and decide,
 * then see what the install made. Nothing is written until the person installs.
 */
export function PackImportDialog({ open, onOpenChange }: PackImportDialogProps) {
  const { t } = useT("squads");
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const previewMut = usePreviewPack();
  const commitMut = useCommitPack();
  const runtimesQuery = useQuery(runtimeListOptions(wsId, "me"));
  const runtimes = useMemo(() => runtimesQuery.data ?? [], [runtimesQuery.data]);
  const defaultRuntimeId = (runtimes.find((r) => r.status === "online") ?? runtimes[0])?.id ?? "";

  const [url, setUrl] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [readError, setReadError] = useState<string | null>(null);
  const [preview, setPreview] = useState<PackPreview | null>(null);
  const [choices, setChoices] = useState<PackChoices | null>(null);
  const [result, setResult] = useState<PackCommitResponse | null>(null);
  const [conflict, setConflict] = useState<PackPreviousInstall | "unknown" | null>(null);
  const [nameTaken, setNameTaken] = useState<string[]>([]);
  const [installError, setInstallError] = useState<string | null>(null);

  // The runtimes may arrive after the preview: an agent that was waiting for one gets the default.
  useEffect(() => {
    if (!defaultRuntimeId) return;
    setChoices((prev) => {
      if (!prev) return prev;
      const agents = Object.fromEntries(Object.entries(prev.agents).map(([k, a]) => [k, a.runtimeId ? a : { ...a, runtimeId: defaultRuntimeId }]));
      return {
        ...prev,
        agents,
        extras: prev.extras.map((e) => (e.runtimeId ? e : { ...e, runtimeId: defaultRuntimeId })),
        genericRuntimeId: prev.genericRuntimeId || defaultRuntimeId,
      };
    });
  }, [defaultRuntimeId]);

  const patchChoices = (patch: Partial<PackChoices>) => setChoices((prev) => (prev ? { ...prev, ...patch } : prev));
  const canRead = (file !== null || url.trim() !== "") && !previewMut.isPending;

  const readPack = async () => {
    setReadError(null);
    try {
      const read = await previewMut.mutateAsync(file ? { file } : { source: sourceOf(url) });
      setPreview(read);
      setChoices(initialChoices(read, defaultRuntimeId));
      setConflict(null);
      setNameTaken([]);
      setInstallError(null);
    } catch (err) {
      setReadError(clientErrorMessage(err) ?? t(($) => $.packs.read_failed));
    }
  };

  const install = async (newInstall: boolean) => {
    if (!preview || !choices) return;
    setInstallError(null);
    setNameTaken([]);
    try {
      const done = await commitMut.mutateAsync(buildCommitRequest(preview, choices, newInstall));
      setConflict(null);
      setResult(done);
    } catch (err) {
      const code = packErrorCode(err);
      if (code === "pack_conflict") {
        setConflict(packConflictPrevious(err) ?? "unknown");
        return;
      }
      if (code === "pack_name_conflict") {
        const taken = packNameConflicts(err);
        setNameTaken([...taken.agents, ...taken.skills]);
        return;
      }
      setInstallError(clientErrorMessage(err) ?? t(($) => $.packs.install_failed));
    }
  };

  const blocker = preview && choices ? installBlocker(preview, choices) : null;
  const designHref = result?.squad_id ? (result.draft_id ? `${paths.squads()}/${result.squad_id}/workflows/${result.draft_id}` : paths.squadDetail(result.squad_id)) : null;
  const squadHref = result?.squad_id ? paths.squadDetail(result.squad_id) : null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="space-y-4 p-4 sm:max-w-3xl sm:p-6">
        <DialogHeader className="space-y-1">
          <DialogTitle className="flex items-center gap-2">
            <Package className="size-4 text-muted-foreground" aria-hidden />
            <span>{t(($) => $.packs.import)}</span>
          </DialogTitle>
        </DialogHeader>

        {result ? (
          <PackResult result={result} designHref={designHref} squadHref={squadHref} onDone={() => onOpenChange(false)} />
        ) : !preview || !choices ? (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <label className="inline-flex h-8 cursor-pointer items-center rounded-md border border-input bg-background px-3 text-caption font-medium hover:bg-muted">
                <span>{t(($) => $.packs.source.choose)}</span>
                <input
                  type="file"
                  accept=".zip,application/zip"
                  className="sr-only"
                  onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                />
              </label>
              {file && <span className="min-w-0 truncate text-caption text-muted-foreground">{file.name}</span>}
            </div>
            <Input
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder={t(($) => $.packs.source.link_placeholder)}
              aria-label={t(($) => $.packs.source.link)}
              disabled={file !== null}
              className="h-8 text-caption"
            />
            {readError && (
              <div role="alert" className="flex items-center gap-2 rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-caption text-destructive">
                <AlertCircle className="size-4 shrink-0" aria-hidden />
                <span>{t(($) => $.packs.read_failed)}</span>
                {readError !== t(($) => $.packs.read_failed) && <span className="min-w-0 truncate text-micro">{readError}</span>}
              </div>
            )}
            <div className="flex justify-end">
              <Button type="button" size="sm" disabled={!canRead} onClick={() => void readPack()} className="gap-1.5">
                {previewMut.isPending && <Loader2 className="size-3.5 animate-spin" aria-hidden />}
                <span>{previewMut.isPending ? t(($) => $.packs.reading) : t(($) => $.packs.preview)}</span>
              </Button>
            </div>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="min-w-0 truncate text-body font-semibold text-foreground">
                {preview.name}
                {preview.version && <span className="ml-2 text-caption font-normal text-muted-foreground">{preview.version}</span>}
              </div>
              <Button type="button" variant="ghost" size="xs" onClick={() => { setPreview(null); setChoices(null); }}>
                {t(($) => $.packs.change_source)}
              </Button>
            </div>

            {preview.issues.length > 0 && (
              <section className="space-y-1" aria-labelledby="pack-problems-title">
                <h3 id="pack-problems-title" className="text-caption font-semibold text-foreground">{t(($) => $.packs.problems)}</h3>
                <ul className="list-disc space-y-0.5 pl-5 text-micro text-muted-foreground">
                  {preview.issues.map((issue, i) => (
                    <li key={i} className={issue.severity === "error" ? "text-destructive" : undefined}>{issue.message || issue.code}</li>
                  ))}
                </ul>
              </section>
            )}

            {preview.scan.length > 0 && (
              <section className="space-y-1" aria-labelledby="pack-scan-title">
                <h3 id="pack-scan-title" className="flex items-center gap-1.5 text-caption font-semibold text-foreground">
                  <AlertTriangle className="size-3.5 text-amber-500" aria-hidden />
                  {t(($) => $.packs.scan.title)}
                </h3>
                <ul className="space-y-0.5 text-micro">
                  {preview.scan.map((f, i) => (
                    <li key={i} className="flex flex-wrap gap-x-2">
                      <span className={f.severity === "block" ? "font-medium text-destructive" : f.severity === "warn" ? "font-medium text-amber-600" : "text-muted-foreground"}>
                        {t(($) => $.packs.scan[f.severity])}
                      </span>
                      <span className="min-w-0 break-all text-muted-foreground">{f.path}</span>
                      <span className="font-mono text-muted-foreground">{f.rule}</span>
                    </li>
                  ))}
                </ul>
                {preview.scan.some((f) => f.severity === "warn") && !preview.scan.some((f) => f.severity === "block") && (
                  <label className="flex cursor-pointer items-center gap-2 pt-1 text-caption">
                    <input type="checkbox" checked={choices.ackScan} onChange={(e) => patchChoices({ ackScan: e.target.checked })} className="size-4" />
                    <span>{t(($) => $.packs.scan.ack)}</span>
                  </label>
                )}
              </section>
            )}

            <PackPreviewAgents preview={preview} choices={choices} runtimes={runtimes} defaultRuntimeId={defaultRuntimeId} onChange={patchChoices} />
            <PackPreviewSkills preview={preview} choices={choices} onChange={patchChoices} />
            <PackLeaderPicker preview={preview} choices={choices} runtimes={runtimes} onChange={patchChoices} />

            <label className="block space-y-1">
              <span className="block text-caption font-semibold text-foreground">{t(($) => $.packs.squad_name)}</span>
              <Input value={choices.squadName} onChange={(e) => patchChoices({ squadName: e.target.value })} maxLength={200} className="h-8 text-caption" />
            </label>

            {preview.flow.present && (
              <label className="flex cursor-pointer items-center gap-2 text-caption">
                <input type="checkbox" checked={choices.draftFromFlow} onChange={(e) => patchChoices({ draftFromFlow: e.target.checked })} className="size-4" />
                <span>{t(($) => $.packs.draft_from_flow)}</span>
              </label>
            )}

            {preview.ignored.length > 0 && (
              <section className="space-y-1" aria-labelledby="pack-ignored-title">
                <h3 id="pack-ignored-title" className="text-caption font-semibold text-foreground">{t(($) => $.packs.ignored)}</h3>
                <ul className="flex flex-wrap gap-x-4 gap-y-0.5 text-micro text-muted-foreground">
                  {preview.ignored.map((item, i) => (
                    <li key={i} className="font-mono">{[item.agent ?? item.skill, item.key].filter(Boolean).join(": ")}</li>
                  ))}
                </ul>
              </section>
            )}

            {conflict && (
              <div className="space-y-2 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-caption">
                <div className="font-medium text-amber-700 dark:text-amber-400">{t(($) => $.packs.conflict.title)}</div>
                {conflict !== "unknown" && <p className="text-muted-foreground">{t(($) => $.packs.conflict.body, { version: conflict.version || "-" })}</p>}
                <div className="flex flex-wrap gap-2">
                  <Button type="button" variant="outline" size="sm" onClick={() => onOpenChange(false)}>
                    {t(($) => $.packs.conflict.keep)}
                  </Button>
                  <Button type="button" size="sm" disabled={commitMut.isPending} onClick={() => void install(true)}>
                    {t(($) => $.packs.conflict.new_install)}
                  </Button>
                </div>
              </div>
            )}

            {nameTaken.length > 0 && (
              <div role="alert" className="rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-caption text-destructive">
                {t(($) => $.packs.name_taken, { names: nameTaken.join(", ") })}
              </div>
            )}
            {installError && (
              <div role="alert" className="rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-caption text-destructive">
                {installError}
              </div>
            )}

            <div className="flex flex-wrap items-center justify-end gap-3 border-t border-border pt-4">
              {blocker && (
                <p id="pack-install-why" className="min-w-0 flex-1 text-caption text-muted-foreground">
                  {t(($) => $.packs.disabled[blocker])}
                </p>
              )}
              <Button
                type="button"
                size="sm"
                disabled={blocker !== null || commitMut.isPending}
                aria-describedby={blocker ? "pack-install-why" : undefined}
                onClick={() => void install(false)}
                className="gap-1.5"
              >
                {commitMut.isPending && <Loader2 className="size-3.5 animate-spin" aria-hidden />}
                <span>{commitMut.isPending ? t(($) => $.packs.installing) : t(($) => $.packs.install)}</span>
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

export { runtimeLabel };
