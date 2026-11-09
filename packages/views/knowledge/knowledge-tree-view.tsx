"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight, Database, Folder, Lock } from "@multica/ui/icons";
import { useT } from "../i18n";
import type { KnowledgeBaseNode, KnowledgeFolderNode, KnowledgeTree } from "./knowledge-tree";

/** Where the reader is in the library: a base, a folder inside it, or all of it. */
export interface KnowledgeSelection {
  /** The base key; empty selects every document. */
  kb: string;
  /** A folder path inside the base; empty selects the whole base. */
  folder: string;
}

export const ALL_DOCUMENTS: KnowledgeSelection = { kb: "", folder: "" };

/**
 * The organisation of the library as a tree: the company's bases, then one base
 * per project, each opening onto its folders. Selecting a node narrows the
 * panel beside it.
 */
export function KnowledgeTreeView({
  tree,
  totalCount,
  selection,
  onSelect,
}: {
  tree: KnowledgeTree;
  totalCount: number;
  selection: KnowledgeSelection;
  onSelect: (selection: KnowledgeSelection) => void;
}) {
  const { t } = useT("knowledge");
  return (
    <nav aria-label={t(($) => $.explorer.tree_label)} className="flex flex-col gap-4 text-sm">
      <button
        type="button"
        onClick={() => onSelect(ALL_DOCUMENTS)}
        aria-current={selection.kb === "" ? "true" : undefined}
        className={rowClass(selection.kb === "")}
      >
        <Database className="size-4 shrink-0 text-muted-foreground" />
        <span className="flex-1 truncate text-left">{t(($) => $.explorer.all_documents)}</span>
        <Count value={totalCount} />
      </button>
      <Group title={t(($) => $.explorer.company)}>
        {tree.company.map((node) => (
          <BaseRow key={node.base.key} node={node} selection={selection} onSelect={onSelect} />
        ))}
      </Group>
      <Group title={t(($) => $.explorer.projects)}>
        {tree.projects.length === 0 ? (
          <p className="px-2 text-caption text-muted-foreground">{t(($) => $.explorer.no_projects)}</p>
        ) : (
          tree.projects.map((node) => (
            <BaseRow key={node.base.key} node={node} selection={selection} onSelect={onSelect} />
          ))
        )}
      </Group>
    </nav>
  );
}

function rowClass(active: boolean): string {
  return `flex w-full items-center gap-2 rounded-md px-2 py-1.5 hover:bg-accent ${
    active ? "bg-accent font-medium" : ""
  }`;
}

function Count({ value }: { value: number }) {
  return <span className="shrink-0 text-caption tabular-nums text-muted-foreground">{value}</span>;
}

function Group({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <h3 className="px-2 pb-1 text-caption font-medium uppercase tracking-wide text-muted-foreground">
        {title}
      </h3>
      {children}
    </div>
  );
}

function BaseRow({
  node,
  selection,
  onSelect,
}: {
  node: KnowledgeBaseNode;
  selection: KnowledgeSelection;
  onSelect: (selection: KnowledgeSelection) => void;
}) {
  const { t } = useT("knowledge");
  const active = selection.kb === node.base.key;
  const [open, setOpen] = useState(active);
  const expandable = node.folders.length > 0;
  const expanded = open || active;
  return (
    <div className="flex flex-col">
      <div className="flex items-center">
        {expandable ? (
          <button
            type="button"
            aria-expanded={expanded}
            aria-label={t(($) => $.explorer.toggle, { name: node.base.name })}
            onClick={() => setOpen(!expanded)}
            className="rounded p-1 text-muted-foreground hover:bg-accent"
          >
            {expanded ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
          </button>
        ) : (
          <span className="w-[1.375rem] shrink-0" aria-hidden="true" />
        )}
        <button
          type="button"
          onClick={() => {
            onSelect({ kb: node.base.key, folder: "" });
            setOpen(true);
          }}
          aria-current={active && selection.folder === "" ? "true" : undefined}
          className={rowClass(active && selection.folder === "")}
        >
          <span className="flex-1 truncate text-left">{node.base.name}</span>
          {node.base.visibility === "restricted" ? <Lock className="size-3.5 text-muted-foreground" /> : null}
          <Count value={node.count} />
        </button>
      </div>
      {expanded && expandable ? (
        <FolderList
          folders={node.folders}
          kb={node.base.key}
          selection={selection}
          onSelect={onSelect}
          depth={1}
        />
      ) : null}
    </div>
  );
}

function FolderList({
  folders,
  kb,
  selection,
  onSelect,
  depth,
}: {
  folders: KnowledgeFolderNode[];
  kb: string;
  selection: KnowledgeSelection;
  onSelect: (selection: KnowledgeSelection) => void;
  depth: number;
}) {
  return (
    <ul className="flex flex-col">
      {folders.map((folder) => (
        <FolderRow key={folder.path} folder={folder} kb={kb} selection={selection} onSelect={onSelect} depth={depth} />
      ))}
    </ul>
  );
}

function FolderRow({
  folder,
  kb,
  selection,
  onSelect,
  depth,
}: {
  folder: KnowledgeFolderNode;
  kb: string;
  selection: KnowledgeSelection;
  onSelect: (selection: KnowledgeSelection) => void;
  depth: number;
}) {
  const active = selection.kb === kb && selection.folder === folder.path;
  const within = selection.kb === kb && selection.folder.startsWith(`${folder.path}/`);
  const { t } = useT("knowledge");
  const [open, setOpen] = useState(false);
  const expanded = open || active || within;
  return (
    <li className="flex flex-col">
      <div className="flex items-center" style={{ paddingInlineStart: `${depth * 0.875}rem` }}>
        {folder.children.length > 0 ? (
          <button
            type="button"
            aria-expanded={expanded}
            aria-label={t(($) => $.explorer.toggle, { name: folder.name })}
            onClick={() => setOpen(!expanded)}
            className="rounded p-1 text-muted-foreground hover:bg-accent"
          >
            {expanded ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
          </button>
        ) : (
          <span className="w-[1.375rem] shrink-0" aria-hidden="true" />
        )}
        <button
          type="button"
          onClick={() => onSelect({ kb, folder: folder.path })}
          aria-current={active ? "true" : undefined}
          className={rowClass(active)}
        >
          <Folder className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="flex-1 truncate text-left">{folder.name}</span>
          <Count value={folder.count} />
        </button>
      </div>
      {expanded && folder.children.length > 0 ? (
        <FolderList folders={folder.children} kb={kb} selection={selection} onSelect={onSelect} depth={depth + 1} />
      ) : null}
    </li>
  );
}
