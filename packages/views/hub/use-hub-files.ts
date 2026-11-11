"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  HubFileConflictError,
  hubFileContentOptions,
  hubFileListOptions,
  useDeleteHubFile,
  useRenameHubFile,
  useSaveHubFile,
  useSavePluginDraft,
  type HubFileContent,
  type HubItemKind,
} from "@multica/core/hub";
import { useT } from "../i18n";
import {
  applyPluginChanges,
  deletePluginChange,
  folderMembers,
  HUB_PRIMARY_FILE,
  renamePluginChange,
  renamePairs,
  validateHubPath,
  type HubPathError,
  type PluginChanges,
} from "./hub-files";

/** A skill file being edited. `baseContent` null means the file does not exist yet. */
export interface HubDraft {
  content: string;
  /** The etag the file had when editing began; "" for a file to be created. */
  baseEtag: string;
  baseContent: string | null;
}

export type ConflictChoice = "reload" | "overwrite";
export type GuardChoice = "save" | "discard" | "keep";

export interface UseHubFilesOptions {
  wsId: string;
  kind: HubItemKind;
  refId: string;
  /** Plugins: the newest published version, which every read and the draft build on. */
  versionId: string | undefined;
  /** False until the plugin's version is known. Skills are always ready. */
  ready: boolean;
  canEdit: boolean;
}

/** Everything the Files tab needs. Owned by the page so drafts survive tab switches. */
export interface HubFilesState {
  paths: string[];
  /** Total size of the listed files, or null while the list is loading. */
  totalBytes: number | null;
  selectedPath: string;
  listLoading: boolean;
  listError: boolean;
  retryList: () => void;
  file: {
    path: string;
    /** Text shown now (the draft, or the server text). Null for binary or too large. */
    content: string | null;
    isBinary: boolean;
    isLoading: boolean;
    isError: boolean;
    retry: () => void;
  };
  /** Skill: the selected file has unsaved text. Plugin: the draft has changes. */
  isSelectedDirty: boolean;
  isDirty: boolean;
  dirtyPaths: ReadonlySet<string>;
  /** Plugin draft: changed or removed paths. Empty for skills. */
  pluginChanges: PluginChanges;
  /** The guard pending for an action that would drop unsaved skill text. */
  guardOpen: boolean;
  conflictPath: string | null;
  saving: boolean;
  publishing: boolean;
  setContent: (value: string) => void;
  select: (path: string) => void;
  resolveGuard: (choice: GuardChoice) => void;
  resolveConflict: (choice: ConflictChoice) => void;
  /** Close the conflict without touching the draft; it stays unsaved. */
  dismissConflict: () => void;
  saveSelected: () => void;
  discardSelected: () => void;
  discardAll: () => void;
  createFile: (path: string) => HubPathError | null;
  createFolder: (name: string) => HubPathError | null;
  renameEntry: (from: string, to: string, isFolder: boolean) => HubPathError | null;
  deleteEntry: (path: string, isFolder: boolean) => void;
  publish: (newVersion: string) => Promise<boolean>;
  /** The draft contents of one skill file, if it has one. */
  draftFor: (path: string) => HubDraft | undefined;
}

function omitKey<T>(record: Record<string, T>, key: string): Record<string, T> {
  const next = { ...record };
  delete next[key];
  return next;
}

export function isHubDraftDirty(draft: HubDraft): boolean {
  return draft.baseContent === null || draft.content !== draft.baseContent;
}

export function useHubFiles({
  wsId,
  kind,
  refId,
  versionId,
  ready,
  canEdit,
}: UseHubFilesOptions): HubFilesState {
  const { t } = useT("hub");
  const qc = useQueryClient();
  const isPlugin = kind === "plugin";
  const listEnabled = ready && (!isPlugin || versionId !== undefined);

  const list = useQuery({
    ...hubFileListOptions(wsId, kind, refId, isPlugin ? versionId : undefined),
    enabled: listEnabled,
  });
  const basePaths = useMemo(() => (list.data ?? []).map((f) => f.path), [list.data]);
  const baseSet = useMemo(() => new Set(basePaths), [basePaths]);

  const [selected, setSelected] = useState(HUB_PRIMARY_FILE);
  const [drafts, setDrafts] = useState<Record<string, HubDraft>>({});
  const [changes, setChanges] = useState<PluginChanges>({});
  const [guardAction, setGuardAction] = useState<(() => void) | null>(null);
  const [conflictPath, setConflictPath] = useState<string | null>(null);
  // The etag the server reported with the conflict, so overwrite does not need another read.
  const conflictEtagRef = useRef<string | null>(null);

  const saveFile = useSaveHubFile();
  const deleteFile = useDeleteHubFile();
  const renameFile = useRenameHubFile();
  const savePlugin = useSavePluginDraft();

  // Skill paths: server files plus files created here but not saved yet.
  const paths = useMemo(() => {
    if (isPlugin) return applyPluginChanges(basePaths, changes);
    const created = Object.keys(drafts).filter((p) => drafts[p]!.baseContent === null);
    return [...new Set([...basePaths, ...created])].sort((a, b) => {
      if (a === HUB_PRIMARY_FILE) return -1;
      if (b === HUB_PRIMARY_FILE) return 1;
      return a.localeCompare(b);
    });
  }, [isPlugin, basePaths, changes, drafts]);

  const selectedPath = paths.includes(selected) ? selected : (paths[0] ?? "");

  const draftFor = useCallback((path: string) => drafts[path], [drafts]);
  const selectedDraft = drafts[selectedPath];
  const selectedIsNewSkill = !isPlugin && selectedDraft?.baseContent === null;
  const selectedIsNewPlugin =
    isPlugin && typeof changes[selectedPath] === "string" && !baseSet.has(selectedPath);

  // The server text of the selected file. Plugins read only paths the base has.
  const contentEnabled =
    listEnabled &&
    selectedPath !== "" &&
    !selectedIsNewSkill &&
    !selectedIsNewPlugin &&
    (!isPlugin || baseSet.has(selectedPath));
  const content = useQuery({
    ...hubFileContentOptions(wsId, kind, refId, selectedPath, isPlugin ? versionId : undefined),
    enabled: contentEnabled,
  });
  const serverFile: HubFileContent | undefined = content.data;

  const displayContent = (() => {
    if (isPlugin) {
      const changed = changes[selectedPath];
      if (typeof changed === "string") return changed;
      return serverFile?.content ?? null;
    }
    if (selectedDraft) return selectedDraft.content;
    return serverFile?.content ?? null;
  })();

  const dirtyPaths = useMemo(() => {
    const set = new Set<string>();
    if (isPlugin) return set;
    for (const [path, draft] of Object.entries(drafts)) {
      if (isHubDraftDirty(draft)) set.add(path);
    }
    return set;
  }, [drafts, isPlugin]);
  const isPluginDraft = isPlugin && Object.keys(changes).length > 0;
  const isDirty = isPlugin ? isPluginDraft : dirtyPaths.size > 0;
  const isSelectedDirty = !isPlugin && dirtyPaths.has(selectedPath);

  // A reload or tab close would drop unsaved text or an unpublished plugin draft.
  useEffect(() => {
    if (!isDirty) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [isDirty]);

  // ---- Editing

  const setContent = useCallback(
    (value: string) => {
      if (!canEdit || !selectedPath) return;
      if (isPlugin) {
        setChanges((prev) => ({ ...prev, [selectedPath]: value }));
        return;
      }
      setDrafts((prev) => {
        const existing = prev[selectedPath];
        if (existing) {
          if (existing.baseContent !== null && value === existing.baseContent) {
            return omitKey(prev, selectedPath);
          }
          return { ...prev, [selectedPath]: { ...existing, content: value } };
        }
        // Editing a server file: the baseline is the text and etag as loaded.
        if (!serverFile || serverFile.content === null) return prev;
        if (value === serverFile.content) return prev;
        return {
          ...prev,
          [selectedPath]: {
            content: value,
            baseEtag: serverFile.etag,
            baseContent: serverFile.content,
          },
        };
      });
    },
    [canEdit, isPlugin, selectedPath, serverFile],
  );

  // ---- Guarded switching

  const guarded = useCallback(
    (action: () => void) => {
      if (isSelectedDirty) setGuardAction(() => action);
      else action();
    },
    [isSelectedDirty],
  );

  const select = useCallback(
    (path: string) => {
      if (path === selectedPath) return;
      guarded(() => setSelected(path));
    },
    [guarded, selectedPath],
  );

  // ---- Server writes (skills)

  const saveDraft = useCallback(
    async (path: string, etag: string, draftContent: string): Promise<boolean> => {
      try {
        await saveFile.mutateAsync({ wsId, refId, path, content: draftContent, etag });
        setDrafts((prev) => omitKey(prev, path));
        toast.success(t(($) => $.toast.saved, { path }));
        return true;
      } catch (error) {
        if (error instanceof HubFileConflictError) {
          conflictEtagRef.current = error.currentEtag;
          setConflictPath(path);
        } else {
          toast.error(t(($) => $.toast.save_failed, { path }));
        }
        return false;
      }
    },
    [saveFile, wsId, refId, t],
  );

  const saveSelected = useCallback(() => {
    const draft = drafts[selectedPath];
    if (!draft) return;
    void saveDraft(selectedPath, draft.baseEtag, draft.content);
  }, [drafts, selectedPath, saveDraft]);

  const discardSelected = useCallback(() => {
    setDrafts((prev) => omitKey(prev, selectedPath));
  }, [selectedPath]);

  const discardAll = useCallback(() => {
    setChanges({});
    setSelected(HUB_PRIMARY_FILE);
  }, []);

  const resolveGuard = useCallback(
    (choice: GuardChoice) => {
      const action = guardAction;
      setGuardAction(null);
      if (!action) return;
      if (choice === "keep") return;
      if (choice === "discard") {
        discardSelected();
        action();
        return;
      }
      const draft = drafts[selectedPath];
      if (!draft) {
        action();
        return;
      }
      void saveDraft(selectedPath, draft.baseEtag, draft.content).then((ok) => {
        if (ok) action();
      });
    },
    [guardAction, discardSelected, drafts, selectedPath, saveDraft],
  );

  const resolveConflict = useCallback(
    async (choice: ConflictChoice) => {
      const path = conflictPath;
      setConflictPath(null);
      if (!path) return;
      if (choice === "reload") {
        conflictEtagRef.current = null;
        setDrafts((prev) => omitKey(prev, path));
        await qc.invalidateQueries({
          queryKey: hubFileContentOptions(wsId, kind, refId, path).queryKey,
        });
        return;
      }
      // Overwrite: save this draft over the newer version, using the etag the server reported.
      const draft = drafts[path];
      if (!draft) return;
      try {
        let etag = conflictEtagRef.current;
        if (etag === null) {
          const fresh = await qc.fetchQuery({
            ...hubFileContentOptions(wsId, kind, refId, path),
            staleTime: 0,
          });
          etag = fresh.etag;
        }
        conflictEtagRef.current = null;
        await saveDraft(path, etag, draft.content);
      } catch {
        toast.error(t(($) => $.toast.save_failed, { path }));
      }
    },
    [conflictPath, drafts, qc, wsId, kind, refId, saveDraft, t],
  );

  // ---- Tree operations

  const createFile = useCallback(
    (path: string): HubPathError | null => {
      const error = validateHubPath(path, paths);
      if (error) return error;
      const target = path.trim();
      guarded(() => {
        if (isPlugin) {
          setChanges((prev) => ({ ...prev, [target]: "" }));
        } else {
          setDrafts((prev) => ({
            ...prev,
            [target]: { content: "", baseEtag: "", baseContent: null },
          }));
        }
        setSelected(target);
      });
      return null;
    },
    [paths, guarded, isPlugin],
  );

  const createFolder = useCallback(
    (name: string): HubPathError | null => {
      const folder = name.trim();
      const error = validateHubPath(folder, paths);
      if (error) return error;
      // A folder exists once it holds a file; `.keep` gives an otherwise empty one a path.
      return createFile(`${folder}/.keep`);
    },
    [paths, createFile],
  );

  const renameEntry = useCallback(
    (from: string, to: string, isFolder: boolean): HubPathError | null => {
      const target = to.trim();
      const members = isFolder ? folderMembers(paths, from) : [from];
      const remaining = paths.filter((p) => !members.includes(p));
      const error = validateHubPath(target, remaining);
      if (error) return error;
      if (target === from) return null;
      if (members.some((p) => dirtyPaths.has(p))) {
        toast.error(t(($) => $.files.rename_dirty));
        return null;
      }

      const pairs = renamePairs(paths, from, target, isFolder);
      const movedSelection = (path: string) => {
        const pair = pairs.find(([source]) => source === path);
        return pair ? pair[1] : path;
      };

      void (async () => {
        try {
          if (isPlugin) {
            let next = changes;
            for (const [source, dest] of pairs) {
              const inBase = baseSet.has(source);
              let fromContent = typeof changes[source] === "string" ? changes[source] : null;
              if (fromContent === null) {
                const file = await qc.fetchQuery({
                  ...hubFileContentOptions(wsId, kind, refId, source, versionId),
                  staleTime: 0,
                });
                if (file.content === null) {
                  toast.error(t(($) => $.toast.rename_failed, { path: source }));
                  return;
                }
                fromContent = file.content;
              }
              next = renamePluginChange(next, source, dest, fromContent, inBase);
            }
            setChanges(next);
          } else {
            for (const [source, dest] of pairs) {
              await renameFile.mutateAsync({ wsId, refId, from: source, to: dest });
            }
          }
          if (members.includes(selectedPath)) setSelected(movedSelection(selectedPath));
          toast.success(t(($) => $.toast.renamed, { path: target }));
        } catch {
          toast.error(t(($) => $.toast.rename_failed, { path: from }));
        }
      })();
      return null;
    },
    [
      paths,
      dirtyPaths,
      t,
      isPlugin,
      changes,
      baseSet,
      qc,
      wsId,
      kind,
      refId,
      versionId,
      renameFile,
      selectedPath,
    ],
  );

  const deleteEntry = useCallback(
    (path: string, isFolder: boolean) => {
      const members = isFolder ? folderMembers(paths, path) : [path];
      if (members.includes(HUB_PRIMARY_FILE)) return;
      const fallback = paths.find((p) => !members.includes(p)) ?? "";
      const apply = async () => {
        try {
          if (isPlugin) {
            let next = changes;
            for (const member of members) {
              next = deletePluginChange(next, member, baseSet.has(member));
            }
            setChanges(next);
          } else {
            for (const member of members) {
              const draft = drafts[member];
              if (draft?.baseContent === null) continue; // never reached the server
              await deleteFile.mutateAsync({ wsId, refId, path: member });
            }
            setDrafts((prev) => {
              const next = { ...prev };
              for (const member of members) delete next[member];
              return next;
            });
          }
          if (members.includes(selectedPath)) setSelected(isPlugin ? fallback : HUB_PRIMARY_FILE);
          toast.success(t(($) => $.toast.deleted, { path }));
        } catch {
          toast.error(t(($) => $.toast.delete_failed, { path }));
        }
      };
      void apply();
    },
    [paths, isPlugin, changes, baseSet, drafts, deleteFile, wsId, refId, selectedPath, t],
  );

  // ---- Plugin publish

  const publish = useCallback(
    async (newVersion: string): Promise<boolean> => {
      if (!isPlugin || versionId === undefined) return false;
      try {
        await savePlugin.mutateAsync({
          wsId,
          refId,
          baseVersionId: versionId,
          changes,
          newVersion,
        });
        setChanges({});
        setSelected(HUB_PRIMARY_FILE);
        toast.success(t(($) => $.toast.published, { version: newVersion }));
        return true;
      } catch {
        toast.error(t(($) => $.toast.publish_failed));
        return false;
      }
    },
    [isPlugin, versionId, savePlugin, wsId, refId, changes, t],
  );

  const retryList = useCallback(() => {
    void list.refetch();
  }, [list]);

  const retryFile = useCallback(() => {
    void content.refetch();
  }, [content]);

  return {
    paths,
    totalBytes: list.data ? list.data.reduce((sum, file) => sum + file.size_bytes, 0) : null,
    selectedPath,
    listLoading: list.isLoading,
    listError: list.isError,
    retryList,
    file: {
      path: selectedPath,
      content: displayContent,
      isBinary: serverFile?.is_binary ?? false,
      isLoading: contentEnabled && content.isLoading,
      isError: contentEnabled && content.isError,
      retry: retryFile,
    },
    isSelectedDirty,
    isDirty,
    dirtyPaths,
    pluginChanges: changes,
    guardOpen: guardAction !== null,
    conflictPath,
    saving: saveFile.isPending,
    publishing: savePlugin.isPending,
    setContent,
    select,
    resolveGuard,
    resolveConflict: (choice) => {
      void resolveConflict(choice);
    },
    dismissConflict: () => setConflictPath(null),
    saveSelected,
    discardSelected,
    discardAll,
    createFile,
    createFolder,
    renameEntry,
    deleteEntry,
    publish,
    draftFor,
  };
}
