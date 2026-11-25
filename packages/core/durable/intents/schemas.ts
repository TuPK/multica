import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import { listOfItems, nullableStr, str } from "../schema-helpers";
import { EFFECT_CLASSES, EFFECT_SOURCES } from "../effects/schemas";

// Response shapes of the intent routes (durable spec 5.2 and 5.3, server durable_intent_fork.go).
// An intent whose state this build does not know reads as "expired": it is not pending, so
// no decision is offered on it.

export const INTENT_STATES = [
  "pending",
  "approved",
  "rejected",
  "expired",
  "executed",
  "failed",
  "withdrawn",
] as const;
export const PREVIEW_KINDS = ["email", "post", "record", "generic"] as const;

export type IntentState = (typeof INTENT_STATES)[number];
export type PreviewKind = (typeof PREVIEW_KINDS)[number];

const PreviewFieldSchema = z.object({ label: str, before: str, after: str }).loose();

// The preview is what the person reads. The server builds it from the call, never from the
// agent's words, so it can be shown as it is. Plain text only: body_markdown is not rendered.
export const PreviewSchema = z
  .object({
    kind: z.enum(PREVIEW_KINDS).catch("generic").default("generic"),
    title: str,
    body_markdown: str,
    fields: listOfItems(PreviewFieldSchema),
    recipients: z.array(z.string()).catch([]).default([]),
    amount_micros: z.number().nullable().catch(null).default(null),
  })
  .loose();

// The call the approval re-hashes and runs. A declared intent has no call.
export const CanonicalCallSchema = z
  .object({
    tool: str,
    account_id: str,
    args: z.record(z.string(), z.unknown()).catch({}).default({}),
  })
  .loose();

export const IntentSchema = z
  .object({
    // An intent without an id cannot be decided on, so it is dropped rather than shown.
    id: z.string().min(1),
    created_at: str,
    expires_at: str,
    agent_id: nullableStr,
    issue_id: nullableStr,
    source: z.enum(EFFECT_SOURCES).catch("connected_app").default("connected_app"),
    target: str,
    effect_class: z.enum(EFFECT_CLASSES).catch("write").default("write"),
    state: z.enum(INTENT_STATES).catch("expired").default("expired"),
    call: CanonicalCallSchema.nullable().catch(null).default(null),
    call_sha256: str,
    preview: PreviewSchema.catch(emptyPreview()),
    rationale: str,
    decision_note: str,
    batch_key: str,
  })
  .loose();

export const IntentListSchema = z.object({ intents: listOfItems(IntentSchema) }).loose();

export const BatchResultSchema = z
  .object({
    id: z.string().min(1),
    state: str,
    error: str,
    status: z.number().int().catch(0).default(0),
  })
  .loose();

export const BatchResultsSchema = z.object({ results: listOfItems(BatchResultSchema) }).loose();

export type Preview = z.infer<typeof PreviewSchema>;
export type CanonicalCall = z.infer<typeof CanonicalCallSchema>;
export type Intent = z.infer<typeof IntentSchema>;
export type BatchResult = z.infer<typeof BatchResultSchema>;

export function emptyPreview(): Preview {
  return { kind: "generic", title: "", body_markdown: "", fields: [], recipients: [], amount_micros: null };
}

/** A single intent that does not parse is null: an empty card would read as a real intent. */
export function parseIntent(raw: unknown, endpoint: string): Intent | null {
  return parseWithFallback<Intent | null>(raw, IntentSchema, null, { endpoint });
}

export function parseIntentList(raw: unknown, endpoint: string): Intent[] {
  return parseWithFallback<{ intents: Intent[] }>(raw, IntentListSchema, { intents: [] }, { endpoint }).intents;
}

export function parseBatchResults(raw: unknown, endpoint: string): BatchResult[] {
  return parseWithFallback<{ results: BatchResult[] }>(raw, BatchResultsSchema, { results: [] }, { endpoint }).results;
}
