"use client";

import { useId, useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Loader2 } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import {
  HubSourceInUseError,
  supportsHubCredential,
  useCreateHubSource,
  useDeleteHubSource,
  useUpdateHubSource,
  type HubSource,
  type HubSourceType,
  type UpdateHubSourceRequest,
} from "@multica/core/hub";
import { useT } from "../i18n";
import { errorMessage, parseWebAddress } from "./hub-import-request";
import { USER_SOURCE_TYPES, isKnownSourceType } from "./hub-source-meta";
import { AUTO_SYNC_INTERVALS, useAutoSyncLabel } from "./hub-source-sync";

export function HubSourceFormDialog({
  wsId,
  source,
  open,
  onClose,
}: {
  wsId: string;
  /** Null adds a source; a source edits it. */
  source: HubSource | null;
  open: boolean;
  onClose: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent>
        <SourceForm key={source?.id ?? "new"} wsId={wsId} source={source} onClose={onClose} />
      </DialogContent>
    </Dialog>
  );
}

/**
 * What the form does with the stored token on save. The token itself is write-only: it is
 * typed into the field, sent once, and never read back from the server.
 */
type CredentialAction = "keep" | "replace" | "remove";

function SourceForm({ wsId, source, onClose }: { wsId: string; source: HubSource | null; onClose: () => void }) {
  const { t } = useT("hub");
  const create = useCreateHubSource();
  const update = useUpdateHubSource();
  const ids = { name: useId(), type: useId(), url: useId(), interval: useId(), token: useId() };
  const [name, setName] = useState(source?.name ?? "");
  const [type, setType] = useState<HubSourceType>(source?.type ?? USER_SOURCE_TYPES[0]!);
  const [url, setUrl] = useState(source?.url ?? "");
  const [autoSync, setAutoSync] = useState(String(source?.auto_sync_interval_sec ?? 0));
  const intervalLabel = useAutoSyncLabel();
  const [token, setToken] = useState("");
  const [credentialAction, setCredentialAction] = useState<CredentialAction>("keep");
  const [fieldErrors, setFieldErrors] = useState<{ name?: string; url?: string; token?: string }>({});
  const [error, setError] = useState("");
  const pending = create.isPending || update.isPending;
  const takesToken = supportsHubCredential(type);
  const hasSavedToken = source?.has_credential === true;
  // The input shows for a new token: on create, or after Replace on an edit.
  const showTokenInput = takesToken && (!hasSavedToken || credentialAction === "replace");

  // An edit keeps the type: the server ties a source's config to its type.
  const typeOptions: HubSourceType[] = source ? [source.type] : [...USER_SOURCE_TYPES];
  const typeItems = typeOptions.map((value) => ({
    value,
    label: isKnownSourceType(value) ? t(($) => $.source_types[value]) : value,
  }));

  const intervalItems = AUTO_SYNC_INTERVALS.map((seconds) => ({
    value: String(seconds),
    label: intervalLabel(seconds),
  }));
  // A value set by another client or a newer backend keeps its own label.
  if (!intervalItems.some((item) => item.value === autoSync)) {
    intervalItems.push({ value: autoSync, label: intervalLabel(Number(autoSync)) });
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const trimmedName = name.trim();
    const trimmedUrl = url.trim();
    const trimmedToken = token.trim();
    const nextErrors: { name?: string; url?: string; token?: string } = {};
    if (!trimmedName) nextErrors.name = t(($) => $.sources.form.errors.name_required);
    if (!parseWebAddress(trimmedUrl)) nextErrors.url = t(($) => $.sources.form.errors.url_invalid);
    if (showTokenInput && credentialAction === "replace" && !trimmedToken) {
      nextErrors.token = t(($) => $.sources.form.errors.token_required);
    }
    setFieldErrors(nextErrors);
    if (nextErrors.name || nextErrors.url || nextErrors.token) return;

    setError("");
    try {
      if (source) {
        const patch: UpdateHubSourceRequest = { name: trimmedName, url: trimmedUrl };
        // Sent only when changed, so saving the other fields leaves the schedule alone.
        const nextInterval = Number(autoSync);
        if (nextInterval !== source.auto_sync_interval_sec) patch.auto_sync_interval_sec = nextInterval;
        if (takesToken && credentialAction === "replace") patch.credential = trimmedToken;
        if (takesToken && credentialAction === "remove") patch.clear_credential = true;
        await update.mutateAsync({ wsId, id: source.id, ...patch });
        toast.success(t(($) => $.sources.toast.updated));
      } else {
        // Sent only when auto-sync is on: a new source without a schedule needs no field.
        const interval = Number(autoSync);
        await create.mutateAsync({
          wsId,
          name: trimmedName,
          type,
          url: trimmedUrl,
          ...(interval > 0 ? { auto_sync_interval_sec: interval } : {}),
          ...(takesToken && trimmedToken ? { credential: trimmedToken } : {}),
        });
        toast.success(t(($) => $.sources.toast.added));
      }
      setToken("");
      onClose();
    } catch (err) {
      // Stays open with the input intact so the user can correct and retry.
      setError(errorMessage(err, t(($) => $.sources.form.save_failed)));
    }
  };

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>
          {source ? t(($) => $.sources.form.edit_title) : t(($) => $.sources.form.add_title)}
        </DialogTitle>
      </DialogHeader>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor={ids.name} className="text-caption text-muted-foreground">
          {t(($) => $.sources.form.name)}
        </Label>
        <Input
          id={ids.name}
          value={name}
          onChange={(event) => setName(event.target.value)}
          aria-invalid={fieldErrors.name ? true : undefined}
          autoComplete="off"
        />
        {fieldErrors.name ? <p className="text-caption text-destructive">{fieldErrors.name}</p> : null}
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor={ids.type} className="text-caption text-muted-foreground">
          {t(($) => $.sources.form.type)}
        </Label>
        <Select
          items={typeItems}
          value={type}
          disabled={source !== null}
          onValueChange={(next) => next && setType(next as HubSourceType)}
        >
          <SelectTrigger id={ids.type} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {typeItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor={ids.url} className="text-caption text-muted-foreground">
          {t(($) => $.sources.form.url)}
        </Label>
        <Input
          id={ids.url}
          type="text"
          inputMode="url"
          spellCheck={false}
          autoComplete="off"
          value={url}
          onChange={(event) => setUrl(event.target.value)}
          placeholder={t(($) => $.sources.form.url_placeholder)}
          aria-invalid={fieldErrors.url ? true : undefined}
        />
        {fieldErrors.url ? <p className="text-caption text-destructive">{fieldErrors.url}</p> : null}
      </div>

      {takesToken ? (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={ids.token} className="text-caption text-muted-foreground">
            {t(($) => $.sources.form.token_label)}
          </Label>
          {hasSavedToken && credentialAction === "keep" ? (
            <div className="flex flex-wrap items-center gap-2 text-label">
              <span className="text-muted-foreground">{t(($) => $.sources.form.token_saved)}</span>
              <Button type="button" variant="outline" size="sm" disabled={pending} onClick={() => setCredentialAction("replace")}>
                {t(($) => $.sources.form.token_replace)}
              </Button>
              <Button type="button" variant="ghost" size="sm" disabled={pending} onClick={() => setCredentialAction("remove")}>
                {t(($) => $.sources.form.token_remove)}
              </Button>
            </div>
          ) : hasSavedToken && credentialAction === "remove" ? (
            <div className="flex flex-wrap items-center gap-2 text-label">
              <span className="text-muted-foreground">{t(($) => $.sources.form.token_removing)}</span>
              <Button type="button" variant="ghost" size="sm" disabled={pending} onClick={() => setCredentialAction("keep")}>
                {t(($) => $.sources.form.token_keep)}
              </Button>
            </div>
          ) : showTokenInput ? (
            <div className="flex flex-col gap-1.5">
              <Input
                id={ids.token}
                type="password"
                autoComplete="new-password"
                spellCheck={false}
                value={token}
                onChange={(event) => setToken(event.target.value)}
                aria-invalid={fieldErrors.token ? true : undefined}
                aria-describedby={`${ids.token}-help`}
              />
              {fieldErrors.token ? <p className="text-caption text-destructive">{fieldErrors.token}</p> : null}
              {hasSavedToken ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="self-start"
                  disabled={pending}
                  onClick={() => {
                    setToken("");
                    setCredentialAction("keep");
                  }}
                >
                  {t(($) => $.sources.form.token_keep)}
                </Button>
              ) : null}
            </div>
          ) : null}
          <p id={`${ids.token}-help`} className="text-caption text-muted-foreground">
            {t(($) => $.sources.form.token_help)}
          </p>
        </div>
      ) : null}

      <div className="flex flex-col gap-1.5">
        <Label htmlFor={ids.interval} className="text-caption text-muted-foreground">
          {t(($) => $.sync.interval.label)}
        </Label>
        <Select
          items={intervalItems}
          value={autoSync}
          disabled={pending}
          onValueChange={(next) => next && setAutoSync(next)}
        >
          <SelectTrigger id={ids.interval} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {intervalItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {error ? (
        <p role="alert" className="text-caption text-destructive">
          {error}
        </p>
      ) : null}

      <DialogFooter>
        <DialogClose render={<Button variant="outline" disabled={pending} />}>
          {t(($) => $.sources.cancel)}
        </DialogClose>
        <Button type="submit" disabled={pending} aria-busy={pending}>
          {pending ? <Loader2 className="animate-spin" aria-hidden="true" /> : null}
          {source ? t(($) => $.sources.form.save) : t(($) => $.sources.form.create)}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function HubSourceDeleteDialog({
  wsId,
  source,
  onClose,
}: {
  wsId: string;
  source: HubSource | null;
  onClose: () => void;
}) {
  return (
    <Dialog open={source !== null} onOpenChange={(next) => !next && onClose()}>
      <DialogContent>
        {source ? <DeleteSourceBody key={source.id} wsId={wsId} source={source} onClose={onClose} /> : null}
      </DialogContent>
    </Dialog>
  );
}

function DeleteSourceBody({ wsId, source, onClose }: { wsId: string; source: HubSource; onClose: () => void }) {
  const { t } = useT("hub");
  const remove = useDeleteHubSource();
  const radioName = useId();
  const [mode, setMode] = useState<"keep" | "delete">("keep");
  const [inUse, setInUse] = useState<HubSourceInUseError["inUse"]>([]);
  const [error, setError] = useState("");

  const confirm = async () => {
    setError("");
    setInUse([]);
    try {
      await remove.mutateAsync({ wsId, id: source.id, items: mode });
      toast.success(t(($) => $.sources.toast.removed, { name: source.name }));
      onClose();
    } catch (err) {
      // Keeps the dialog open: the user can switch to "keep" and confirm.
      if (err instanceof HubSourceInUseError) {
        setInUse(err.inUse);
      } else {
        setError(errorMessage(err, t(($) => $.sources.delete.failed)));
      }
    }
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t(($) => $.sources.delete.title, { name: source.name })}</DialogTitle>
      </DialogHeader>

      <fieldset className="flex flex-col gap-3">
        <legend className="mb-1 text-caption text-muted-foreground">
          {t(($) => $.sources.delete.items_legend, { count: source.item_count })}
        </legend>
        <label className="flex items-start gap-2 text-label">
          <input
            type="radio"
            name={radioName}
            value="keep"
            checked={mode === "keep"}
            onChange={() => setMode("keep")}
            className="mt-1 accent-primary"
          />
          <span className="flex flex-col">
            {t(($) => $.sources.delete.keep)}
            <span className="text-caption text-muted-foreground">{t(($) => $.sources.delete.keep_hint)}</span>
          </span>
        </label>
        <label className="flex items-start gap-2 text-label">
          <input
            type="radio"
            name={radioName}
            value="delete"
            checked={mode === "delete"}
            onChange={() => setMode("delete")}
            className="mt-1 accent-primary"
          />
          <span className="flex flex-col">
            {t(($) => $.sources.delete.delete_items)}
            <span className="text-caption text-muted-foreground">{t(($) => $.sources.delete.delete_hint)}</span>
          </span>
        </label>
      </fieldset>

      {inUse.length > 0 ? (
        <div role="alert" className="flex flex-col gap-2 rounded-md bg-destructive/10 px-3 py-2 text-caption text-destructive">
          <p>{t(($) => $.sources.delete.in_use)}</p>
          <ul className="flex max-h-40 flex-col gap-1 overflow-y-auto text-foreground">
            {inUse.map((item) => (
              <li key={`${item.kind}:${item.id}`} className="truncate">
                {item.name}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {error ? (
        <p role="alert" className="text-caption text-destructive">
          {error}
        </p>
      ) : null}

      <DialogFooter>
        <DialogClose render={<Button variant="outline" disabled={remove.isPending} />}>
          {t(($) => $.sources.cancel)}
        </DialogClose>
        <Button
          type="button"
          variant={mode === "delete" ? "destructive" : "default"}
          disabled={remove.isPending}
          aria-busy={remove.isPending}
          onClick={() => void confirm()}
        >
          {remove.isPending ? <Loader2 className="animate-spin" aria-hidden="true" /> : null}
          {t(($) => $.sources.delete.confirm)}
        </Button>
      </DialogFooter>
    </>
  );
}
