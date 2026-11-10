"use client";

import { useMemo } from "react";
import { AlertTriangle, File } from "@multica/ui/icons";
import { Button } from "@multica/ui/components/ui/button";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { parseFrontmatter } from "@multica/core/skills/frontmatter";
import { cn } from "@multica/ui/lib/utils";
import { RichContent } from "../rich-content";
import { useT } from "../i18n";
import { previewKind, prettyJson } from "./hub-files";

/** Preview renders the file; Source (or Edit, when writable) shows its text. */
export type HubViewMode = "preview" | "source";

/** Preview/Source toggle for files with a rendered form (markdown, JSON, SVG). */
export function HubViewModeToggle({
  mode,
  editable,
  onChange,
}: {
  mode: HubViewMode;
  editable: boolean;
  onChange: (mode: HubViewMode) => void;
}) {
  const { t } = useT("hub");
  return (
    <div
      role="group"
      aria-label={t(($) => $.files.mode.aria)}
      className="flex items-center gap-0.5 rounded-md bg-muted p-0.5"
    >
      {(["preview", "source"] as const).map((value) => (
        <button
          key={value}
          type="button"
          aria-pressed={mode === value}
          onClick={() => onChange(value)}
          className={cn(
            "h-6 rounded-xs px-2 text-caption font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            mode === value ? "bg-surface text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
          )}
        >
          {value === "preview"
            ? t(($) => $.files.mode.preview)
            : editable
              ? t(($) => $.files.mode.edit)
              : t(($) => $.files.mode.source)}
        </button>
      ))}
    </div>
  );
}

function Notice({ children }: { children: React.ReactNode }) {
  return (
    <div
      role="status"
      className="mx-4 mt-4 flex items-start gap-2 rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-caption text-muted-foreground"
    >
      <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-warning" aria-hidden="true" />
      <span className="min-w-0 flex-1">{children}</span>
    </div>
  );
}

/**
 * The selected file's body. Text is editable when `editable` is set; binary and
 * oversized files show a notice instead. The caller owns the mode so switching
 * files keeps the user's view.
 */
export function HubFileViewer({
  path,
  content,
  isBinary,
  isLoading,
  isError,
  mode,
  editable,
  onChange,
  onRetry,
}: {
  path: string;
  /** Text to show. Null when the file is binary or too large to load. */
  content: string | null;
  isBinary: boolean;
  isLoading: boolean;
  isError: boolean;
  mode: HubViewMode;
  editable: boolean;
  onChange: (value: string) => void;
  onRetry: () => void;
}) {
  const { t } = useT("hub");
  const kind = previewKind(path, isBinary);

  const pretty = useMemo(
    () => (kind === "json" && content !== null ? prettyJson(content) : null),
    [kind, content],
  );
  const body = useMemo(
    () => (kind === "markdown" && content !== null ? parseFrontmatter(content).body : ""),
    [kind, content],
  );

  if (isLoading) {
    return (
      <div className="flex flex-col gap-2 p-6" aria-busy="true">
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-4 w-1/2" />
        <Skeleton className="h-4 w-3/5" />
      </div>
    );
  }

  if (isError) {
    return (
      <div className="flex flex-col items-start gap-3 p-6 text-caption">
        <p className="text-destructive">{t(($) => $.files.load_error)}</p>
        <Button type="button" variant="outline" size="sm" onClick={onRetry}>
          {t(($) => $.retry)}
        </Button>
      </div>
    );
  }

  if (isBinary || kind === "binary") {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center text-muted-foreground">
        <File className="size-5 text-faint-foreground" aria-hidden="true" />
        <p className="max-w-sm text-caption">{t(($) => $.files.binary)}</p>
      </div>
    );
  }

  if (content === null) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center text-muted-foreground">
        <p className="max-w-sm text-caption">{t(($) => $.files.unavailable)}</p>
      </div>
    );
  }

  if (mode === "preview" && kind === "markdown") {
    return (
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-[68ch] px-6 pb-12 pt-7 sm:px-8">
          <RichContent content={body} density="document" phase="settled" />
        </div>
      </div>
    );
  }

  if (mode === "preview" && kind === "svg") {
    return (
      <div className="flex h-full items-center justify-center overflow-auto p-6">
        {/* An SVG loaded through <img> runs no scripts, so the text is safe to show here. */}
        <img
          src={`data:image/svg+xml;charset=utf-8,${encodeURIComponent(content)}`}
          alt={path}
          className="max-h-full max-w-full"
        />
      </div>
    );
  }

  if (mode === "preview" && kind === "json" && pretty !== null) {
    return (
      <pre className="h-full overflow-auto whitespace-pre px-6 pb-12 pt-5 font-mono text-caption leading-relaxed sm:px-8">
        {pretty}
      </pre>
    );
  }

  return (
    <>
      {mode === "preview" && kind === "json" && (
        <Notice>{t(($) => $.files.invalid_json)}</Notice>
      )}
      <Textarea
        value={content}
        readOnly={!editable}
        onChange={(event) => onChange(event.target.value)}
        aria-label={t(($) => $.files.editor_label, { path })}
        spellCheck={false}
        className="h-full min-h-full resize-none rounded-none border-0 px-6 pb-12 pt-5 font-mono text-body leading-relaxed read-only:cursor-default focus-visible:ring-0 sm:px-8"
      />
    </>
  );
}
