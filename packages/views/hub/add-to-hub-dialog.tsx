"use client";

import { useId, useState, type FormEvent } from "react";
import { FileArchive, Loader2, Plus } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { usePreviewHubImport, type HubImportCommitResult, type HubImportPreview } from "@multica/core/hub";
import { useNavigation } from "../navigation";
import { useT } from "../i18n";
import { HubImportPreviewStep } from "./hub-import-preview";
import { HubImportRegistry } from "./hub-import-registry";
import { HubImportUpload } from "./hub-import-upload";
import {
  errorMessage,
  importNeedsWebAddress,
  parseWebAddress,
  previewVars,
  type HubImportRequest,
  type HubImportState,
  type UrlImportType,
} from "./hub-import-request";

type ImportTab = "url" | "registry" | "upload" | "blank";

const URL_TYPES: readonly UrlImportType[] = ["github", "url_zip", "skills_sh", "clawhub"];

/**
 * Every import path ends in the preview step: the wizard owns the previewed
 * request so an expired preview can be run again and the tabs keep their input.
 */
export function AddToHubDialog({ onBlank }: { onBlank: () => void }) {
  const { t } = useT("hub");
  const wsId = useWorkspaceId();
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<ImportTab>("url");
  const [state, setState] = useState<HubImportState | null>(null);
  const previewMutation = usePreviewHubImport();
  const navigation = useNavigation();
  const paths = useWorkspacePaths();

  const reset = () => {
    setTab("url");
    setState(null);
  };

  const handleOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) reset();
  };

  // Rejects with the error so the calling form can show it beside its input.
  const preview = async (request: HubImportRequest) => {
    const next = await previewMutation.mutateAsync(previewVars(wsId, request));
    setState({ request, preview: next });
  };

  const replacePreview = (next: HubImportPreview) => {
    setState((current) => (current ? { ...current, preview: next } : current));
  };

  const finish = (result: HubImportCommitResult) => {
    handleOpenChange(false);
    navigation.push(paths.hubItem(result.kind === "plugin" ? "plugin" : "skill", result.id));
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button size="sm" />}>
        <Plus aria-hidden="true" />
        {t(($) => $.add.button)}
      </DialogTrigger>
      <DialogContent className="flex max-h-[85vh] flex-col sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{state ? t(($) => $.add.review_title) : t(($) => $.add.title)}</DialogTitle>
        </DialogHeader>

        {state ? (
          <HubImportPreviewStep
            key={state.preview.preview_id}
            wsId={wsId}
            request={state.request}
            preview={state.preview}
            onBack={() => setState(null)}
            onPreviewReplaced={replacePreview}
            onFinished={finish}
          />
        ) : null}

        {/* Kept mounted while previewing so the forms keep their input on Back. */}
        <Tabs
          value={tab}
          onValueChange={(next) => setTab(next as ImportTab)}
          className={state ? "hidden" : "min-h-0 flex-1 gap-4"}
        >
          <TabsList className="w-full justify-start overflow-x-auto">
            <TabsTrigger value="url">{t(($) => $.add.tabs.url)}</TabsTrigger>
            <TabsTrigger value="registry">{t(($) => $.add.tabs.registry)}</TabsTrigger>
            <TabsTrigger value="upload">{t(($) => $.add.tabs.upload)}</TabsTrigger>
            <TabsTrigger value="blank">{t(($) => $.add.tabs.blank)}</TabsTrigger>
          </TabsList>
          <TabsContent value="url">
            <UrlImportForm onPreview={preview} />
          </TabsContent>
          <TabsContent value="registry">
            <HubImportRegistry onPreview={preview} />
          </TabsContent>
          <TabsContent value="upload">
            <HubImportUpload onPreview={preview} />
          </TabsContent>
          <TabsContent value="blank">
            <div className="flex flex-col items-start gap-3">
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  handleOpenChange(false);
                  onBlank();
                }}
              >
                {t(($) => $.add.blank.create)}
              </Button>
            </div>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

function UrlImportForm({ onPreview }: { onPreview: (request: HubImportRequest) => Promise<void> }) {
  const { t } = useT("hub");
  const ids = { type: useId(), url: useId(), ref: useId(), subdir: useId() };
  const [type, setType] = useState<UrlImportType>("github");
  const [url, setUrl] = useState("");
  const [ref, setRef] = useState("");
  const [subdir, setSubdir] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const trimmed = url.trim();
    if (!trimmed) return;
    if (importNeedsWebAddress(type) && !parseWebAddress(trimmed)) {
      setError(t(($) => $.add.url.invalid_url));
      return;
    }
    const isGithub = type === "github";
    setBusy(true);
    setError("");
    try {
      await onPreview({
        kind: "source",
        source: {
          type,
          url: trimmed,
          ...(isGithub && ref.trim() ? { ref: ref.trim() } : {}),
          ...(isGithub && subdir.trim() ? { subdir: subdir.trim() } : {}),
        },
      });
    } catch (err) {
      setError(errorMessage(err, t(($) => $.add.url.preview_failed)));
    } finally {
      setBusy(false);
    }
  };

  const typeItems = URL_TYPES.map((value) => ({
    value,
    label: t(($) => $.add.url.types[value]),
  }));

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <div className="grid gap-3 sm:grid-cols-[10rem_1fr]">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={ids.type} className="text-caption text-muted-foreground">
            {t(($) => $.add.url.type_label)}
          </Label>
          <Select
            items={typeItems}
            value={type}
            onValueChange={(next) => next && setType(next as UrlImportType)}
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
        <div className="flex min-w-0 flex-col gap-1.5">
          <Label htmlFor={ids.url} className="text-caption text-muted-foreground">
            {t(($) => $.add.url.url_label)}
          </Label>
          <Input
            id={ids.url}
            type="text"
            inputMode="url"
            autoComplete="off"
            spellCheck={false}
            value={url}
            onChange={(event) => setUrl(event.target.value)}
            placeholder={t(($) => $.add.url.url_placeholder[type])}
            aria-invalid={error ? true : undefined}
          />
        </div>
      </div>

      {type === "github" ? (
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={ids.ref} className="text-caption text-muted-foreground">
              {t(($) => $.add.url.ref_label)}
            </Label>
            <Input
              id={ids.ref}
              value={ref}
              onChange={(event) => setRef(event.target.value)}
              placeholder={t(($) => $.add.url.ref_placeholder)}
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={ids.subdir} className="text-caption text-muted-foreground">
              {t(($) => $.add.url.subdir_label)}
            </Label>
            <Input
              id={ids.subdir}
              value={subdir}
              onChange={(event) => setSubdir(event.target.value)}
              placeholder={t(($) => $.add.url.subdir_placeholder)}
              autoComplete="off"
              spellCheck={false}
            />
          </div>
        </div>
      ) : null}

      {error ? (
        <p role="alert" className="text-caption text-destructive">
          {error}
        </p>
      ) : null}

      <DialogFooter>
        <Button type="submit" disabled={busy || !url.trim()} aria-busy={busy}>
          {busy ? <Loader2 className="animate-spin" aria-hidden="true" /> : <FileArchive aria-hidden="true" />}
          {t(($) => $.add.url.submit)}
        </Button>
      </DialogFooter>
    </form>
  );
}
