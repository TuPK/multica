import type { HubImportPreview, HubImportSourceRequest } from "@multica/core/hub";

/** Source types the From URL tab can preview. Registry items are picked from a registry listing. */
export type UrlImportType = "github" | "url_zip" | "skills_sh" | "clawhub";

/** What the wizard previewed, kept so an expired preview can be run again. */
export type HubImportRequest =
  | { kind: "source"; source: HubImportSourceRequest }
  | { kind: "file"; file: File };

export interface HubImportState {
  request: HubImportRequest;
  preview: HubImportPreview;
}

export function previewVars(wsId: string, request: HubImportRequest) {
  return request.kind === "file" ? { wsId, file: request.file } : { wsId, source: request.source };
}

/** Only ClawHub takes a slug; every other source is an address the server fetches. */
export function importNeedsWebAddress(type: UrlImportType): boolean {
  return type !== "clawhub";
}

/** Returns the absolute http(s) URL, or null when the text is not one. */
export function parseWebAddress(raw: string): string | null {
  try {
    const url = new URL(raw.trim());
    return url.protocol === "https:" || url.protocol === "http:" ? url.toString() : null;
  } catch {
    return null;
  }
}

/** Registry items may link relative to the registry file. */
export function resolveRegistryItemUrl(registryUrl: string, itemUrl: string): string {
  try {
    return new URL(itemUrl, registryUrl).toString();
  } catch {
    return itemUrl;
  }
}

/**
 * A default name for a source saved from a URL: "owner/repo" for GitHub, otherwise the host
 * and last path segment. The user can rename it later in the sources manager.
 */
export function suggestSourceName(type: UrlImportType, rawUrl: string): string {
  const trimmed = rawUrl.trim();
  let url: URL;
  try {
    url = new URL(trimmed);
  } catch {
    return trimmed;
  }
  const segments = url.pathname.split("/").filter(Boolean);
  if (type === "github" && segments.length >= 2) {
    return `${segments[0]}/${segments[1]!.replace(/\.git$/, "")}`;
  }
  const last = segments.at(-1);
  return last ? `${url.host}/${last}` : url.host;
}

export function errorMessage(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback;
}
