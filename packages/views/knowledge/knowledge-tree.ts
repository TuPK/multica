import type { KnowledgeBase, KnowledgeDocument } from "@multica/core/knowledge";

/** A folder inside a base, nested by the segments of its path. */
export interface KnowledgeFolderNode {
  /** The full path, such as `docs/planning-artifacts`. It is the folder's id. */
  path: string;
  /** The last segment, shown in the tree. */
  name: string;
  /** Documents in this folder and everything below it. */
  count: number;
  children: KnowledgeFolderNode[];
}

export interface KnowledgeBaseNode {
  base: KnowledgeBase;
  /** Every document in the base, folders included. */
  count: number;
  /** Documents held back from agents: drafts waiting for a person. */
  drafts: number;
  folders: KnowledgeFolderNode[];
}

export interface KnowledgeTree {
  /** The company's own bases, in their set order. */
  company: KnowledgeBaseNode[];
  /** Project bases, by project name. */
  projects: KnowledgeBaseNode[];
}

/** A project base is the one that names a project. */
export function isProjectBase(base: KnowledgeBase): boolean {
  return !!base.project_id;
}

function folderSegments(path: string): string[] {
  return path.split("/").filter((segment) => segment.length > 0);
}

function buildFolders(paths: string[]): KnowledgeFolderNode[] {
  const roots: KnowledgeFolderNode[] = [];
  for (const path of paths) {
    let level = roots;
    let prefix = "";
    for (const segment of folderSegments(path)) {
      prefix = prefix ? `${prefix}/${segment}` : segment;
      let node = level.find((candidate) => candidate.name === segment);
      if (!node) {
        node = { path: prefix, name: segment, count: 0, children: [] };
        level.push(node);
      }
      node.count += 1;
      level = node.children;
    }
  }
  const sort = (nodes: KnowledgeFolderNode[]) => {
    nodes.sort((a, b) => a.name.localeCompare(b.name));
    nodes.forEach((node) => sort(node.children));
  };
  sort(roots);
  return roots;
}

/**
 * The organisation of the library: the company's bases, then one base per
 * project, each with the folders its documents are filed under. A base with no
 * documents is still listed, so an empty project base can be seen and filled.
 */
export function buildKnowledgeTree(
  bases: KnowledgeBase[],
  documents: KnowledgeDocument[],
): KnowledgeTree {
  const byBase = new Map<string, KnowledgeDocument[]>();
  for (const doc of documents) {
    const list = byBase.get(doc.knowledge_base) ?? [];
    list.push(doc);
    byBase.set(doc.knowledge_base, list);
  }
  const nodeOf = (base: KnowledgeBase): KnowledgeBaseNode => {
    const docs = byBase.get(base.key) ?? [];
    return {
      base,
      count: docs.length,
      drafts: docs.filter((doc) => doc.state === "draft").length,
      folders: buildFolders(docs.map((doc) => doc.folder ?? "").filter((path) => path !== "")),
    };
  };
  const sorted = [...bases].sort((a, b) => a.position - b.position || a.name.localeCompare(b.name));
  return {
    company: sorted.filter((base) => !isProjectBase(base)).map(nodeOf),
    projects: sorted
      .filter(isProjectBase)
      .map(nodeOf)
      .sort((a, b) => a.base.name.localeCompare(b.base.name)),
  };
}

/** Whether a document is filed in the folder or below it. */
export function inFolder(doc: KnowledgeDocument, folder: string): boolean {
  if (folder === "") return true;
  const own = doc.folder ?? "";
  return own === folder || own.startsWith(`${folder}/`);
}
