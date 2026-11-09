"use client";

/**
 * A plain-text instruction box that still takes files: paste an image, drop a
 * file, or pick one with the paperclip. The upload is bound to `issueId` and a
 * Markdown link lands at the caret, so the agent reading the instruction
 * receives the same link an issue description would carry.
 *
 * For instructions stored as plain strings (wakeups). Rich bodies use
 * `ContentEditor`.
 */

import { useCallback, useEffect, useRef, useState, type ComponentProps } from "react";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { FileUploadButton } from "@multica/ui/components/common/file-upload-button";
import { cn } from "@multica/ui/lib/utils";
import { useEditorUpload } from "./use-editor-upload";

type AttachableTextareaProps = Omit<ComponentProps<typeof Textarea>, "value" | "onChange"> & {
  value: string;
  onValueChange: (value: string) => void;
  /** Attachments are filed under this issue. */
  issueId: string;
  /** Fires when the number of in-flight uploads goes from zero to some, or back. */
  onUploadingChange?: (uploading: boolean) => void;
};

function markdownFor(file: File, link: string): string {
  const name = file.name.replace(/[[\]]/g, "");
  return file.type.startsWith("image/") ? `![${name}](${link})` : `[${name}](${link})`;
}

function AttachableTextarea({
  value,
  onValueChange,
  issueId,
  onUploadingChange,
  disabled,
  className,
  ...props
}: AttachableTextareaProps) {
  const { uploadWithToast } = useEditorUpload();
  const ref = useRef<HTMLTextAreaElement>(null);
  const valueRef = useRef(value);
  valueRef.current = value;
  const [inFlight, setInFlight] = useState(0);
  const [dragging, setDragging] = useState(false);

  useEffect(() => {
    onUploadingChange?.(inFlight > 0);
  }, [inFlight, onUploadingChange]);

  const attach = useCallback(
    async (file: File) => {
      const textarea = ref.current;
      // A box the user is typing in takes the link at the caret; otherwise (the
      // paperclip, a drop onto an unfocused box) it goes at the end.
      const at =
        textarea && document.activeElement === textarea
          ? textarea.selectionEnd
          : valueRef.current.length;
      setInFlight((n) => n + 1);
      try {
        const result = await uploadWithToast(file, { issueId });
        if (!result) return;
        const current = valueRef.current;
        const caret = Math.min(at, current.length);
        const before = current.slice(0, caret);
        const after = current.slice(caret);
        const insert = `${before && !before.endsWith("\n") ? "\n" : ""}${markdownFor(file, result.markdownLink)}\n`;
        onValueChange(before + insert + after);
      } finally {
        setInFlight((n) => n - 1);
      }
    },
    [issueId, onValueChange, uploadWithToast],
  );

  return (
    <div
      className="relative"
      onDragEnter={(event) => {
        if (event.dataTransfer.types.includes("Files")) setDragging(true);
      }}
      onDragOver={(event) => {
        if (event.dataTransfer.types.includes("Files")) event.preventDefault();
      }}
      onDragLeave={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false);
      }}
      onDrop={(event) => {
        setDragging(false);
        if (disabled || event.dataTransfer.files.length === 0) return;
        event.preventDefault();
        Array.from(event.dataTransfer.files).forEach((file) => void attach(file));
      }}
    >
      <Textarea
        ref={ref}
        {...props}
        disabled={disabled}
        value={value}
        className={cn(dragging && "border-dashed border-brand/40", className)}
        onChange={(event) => onValueChange(event.target.value)}
        onPaste={(event) => {
          props.onPaste?.(event);
          if (disabled || event.defaultPrevented) return;
          const files = Array.from(event.clipboardData.files);
          if (files.length === 0) return;
          event.preventDefault();
          files.forEach((file) => void attach(file));
        }}
      />
      <div className="mt-1 flex items-center justify-end gap-1.5 text-caption text-muted-foreground">
        <FileUploadButton
          size="sm"
          multiple
          disabled={disabled}
          onSelect={(file) => void attach(file)}
        />
      </div>
    </div>
  );
}

export { AttachableTextarea, type AttachableTextareaProps };
