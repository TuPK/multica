import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { pluginKeys, pluginPackagesOptions } from "../plugins/queries";
import { packStoreZip, uint8ToArrayBuffer } from "../skills/pack-archive";
import type { PluginPackage } from "../types";
import { hubKeys, validateHubFilePath } from "./files";

/** Conventional manifest name inside a package, from server/pkg/plugincontract. */
export const PLUGIN_MANIFEST_FILENAME = "multica.plugin.json";

/** Mirrors the server's semver pattern and plugin_installation.version bound. */
const SEMVER_PATTERN =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;
const MAX_VERSION_LENGTH = 64;

export interface SavePluginDraftVars {
  wsId: string;
  refId: string;
  baseVersionId: string;
  /** path -> new text; null deletes the file. */
  changes: Record<string, string | null>;
  newVersion: string;
}

export interface PluginDraftInput {
  pluginKey: string;
  /** Versions already published for this plugin. The new one must not be among them. */
  existingVersions: readonly string[];
  newVersion: string;
  changes: Record<string, string | null>;
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function parseManifest(text: string): Record<string, unknown> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    throw new Error("The plugin manifest is not valid JSON");
  }
  if (!isPlainObject(parsed)) throw new Error("The plugin manifest must be a JSON object");
  return parsed;
}

/**
 * Paths the manifest names and publish requires: surface and resource entries,
 * and the icon. Other files are dropped at publish, so they are not carried over.
 */
function manifestReferences(manifest: Record<string, unknown>): string[] {
  const contributes = isPlainObject(manifest.contributes) ? manifest.contributes : {};
  const entries: unknown[] = [];
  for (const group of [contributes.surfaces, contributes.resources]) {
    if (!Array.isArray(group)) continue;
    for (const item of group) {
      if (isPlainObject(item)) entries.push(item.entry);
    }
  }
  entries.push(manifest.icon);
  const paths = new Set<string>();
  for (const entry of entries) {
    if (typeof entry === "string" && entry !== "") paths.add(entry);
  }
  return [...paths];
}

/**
 * Builds the zip for a new draft version: the manifest with its version replaced,
 * the files it references, and any changed files. `loadBaseText` supplies unchanged
 * base files and must throw if a file cannot be read as text.
 */
export async function buildPluginDraftFile(
  input: PluginDraftInput,
  loadBaseText: (path: string) => Promise<string>,
): Promise<File> {
  const { newVersion, changes } = input;
  if (newVersion.length > MAX_VERSION_LENGTH || !SEMVER_PATTERN.test(newVersion)) {
    throw new Error(`Version must be semantic versioning, got "${newVersion}"`);
  }
  if (input.existingVersions.includes(newVersion)) {
    throw new Error(`Version ${newVersion} is already published`);
  }

  const manifestChange = changes[PLUGIN_MANIFEST_FILENAME];
  if (manifestChange === null) throw new Error("The plugin manifest cannot be deleted");
  const manifest = parseManifest(
    manifestChange ?? (await loadBaseText(PLUGIN_MANIFEST_FILENAME)),
  );
  if (manifest.key !== input.pluginKey) {
    throw new Error(`The manifest key must stay "${input.pluginKey}"`);
  }
  manifest.version = newVersion;

  const files = new Map<string, string>();
  files.set(PLUGIN_MANIFEST_FILENAME, `${JSON.stringify(manifest, null, 2)}\n`);
  const references = manifestReferences(manifest);
  for (const [path, text] of Object.entries(changes)) {
    if (path === PLUGIN_MANIFEST_FILENAME) continue;
    validateHubFilePath(path);
    if (text === null) {
      if (references.includes(path)) {
        throw new Error(`${path} is named by the manifest and cannot be deleted`);
      }
      continue;
    }
    files.set(path, text);
  }
  for (const path of references) {
    validateHubFilePath(path);
    if (files.has(path)) continue;
    files.set(path, await loadBaseText(path));
  }

  const encoder = new TextEncoder();
  const zip = packStoreZip(
    [...files].map(([path, text]) => ({ path, data: encoder.encode(text) })),
  );
  return new File([uint8ToArrayBuffer(zip)], `${input.pluginKey}-${newVersion}.zip`, {
    type: "application/zip",
  });
}

/**
 * Saves changes as a new draft version and publishes it. Published versions are
 * immutable, so the result is always a new version, never an edit in place.
 */
export function useSavePluginDraft() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (vars: SavePluginDraftVars): Promise<PluginPackage> => {
      const { wsId, refId, baseVersionId, changes, newVersion } = vars;
      const { packages } = await qc.fetchQuery({ ...pluginPackagesOptions(wsId), staleTime: 0 });
      const pkg = packages.find((candidate) => candidate.id === refId);
      if (!pkg) throw new Error("Plugin not found");
      if (!pkg.versions.some((version) => version.id === baseVersionId)) {
        throw new Error("The base version does not belong to this plugin");
      }

      const listing = await api.listPluginVersionFiles(wsId, refId, baseVersionId);
      const listed = new Set(listing.files.map((file) => file.path));
      const file = await buildPluginDraftFile(
        {
          pluginKey: pkg.plugin_key,
          existingVersions: pkg.versions.map((version) => version.version),
          newVersion,
          changes,
        },
        async (path) => {
          if (path !== PLUGIN_MANIFEST_FILENAME && !listed.has(path)) {
            throw new Error(`${path} is named by the manifest but missing from the version`);
          }
          const base = await api.getPluginVersionFileContent(wsId, refId, baseVersionId, path);
          if (base.content === null) {
            throw new Error(`${path} is binary or too large to edit in a draft`);
          }
          return base.content;
        },
      );
      return api.publishPluginPackage(wsId, file);
    },
    onSettled: (_data, _error, vars) => {
      if (!vars) return;
      qc.invalidateQueries({ queryKey: pluginKeys.packages(vars.wsId) });
      qc.invalidateQueries({ queryKey: hubKeys.files(vars.wsId, "plugin", vars.refId) });
    },
  });
}
