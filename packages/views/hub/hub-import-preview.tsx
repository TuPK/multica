"use client";

import { useEffect, useId, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { Ban, CircleCheck, File, Folder, Loader2, TriangleAlert } from "@multica/ui/icons";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { DialogFooter } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import {
  HubImportBlockedError,
  HubImportWarningsError,
  HubPreviewExpiredError,
  HubQuotaExceededError,
  useCommitHubImport,
  usePreviewHubImport,
  type HubImportCommitResult,
  type HubImportPreview,
} from "@multica/core/hub";
import { useT } from "../i18n";
import { flattenHubTree, formatHubSize } from "./hub-files";
import { errorMessage, previewVars, type HubImportRequest } from "./hub-import-request";

type ConflictChoice = "rename" | "replace" | "skip";
const CONFLICT_CHOICES: readonly ConflictChoice[] = ["rename", "replace", "skip"];

/**
 * Shows what an import would create and commits it. The commit button is the
 * only way through: a blocked scan disables it, a warning needs an explicit
 * acknowledgement, and an expired preview offers a fresh one.
 */
export function HubImportPreviewStep({
  wsId,
  request,
  preview,
  onBack,
  onPreviewReplaced,
  onFinished,
}: {
  wsId: string;
  request: HubImportRequest;
  preview: HubImportPreview;
  onBack: () => void;
  onPreviewReplaced: (next: HubImportPreview) => void;
  onFinished: (result: HubImportCommitResult) => void;
}) {
  const { t } = useT("hub");
  const titleRef = useRef<HTMLHeadingElement>(null);
  const commit = useCommitHubImport();
  const again = usePreviewHubImport();
  const radioName = useId();
  const [choice, setChoice] = useState<ConflictChoice>("rename");
  const [newName, setNewName] = useState(`${preview.name}-2`);
  const [understood, setUnderstood] = useState(false);
  const [error, setError] = useState("");
  const [expired, setExpired] = useState(false);

  useEffect(() => {
    titleRef.current?.focus();
  }, []);

  const conflict = preview.conflict.exists;
  const scan = preview.scan.status;
  const blocked = scan === "blocked";
  const warned = scan === "warn";
  const renaming = conflict && choice === "rename";
  const renameMissing = renaming && newName.trim() === "";
  const commitDisabled = blocked || (warned && !understood) || renameMissing || commit.isPending;

  const onCommit = async () => {
    setError("");
    const onConflict: ConflictChoice = conflict ? choice : "rename";
    try {
      const result = await commit.mutateAsync({
        wsId,
        previewId: preview.preview_id,
        onConflict,
        name: renaming ? newName.trim() : undefined,
        confirmWarnings: understood,
      });
      if (result.skipped) {
        toast.success(t(($) => $.add.toast.skipped, { name: preview.name }));
      } else if (conflict && choice === "replace") {
        toast.success(t(($) => $.add.toast.replaced, { name: preview.name }));
      } else {
        toast.success(t(($) => $.add.toast.imported, { name: renaming ? newName.trim() : preview.name }));
      }
      onFinished(result);
    } catch (err) {
      if (err instanceof HubPreviewExpiredError) {
        setExpired(true);
      } else if (err instanceof HubQuotaExceededError) {
        setError(t(($) => $.add.preview.quota_error));
      } else if (err instanceof HubImportBlockedError) {
        setError(t(($) => $.add.preview.scan.blocked_error));
      } else if (err instanceof HubImportWarningsError) {
        setError(t(($) => $.add.preview.scan.warn_error));
      } else {
        setError(errorMessage(err, t(($) => $.add.toast.failed)));
      }
    }
  };

  const previewAgain = async () => {
    setError("");
    try {
      const next = await again.mutateAsync(previewVars(wsId, request));
      onPreviewReplaced(next);
    } catch (err) {
      setError(errorMessage(err, t(($) => $.add.preview.preview_failed)));
    }
  };

  const kindLabel = preview.kind === "plugin" ? t(($) => $.kind_singular.plugin) : t(($) => $.kind_singular.skill);

  return (
    <>
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto">
        <section className="flex min-w-0 flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2 text-caption text-muted-foreground">
            <span>{kindLabel}</span>
            {preview.version ? <span>{t(($) => $.add.preview.version, { version: preview.version })}</span> : null}
          </div>
          <h3 ref={titleRef} tabIndex={-1} className="truncate text-title-sm font-medium outline-none">
            {preview.name}
          </h3>
          {preview.description ? (
            <p className="line-clamp-3 text-label text-muted-foreground">{preview.description}</p>
          ) : null}
        </section>

        <ScanSection scan={preview.scan} />

        {conflict ? (
          <fieldset className="flex flex-col gap-2">
            <legend className="mb-1 text-caption text-muted-foreground">
              {t(($) => $.add.preview.conflict.legend, { name: preview.name })}
            </legend>
            {CONFLICT_CHOICES.map((value) => (
              <div key={value} className="flex flex-col gap-1.5">
                <label className="flex items-center gap-2 text-label">
                  <input
                    type="radio"
                    name={radioName}
                    value={value}
                    checked={choice === value}
                    onChange={() => setChoice(value)}
                    className="accent-primary"
                  />
                  {t(($) => $.add.preview.conflict[value])}
                </label>
                {value === "rename" && choice === "rename" ? (
                  <Input
                    aria-label={t(($) => $.add.preview.conflict.rename_label)}
                    value={newName}
                    onChange={(event) => setNewName(event.target.value)}
                    className="ml-6 w-auto"
                    spellCheck={false}
                  />
                ) : null}
                {value === "replace" ? (
                  <p className="ml-6 text-caption text-muted-foreground">{t(($) => $.add.preview.conflict.replace_hint)}</p>
                ) : null}
              </div>
            ))}
          </fieldset>
        ) : null}

        <FilesSection files={preview.files} />

        {warned ? (
          <div className="flex items-start gap-2">
            <Checkbox
              id={`${radioName}-ack`}
              checked={understood}
              onCheckedChange={(checked) => setUnderstood(checked === true)}
              className="mt-0.5"
            />
            <Label htmlFor={`${radioName}-ack`} className="text-label font-normal">
              {t(($) => $.add.preview.scan.acknowledge)}
            </Label>
          </div>
        ) : null}

        {expired ? (
          <div
            role="alert"
            className="flex flex-wrap items-center justify-between gap-3 rounded-md bg-destructive/10 px-3 py-2 text-caption text-destructive"
          >
            <span>{t(($) => $.add.preview.expired)}</span>
            <Button type="button" size="sm" variant="outline" disabled={again.isPending} aria-busy={again.isPending} onClick={() => void previewAgain()}>
              {again.isPending ? <Loader2 className="animate-spin" aria-hidden="true" /> : null}
              {t(($) => $.add.preview.preview_again)}
            </Button>
          </div>
        ) : null}

        {error ? (
          <p role="alert" className="text-caption text-destructive">
            {error}
          </p>
        ) : null}
      </div>

      <DialogFooter>
        <Button type="button" variant="ghost" onClick={onBack} disabled={commit.isPending}>
          {t(($) => $.add.preview.back)}
        </Button>
        <Button type="button" disabled={commitDisabled} aria-busy={commit.isPending} onClick={() => void onCommit()}>
          {commit.isPending ? <Loader2 className="animate-spin" aria-hidden="true" /> : null}
          {commit.isPending ? t(($) => $.add.preview.committing) : t(($) => $.add.preview.commit)}
        </Button>
      </DialogFooter>
    </>
  );
}

const SCAN_ICON = { clean: CircleCheck, warn: TriangleAlert, blocked: Ban } as const;
const SEVERITY_VARIANT = { info: "secondary", warn: "outline", block: "destructive" } as const;

function ScanSection({ scan }: { scan: HubImportPreview["scan"] }) {
  const { t } = useT("hub");
  const Icon = SCAN_ICON[scan.status];
  const badgeVariant = scan.status === "clean" ? "secondary" : scan.status === "warn" ? "outline" : "destructive";
  return (
    <section className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <h4 className="text-caption text-muted-foreground">{t(($) => $.add.preview.scan.title)}</h4>
        <Badge variant={badgeVariant}>
          <Icon aria-hidden="true" />
          {t(($) => $.add.preview.scan[scan.status])}
        </Badge>
      </div>
      {scan.status === "blocked" ? (
        <p className="text-caption text-destructive">{t(($) => $.add.preview.scan.blocked_hint)}</p>
      ) : scan.status === "warn" ? (
        <p className="text-caption text-muted-foreground">{t(($) => $.add.preview.scan.warn_hint)}</p>
      ) : null}
      {scan.findings.length > 0 ? (
        <ul className="flex flex-col gap-2">
          {scan.findings.map((finding, index) => (
            <li key={`${finding.rule}-${finding.path}-${index}`} className="flex min-w-0 items-start gap-2 text-caption">
              <Badge variant={SEVERITY_VARIANT[finding.severity]} className="shrink-0">
                {t(($) => $.add.preview.scan.severity[finding.severity])}
              </Badge>
              <span className="min-w-0 flex-1">
                <span className="break-words">{finding.message}</span>
                {finding.path ? (
                  <code className="mt-0.5 block truncate text-muted-foreground">{finding.path}</code>
                ) : null}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  );
}

function FilesSection({ files }: { files: HubImportPreview["files"] }) {
  const { t } = useT("hub");
  const rows = useMemo(
    () => flattenHubTree(files.map((file) => file.path), new Set()),
    [files],
  );
  const sizes = useMemo(() => new Map(files.map((file) => [file.path, file.size_bytes])), [files]);
  return (
    <section className="flex flex-col gap-2">
      <h4 className="text-caption text-muted-foreground">
        {t(($) => $.add.preview.files.title, { count: files.length })}
      </h4>
      <ul aria-label={t(($) => $.add.preview.files.list_label)} className="max-h-56 overflow-y-auto rounded-md border p-1 text-label">
        {rows.map((row) => {
          const Icon = row.isFolder ? Folder : File;
          return (
            <li
              key={`${row.isFolder ? "dir" : "file"}:${row.path}`}
              className="flex min-w-0 items-center gap-2 py-1 pr-2"
              style={{ paddingLeft: `${row.depth * 16 + 8}px` }}
            >
              <Icon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
              <span className="min-w-0 flex-1 truncate">{row.name}</span>
              {!row.isFolder ? (
                <span className="shrink-0 text-caption text-muted-foreground">
                  {formatHubSize(sizes.get(row.path) ?? 0)}
                </span>
              ) : null}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
