import { z } from "zod";

/** A workspace the deletion job will delete, because the user is its only member. */
export interface AccountDeletionWorkspace {
  id: string;
  name: string;
}

/** A workspace that blocks the request: the user owns it and other members remain. */
export interface BlockingWorkspace {
  id: string;
  name: string;
  member_count: number;
}

/** Response of POST /api/me/deletion?preview=1. `scheduled_at` is always null. */
export interface AccountDeletionPreview {
  sole_workspaces: AccountDeletionWorkspace[];
  scheduled_at: string | null;
}

/**
 * Response of POST /api/me/deletion. An empty `scheduled_at` means the body
 * could not be read; the request itself still succeeded.
 */
export interface AccountDeletionScheduled {
  scheduled_at: string;
}

/** Response of DELETE /api/me/deletion. */
export interface AccountDeletionCanceled {
  status: string;
}

const AccountDeletionWorkspaceSchema = z
  .object({ id: z.string(), name: z.string().default("") })
  .loose();

export const AccountDeletionPreviewSchema = z
  .object({
    sole_workspaces: z.array(AccountDeletionWorkspaceSchema).default([]),
    scheduled_at: z.string().nullable().default(null),
  })
  .loose();

export const EMPTY_ACCOUNT_DELETION_PREVIEW: AccountDeletionPreview = {
  sole_workspaces: [],
  scheduled_at: null,
};

export const AccountDeletionScheduledSchema = z
  .object({ scheduled_at: z.string().default("") })
  .loose();

export const EMPTY_ACCOUNT_DELETION_SCHEDULED: AccountDeletionScheduled = { scheduled_at: "" };

export const AccountDeletionCanceledSchema = z
  .object({ status: z.string().default("") })
  .loose();

export const EMPTY_ACCOUNT_DELETION_CANCELED: AccountDeletionCanceled = { status: "" };

/** The `workspaces` list of a 409 `sole_owner` body. */
export const SoleOwnerBodySchema = z
  .object({
    workspaces: z
      .array(
        z
          .object({
            id: z.string(),
            name: z.string().default(""),
            member_count: z.number().default(0),
          })
          .loose(),
      )
      .default([]),
  })
  .loose();

export const EMPTY_SOLE_OWNER_BODY: { workspaces: BlockingWorkspace[] } = { workspaces: [] };
