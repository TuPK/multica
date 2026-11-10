/**
 * Pure file-model helpers for the Hub item Files tab: path rules, the folder
 * tree, the plugin draft overlay and how a file is previewed. No React, so the
 * matrices are unit tested directly.
 *
 * Directories are not stored; they are inferred from paths. A folder that must
 * exist without files gets a `.keep` placeholder, which is the only way the
 * flat file API can represent an empty folder.
 */

/** The file every skill must keep. It is the skill's primary content. */
export const HUB_PRIMARY_FILE = "SKILL.md";

/** Placeholder written so an otherwise empty folder exists. */
export const HUB_FOLDER_KEEP = ".keep";

/** A plugin draft: path -> new text content, or `null` for a removed path. */
export type PluginChanges = Record<string, string | null>;

export type HubPathError =
  | "empty"
  | "absolute"
  | "double_dot"
  | "reserved"
  | "exists"
  | "is_directory"
  | "under_file";

/**
 * Validate a path for a new file or a rename target. `existing` are the paths
 * that would remain, so a rename does not collide with itself.
 */
export function validateHubPath(path: string, existing: readonly string[]): HubPathError | null {
  const p = path.trim();
  if (!p) return "empty";
  if (p.startsWith("/")) return "absolute";
  if (p.split("/").includes("..")) return "double_dot";
  if (p === HUB_PRIMARY_FILE) return "reserved";
  if (existing.includes(p)) return "exists";
  // A file cannot take a folder's name, nor sit under an existing file's name.
  if (existing.some((other) => other.startsWith(`${p}/`))) return "is_directory";
  if (existing.some((other) => p.startsWith(`${other}/`))) return "under_file";
  return null;
}

/** Every path under a folder, plus the folder's own rename pairs. */
export function folderMembers(paths: readonly string[], folder: string): string[] {
  const prefix = `${folder}/`;
  return paths.filter((path) => path.startsWith(prefix));
}

/** Pairs of [from, to] for renaming a file, or every file under a folder. */
export function renamePairs(
  paths: readonly string[],
  from: string,
  to: string,
  isFolder: boolean,
): Array<[string, string]> {
  if (!isFolder) return [[from, to]];
  return folderMembers(paths, from).map((path) => [path, `${to}${path.slice(from.length)}`]);
}

// Tree

export interface HubTreeRow {
  /** Full path: a file path, or a folder path without a trailing slash. */
  path: string;
  name: string;
  depth: number;
  isFolder: boolean;
  expanded: boolean;
}

interface TreeNode {
  name: string;
  path: string;
  isFolder: boolean;
  children: TreeNode[];
}

function buildNodes(paths: readonly string[]): TreeNode[] {
  const root: TreeNode[] = [];
  for (const filePath of paths) {
    const parts = filePath.split("/");
    let level = root;
    parts.forEach((name, index) => {
      const path = parts.slice(0, index + 1).join("/");
      const isFolder = index < parts.length - 1;
      let node = level.find((candidate) => candidate.name === name);
      if (!node) {
        node = { name, path, isFolder, children: [] };
        level.push(node);
      }
      level = node.children;
    });
  }
  return root;
}

function sortNodes(nodes: TreeNode[]): TreeNode[] {
  nodes.sort((a, b) => {
    // The primary file leads the root, ahead of folders.
    if (a.path === HUB_PRIMARY_FILE) return -1;
    if (b.path === HUB_PRIMARY_FILE) return 1;
    if (a.isFolder !== b.isFolder) return a.isFolder ? -1 : 1;
    return a.name.localeCompare(b.name);
  });
  for (const node of nodes) if (node.isFolder) sortNodes(node.children);
  return nodes;
}

/**
 * The visible rows of the tree, depth-first. Folders are expanded unless their
 * path is in `collapsed`, so a fresh tree shows everything.
 */
export function flattenHubTree(paths: readonly string[], collapsed: ReadonlySet<string>): HubTreeRow[] {
  const rows: HubTreeRow[] = [];
  const visit = (nodes: TreeNode[], depth: number) => {
    for (const node of nodes) {
      const expanded = node.isFolder && !collapsed.has(node.path);
      rows.push({
        path: node.path,
        name: node.name,
        depth,
        isFolder: node.isFolder,
        expanded,
      });
      if (expanded) visit(node.children, depth + 1);
    }
  };
  visit(sortNodes(buildNodes(paths)), 0);
  return rows;
}

// Plugin draft

/** The paths a plugin shows once its draft is applied over the base version. */
export function applyPluginChanges(basePaths: readonly string[], changes: PluginChanges): string[] {
  const paths = new Set(basePaths);
  for (const [path, value] of Object.entries(changes)) {
    if (value === null) paths.delete(path);
    else paths.add(path);
  }
  return [...paths].sort();
}

/**
 * Apply a rename to a plugin draft. `fromContent` is the file's text as it reads
 * now (the draft value, or the base version's text). The base path is marked
 * removed, and the new path carries the content.
 */
export function renamePluginChange(
  changes: PluginChanges,
  from: string,
  to: string,
  fromContent: string,
  fromInBase: boolean,
): PluginChanges {
  const next = { ...changes };
  if (fromInBase) next[from] = null;
  else delete next[from];
  next[to] = fromContent;
  return next;
}

/** Delete a path from a plugin draft: base files are marked removed, drafts dropped. */
export function deletePluginChange(
  changes: PluginChanges,
  path: string,
  inBase: boolean,
): PluginChanges {
  const next = { ...changes };
  if (inBase) next[path] = null;
  else delete next[path];
  return next;
}

// Preview

export type HubFilePreview = "markdown" | "json" | "svg" | "text" | "binary";

const RASTER_IMAGE_EXTENSIONS = new Set(["png", "jpg", "jpeg", "gif", "webp", "avif", "bmp", "ico"]);

function extensionOf(path: string): string {
  const dot = path.lastIndexOf(".");
  return dot < 0 ? "" : path.slice(dot + 1).toLowerCase();
}

/**
 * How a file is shown. The server decides binary-ness; the extension only picks
 * the renderer for text. Raster images have no text to show, so they fall back
 * to the binary notice with the rest of the binary files.
 */
export function previewKind(path: string, isBinary: boolean): HubFilePreview {
  if (isBinary) return "binary";
  const ext = extensionOf(path);
  if (ext === "md" || ext === "mdx") return "markdown";
  if (ext === "json") return "json";
  if (ext === "svg") return "svg";
  if (RASTER_IMAGE_EXTENSIONS.has(ext)) return "binary";
  return "text";
}

/** Pretty-printed JSON, or `null` when the text does not parse. */
export function prettyJson(text: string): string | null {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return null;
  }
}

// Versions

const SEMVER_PATTERN =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

/** The version format plugin publish accepts (semantic versioning). */
export function isSemanticVersion(version: string): boolean {
  return version.length <= 64 && SEMVER_PATTERN.test(version);
}

/** A byte count in the reader's locale, such as "12 kB". Units are the browser's own names. */
export function formatHubSize(bytes: number): string {
  const units = [
    { unit: "gigabyte", size: 1e9 },
    { unit: "megabyte", size: 1e6 },
    { unit: "kilobyte", size: 1e3 },
    { unit: "byte", size: 1 },
  ] as const;
  const picked = units.find((candidate) => bytes >= candidate.size) ?? units[3];
  return new Intl.NumberFormat(undefined, {
    style: "unit",
    unit: picked.unit,
    unitDisplay: "narrow",
    maximumFractionDigits: picked.size === 1 ? 0 : 1,
  }).format(bytes / picked.size);
}
