"use client";

import { useRef, useState, type ChangeEvent } from "react";
import { FileArchive, FolderOpen, Loader2, Upload } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import type { PreparedSkillArchive } from "@multica/core/skills/pack-archive";
import { useT } from "../i18n";
import { errorMessage, type HubImportRequest } from "./hub-import-request";
import { filesFromDrop, prepareImportFiles } from "./hub-upload-drop";

/** Zip, .skill archive or folder. Reads the same way the skill import does, then previews. */
export function HubImportUpload({
  onPreview,
}: {
  onPreview: (request: HubImportRequest) => Promise<void>;
}) {
  const { t } = useT("hub");
  const zipRef = useRef<HTMLInputElement>(null);
  const folderRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const bindDirectoryInput = (el: HTMLInputElement | null) => {
    folderRef.current = el;
    if (!el) return;
    el.setAttribute("webkitdirectory", "");
    el.setAttribute("directory", "");
  };

  const run = async (read: () => Promise<PreparedSkillArchive>) => {
    setBusy(true);
    setError("");
    try {
      const prepared = await read();
      if (!prepared.ok) {
        setError(t(($) => $.add.upload.errors[prepared.error]));
        return;
      }
      await onPreview({ kind: "file", file: prepared.file });
    } catch (err) {
      setError(errorMessage(err, t(($) => $.add.upload.read_failed)));
    } finally {
      setBusy(false);
    }
  };

  const fromFiles = (files: File[]) => run(() => prepareImportFiles(files));

  const onPicked = (event: ChangeEvent<HTMLInputElement>) => {
    // Snapshot before clearing: a live FileList empties with the input value.
    const files = Array.from(event.target.files ?? []);
    event.target.value = "";
    if (files.length > 0) void fromFiles(files);
  };

  return (
    <div className="flex flex-col gap-4">
      <div
        onDragOver={(event) => {
          event.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(event) => {
          event.preventDefault();
          setDragging(false);
          if (busy) return;
          void run(async () => prepareImportFiles(await filesFromDrop(event.dataTransfer)));
        }}
        data-dragging={dragging || undefined}
        className="flex flex-col items-center gap-3 rounded-lg border border-dashed px-6 py-8 text-center data-dragging:border-primary data-dragging:bg-muted/40"
      >
        {busy ? (
          <Loader2 className="size-5 animate-spin text-muted-foreground" aria-hidden="true" />
        ) : (
          <Upload className="size-5 text-muted-foreground" aria-hidden="true" />
        )}
        <p className="text-label text-muted-foreground">
          {busy ? t(($) => $.add.upload.reading) : t(($) => $.add.upload.drop)}
        </p>
        <div className="flex flex-wrap justify-center gap-2">
          <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => zipRef.current?.click()}>
            <FileArchive aria-hidden="true" />
            {t(($) => $.add.upload.choose_archive)}
          </Button>
          <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => folderRef.current?.click()}>
            <FolderOpen aria-hidden="true" />
            {t(($) => $.add.upload.choose_folder)}
          </Button>
        </div>
        {/* Hidden pickers stay mounted so the button click keeps its user gesture. */}
        <input
          ref={zipRef}
          type="file"
          accept=".zip,.skill,application/zip"
          className="sr-only"
          tabIndex={-1}
          aria-hidden
          onChange={onPicked}
        />
        <input ref={bindDirectoryInput} type="file" multiple className="sr-only" tabIndex={-1} aria-hidden onChange={onPicked} />
      </div>

      {error ? (
        <p role="alert" className="text-caption text-destructive">
          {error}
        </p>
      ) : null}
    </div>
  );
}
