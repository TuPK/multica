import { useMutation } from "@tanstack/react-query";
import { api } from "../api";
import { ApiError, errorCode } from "../api/client";
import { parseWithFallback } from "../api/schema";
import { useAuthStore } from "../auth";
import {
  EMPTY_SOLE_OWNER_BODY,
  SoleOwnerBodySchema,
  type AccountDeletionCanceled,
  type AccountDeletionPreview,
  type AccountDeletionScheduled,
  type BlockingWorkspace,
} from "./deletion-schema";

export const SOLE_OWNER = "sole_owner";
export const RECENT_LOGIN_REQUIRED = "recent_login_required";

/**
 * The user owns a workspace that still has other members, so deletion is
 * refused until they transfer ownership or delete it. `workspaces` names them.
 */
export class SoleOwnerError extends Error {
  readonly code = SOLE_OWNER;
  readonly workspaces: BlockingWorkspace[];

  constructor(workspaces: BlockingWorkspace[], message = "Transfer ownership or delete your shared workspaces first") {
    super(message);
    this.name = "SoleOwnerError";
    this.workspaces = workspaces;
  }
}

/** The login is older than 10 minutes, or the credential is not a browser session. */
export class RecentLoginRequiredError extends Error {
  readonly code = RECENT_LOGIN_REQUIRED;

  constructor(message = "Sign in again to confirm") {
    super(message);
    this.name = "RecentLoginRequiredError";
  }
}

/** True for an error that carries the `sole_owner` code. */
export function isSoleOwnerError(err: unknown): err is SoleOwnerError {
  return err instanceof SoleOwnerError;
}

/** True for an error that carries the `recent_login_required` code. */
export function isRecentLoginRequiredError(err: unknown): err is RecentLoginRequiredError {
  return err instanceof RecentLoginRequiredError;
}

async function mapDeletionErrors<T>(call: () => Promise<T>): Promise<T> {
  try {
    return await call();
  } catch (err) {
    if (err instanceof ApiError) {
      const code = errorCode(err);
      if (err.status === 409 && code === SOLE_OWNER) {
        const body = parseWithFallback(err.body, SoleOwnerBodySchema, EMPTY_SOLE_OWNER_BODY, {
          endpoint: "POST /api/me/deletion 409",
        });
        throw new SoleOwnerError(body.workspaces, err.message);
      }
      if (err.status === 403 && code === RECENT_LOGIN_REQUIRED) {
        throw new RecentLoginRequiredError(err.message);
      }
    }
    throw err;
  }
}

/**
 * Asks what deleting the account would remove, without changing anything.
 * Rejects with `SoleOwnerError` or `RecentLoginRequiredError`.
 */
export function usePreviewAccountDeletion() {
  return useMutation<AccountDeletionPreview, Error, void>({
    mutationFn: () => mapDeletionErrors(() => api.previewAccountDeletion()),
  });
}

/**
 * Schedules the deletion. The server revokes this session, so the caller signs
 * out afterwards; nothing is patched here. Rejects with `SoleOwnerError` or
 * `RecentLoginRequiredError`.
 */
export function useRequestAccountDeletion() {
  return useMutation<AccountDeletionScheduled, Error, void>({
    mutationFn: () => mapDeletionErrors(() => api.requestAccountDeletion()),
  });
}

/**
 * Cancels a pending deletion and clears `deletion_scheduled_at` on the signed-in
 * user, which closes the pending gate.
 */
export function useCancelAccountDeletion() {
  return useMutation<AccountDeletionCanceled, Error, void>({
    mutationFn: () => api.cancelAccountDeletion(),
    onSuccess: () => {
      const { user, setUser } = useAuthStore.getState();
      if (user) setUser({ ...user, deletion_scheduled_at: null });
    },
  });
}
