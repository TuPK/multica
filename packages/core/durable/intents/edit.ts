import type { CanonicalCall, Intent } from "./schemas";

// The arguments a person may change before approving: the text of a message or a post
// (durable spec 5.3). The server rejects an edit that changes any other argument, so the
// edited call keeps all of them as they were. Same list and order as the server's
// intentEditableArgs.
const EDITABLE_ARGS = ["body", "body_markdown", "html_body", "text", "content", "message"] as const;

// Only these preview kinds have text a person can change.
const EDITABLE_KINDS = new Set<Intent["preview"]["kind"]>(["email", "post"]);

/** The argument that holds the editable text of a call, or null when it has none. */
export function editableArgKey(call: CanonicalCall | null): string | null {
  if (!call) return null;
  for (const key of EDITABLE_ARGS) {
    if (typeof call.args[key] === "string") return key;
  }
  return null;
}

/** The call with its editable text replaced. The tool, the account and the other arguments stay. */
export function buildEditedCall(call: CanonicalCall, text: string): CanonicalCall | null {
  const key = editableArgKey(call);
  if (!key) return null;
  return { tool: call.tool, account_id: call.account_id, args: { ...call.args, [key]: text } };
}

/** Whether "Sửa rồi duyệt" is offered: a pending message or post whose call has editable text. */
export function canEditIntent(intent: Intent): boolean {
  return intent.state === "pending" && EDITABLE_KINDS.has(intent.preview.kind) && editableArgKey(intent.call) !== null;
}
