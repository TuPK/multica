import { api } from "../api";

/**
 * The export zip of every item's bundle, plus manifest.json. Admin and owner only.
 * Returns bytes rather than a URL: a plain link cannot carry the auth headers
 * that a bearer-token client needs. The caller saves the blob.
 */
export function getHubExportBlob(wsId: string): Promise<Blob> {
  return api.getHubExportBlob(wsId);
}
