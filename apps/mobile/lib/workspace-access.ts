/**
 * Pure write-permission rules and workspace refusal helpers.
 *
 * Kept React/RN-free so they can be unit tested under Node without mocks.
 */

export type WorkspaceBlockReason =
  | "read_only"
  | "suspended"
  | "pending_deletion";

export type WorkspaceRefusalCode =
  | "workspace_read_only"
  | "workspace_suspended"
  | "workspace_pending_deletion";

const REFUSAL_TO_BLOCK: Record<WorkspaceRefusalCode, WorkspaceBlockReason> = {
  workspace_read_only: "read_only",
  workspace_suspended: "suspended",
  workspace_pending_deletion: "pending_deletion",
};

export function isWorkspaceRefusalCode(
  code: string | undefined,
): code is WorkspaceRefusalCode {
  return code !== undefined && code in REFUSAL_TO_BLOCK;
}

export function workspaceBlockFromCode(
  code: string | undefined,
): WorkspaceBlockReason | null {
  if (!isWorkspaceRefusalCode(code)) return null;
  return REFUSAL_TO_BLOCK[code];
}

const WRITABLE_ROLES = new Set(["owner", "admin", "member"]);

/**
 * Only owner, admin, and member roles in an unblocked workspace may perform
 * non-personal workspace writes. Unknown, missing, or "viewer" roles and any
 * blocked workspace ("read_only", "suspended", "pending_deletion") fail closed.
 */
export function canWriteInWorkspace({
  role,
  block,
}: {
  role: string | null | undefined;
  block: WorkspaceBlockReason | null | undefined;
}): boolean {
  if (block != null) return false;
  if (!role) return false;
  return WRITABLE_ROLES.has(role);
}
