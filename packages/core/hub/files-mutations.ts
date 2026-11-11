import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { MAX_SKILL_FILE_BYTES } from "../skills/pack-archive";
import { workspaceKeys } from "../workspace/queries";
import {
  hubKeys,
  isSkillContentPath,
  SKILL_CONTENT_PATH,
  utf8ByteLength,
  validateHubFilePath,
  type HubFileContent,
} from "./files";

export interface SaveHubFileVars {
  wsId: string;
  refId: string;
  path: string;
  content: string;
  /** The etag the editor loaded with; "" for a file that should not exist yet. */
  etag: string;
}

export interface DeleteHubFileVars {
  wsId: string;
  refId: string;
  path: string;
}

export interface RenameHubFileVars {
  wsId: string;
  refId: string;
  from: string;
  to: string;
}

function invalidateSkillFiles(qc: QueryClient, wsId: string, refId: string) {
  qc.invalidateQueries({ queryKey: hubKeys.files(wsId, "skill", refId) });
  // Skill list and detail caches carry the file bodies and the skill updated_at.
  qc.invalidateQueries({ queryKey: workspaceKeys.skills(wsId) });
}

function assertNotSkillContent(path: string, action: string) {
  if (isSkillContentPath(path)) throw new Error(`SKILL.md cannot be ${action}`);
}

/**
 * Saves one skill file. The server checks the etag under its own lock: a stale etag
 * is refused with 409 and surfaces as HubFileConflictError, carrying the current etag.
 * SKILL.md is saved through the same route.
 */
export function useSaveHubFile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (vars: SaveHubFileVars): Promise<HubFileContent> => {
      const path = validateHubFilePath(vars.path);
      const size = utf8ByteLength(vars.content);
      if (size > MAX_SKILL_FILE_BYTES) throw new Error("File is larger than 1 MiB");

      const savePath = isSkillContentPath(path) ? SKILL_CONTENT_PATH : path;
      const saved = await api.saveHubSkillFile(vars.wsId, vars.refId, {
        path: savePath,
        content: vars.content,
        etag: vars.etag,
      });
      return { path: savePath, content: vars.content, is_binary: false, size_bytes: size, etag: saved.etag };
    },
    onSettled: (_data, _error, vars) => {
      if (vars) invalidateSkillFiles(qc, vars.wsId, vars.refId);
    },
  });
}

/** Deletes a skill file by path. SKILL.md is refused. A path that is already gone is a no-op. */
export function useDeleteHubFile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (vars: DeleteHubFileVars): Promise<void> => {
      assertNotSkillContent(vars.path, "deleted");
      const skill = await api.getSkill(vars.refId);
      const file = (skill.files ?? []).find((candidate) => candidate.path === vars.path);
      if (!file) return;
      await api.deleteSkillFile(vars.refId, file.id);
    },
    onSettled: (_data, _error, vars) => {
      if (vars) invalidateSkillFiles(qc, vars.wsId, vars.refId);
    },
  });
}

/**
 * Renames a skill file: writes the new path, then deletes the old one. Two
 * requests, not one transaction. If the delete fails the file exists at both
 * paths and the error says so.
 */
export function useRenameHubFile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (vars: RenameHubFileVars): Promise<HubFileContent> => {
      assertNotSkillContent(vars.from, "renamed");
      assertNotSkillContent(vars.to, "a rename target");
      const to = validateHubFilePath(vars.to);
      if (vars.from === to) throw new Error("The new path is the same as the old one");

      const skill = await api.getSkill(vars.refId);
      const source = (skill.files ?? []).find((file) => file.path === vars.from);
      if (!source) throw new Error(`File not found: ${vars.from}`);
      if ((skill.files ?? []).some((file) => file.path === to)) {
        throw new Error(`A file already exists at ${to}`);
      }

      const content = source.content ?? "";
      const saved = await api.upsertSkillFile(vars.refId, { path: to, content });
      try {
        await api.deleteSkillFile(vars.refId, source.id);
      } catch (error) {
        const reason = error instanceof Error ? error.message : String(error);
        throw new Error(`Renamed to ${to}, but ${vars.from} could not be removed: ${reason}`);
      }
      return { path: to, content, is_binary: false, size_bytes: utf8ByteLength(content), etag: saved.updated_at };
    },
    onSettled: (_data, _error, vars) => {
      if (vars) invalidateSkillFiles(qc, vars.wsId, vars.refId);
    },
  });
}
