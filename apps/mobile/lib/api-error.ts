/** Read the stable machine-readable `code` from a structured server error body. */
export function extractErrorCode(body: unknown): string | undefined {
  if (body && typeof body === "object" && "code" in body) {
    const raw = (body as { code?: unknown }).code;
    if (typeof raw === "string" && raw.length > 0) return raw;
  }
  return undefined;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string | undefined;
  readonly body?: unknown;

  constructor(message: string, status: number, body?: unknown, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
    this.code = code ?? extractErrorCode(body);
  }
}
