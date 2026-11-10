import type { HubSourceType } from "../hub/items";

/**
 * A place Multica imports hub items from. Sources are workspace-scoped; the
 * items they bring in are ordinary skills or plugin packages afterwards.
 */
export interface HubSource {
  id: string;
  name: string;
  type: HubSourceType;
  url: string;
  config: Record<string, unknown>;
  enabled: boolean;
  auto_sync_interval_sec: number;
  last_sync_at: string | null;
  last_sync_status: string;
  last_sync_error: string | null;
  item_count: number;
  created_at: string;
  updated_at: string;
}

export interface CreateHubSourceRequest {
  name: string;
  type: HubSourceType;
  url: string;
  config?: Record<string, unknown>;
}

export interface UpdateHubSourceRequest {
  name?: string;
  url?: string;
  config?: Record<string, unknown>;
  enabled?: boolean;
  auto_sync_interval_sec?: number;
}

/** "keep" detaches the source's items; "delete" removes the ones no agent uses. */
export type HubSourceDeleteItems = "keep" | "delete";

export interface HubSourceInUseRef {
  kind: string;
  id: string;
  name: string;
}

/** Source types a preview can be made from. Uploads go through a file instead. */
export type HubImportSourceType = "github" | "url_zip" | "skills_sh" | "clawhub" | "registry";

export interface HubImportSourceRequest {
  type: HubImportSourceType;
  url: string;
  ref?: string;
  subdir?: string;
  sha256?: string;
}

export type HubImportKind = "skill" | "plugin" | "registry";

export type HubScanStatus = "clean" | "warn" | "blocked";

export type HubFindingSeverity = "info" | "warn" | "block";

export interface HubImportFinding {
  rule: string;
  severity: HubFindingSeverity;
  path: string;
  message: string;
}

export interface HubImportScan {
  status: HubScanStatus;
  findings: HubImportFinding[];
}

export interface HubImportPreviewFile {
  path: string;
  size_bytes: number;
}

export interface HubImportConflict {
  exists: boolean;
  item_id?: string;
  kind?: "skill" | "plugin";
}

/** One entry of a registry index. Previewing it means a url_zip preview of its url and sha256. */
export interface HubRegistryItem {
  kind: "skill" | "plugin";
  name: string;
  description: string;
  version: string | null;
  url: string;
  sha256: string;
  tags: string[];
}

export interface HubRegistryIndex {
  name: string;
  items: HubRegistryItem[];
}

export interface HubImportPreview {
  preview_id: string;
  kind: HubImportKind;
  name: string;
  description: string;
  version: string | null;
  files: HubImportPreviewFile[];
  scan: HubImportScan;
  conflict: HubImportConflict;
  /** Only present when `kind` is "registry". */
  registry?: HubRegistryIndex;
}

export type HubConflictPolicy = "rename" | "replace" | "skip";

export interface HubImportCommitRequest {
  preview_id: string;
  on_conflict: HubConflictPolicy;
  name?: string;
  confirm_warnings: boolean;
  source_id?: string;
}

export interface HubImportCommitResult {
  kind: "skill" | "plugin";
  id: string;
  hub_item_id: string;
  skipped: boolean;
}
