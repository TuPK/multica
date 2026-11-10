"use client";

import { useMemo, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import {
  ChevronDown,
  ChevronRight,
  File,
  FileCode,
  FileText,
  Folder,
  FolderOpen,
  MoreHorizontal,
  Pencil,
  Trash2,
} from "@multica/ui/icons";
import { Input } from "@multica/ui/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../i18n";
import { flattenHubTree, type HubPathError, type HubTreeRow } from "./hub-files";

const ROW_HEIGHT = 32;

export interface HubFileTreeActions {
  /** The path that cannot be renamed or deleted. */
  reservedPath: string;
  /** Returns a validation error, or null when the rename has started. */
  onRename: (from: string, to: string, isFolder: boolean) => HubPathError | null;
  onDelete: (path: string, isFolder: boolean) => void;
}

function rowIcon(row: HubTreeRow) {
  if (row.isFolder) return row.expanded ? FolderOpen : Folder;
  if (row.name.endsWith(".md") || row.name.endsWith(".mdx")) return FileText;
  if (row.name.endsWith(".json")) return FileCode;
  return File;
}

/**
 * Folder tree of a Hub item's files. Only the visible window is mounted, so a
 * large plugin does not put every row in the DOM. Folders are inferred from
 * paths; collapsing a folder hides its rows without changing the paths.
 */
export function HubFileTree({
  paths,
  selectedPath,
  dirtyPaths,
  actions,
  onSelect,
}: {
  paths: string[];
  selectedPath: string;
  dirtyPaths: ReadonlySet<string>;
  /** Omit for a read-only tree. */
  actions?: HubFileTreeActions;
  onSelect: (path: string) => void;
}) {
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set());
  const [renamingPath, setRenamingPath] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement | null>(null);

  const rows = useMemo(() => flattenHubTree(paths, collapsed), [paths, collapsed]);

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  });

  const toggleFolder = (path: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  };

  return (
    <div ref={scrollRef} className="h-full overflow-y-auto py-1">
      <div role="list" className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((item) => {
          const row = rows[item.index];
          if (!row) return null;
          return (
            <div
              key={item.key}
              role="listitem"
              className="absolute left-0 top-0 w-full"
              style={{ height: ROW_HEIGHT, transform: `translateY(${item.start}px)` }}
            >
              {renamingPath === row.path && actions ? (
                <RenameRow
                  row={row}
                  onDone={() => setRenamingPath(null)}
                  onSubmit={(to) => {
                    const error = actions.onRename(row.path, to, row.isFolder);
                    if (!error) setRenamingPath(null);
                    return error;
                  }}
                />
              ) : (
                <TreeRow
                  row={row}
                  selected={!row.isFolder && row.path === selectedPath}
                  dirty={dirtyPaths.has(row.path)}
                  actions={actions && row.path !== actions.reservedPath ? actions : undefined}
                  onToggle={() => toggleFolder(row.path)}
                  onSelect={() => onSelect(row.path)}
                  onRename={() => setRenamingPath(row.path)}
                  onDelete={() => actions?.onDelete(row.path, row.isFolder)}
                />
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function TreeRow({
  row,
  selected,
  dirty,
  actions,
  onToggle,
  onSelect,
  onRename,
  onDelete,
}: {
  row: HubTreeRow;
  selected: boolean;
  dirty: boolean;
  actions?: HubFileTreeActions;
  onToggle: () => void;
  onSelect: () => void;
  onRename: () => void;
  onDelete: () => void;
}) {
  const { t } = useT("hub");
  const Icon = rowIcon(row);
  const indent = { paddingLeft: `${row.depth * 12 + 8}px` };

  if (row.isFolder) {
    const Chevron = row.expanded ? ChevronDown : ChevronRight;
    return (
      <div className="group/row relative flex h-full items-center rounded-md hover:bg-surface-hover">
        <button
          type="button"
          aria-expanded={row.expanded}
          onClick={onToggle}
          style={indent}
          className="flex h-full min-w-0 flex-1 items-center gap-1.5 pr-2 text-left text-caption text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-md"
        >
          <Chevron className="size-3 shrink-0" aria-hidden="true" />
          <Icon className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="truncate">{row.name}</span>
        </button>
        {actions && <RowMenu path={row.path} onRename={onRename} onDelete={onDelete} />}
      </div>
    );
  }

  return (
    <div
      className={cn(
        "group/row relative flex h-full items-center rounded-md",
        selected ? "bg-surface-selected" : "hover:bg-surface-hover",
      )}
    >
      <button
        type="button"
        aria-current={selected ? "true" : undefined}
        onClick={onSelect}
        style={indent}
        className={cn(
          "flex h-full min-w-0 flex-1 items-center gap-2 rounded-md pr-2 text-left text-caption transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          selected ? "text-surface-selected-foreground" : "text-muted-foreground group-hover/row:text-foreground",
        )}
      >
        <Icon className="size-3.5 shrink-0" aria-hidden="true" />
        <span className="min-w-0 truncate" title={row.path}>
          {row.name}
        </span>
        {dirty && (
          <span
            role="img"
            aria-label={t(($) => $.files.dirty)}
            className="size-1.5 shrink-0 rounded-full bg-brand"
          />
        )}
      </button>
      {actions && <RowMenu path={row.path} onRename={onRename} onDelete={onDelete} />}
    </div>
  );
}

function RowMenu({
  path,
  onRename,
  onDelete,
}: {
  path: string;
  onRename: () => void;
  onDelete: () => void;
}) {
  const { t } = useT("hub");
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <button
            type="button"
            aria-label={t(($) => $.files.actions.label, { path })}
            // Hidden until the row is hovered or focused; the 2px negative inset
            // widens the hit area past the glyph without covering the name.
            className="relative mr-1 shrink-0 rounded-xs p-0.5 text-faint-foreground opacity-0 transition-opacity after:absolute after:-inset-1 hover:text-foreground focus-visible:opacity-100 group-hover/row:opacity-100 aria-expanded:opacity-100"
            onClick={(event) => event.stopPropagation()}
          >
            <MoreHorizontal className="size-3.5" aria-hidden="true" />
          </button>
        }
      />
      <DropdownMenuContent align="end" className="w-36">
        <DropdownMenuItem onClick={onRename}>
          <Pencil />
          {t(($) => $.files.actions.rename)}
        </DropdownMenuItem>
        <DropdownMenuItem variant="destructive" onClick={onDelete}>
          <Trash2 />
          {t(($) => $.files.actions.delete)}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function RenameRow({
  row,
  onDone,
  onSubmit,
}: {
  row: HubTreeRow;
  onDone: () => void;
  onSubmit: (to: string) => HubPathError | null;
}) {
  const { t } = useT("hub");
  const [value, setValue] = useState(row.path);
  const [error, setError] = useState<HubPathError | null>(null);
  const message = (code: HubPathError) => t(($) => $.files.errors[code]);

  const submit = () => {
    const next = value.trim();
    if (next === row.path) {
      onDone();
      return;
    }
    const result = onSubmit(next);
    if (result) setError(result);
  };

  return (
    <div style={{ paddingLeft: `${row.depth * 12 + 8}px` }} className="pr-1">
      <Input
        autoFocus
        value={value}
        aria-label={t(($) => $.files.rename_aria, { path: row.path })}
        onChange={(event) => {
          setValue(event.target.value);
          setError(null);
        }}
        onBlur={onDone}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault();
            submit();
          }
          if (event.key === "Escape") onDone();
        }}
        className="h-7 font-mono text-caption"
      />
      {error && (
        <p role="alert" className="mt-1 text-caption text-destructive">
          {message(error)}
        </p>
      )}
    </div>
  );
}
