"use client";

import { useState, type FormEvent } from "react";
import { Loader2 } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { Input } from "@multica/ui/components/ui/input";
import { useT } from "../i18n";
import { isSemanticVersion } from "./hub-files";
import type { ConflictChoice, GuardChoice } from "./use-hub-files";

/** Unsaved skill text is about to be dropped by a file switch or a rename. */
export function UnsavedChangesDialog({
  path,
  saving,
  onChoose,
}: {
  /** The file with unsaved text, or null when the dialog is closed. */
  path: string | null;
  saving: boolean;
  onChoose: (choice: GuardChoice) => void;
}) {
  const { t } = useT("hub");
  return (
    <Dialog open={path !== null} onOpenChange={(open) => !open && onChoose("keep")}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t(($) => $.guard.title, { path: path ?? "" })}</DialogTitle>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onChoose("keep")}>
            {t(($) => $.guard.keep)}
          </Button>
          <Button type="button" variant="destructive" onClick={() => onChoose("discard")}>
            {t(($) => $.guard.discard)}
          </Button>
          <Button
            type="button"
            disabled={saving}
            aria-busy={saving}
            onClick={() => onChoose("save")}
          >
            {saving && <Loader2 className="animate-spin" aria-hidden="true" />}
            {t(($) => $.guard.save)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** A save failed because the file changed on the server since it was loaded. */
export function SaveConflictDialog({
  path,
  busy,
  onChoose,
  onDismiss,
}: {
  path: string | null;
  busy: boolean;
  onChoose: (choice: ConflictChoice) => void;
  onDismiss: () => void;
}) {
  const { t } = useT("hub");
  return (
    <Dialog open={path !== null} onOpenChange={(open) => !open && onDismiss()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t(($) => $.conflict.title, { path: path ?? "" })}</DialogTitle>
          <DialogDescription>{t(($) => $.conflict.description)}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" disabled={busy} onClick={() => onChoose("overwrite")}>
            {t(($) => $.conflict.overwrite)}
          </Button>
          <Button type="button" disabled={busy} aria-busy={busy} onClick={() => onChoose("reload")}>
            {t(($) => $.conflict.reload)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export interface DeleteTarget {
  path: string;
  isFolder: boolean;
  /** Files under a folder, counted for the description. */
  fileCount: number;
  /** Plugins: the removal is a draft change, not an immediate delete. */
  isPluginDraft: boolean;
}

export function DeletePathDialog({
  target,
  onConfirm,
  onClose,
}: {
  target: DeleteTarget | null;
  onConfirm: (target: DeleteTarget) => void;
  onClose: () => void;
}) {
  const { t } = useT("hub");
  const description = !target
    ? ""
    : target.isPluginDraft
      ? t(($) => $.delete.description_draft)
      : target.isFolder
        ? t(($) => $.delete.description_folder, { count: target.fileCount })
        : t(($) => $.delete.description_file);
  return (
    <AlertDialog open={target !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t(($) => $.delete.title, { path: target?.path ?? "" })}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t(($) => $.files.cancel)}</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={() => target && onConfirm(target)}>
            {t(($) => $.delete.confirm)}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/**
 * Publishes a plugin draft as a new version. The version is typed, checked
 * against the format and the versions already published, and the dialog only
 * closes after the publish succeeds, so a failed attempt keeps the input.
 */
export function PublishPluginDialog({
  open,
  currentVersion,
  existingVersions,
  publishing,
  onOpenChange,
  onPublish,
}: {
  open: boolean;
  currentVersion: string | null;
  existingVersions: readonly string[];
  publishing: boolean;
  onOpenChange: (open: boolean) => void;
  onPublish: (version: string) => Promise<boolean>;
}) {
  const { t } = useT("hub");
  const [version, setVersion] = useState("");
  const [error, setError] = useState<string | null>(null);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const next = version.trim();
    if (!isSemanticVersion(next)) {
      setError(t(($) => $.publish.errors.format));
      return;
    }
    if (existingVersions.includes(next)) {
      setError(t(($) => $.publish.errors.exists, { version: next }));
      return;
    }
    setError(null);
    const ok = await onPublish(next);
    if (ok) {
      setVersion("");
      onOpenChange(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => !publishing && onOpenChange(next)}>
      <DialogContent>
        <form onSubmit={submit} noValidate className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>{t(($) => $.publish.title)}</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-1.5">
            <label htmlFor="hub-publish-version" className="text-caption text-muted-foreground">
              {t(($) => $.publish.version_label)}
            </label>
            <Input
              id="hub-publish-version"
              autoFocus
              value={version}
              placeholder={t(($) => $.publish.version_placeholder)}
              aria-invalid={error !== null}
              aria-describedby={error ? "hub-publish-version-error" : undefined}
              onChange={(event) => {
                setVersion(event.target.value);
                setError(null);
              }}
              className="font-mono"
            />
            {error ? (
              <p id="hub-publish-version-error" role="alert" className="text-caption text-destructive">
                {error}
              </p>
            ) : currentVersion ? (
              <p className="text-caption text-muted-foreground">
                {t(($) => $.publish.current, { version: currentVersion })}
              </p>
            ) : null}
          </div>
          <DialogFooter>
            <DialogClose render={<Button type="button" variant="outline" disabled={publishing} />}>
              {t(($) => $.publish.cancel)}
            </DialogClose>
            <Button type="submit" disabled={publishing || version.trim() === ""} aria-busy={publishing}>
              {publishing && <Loader2 className="animate-spin" aria-hidden="true" />}
              {t(($) => $.publish.confirm)}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
