import { api } from "../api";
import type { HubBundle } from "../types/hub";
import type { HubItemKind } from "./items";

/**
 * The bundle's download url and metadata. Not cached: a presigned url expires,
 * so each download asks for a fresh one.
 */
export function getHubBundle(wsId: string, kind: HubItemKind, refId: string): Promise<HubBundle> {
  return api.getHubBundle(wsId, kind, refId);
}

/**
 * Resolves the download url for the view to open. Core stays DOM-free, so it
 * does not create or click a link. A relative url (the same-origin download
 * route) is made absolute against the API base. A bearer-token client cannot
 * send its token with a plain link, so such a client should save the bytes
 * through its own fetch instead.
 */
export async function downloadHubBundle(wsId: string, kind: HubItemKind, refId: string): Promise<string> {
  const bundle = await getHubBundle(wsId, kind, refId);
  if (/^https?:\/\//.test(bundle.url)) return bundle.url;
  return `${api.getBaseUrl()}${bundle.url}`;
}

export type HubBundleDownload =
  | { kind: "url"; url: string; version: string }
  | { kind: "blob"; blob: Blob; version: string; digest: string };

/**
 * Resolves a bundle for saving. A presigned (absolute) url is returned for the
 * view to open; a relative same-origin route needs the client's auth headers,
 * so its bytes are fetched here and returned as a blob for the view to save.
 */
export async function fetchHubBundleDownload(
  wsId: string,
  kind: HubItemKind,
  refId: string,
): Promise<HubBundleDownload> {
  const bundle = await getHubBundle(wsId, kind, refId);
  if (/^https?:\/\//.test(bundle.url)) return { kind: "url", url: bundle.url, version: bundle.version };
  const blob = await api.getHubBundleBlob(wsId, bundle.url);
  return { kind: "blob", blob, version: bundle.version, digest: bundle.digest };
}
