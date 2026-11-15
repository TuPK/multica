const BASE_PATH = "/api/platform";

/** A non-2xx answer from the platform API. */
export class PlatformApiError extends Error {
  readonly status: number;
  /** The `code` of the error body, or null when the body has none. */
  readonly code: string | null;

  constructor(status: number, code: string | null, message: string) {
    super(message);
    this.name = "PlatformApiError";
    this.status = status;
    this.code = code;
  }
}

export type PlatformRequestInit = Omit<RequestInit, "body" | "credentials"> & {
  /** Serialized as JSON. */
  body?: unknown;
};

async function readErrorBody(res: Response): Promise<{ code: string | null; message: string }> {
  const fallback = { code: null, message: `Request failed with status ${res.status}` };
  let parsed: unknown;
  try {
    parsed = JSON.parse(await res.text());
  } catch {
    return fallback;
  }
  if (typeof parsed !== "object" || parsed === null) return fallback;
  const { code, error } = parsed as { code?: unknown; error?: unknown };
  return {
    code: typeof code === "string" ? code : null,
    message: typeof error === "string" && error !== "" ? error : fallback.message,
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
    const { code, message } = await readErrorBody(res);
    throw new PlatformApiError(res.status, code, message);
  }

  const text = await res.text();
  if (text === "") return null;
  try {
    return JSON.parse(text);
  } catch {
    throw new PlatformApiError(res.status, "malformed_response", "The server answered with a body that is not JSON");
  }
}
