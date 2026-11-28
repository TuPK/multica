import { useMutation } from "@tanstack/react-query";
import { api, ApiError } from "../../api";
import { parseWithFallback } from "../../api/schema";
import { getCurrentSlug } from "../../platform";
import { toQueryString } from "../query-string";
import { ocfErrorSchema } from "./schemas";

export type ExportOCFRequest = {
  /** Include settled outcomes, track records, governance log and active dossiers. */
  history: boolean;
  /**
   * Include the requesting person's principal profile. Only honored when
   * `history` is true and the person has consented to a principal profile.
   */
  include_principal?: boolean;
};

/** Builds the OCF export endpoint path matching server handler company_ocf_fork.go. */
export function buildOCFExportPath(opts: ExportOCFRequest): string {
  const params: Record<string, string> = {
    format: "ocf",
    history: opts.history ? "true" : "false",
  };
  if (opts.history && opts.include_principal) {
    params.include_principal = "true";
  }
  return `/api/company/export${toQueryString(params)}`;
}

async function parseErrorResponse(res: Response): Promise<{ message: string; body: unknown }> {
  const fallback = `API error: ${res.status} ${res.statusText}`;
  try {
    const data = parseWithFallback<{ error?: string }>(await res.json(), ocfErrorSchema, {}, {
      endpoint: "company/export",
    });
    const message = typeof data.error === "string" && data.error ? data.error : fallback;
    return { message, body: data };
  } catch {
    return { message: fallback, body: undefined };
  }
}

/**
 * Downloads the workspace as an OCF v1 zip archive (`company.ocf.zip`). Uses the
 * shared API client's base URL and bearer token when present so it works on both
 * web (cookie session + proxy) and desktop (bearer token + remote base URL).
 */
export async function downloadOCFExport(wsId: string, opts: ExportOCFRequest): Promise<Blob> {
  const path = buildOCFExportPath(opts);
  const baseUrl = (api.getBaseUrl?.() ?? "").replace(/\/+$/, "");
  const headers: Record<string, string> = {};
  const token = api.getToken?.();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const slug = getCurrentSlug();
  if (slug) headers["X-Workspace-Slug"] = slug;
  if (wsId) headers["X-Workspace-ID"] = wsId;

  const res = await fetch(`${baseUrl}${path}`, {
    method: "GET",
    headers,
    credentials: "include",
  });
  if (!res.ok) {
    const { message, body } = await parseErrorResponse(res);
    throw new ApiError(message, res.status, res.statusText, body);
  }
  return res.blob();
}

/** Mutation wrapper for exporting an OCF v1 package with optional history and principal profile. */
export function useExportOCF(wsId: string) {
  return useMutation<Blob, Error, ExportOCFRequest>({
    mutationFn: (opts) => downloadOCFExport(wsId, opts),
  });
}
