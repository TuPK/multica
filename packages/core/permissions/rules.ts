import type {
  Agent,
  Comment,
  Member,
  MemberRole,
  RuntimeDevice,
  Skill,
} from "../types";
import { ALLOW, deny, type Decision, type PermissionContext } from "./types";

/**
 * Pure permission rules — single source of truth that mirrors the Go backend
 * gates in `server/internal/handler/`. Hooks in `use-resource-permissions.ts`
 * are thin wrappers that pull `PermissionContext` from auth + member queries
 * and forward to these.
 *
 * Returning a `Decision` (not a boolean) lets every surface — disabled state,
 * tooltip, banner copy — read the same `reason` and stay consistent without
 * sprinkling copy through the view layer.
 *
 * Guests (MUL-9420) are deny-by-default, like the server's route allowlist in
 * `server/internal/middleware/guest.go`: a rule allows a guest only for what
 * that list lets through, even on a resource the guest created or owned
 * before it became a guest. Add every new rule to the guest table in
 * `guest-deny.test.ts`.
 */

/**
 * Context with optional guest marker (`is_guest` / `isGuest`) so existing
 * callers passing `PermissionContext` remain compatible while guest checks
 * enforce spec 7.1 rule 9.
 */
export type GuestPermissionContext = PermissionContext & {
  isGuest?: boolean;
  is_guest?: boolean;
};

const isAdminLike = (role: MemberRole | null) =>
  role === "owner" || role === "admin";

/**
 * A viewer writes nothing (spec 6.6). A role this client does not know counts
 * as a viewer, so a newer server role never gains write rights here. A guest
 * is not a viewer: its rights come from the guest checks beside each rule.
 */
const isViewerRole = (role: string | null) =>
  role !== null &&
  role !== "owner" &&
  role !== "admin" &&
  role !== "member" &&
  role !== "guest";

const isGuest = (
  ctx:
    | { isGuest?: boolean; is_guest?: boolean; role?: string | null }
    | null
    | undefined,
) => ctx?.isGuest === true || ctx?.is_guest === true || ctx?.role === "guest";

const viewerDenial = () =>
  deny("viewer_read_only", "Viewers can only read this workspace.");

const guestDenial = () =>
  deny("unknown", "Guests cannot perform this action.");

// ---- Issues (spec 7.1 rule 9) ----------------------------------------------

export function canReadIssue(ctx: GuestPermissionContext): Decision {
  if (ctx.userId === null) {
    return deny("not_authenticated", "Sign in to view issues.");
  }
  if (ctx.role === null) {
    return deny("not_member", "Join this workspace to view issues.");
  }
  return ALLOW;
}

export function canWriteIssue(ctx: GuestPermissionContext): Decision {
  if (ctx.userId === null) {
    return deny("not_authenticated", "Sign in to edit issues.");
  }
  if (ctx.role === null) {
    return deny("not_member", "Join this workspace to edit issues.");
  }
  if (isViewerRole(ctx.role)) return viewerDenial();
  return ALLOW;
}

// ---- Account-group permission keys (spec 7b.2) ----------------------------

/**
 * The fixed catalogue of permission keys. Mirrors `membergroup.catalogue` on
 * the server. A key outside this list is refused here even if the server
 * lists it, so a newer build never grants a right this client cannot name.
 */
export const PERMISSION_KEYS = [
  "members.invite",
  "audit.read",
  "billing.view",
  "billing.manage",
  "integrations.manage",
  "runtime_profiles.manage",
] as const;

export type PermissionKey = (typeof PERMISSION_KEYS)[number];

/** What `/permissions/me` says about the caller. `null` means not loaded yet. */
export interface PermissionGrant {
  /** The server's role string. A role this client does not know is a viewer. */
  role: string | null;
  permissions: readonly string[];
  isGuest?: boolean;
  is_guest?: boolean;
}

const isPermissionKey = (key: string): key is PermissionKey =>
  (PERMISSION_KEYS as readonly string[]).includes(key);

/**
 * Whether the caller holds `key`, read from `/permissions/me`. Unknown keys
 * and an absent grant are refused. Pass `write: true` for a mutation: a
 * viewer never writes, even with a group that grants every key (spec 7b.2
 * rule 7). Reads use the key like any member.
 */
export function can(
  key: string,
  grant: PermissionGrant | null,
  opts: { write?: boolean } = {},
): Decision {
  if (!isPermissionKey(key)) {
    return deny("unknown", "This permission is not known to this version.");
  }
  if (grant === null) {
    return deny("unknown", "You do not have this permission.");
  }
  if (opts.write && isViewerRole(grant.role)) return viewerDenial();
  if (opts.write && isGuest(grant)) return guestDenial();
  if (grant.permissions.includes(key)) return ALLOW;
  return deny("unknown", "You do not have this permission.");
}
const guestDenied = (message: string) => deny("guest_role", message);

// ---- Agents ----------------------------------------------------------------

/**
 * Update / archive / restore agent fields. The backend gates archive and
 * restore identically to edit (`server/internal/handler/agent.go:519-535`),
 * so callers can use `canEditAgent` for all three.
 */
export function canEditAgent(agent: Agent, ctx: GuestPermissionContext): Decision {
  if (ctx.userId === null) {
    return deny("not_authenticated", "Sign in to edit this agent.");
  }
  if (isViewerRole(ctx.role)) return viewerDenial();
  if (ctx.role === "guest") return guestDenied("Guests can't edit agents.");
  if (isGuest(ctx)) return guestDenial();
  if (isAdminLike(ctx.role)) return ALLOW;
  if (agent.owner_id !== null && agent.owner_id === ctx.userId) return ALLOW;
  return deny(
    "not_resource_owner",
    "Only the agent owner and workspace admins can edit this agent.",
  );
}

/**
 * Invoke an agent — assign it to an issue, @mention it, chat with it, or
 * otherwise trigger a run. Mirrors the MUL-3963 backend invocation gate,
 * which reads `permission_mode` + `invocation_targets` (NOT the derived
 * `visibility` field):
 *
 *   - owner: always
 *   - permission_mode "private": ONLY the owner. This is the key behavior
 *     change — workspace admins NO LONGER bypass a private agent.
 *   - permission_mode "public_to" + workspace target: any workspace member
 *   - permission_mode "public_to" + member target: only the matching user
 *   - team target: reserved, INERT in v1 (never grants)
 */
export function canAssignAgentToIssue(
  agent: Agent,
  ctx: GuestPermissionContext,
): Decision {
  if (ctx.userId === null) {
    return deny("not_authenticated", "Sign in to assign agents.");
  }
  if (isViewerRole(ctx.role)) return viewerDenial();
  // Guests never run agents, not even one they owned as a member.
  if (ctx.role === "guest") return guestDenied("Guests can't assign work to agents.");
  if (isGuest(ctx)) return guestDenial();

  // The owner may always invoke their own agent, regardless of mode.
  if (agent.owner_id !== null && agent.owner_id === ctx.userId) {
    return ALLOW;
  }

  if (agent.permission_mode === "private") {
    return deny(
      "private_visibility",
      "Personal agent — only the owner can assign work.",
    );
  }

  // permission_mode === "public_to": resolve the invocation grants. A
  // workspace grant opens invocation to any workspace member. The `?? []`
  // guards against legacy self-host backends / stale caches that omit the
  // field even though the type says required-array (GH #4915).
  const targets = agent.invocation_targets ?? [];
  if (targets.some((t) => t.target_type === "workspace")) {
    if (ctx.role === null) {
      return deny("not_member", "Join this workspace to assign agents.");
    }
    return ALLOW;
  }

  // A member grant opens invocation to exactly the targeted user. Team
  // targets are reserved and INERT in v1 — they never grant.
  if (
    targets.some(
      (t) => t.target_type === "member" && t.target_id === ctx.userId,
    )
  ) {
    return ALLOW;
  }

  return deny(
    "private_visibility",
    "Restricted agent — you don't have access to assign work to it.",
  );
}

// ---- Skills ----------------------------------------------------------------

export function canEditSkill(
  skill: Skill,
  ctx: GuestPermissionContext,
): Decision {
  if (ctx.userId === null) {
    return deny("not_authenticated", "Sign in to edit this skill.");
  }
  if (isViewerRole(ctx.role)) return viewerDenial();
  if (ctx.role === "guest") return guestDenied("Guests can't edit skills.");
  if (isGuest(ctx)) return guestDenial();
  if (isAdminLike(ctx.role)) return ALLOW;
  if (skill.created_by !== null && skill.created_by === ctx.userId) {
    return ALLOW;
  }
  return deny(
    "not_resource_owner",
    "Only the creator and workspace admins can edit this skill.",
  );
}

export function canDeleteSkill(
  skill: Skill,
  ctx: GuestPermissionContext,
): Decision {
  return canEditSkill(skill, ctx);
}

// ---- Comments --------------------------------------------------------------

// A guest may edit and delete its own comments: the server lets those routes
// through and the handler allows only the author.

export function canEditComment(
  comment: Comment,
  ctx: GuestPermissionContext,
): Decision {
  if (ctx.userId === null) {
    return deny("not_authenticated", "Sign in to edit comments.");
  }
  if (isViewerRole(ctx.role)) return viewerDenial();
  // Only member-authored comments can be edited; agent-authored comments are
  // immutable from any human's perspective.
  if (comment.author_type !== "member") {
    return deny(
      "not_resource_owner",
      "Agent-authored comments cannot be edited.",
    );
  }
  if (comment.author_id === ctx.userId) return ALLOW;
  if (!isGuest(ctx) && isAdminLike(ctx.role)) return ALLOW;
  return deny(
    "not_resource_owner",
    "Only the author and workspace admins can edit this comment.",
  );
}

export function canDeleteComment(
  comment: Comment,
  ctx: GuestPermissionContext,
): Decision {
  if (ctx.userId === null) {
    return deny("not_authenticated", "Sign in to delete comments.");
  }
  if (isViewerRole(ctx.role)) return viewerDenial();
  if (comment.author_type === "member" && comment.author_id === ctx.userId) {
    return ALLOW;
  }
  if (!isGuest(ctx) && isAdminLike(ctx.role)) return ALLOW;
  return deny(
    "not_resource_owner",
    "Only the author and workspace admins can delete this comment.",
  );
}

// ---- Runtimes --------------------------------------------------------------

export function canDeleteRuntime(
  runtime: RuntimeDevice,
  ctx: GuestPermissionContext,
): Decision {
  if (ctx.userId === null) {
    return deny("not_authenticated", "Sign in to delete runtimes.");
  }
  if (isViewerRole(ctx.role)) return viewerDenial();
  if (ctx.role === "guest") return guestDenied("Guests can't delete runtimes.");
  if (isGuest(ctx)) return guestDenial();
  if (isAdminLike(ctx.role)) return ALLOW;
  if (runtime.owner_id !== null && runtime.owner_id === ctx.userId) {
    return ALLOW;
  }
  return deny(
    "not_resource_owner",
    "Only the runtime owner and workspace admins can delete this runtime.",
  );
}

// ---- Workspace -------------------------------------------------------------

export function canUpdateWorkspaceSettings(
  ctx: GuestPermissionContext,
): Decision {
  if (isViewerRole(ctx.role)) return viewerDenial();
  if (isGuest(ctx)) return guestDenial();
  if (isAdminLike(ctx.role)) return ALLOW;
  return deny(
    "not_admin_role",
    "Only workspace owners and admins can update workspace settings.",
  );
}

export function canDeleteWorkspace(ctx: GuestPermissionContext): Decision {
  if (isViewerRole(ctx.role)) return viewerDenial();
  if (isGuest(ctx)) return guestDenial();
  if (ctx.role === "owner") return ALLOW;
  return deny(
    "not_owner_role",
    "Only the workspace owner can delete this workspace.",
  );
}

// ---- Knowledge -------------------------------------------------------------

/**
 * Enable knowledge, publish and discard drafts, delete documents: owners and
 * admins. Mirrors the router's role gate on the knowledge endpoints.
 */
export function canManageKnowledge(ctx: GuestPermissionContext): Decision {
  if (isViewerRole(ctx.role)) return viewerDenial();
  if (isGuest(ctx)) return guestDenial();
  if (isAdminLike(ctx.role)) return ALLOW;
  return deny(
    "not_admin_role",
    "Only workspace owners and admins can manage company knowledge.",
  );
}

export function canManageMembers(ctx: GuestPermissionContext): Decision {
  if (isViewerRole(ctx.role)) return viewerDenial();
  if (isGuest(ctx)) return guestDenial();
  if (isAdminLike(ctx.role)) return ALLOW;
  return deny(
    "not_admin_role",
    "Only workspace owners and admins can manage members.",
  );
}

/**
 * Encodes the role-change matrix from `workspace.go:458-530`:
 *   - admins cannot touch the owner role (neither demote owners nor promote)
 *   - the last owner cannot be demoted
 *   - non-managers cannot change roles at all
 *
 * `ownerCount` is the number of workspace members currently with role=owner.
 * Caller derives it locally from the cached member list.
 */
export function canChangeMemberRole(
  target: Pick<Member, "role">,
  ownerCount: number,
  ctx: GuestPermissionContext,
): Decision {
  const manage = canManageMembers(ctx);
  if (!manage.allowed) return manage;

  if (target.role === "owner") {
    if (ctx.role !== "owner") {
      return deny(
        "not_owner_role",
        "Only the workspace owner can change another owner's role.",
      );
    }
    if (ownerCount <= 1) {
      return deny(
        "last_owner",
        "Promote another member to owner first — a workspace must keep at least one owner.",
      );
    }
  }
  return ALLOW;
}
