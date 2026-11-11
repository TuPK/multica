"use client";

import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
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
  if (
    row.name.endsWith(".json") ||
    row.name.endsWith(".ts") ||
    row.name.endsWith(".tsx") ||
    row.name.endsWith(".js") ||
    row.name.endsWith(".jsx") ||
    row.name.endsWith(".py") ||
    row.name.endsWith(".sh") ||
    row.name.endsWith(".bash") ||
    row.name.endsWith(".yml") ||
    row.name.endsWith(".yaml") ||
    row.name.endsWith(".toml") ||
    row.name.endsWith(".css")
  ) {
    return FileCode;
  }
  return File;
}

/** The nearest row above `index` that is one level shallower: the parent folder. */
function parentIndex(rows: readonly HubTreeRow[], index: number): number {
  const depth = rows[index]?.depth ?? 0;
  for (let i = index - 1; i >= 0; i--) {
    if (rows[i]!.depth < depth) return i;
  }
  return -1;
}

/**
 * Folder tree of a Hub item's files, as an ARIA tree. Only the visible window is
 * mounted, so a large plugin does not put every row in the DOM. Folders are
 * inferred from paths; collapsing a folder hides its rows without changing the paths.
 *
 * Keyboard: Up/Down move, Home/End jump, Right expands or enters a folder, Left
 * collapses or moves to the parent, Enter or Space opens a row. One row is in the
 * tab order at a time (roving tabindex).
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
  const { t } = useT("hub");
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set());
  const [renamingPath, setRenamingPath] = useState<string | null>(null);
  // The row the user last moved to or clicked. Null until the user interacts.
  const [focusPath, setFocusPath] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const rowElements = useRef(new Map<string, HTMLDivElement>());
  // Set when a key press moves focus to a row that may not be mounted yet.
  const focusPending = useRef(false);

  const rows = useMemo(() => flattenHubTree(paths, collapsed), [paths, collapsed]);

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  });

  // The tab stop: the focused row if it is still shown, else the selected file, else the first row.
  const tabStop =
    rows.find((row) => row.path === focusPath)?.path ??
    rows.find((row) => !row.isFolder && row.path === selectedPath)?.path ??
    rows[0]?.path;

  useEffect(() => {
    if (!focusPending.current || tabStop === undefined) return;
    const element = rowElements.current.get(tabStop);
    if (!element) return;
    focusPending.current = false;
    element.focus();
  });

  const toggleFolder = (path: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  };

  const moveFocus = (index: number) => {
    const row = rows[index];
    if (!row) return;
    setFocusPath(row.path);
    focusPending.current = true;
    virtualizer.scrollToIndex(index);
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    // Only the row itself handles keys; the rename field and the row menu keep their own.
    const path = (event.target as HTMLElement).dataset.treePath;
    if (path === undefined) return;
    const index = rows.findIndex((row) => row.path === path);
    const row = rows[index];
    if (!row) return;

    switch (event.key) {
      case "ArrowDown":
        moveFocus(Math.min(index + 1, rows.length - 1));
        break;
      case "ArrowUp":
        moveFocus(Math.max(index - 1, 0));
        break;
      case "Home":
        moveFocus(0);
        break;
      case "End":
        moveFocus(rows.length - 1);
        break;
      case "ArrowRight":
        if (row.isFolder && !row.expanded) toggleFolder(row.path);
        else if (row.isFolder && rows[index + 1] && rows[index + 1]!.depth > row.depth) moveFocus(index + 1);
        break;
      case "ArrowLeft":
        if (row.isFolder && row.expanded) toggleFolder(row.path);
        else {
          const parent = parentIndex(rows, index);
          if (parent >= 0) moveFocus(parent);
        }
        break;
      case "Enter":
      case " ":
        if (row.isFolder) toggleFolder(row.path);
        else onSelect(row.path);
        break;
      default:
        return;
    }
    event.preventDefault();
  };

  return (
    <div
      ref={scrollRef}
      role="tree"
      aria-label={t(($) => $.files.list)}
      onKeyDown={handleKeyDown}
      className="h-full overflow-y-auto py-1"
    >
      <div className="relative w-full" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((item) => {
          const row = rows[item.index];
          if (!row) return null;
          return (
            <div
              key={item.key}
              role="none"
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
                  tabStop={row.path === tabStop}
                  registerElement={(element) => {
                    if (element) rowElements.current.set(row.path, element);
                    else rowElements.current.delete(row.path);
                  }}
                  actions={actions && row.path !== actions.reservedPath ? actions : undefined}
                  onFocusRow={() => setFocusPath(row.path)}
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
  tabStop,
  registerElement,
  actions,
  onFocusRow,
  onToggle,
  onSelect,
  onRename,
  onDelete,
}: {
  row: HubTreeRow;
  selected: boolean;
  dirty: boolean;
  tabStop: boolean;
  registerElement: (element: HTMLDivElement | null) => void;
  actions?: HubFileTreeActions;
  onFocusRow: () => void;
  onToggle: () => void;
  onSelect: () => void;
  onRename: () => void;
  onDelete: () => void;
}) {
  const { t } = useT("hub");
  const Icon = rowIcon(row);
  const indent = { paddingLeft: `${row.depth * 12 + 8}px` };
  const label = dirty ? `${row.name}, ${t(($) => $.files.dirty)}` : row.name;

  const item = (
    <div
      ref={registerElement}
      role="treeitem"
      tabIndex={tabStop ? 0 : -1}
      aria-level={row.depth + 1}
      aria-expanded={row.isFolder ? row.expanded : undefined}
      aria-selected={selected}
      aria-label={label}
      data-tree-path={row.path}
      style={indent}
      onFocus={onFocusRow}
      onClick={row.isFolder ? onToggle : onSelect}
      className={cn(
        "flex h-full min-w-0 flex-1 items-center gap-2 rounded-md pr-2 text-left text-caption transition-colors focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring",
        row.isFolder
          ? "text-muted-foreground hover:bg-surface-hover hover:text-foreground"
          : selected
            ? "bg-surface-selected font-medium text-surface-selected-foreground hover:bg-surface-selected/90"
            : "text-muted-foreground hover:bg-surface-hover hover:text-foreground",
      )}
    >
      {row.isFolder ? (
        <ChevronCaret expanded={row.expanded} />
      ) : null}
      <Icon className="size-3.5 shrink-0" aria-hidden="true" />
      <span className="min-w-0 truncate" title={row.path}>
        {row.name}
      </span>
      {dirty && !row.isFolder && (
        <span
          role="img"
          aria-label={t(($) => $.files.dirty)}
          title={t(($) => $.files.dirty)}
          className="size-1.5 shrink-0 rounded-full bg-brand"
        />
      )}
    </div>
  );

  return (
    <div className="group/row relative flex h-full items-center rounded-md" role="none">
      {item}
      {actions && <RowMenu path={row.path} onRename={onRename} onDelete={onDelete} />}
    </div>
  );
}

function ChevronCaret({ expanded }: { expanded: boolean }) {
  const Chevron = expanded ? ChevronDown : ChevronRight;
  return <Chevron className="size-3 shrink-0" aria-hidden="true" />;
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
            className="relative mr-1 shrink-0 rounded-xs p-0.5 text-faint-foreground opacity-0 transition-opacity after:absolute after:-inset-1 hover:text-foreground focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-ring group-hover/row:opacity-100 group-focus-within/row:opacity-100 aria-expanded:opacity-100"
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
        aria-invalid={error !== null}
        aria-describedby={error ? `rename-error-${row.path}` : undefined}
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
        <p id={`rename-error-${row.path}`} role="alert" className="mt-1 text-caption text-destructive">
          {message(error)}
        </p>
      )}
    </div>
  );
}
