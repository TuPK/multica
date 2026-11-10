import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import { MAX_SKILL_FILE_BYTES } from "../skills/pack-archive";
import type { Skill } from "../types";
import type { HubItemKind } from "./items";

/** A skill's SKILL.md is shown as a file, but it lives in the skill's `content` field. */
export const SKILL_CONTENT_PATH = "SKILL.md";

export interface HubFileMeta {
  path: string;
  size_bytes: number;
}

export interface HubFileContent {
  path: string;
  /** Null when the file is binary or larger than 1 MiB. */
  content: string | null;
  is_binary: boolean;
  size_bytes: number;
  /** Skill file `updated_at`, or plugin file `sha256`. Pass it back when saving. */
  etag: string;
}

export class HubFileConflictError extends Error {
  constructor(message = "The file changed since it was loaded") {
    super(message);
    this.name = "HubFileConflictError";
  }
}

export const hubKeys = {
  all: (wsId: string) => ["workspaces", wsId, "hub"] as const,
  files: (wsId: string, kind: HubItemKind, refId: string) =>
    [...hubKeys.all(wsId), "files", kind, refId] as const,
  fileList: (wsId: string, kind: HubItemKind, refId: string, versionId?: string) =>
    [...hubKeys.files(wsId, kind, refId), "list", versionId ?? "current"] as const,
  fileContent: (wsId: string, kind: HubItemKind, refId: string, path: string, versionId?: string) =>
    [...hubKeys.files(wsId, kind, refId), "content", versionId ?? "current", path] as const,
};

/** SKILL.md is matched case-insensitively, as the server reserves it. */
export function isSkillContentPath(path: string): boolean {
  return path.toLowerCase() === SKILL_CONTENT_PATH.toLowerCase();
}

/** Mirrors server/internal/skill IsSafeFilePath: relative, clean, no backslashes or drive letters. */
export function validateHubFilePath(path: string): string {
  const invalid =
    path === "" ||
    path.includes("\0") ||
    path.startsWith("/") ||
    path.includes("\\") ||
    /^[A-Za-z]:/.test(path) ||
    path.split("/").some((segment) => segment === "" || segment === "." || segment === "..");
  if (invalid) throw new Error(`Invalid file path: ${path}`);
  return path;
}

export function utf8ByteLength(text: string): number {
  return new TextEncoder().encode(text).byteLength;
}

function sortByPath<T extends { path: string }>(items: T[]): T[] {
  return items.sort((a, b) => a.path.localeCompare(b.path));
}

/** SKILL.md first, then every other file by path. */
export function skillFileList(skill: Skill): HubFileMeta[] {
  const others = (skill.files ?? [])
    .filter((file) => !isSkillContentPath(file.path))
    .map((file) => ({ path: file.path, size_bytes: utf8ByteLength(file.content ?? "") }));
  return [
    { path: SKILL_CONTENT_PATH, size_bytes: utf8ByteLength(skill.content ?? "") },
    ...sortByPath(others),
  ];
}

/** The etag a file currently has, or null when the file does not exist. */
export function skillFileEtag(skill: Skill, path: string): string | null {
  if (isSkillContentPath(path)) return skill.updated_at;
  const file = (skill.files ?? []).find((candidate) => candidate.path === path);
  return file ? file.updated_at ?? "" : null;
}

export function skillFileContent(skill: Skill, path: string): HubFileContent {
  const etag = skillFileEtag(skill, path);
  if (etag === null) throw new Error(`File not found: ${path}`);
  const text = isSkillContentPath(path)
    ? skill.content ?? ""
    : (skill.files ?? []).find((file) => file.path === path)?.content ?? "";
  const size = utf8ByteLength(text);
  return {
    path: isSkillContentPath(path) ? SKILL_CONTENT_PATH : path,
    content: size > MAX_SKILL_FILE_BYTES ? null : text,
    is_binary: false,
    size_bytes: size,
    etag,
  };
}

/**
 * Refuses a write when the file changed since the caller read it. `etag` "" means
 * the caller expects the file not to exist yet. There is no server-side compare,
 * so a change landing between this read and the write is not caught.
 */
export function assertSkillFileUnchanged(skill: Skill, path: string, etag: string): void {
  const current = skillFileEtag(skill, path);
  if (current === null) {
    if (etag === "") return;
    throw new HubFileConflictError(`${path} was deleted since it was loaded`);
  }
  if (etag === "" || current !== etag) {
    throw new HubFileConflictError(`${path} changed since it was loaded`);
  }
}

/** The newest published version when none is named. Versions are newest first. */
async function resolvePluginVersionId(wsId: string, packageId: string, versionId?: string): Promise<string> {
  if (versionId) return versionId;
  const { packages } = await api.listPluginPackages(wsId);
  const newest = packages.find((pkg) => pkg.id === packageId)?.versions[0];
  if (!newest) throw new Error("This plugin has no published version");
  return newest.id;
}

async function loadFileList(
  wsId: string,
  kind: HubItemKind,
  refId: string,
  versionId?: string,
): Promise<HubFileMeta[]> {
  if (kind === "skill") return skillFileList(await api.getSkill(refId));
  const resolved = await resolvePluginVersionId(wsId, refId, versionId);
  const { files } = await api.listPluginVersionFiles(wsId, refId, resolved);
  return sortByPath(files.map((file) => ({ path: file.path, size_bytes: file.size_bytes })));
}

async function loadFileContent(
  wsId: string,
  kind: HubItemKind,
  refId: string,
  path: string,
  versionId?: string,
): Promise<HubFileContent> {
  if (kind === "skill") return skillFileContent(await api.getSkill(refId), path);
  const resolved = await resolvePluginVersionId(wsId, refId, versionId);
  const file = await api.getPluginVersionFileContent(wsId, refId, resolved, path);
  return {
    path,
    content: file.content,
    is_binary: file.is_binary,
    size_bytes: file.size_bytes,
    etag: file.sha256,
  };
}

/** Files of a skill (SKILL.md first) or of a plugin version (newest when `versionId` is omitted). */
export function hubFileListOptions(
  wsId: string,
  kind: HubItemKind,
  refId: string,
  versionId?: string,
) {
  return queryOptions({
    queryKey: hubKeys.fileList(wsId, kind, refId, versionId),
    queryFn: () => loadFileList(wsId, kind, refId, versionId),
    enabled: wsId.length > 0 && refId.length > 0,
  });
}

export function hubFileContentOptions(
  wsId: string,
  kind: HubItemKind,
  refId: string,
  path: string,
  versionId?: string,
) {
  return queryOptions({
    queryKey: hubKeys.fileContent(wsId, kind, refId, path, versionId),
    queryFn: () => loadFileContent(wsId, kind, refId, path, versionId),
    enabled: wsId.length > 0 && refId.length > 0 && path.length > 0,
  });
}
