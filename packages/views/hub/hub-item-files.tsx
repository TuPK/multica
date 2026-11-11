"use client";

import { useState, type FormEvent } from "react";
import { FilePlus, FileText, FolderPlus, Loader2, Plus, Save, Undo2 } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@multica/ui/components/ui/resizable";
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
import { useIsMobile } from "@multica/ui/hooks/use-mobile";
import type { HubItemKind } from "@multica/core/hub";
import { useT } from "../i18n";
import { HubFileTree } from "./hub-file-tree";
import { HubFileViewer, HubViewModeToggle, type HubViewMode } from "./hub-file-viewer";
import { folderMembers, previewKind, type HubPathError } from "./hub-files";
import { PublishPluginDialog, UnsavedChangesDialog, SaveConflictDialog, DeletePathDialog, type DeleteTarget } from "./hub-item-dialogs";
import type { HubFilesState } from "./use-hub-files";

export interface HubItemFilesProps {
  kind: HubItemKind;
  canEdit: boolean;
  files: HubFilesState;
  /** The file that cannot be renamed or deleted: SKILL.md, or the plugin manifest. */
  reservedPath: string;
  /** Plugins: the newest published version, shown in the publish dialog. */
  currentVersion: string | null;
  publishedVersions: readonly string[];
}

type CreateKind = "file" | "folder";

export function HubItemFiles({
  kind,
  canEdit,
  files,
  reservedPath,
  currentVersion,
  publishedVersions,
}: HubItemFilesProps) {
  const { t } = useT("hub");
  const isMobile = useIsMobile();
  const isPlugin = kind === "plugin";
  const [mode, setMode] = useState<HubViewMode>("preview");
  const [creating, setCreating] = useState<CreateKind | null>(null);
  const [createValue, setCreateValue] = useState("");
  const [createError, setCreateError] = useState<HubPathError | null>(null);
  const [pendingDelete, setPendingDelete] = useState<DeleteTarget | null>(null);
  const [publishOpen, setPublishOpen] = useState(false);
  const [discardDraftOpen, setDiscardDraftOpen] = useState(false);

  const previewable = ["markdown", "json", "svg"].includes(
    previewKind(files.selectedPath, files.file.isBinary),
  );
  const draftCount = Object.keys(files.pluginChanges).length;
  const errorMessage = (code: HubPathError) => t(($) => $.files.errors[code]);

  const submitCreate = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const error =
      creating === "folder" ? files.createFolder(createValue) : files.createFile(createValue);
    if (error) {
      setCreateError(error);
      return;
    }
    setCreating(null);
    setCreateValue("");
    setCreateError(null);
  };

  const cancelCreate = () => {
    setCreating(null);
    setCreateValue("");
    setCreateError(null);
  };

  const tree = (() => {
    if (files.listLoading) {
      return (
        <div className="flex flex-col gap-2 p-3" aria-busy="true">
          <Skeleton className="h-4 w-4/5" />
          <Skeleton className="h-4 w-3/5" />
          <Skeleton className="h-4 w-2/3" />
        </div>
      );
    }
    if (files.listError) {
      return (
        <div className="flex flex-col items-start gap-2 p-3 text-caption">
          <p className="text-destructive">{t(($) => $.files.list_error)}</p>
          <Button type="button" variant="outline" size="sm" onClick={files.retryList}>
            {t(($) => $.retry)}
          </Button>
        </div>
      );
    }
    if (files.paths.length === 0) {
      return <p className="p-3 text-caption text-muted-foreground">{t(($) => $.files.empty)}</p>;
    }
    return (
      <HubFileTree
        paths={files.paths}
        selectedPath={files.selectedPath}
        dirtyPaths={files.dirtyPaths}
        onSelect={files.select}
        actions={
          canEdit
            ? {
                reservedPath,
                onRename: files.renameEntry,
                onDelete: (path, isFolder) =>
                  setPendingDelete({
                    path,
                    isFolder,
                    fileCount: isFolder ? folderMembers(files.paths, path).length : 1,
                    isPluginDraft: isPlugin,
                  }),
              }
            : undefined
        }
      />
    );
  })();

  const editor = (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b px-3 sm:px-4">
        {files.selectedPath ? (
          <>
            <span className="min-w-0 truncate font-mono text-caption text-muted-foreground" title={files.selectedPath}>
              {files.selectedPath}
            </span>
            {files.isSelectedDirty && (
              <span className="flex shrink-0 items-center gap-1.5 text-caption text-muted-foreground">
                <span className="size-1.5 rounded-full bg-brand" aria-hidden="true" />
                {t(($) => $.files.dirty)}
              </span>
            )}
          </>
        ) : null}
        <div className="ml-auto flex shrink-0 items-center gap-1.5">
          {previewable && (
            <HubViewModeToggle mode={mode} editable={canEdit} onChange={setMode} />
          )}
          {!isPlugin && canEdit && files.isSelectedDirty && (
            <>
              <Button type="button" variant="ghost" size="sm" onClick={files.discardSelected}>
                <Undo2 aria-hidden="true" />
                {t(($) => $.files.discard)}
              </Button>
              <Button
                type="button"
                size="sm"
                disabled={files.saving}
                aria-busy={files.saving}
                onClick={files.saveSelected}
              >
                {files.saving ? (
                  <Loader2 className="animate-spin" aria-hidden="true" />
                ) : (
                  <Save aria-hidden="true" />
                )}
                {t(($) => $.files.save)}
              </Button>
            </>
          )}
        </div>
      </div>
      <div className="min-h-0 flex-1">
        {files.selectedPath ? (
          <HubFileViewer
            path={files.selectedPath}
            content={files.file.content}
            isBinary={files.file.isBinary}
            isLoading={files.file.isLoading}
            isError={files.file.isError}
            mode={mode}
            editable={canEdit && !files.file.isBinary}
            onChange={files.setContent}
            onRetry={files.file.retry}
          />
        ) : (
          <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center text-muted-foreground">
            <FileText className="size-8 text-faint-foreground" aria-hidden="true" />
            <p className="max-w-sm text-caption text-muted-foreground">{t(($) => $.files.select)}</p>
          </div>
        )}
      </div>
    </div>
  );

  return (
    <div className="flex min-h-full flex-col md:h-full">
      {isPlugin && (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b px-4 py-2.5">
          <div className="min-w-0 flex-1 text-caption text-muted-foreground">
            <p>{t(($) => $.files.plugin_banner)}</p>
            {!canEdit && <p>{t(($) => $.files.plugin_readonly)}</p>}
          </div>
          {canEdit && files.isDirty && (
            <div className="flex shrink-0 items-center gap-2">
              <span className="text-caption font-medium">
                {t(($) => $.files.draft_count, { count: draftCount })}
              </span>
              <Button type="button" variant="ghost" size="sm" onClick={() => setDiscardDraftOpen(true)}>
                {t(($) => $.files.draft_discard)}
              </Button>
              <Button
                type="button"
                size="sm"
                disabled={files.publishing}
                onClick={() => setPublishOpen(true)}
              >
                {t(($) => $.files.draft_publish)}
              </Button>
            </div>
          )}
        </div>
      )}

      <ResizablePanelGroup
        orientation={isMobile ? "vertical" : "horizontal"}
        className="min-h-[32rem] flex-1 md:min-h-0"
      >
        <ResizablePanel
          defaultSize={isMobile ? "40%" : "28%"}
          minSize={isMobile ? "20%" : "16%"}
          maxSize={isMobile ? "70%" : "50%"}
        >
          <aside className="flex h-full min-h-0 flex-col border-r border-surface-border">
            <div className="flex h-10 shrink-0 items-center gap-1 px-2.5">
              <p className="text-micro font-semibold uppercase tracking-wider text-muted-foreground">
                {t(($) => $.files.list)}
              </p>
              {canEdit && (
                <div className="ml-auto flex items-center">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    aria-label={t(($) => $.files.new_file)}
                    title={t(($) => $.files.new_file)}
                    onClick={() => {
                      setCreating("file");
                      setCreateError(null);
                    }}
                  >
                    <FilePlus aria-hidden="true" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    aria-label={t(($) => $.files.new_folder)}
                    title={t(($) => $.files.new_folder)}
                    onClick={() => {
                      setCreating("folder");
                      setCreateError(null);
                    }}
                  >
                    <FolderPlus aria-hidden="true" />
                  </Button>
                </div>
              )}
            </div>
            {creating && (
              <form onSubmit={submitCreate} noValidate className="mx-2 mb-2 flex flex-col gap-1.5 rounded-md border bg-muted/30 p-2">
                <Input
                  autoFocus
                  value={createValue}
                  aria-label={creating === "folder" ? t(($) => $.files.new_folder) : t(($) => $.files.new_file)}
                  placeholder={
                    creating === "folder"
                      ? t(($) => $.files.folder_placeholder)
                      : t(($) => $.files.file_placeholder)
                  }
                  aria-invalid={createError !== null}
                  onChange={(event) => {
                    setCreateValue(event.target.value);
                    setCreateError(null);
                  }}
                  onKeyDown={(event) => {
                    if (event.key === "Escape") cancelCreate();
                  }}
                  className="h-7 bg-background font-mono text-caption"
                />
                {createError && (
                  <p role="alert" className="text-caption text-destructive">
                    {errorMessage(createError)}
                  </p>
                )}
                <div className="flex items-center gap-1.5 pt-0.5">
                  <Button type="submit" size="xs">
                    <Plus aria-hidden="true" />
                    {t(($) => $.files.create)}
                  </Button>
                  <Button type="button" size="xs" variant="ghost" onClick={cancelCreate}>
                    {t(($) => $.files.cancel)}
                  </Button>
                </div>
              </form>
            )}
            <div className="min-h-0 flex-1">{tree}</div>
          </aside>
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel defaultSize={isMobile ? "60%" : "72%"} minSize="40%">
          <section className="h-full min-w-0">{editor}</section>
        </ResizablePanel>
      </ResizablePanelGroup>

      <UnsavedChangesDialog
        path={files.guardOpen ? files.selectedPath : null}
        saving={files.saving}
        onChoose={files.resolveGuard}
      />
      <SaveConflictDialog
        path={files.conflictPath}
        busy={files.saving}
        onChoose={files.resolveConflict}
        onDismiss={files.dismissConflict}
      />
      <DeletePathDialog
        target={pendingDelete}
        onClose={() => setPendingDelete(null)}
        onConfirm={(target) => {
          setPendingDelete(null);
          files.deleteEntry(target.path, target.isFolder);
        }}
      />
      {isPlugin && (
        <>
          <PublishPluginDialog
            open={publishOpen}
            currentVersion={currentVersion}
            existingVersions={publishedVersions}
            publishing={files.publishing}
            onOpenChange={setPublishOpen}
            onPublish={files.publish}
          />
          <AlertDialog open={discardDraftOpen} onOpenChange={setDiscardDraftOpen}>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>{t(($) => $.files.discard_draft_title)}</AlertDialogTitle>
                <AlertDialogDescription>{t(($) => $.files.discard_draft_description)}</AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>{t(($) => $.files.cancel)}</AlertDialogCancel>
                <AlertDialogAction
                  variant="destructive"
                  onClick={() => {
                    files.discardAll();
                    setDiscardDraftOpen(false);
                  }}
                >
                  {t(($) => $.files.draft_discard)}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </>
      )}
    </div>
  );
}
