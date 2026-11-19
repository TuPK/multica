const BASE_PATH = "/api/platform";

/** A non-2xx answer from the platform API. */
export class PlatformApiError extends Error {
  readonly status: number;
  /** The `code` of the error body, or null when the body has none. */
  readonly code: string | null;
  /** Extra fields from the error JSON body other than `code` and `error`. */
  readonly details: Record<string, unknown>;

  constructor(
    status: number,
    code: string | null,
    message: string,
    details: Record<string, unknown> = {},
  ) {
    super(message);
    this.name = "PlatformApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

export type PlatformRequestInit = Omit<RequestInit, "body" | "credentials"> & {
  /** Serialized as JSON. */
  body?: unknown;
};

async function readErrorBody(res: Response): Promise<{
  code: string | null;
  message: string;
  details: Record<string, unknown>;
}> {
  const fallback = {
    code: null,
    message: `Request failed with status ${res.status}`,
    details: {},
  };
  let parsed: unknown;
  try {
    parsed = JSON.parse(await res.text());
  } catch {
    return fallback;
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return fallback;
  const { code, error, ...details } = parsed as Record<string, unknown>;
  return {
    code: typeof code === "string" ? code : null,
    message: typeof error === "string" && error !== "" ? error : fallback.message,
    details,
  };
}

/**
 * Calls the platform API on this origin and returns the decoded JSON body, or
 * null when the answer has no body (204). The caller validates the shape.
 *
 * The session cookie travels with `credentials: "same-origin"`; this module
 * never reads or writes a cookie.
 */
export async function platformFetch(path: string, init: PlatformRequestInit = {}): Promise<unknown> {
  const { body, headers, ...rest } = init;
  const requestHeaders = new Headers(headers);
  requestHeaders.set("Accept", "application/json");
  if (body !== undefined) requestHeaders.set("Content-Type", "application/json");

  const res = await fetch(BASE_PATH + path, {
    ...rest,
    headers: requestHeaders,
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: "same-origin",
  });

  if (!res.ok) {
    const { code, message, details } = await readErrorBody(res);
    throw new PlatformApiError(res.status, code, message, details);
  }

  const text = await res.text();
  if (text === "") return null;
  try {
    return JSON.parse(text);
  } catch {
    throw new PlatformApiError(res.status, "malformed_response", "The server answered with a body that is not JSON");
  }
}

export interface PlatformTextResponse {
  text: string;
  contentType: string;
  nextCursor: string | null;
}

/**
 * Calls the platform API and returns a plain-text / streaming response body
 * (e.g. NDJSON or CSV audit exports), mapping non-2xx JSON errors to PlatformApiError.
 */
export async function platformFetchText(
  path: string,
  init: PlatformRequestInit & { accept?: string } = {},
): Promise<PlatformTextResponse> {
  const { body, headers, accept, ...rest } = init;
  const requestHeaders = new Headers(headers);
  if (accept) requestHeaders.set("Accept", accept);
  if (body !== undefined) requestHeaders.set("Content-Type", "application/json");

  const res = await fetch(BASE_PATH + path, {
    ...rest,
    headers: requestHeaders,
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: "same-origin",
  });

  if (!res.ok) {
    const { code, message, details } = await readErrorBody(res);
    throw new PlatformApiError(res.status, code, message, details);
  }

  return {
    text: await res.text(),
    contentType: res.headers.get("Content-Type") ?? "",
    nextCursor: res.headers.get("X-Next-Cursor"),
  };
}

/** Extracts a non-empty user-facing message from an API or generic error. */
export function getApiErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message.trim() !== "") return err.message;
  return fallback;
}

const MEMBER_CHANGE_ERROR_MESSAGES: Record<string, string> = {
  owner_role_immutable: "The owner role cannot be changed or removed by staff.",
  cannot_change_own_role: "An admin cannot change their own role.",
  user_is_staff: "An active staff member cannot be changed here.",
  member_role_invalid: "Role must be admin, member, or viewer.",
  sole_owner: "The sole owner of a workspace cannot be removed.",
};

/** Maps member-change API error codes to clear guidance, falling back to the server message. */
export function getMemberChangeErrorMessage(err: unknown, fallback: string): string {
  const mapped =
    err instanceof PlatformApiError && err.code
      ? MEMBER_CHANGE_ERROR_MESSAGES[err.code]
      : undefined;
  if (mapped) return mapped;
  return getApiErrorMessage(err, fallback);
}

const LIMIT_OVERRIDE_ERROR_MESSAGES: Record<string, string> = {
  limit_gate_invalid: "Select a valid plan limit gate.",
  limit_value_invalid: "Value must be a non-negative integer, or leave empty to release the override.",
  reason_required: "A reason of 1 to 500 characters is required.",
  workspace_not_in_tenant: "Every selected workspace must belong to this tenant.",
  concurrent_change: "The subscription changed concurrently. Refresh and try again.",
};

/** Maps tenant limit-override API error codes to clear guidance, falling back to the server message. */
export function getLimitOverrideErrorMessage(err: unknown, fallback: string): string {
  const mapped =
    err instanceof PlatformApiError && err.code
      ? LIMIT_OVERRIDE_ERROR_MESSAGES[err.code]
      : undefined;
  if (mapped) return mapped;
  return getApiErrorMessage(err, fallback);
}

const TENANT_POLICY_ERROR_MESSAGES: Record<string, string> = {
  plan_feature_unavailable:
    "At least one workspace in this tenant must be on a plan with workspace policies to save a tenant policy.",
  policy_version_conflict:
    "The policy changed since it was read. Reload and try again.",
};

/** Maps tenant-policy API error codes to clear guidance, falling back to the server message. */
export function getTenantPolicyErrorMessage(err: unknown, fallback: string): string {
  const mapped =
    err instanceof PlatformApiError && err.code
      ? TENANT_POLICY_ERROR_MESSAGES[err.code]
      : undefined;
  if (mapped) return mapped;
  return getApiErrorMessage(err, fallback);
}

const APPROVAL_ERROR_MESSAGES: Record<string, string> = {
  approver_unavailable: "At least two active system administrators are required for two-person approval.",
  approval_not_pending: "This deletion request is no longer awaiting approval.",
  legal_hold: "Deletion is blocked by an active legal hold.",
};

/** Maps approval API error codes to clear guidance, preserving server messages such as approver_must_differ. */
export function getApprovalErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof PlatformApiError && err.code === "approver_must_differ") {
    return err.message || "The approver must differ from the requester.";
  }
  const mapped =
    err instanceof PlatformApiError && err.code ? APPROVAL_ERROR_MESSAGES[err.code] : undefined;
  if (mapped) return mapped;
  return getApiErrorMessage(err, fallback);
}

const LEGAL_HOLD_ERROR_MESSAGES: Record<string, string> = {
  legal_hold_exists: "An active legal hold already covers this scope.",
  legal_hold_not_found: "Legal hold not found or already released.",
};

/** Maps legal-hold API error codes to clear guidance, falling back to the server message. */
export function getLegalHoldErrorMessage(err: unknown, fallback: string): string {
  const mapped =
    err instanceof PlatformApiError && err.code ? LEGAL_HOLD_ERROR_MESSAGES[err.code] : undefined;
  if (mapped) return mapped;
  return getApiErrorMessage(err, fallback);
}
