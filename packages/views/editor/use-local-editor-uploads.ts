"use client";

/**
 * Attachment support for a short-lived composer that keeps no draft store: the
 * company issue form and the agent request pane. Pasted, dropped and picked
 * files upload straight away; the ids still referenced by the body at submit
 * time are what the create request binds as `attachment_ids`.
 *
 * Composers backed by the issue draft store use `useIssueCreateUploads`
 * instead, because their uploads must survive a dialog close.
 */

import { useCallback, useState, type RefObject } from "react";
import { contentReferencesAttachment } from "@multica/core/types";
import type { UploadResult } from "@multica/core/hooks/use-file-upload";
import type { ContentEditorRef } from "./content-editor";
import { useEditorUpload } from "./use-editor-upload";
import { useFileDropZone } from "./use-file-drop-zone";
import { useUploadGate } from "./use-upload-gate";

function useLocalEditorUploads(editorRef: RefObject<ContentEditorRef | null>) {
  const { uploadWithToast } = useEditorUpload();
  const gate = useUploadGate(editorRef);
  const [attachments, setAttachments] = useState<UploadResult[]>([]);

  const onUploadFile = useCallback(
    async (file: File): Promise<UploadResult | null> => {
      const result = await uploadWithToast(file);
      if (result) {
        setAttachments((current) =>
          current.some((a) => a.id === result.id) ? current : [...current, result],
        );
      }
      return result;
    },
    [uploadWithToast],
  );

  const uploadFile = useCallback(
    (file: File) => editorRef.current?.uploadFile(file),
    [editorRef],
  );
  const { isDragOver, dropZoneProps } = useFileDropZone({
    onDrop: (files) => files.forEach(uploadFile),
  });

  /** Ids of uploaded files the body still references. */
  const attachmentIdsIn = useCallback(
    (markdown: string) =>
      attachments.filter((a) => contentReferencesAttachment(markdown, a)).map((a) => a.id),
    [attachments],
  );

  const reset = useCallback(() => setAttachments([]), []);

  return {
    gate,
    attachments,
    onUploadFile,
    uploadFile,
    isDragOver,
    dropZoneProps,
    attachmentIdsIn,
    reset,
  };
}

export { useLocalEditorUploads };
