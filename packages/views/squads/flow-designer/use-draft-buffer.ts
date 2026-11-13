import { useCallback, useEffect, useRef, useState } from "react";
import type { FlowDoc, FlowDraft } from "@multica/core/types";
import { useCreateFlowDraft, useUpdateFlowDraft } from "@multica/core/flow-drafts";
import { useNavigation } from "../../navigation";
import { useWorkspacePaths } from "@multica/core/paths";

export interface UseDraftBufferOptions {
  wsId: string;
  squadId: string;
  draftId: string;
  initialDraft: FlowDraft;
  onConflict?: () => void;
}

export function useDraftBuffer({
  wsId: _wsId,
  squadId,
  draftId,
  initialDraft,
  onConflict,
}: UseDraftBufferOptions) {
  const [doc, setDocState] = useState<FlowDoc>(initialDraft.doc);
  const [name, setNameState] = useState<string>(initialDraft.name);
  const [revision, setRevision] = useState<number>(initialDraft.revision);
  const [isDirty, setIsDirty] = useState<boolean>(false);
  const [isSaving, setIsSaving] = useState<boolean>(false);
  const [saveError, setSaveError] = useState<unknown | null>(null);
  const [isConflict, setIsConflict] = useState<boolean>(false);
  const [isOffline, setIsOffline] = useState<boolean>(
    typeof navigator !== "undefined" && !navigator.onLine,
  );

  const updateMut = useUpdateFlowDraft();
  const createMut = useCreateFlowDraft();
  const nav = useNavigation();
  const paths = useWorkspacePaths();

  // Keep latest refs for timeout callback
  const docRef = useRef(doc);
  docRef.current = doc;
  const nameRef = useRef(name);
  nameRef.current = name;
  const revRef = useRef(revision);
  revRef.current = revision;
  const isDirtyRef = useRef(isDirty);
  isDirtyRef.current = isDirty;
  const isSavingRef = useRef(isSaving);
  isSavingRef.current = isSaving;

  // Track online / offline events
  useEffect(() => {
    const handleOnline = () => setIsOffline(false);
    const handleOffline = () => setIsOffline(true);
    window.addEventListener("online", handleOnline);
    window.addEventListener("offline", handleOffline);
    return () => {
      window.removeEventListener("online", handleOnline);
      window.removeEventListener("offline", handleOffline);
    };
  }, []);

  // Leaving with unsaved changes prompt
  useEffect(() => {
    const handleBeforeUnload = (e: BeforeUnloadEvent) => {
      if (isDirtyRef.current) {
        e.preventDefault();
        e.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, []);

  // Saves what the buffer holds. Resolves to the revision the server now has, or null when the
  // buffer could not be saved (offline, a conflict, an error): a caller that needs the draft on the
  // server, such as publish, must not go on then.
  const saveNow = useCallback(async (): Promise<number | null> => {
    if (!isDirtyRef.current) return revRef.current;
    if (isSavingRef.current) return null;
    if (typeof navigator !== "undefined" && !navigator.onLine) {
      setIsOffline(true);
      return null;
    }

    setIsSaving(true);
    setSaveError(null);
    const sentDoc = docRef.current;
    const sentName = nameRef.current;
    try {
      const updated = await updateMut.mutateAsync({
        squadId,
        draftId,
        data: {
          revision: revRef.current,
          name: sentName,
          doc: sentDoc,
        },
      });
      revRef.current = updated.revision;
      setRevision(updated.revision);
      // An edit made while the save was in flight is not in what was sent: it stays unsaved.
      const sentEverything = docRef.current === sentDoc && nameRef.current === sentName;
      isDirtyRef.current = !sentEverything;
      setIsDirty(!sentEverything);
      setIsConflict(false);
      setSaveError(null);
      return updated.revision;
    } catch (err: unknown) {
      const errorObj = err as { status?: number; code?: string; message?: string };
      if (errorObj?.status === 409 || errorObj?.code === "flow_draft_conflict") {
        setIsConflict(true);
        onConflict?.();
      } else if (!navigator.onLine || errorObj?.message?.includes("network")) {
        setIsOffline(true);
        setSaveError(err);
      } else {
        setSaveError(err);
      }
    } finally {
      setIsSaving(false);
    }
    return null;
  }, [squadId, draftId, updateMut, onConflict]);

  // Debounced autosave after 1.5s (1500ms)
  useEffect(() => {
    if (!isDirty || isSaving || isConflict || isOffline) return;

    const timer = setTimeout(() => {
      void saveNow();
    }, 1500);

    return () => clearTimeout(timer);
  }, [isDirty, isSaving, isConflict, isOffline, doc, name, saveNow]);

  const setDoc = useCallback((updater: FlowDoc | ((prev: FlowDoc) => FlowDoc)) => {
    setDocState((prev) => {
      const next = typeof updater === "function" ? (updater as (prev: FlowDoc) => FlowDoc)(prev) : updater;
      return next;
    });
    setIsDirty(true);
  }, []);

  const setName = useCallback((newName: string) => {
    setNameState(newName);
    setIsDirty(true);
  }, []);

  // Replaces the buffer with the draft the server has: the answer to a conflict.
  const reload = useCallback((latest: FlowDraft) => {
    revRef.current = latest.revision;
    isDirtyRef.current = false;
    setDocState(latest.doc);
    setNameState(latest.name);
    setRevision(latest.revision);
    setIsDirty(false);
    setIsConflict(false);
    setSaveError(null);
  }, []);

  const keepAsCopy = useCallback(async () => {
    try {
      const newDraft = await createMut.mutateAsync({
        squadId,
        data: {
          name: `${nameRef.current} (copy)`.slice(0, 56),
          from: { kind: "blank" },
        },
      });
      // Update new draft with current buffer doc
      await updateMut.mutateAsync({
        squadId,
        draftId: newDraft.id,
        data: {
          revision: newDraft.revision,
          name: newDraft.name,
          doc: docRef.current,
        },
      });
      setIsConflict(false);
      setIsDirty(false);
      nav.push(`${paths.squads()}/${squadId}/workflows/${newDraft.id}`);
    } catch (err) {
      setSaveError(err);
    }
  }, [squadId, createMut, updateMut, nav, paths]);

  return {
    doc,
    name,
    revision,
    isDirty,
    isSaving,
    saveError,
    isConflict,
    isOffline,
    setDoc,
    setName,
    saveNow,
    reload,
    keepAsCopy,
  };
}
