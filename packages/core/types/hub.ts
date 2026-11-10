import type { HubItemKind, HubSourceType } from "../hub/items";

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

/** Phase 4-6 types: per-item metadata, bundles, sync and update checks, activity, quota. */

/** `tracking` follows the source; `forked` detaches from it; `manual` has no source. */
export type HubItemTracking = "tracking" | "forked" | "manual";

/** One row of GET /api/hub/items: every skill and plugin package of the workspace. */
export interface HubItemMeta {
  id: string;
  kind: HubItemKind;
  ref_id: string;
  source_id: string | null;
  source_ref: string;
  tracking: HubItemTracking;
  latest_version: string;
  bundle_digest: string;
  size_bytes: number;
  file_count: number;
  scan_status: HubScanStatus;
  update_available: boolean;
  remote_digest: string;
  checked_at: string | null;
  local_modified: boolean;
  created_at: string;
  updated_at: string;
}

export interface HubBundle {
  url: string;
  expires_in: number;
  digest: string;
  size_bytes: number;
  version: string;
}

export type HubSyncChange = "update_available" | "unchanged" | "error";

export interface HubSyncItem {
  kind: HubItemKind;
  id: string;
  name: string;
  change: HubSyncChange;
  error?: string;
}

export interface HubSyncResult {
  status: "ok" | "error";
  checked: number;
  updates_available: number;
  unchanged: number;
  error?: string;
  items: HubSyncItem[];
}

export type HubUpdateFileStatus = "added" | "modified" | "removed" | "unchanged";

export interface HubUpdateFile {
  path: string;
  status: HubUpdateFileStatus;
  old_size: number | null;
  new_size: number | null;
}

export interface HubUpdateCheck {
  update_available: boolean;
  current_digest: string;
  remote_digest: string;
  local_modified: boolean;
  files: HubUpdateFile[];
}

export interface HubUpdateDiff {
  path: string;
  old: string | null;
  new: string | null;
  binary: boolean;
}

/** `overwrite` replaces local edits; `fork` keeps them by detaching the item first. */
export type HubLocalChangesPolicy = "overwrite" | "fork";

export interface HubApplyUpdateResult {
  kind: HubItemKind;
  id: string;
  version: string;
  tracking: HubItemTracking;
}

export interface HubForkResult {
  kind: HubItemKind;
  id: string;
  tracking: "forked";
}

export interface HubActivityEntry {
  id: string;
  action: string;
  actor_type: string;
  actor_id: string | null;
  actor_name: string;
  kind: HubItemKind | null;
  ref_id: string | null;
  item_name: string;
  source_id: string | null;
  detail: Record<string, unknown>;
  created_at: string;
}

export interface HubActivityFilter {
  kind?: HubItemKind;
  refId?: string;
  sourceId?: string;
}

export interface HubActivityPage {
  entries: HubActivityEntry[];
  next_cursor: string | null;
}

export interface HubQuotaUsage {
  used: number;
  /** A limit of 0 or less means unlimited. */
  limit: number;
}

export interface HubQuota {
  items: HubQuotaUsage;
  storage_bytes: HubQuotaUsage;
}
