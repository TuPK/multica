import type { HubSourceInUseRef } from "../types/hub";

/** Machine-readable `code` values on 409 responses from the import commit endpoint. */
export const HUB_BLOCKED_CODE = "scan_blocked";
export const HUB_WARNINGS_CODE = "warnings_unconfirmed";
export const HUB_QUOTA_CODE = "quota_exceeded";
export const HUB_LOCAL_CHANGES_CODE = "local_changes";

export class HubSourceInUseError extends Error {
  readonly inUse: HubSourceInUseRef[];

  constructor(inUse: HubSourceInUseRef[], message = "Items from this source are still in use by agents") {
    super(message);
    this.name = "HubSourceInUseError";
    this.inUse = inUse;
  }
}

/** The scan found something the server refuses to import. */
export class HubImportBlockedError extends Error {
  constructor(message = "The scan blocked this import") {
    super(message);
    this.name = "HubImportBlockedError";
  }
}

/** The scan found warnings that must be confirmed before the import can continue. */
export class HubImportWarningsError extends Error {
  constructor(message = "The scan found warnings that need confirmation") {
    super(message);
    this.name = "HubImportWarningsError";
  }
}

/** The preview expired before it was committed. Preview again. */
export class HubPreviewExpiredError extends Error {
  constructor(message = "This import preview has expired. Preview it again") {
    super(message);
    this.name = "HubPreviewExpiredError";
  }
}

/** The item was edited locally, so an update needs an explicit choice: overwrite or fork. */
export class HubLocalChangesError extends Error {
  constructor(message = "This item has local changes. Choose to overwrite them or fork the item") {
    super(message);
    this.name = "HubLocalChangesError";
  }
}

/** The workspace is at its item or storage limit. */
export class HubQuotaExceededError extends Error {
  constructor(message = "The workspace has reached its hub quota") {
    super(message);
    this.name = "HubQuotaExceededError";
  }
}

interface HttpErrorLike {
  status: number;
  message: string;
  body?: unknown;
}

// Duck-typed so this module does not import ApiError (api/client imports it).
function isHttpErrorLike(error: unknown): error is HttpErrorLike {
  return (
    typeof error === "object" &&
    error !== null &&
    typeof (error as { status?: unknown }).status === "number" &&
    typeof (error as { message?: unknown }).message === "string"
  );
}

function bodyRecord(body: unknown): Record<string, unknown> | null {
  return typeof body === "object" && body !== null ? (body as Record<string, unknown>) : null;
}

function parseInUse(body: unknown): HubSourceInUseRef[] | null {
  const list = bodyRecord(body)?.in_use;
  if (!Array.isArray(list)) return null;
  return list.map((entry) => {
    const ref = bodyRecord(entry) ?? {};
    return {
      kind: typeof ref.kind === "string" ? ref.kind : "",
      id: typeof ref.id === "string" ? ref.id : "",
      name: typeof ref.name === "string" ? ref.name : "",
    };
  });
}

/** A 409 carrying `in_use` becomes HubSourceInUseError; anything else passes through unchanged. */
export function mapHubSourceDeleteError(error: unknown): unknown {
  if (!isHttpErrorLike(error) || error.status !== 409) return error;
  const inUse = parseInUse(error.body);
  return inUse === null ? error : new HubSourceInUseError(inUse, error.message || undefined);
}

/** Maps the commit endpoint's 409 codes and 410 expiry onto typed errors. */
export function mapHubImportCommitError(error: unknown): unknown {
  if (!isHttpErrorLike(error)) return error;
  if (error.status === 410) return new HubPreviewExpiredError(error.message || undefined);
  if (error.status !== 409) return error;
  const code = bodyRecord(error.body)?.code;
  if (code === HUB_BLOCKED_CODE) return new HubImportBlockedError(error.message || undefined);
  if (code === HUB_WARNINGS_CODE) return new HubImportWarningsError(error.message || undefined);
  if (code === HUB_QUOTA_CODE) return new HubQuotaExceededError(error.message || undefined);
  return error;
}

/**
 * Maps the update apply endpoint's 409 codes onto typed errors. The scan codes
 * reuse the import errors, since the same scanner gates both.
 */
export function mapHubUpdateApplyError(error: unknown): unknown {
  if (!isHttpErrorLike(error) || error.status !== 409) return error;
  const code = bodyRecord(error.body)?.code;
  if (code === HUB_LOCAL_CHANGES_CODE) return new HubLocalChangesError(error.message || undefined);
  if (code === HUB_QUOTA_CODE) return new HubQuotaExceededError(error.message || undefined);
  if (code === HUB_BLOCKED_CODE) return new HubImportBlockedError(error.message || undefined);
  if (code === HUB_WARNINGS_CODE) return new HubImportWarningsError(error.message || undefined);
  return error;
}
