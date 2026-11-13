/**
 * What a flow route put in its error body (spec 7.7): `code`, and the fields that belong to the
 * code, such as `latest_workflow_id` on `flow_draft_stale` or `required` on `flow_ack_required`.
 */
export function flowErrorBody(err: unknown): Record<string, unknown> {
  const body = (err as { body?: unknown } | null)?.body;
  return body && typeof body === "object" ? (body as Record<string, unknown>) : {};
}

export function flowErrorCode(err: unknown): string | undefined {
  const code = flowErrorBody(err).code;
  return typeof code === "string" ? code : undefined;
}

/** The names a `flow_ack_required` answer lists, or none when the field is missing or malformed. */
export function flowRequiredAcks(err: unknown): string[] {
  const required = flowErrorBody(err).required;
  return Array.isArray(required) ? required.filter((k): k is string => typeof k === "string") : [];
}
