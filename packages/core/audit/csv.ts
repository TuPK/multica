import { api } from "../api";
import type { AuditFilters } from "./params";

export interface AuditCsvExport {
  text: string;
  /** True when the server stopped at its row cap and more rows exist. */
  truncated: boolean;
}

/** Fetches the CSV export of the workspace audit log for a download. */
export function exportAuditCsv(
  wsId: string,
  filters: AuditFilters = {},
): Promise<AuditCsvExport> {
  return api.exportWorkspaceAuditCsv(wsId, filters);
}
