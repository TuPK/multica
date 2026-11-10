import { z } from "zod";
import { parseWithFallback } from "../api/schema";

/**
 * The Web engine's status on a machine, as the daemon reported it into the runtime's metadata
 * (docs/engineering/sales-loop-squad.md, section 4.13). A status this build does not know is "unknown".
 */
export type WebEngineStatus = "running" | "not_running" | "not_configured" | "unknown";

export interface WebEngine {
  status: WebEngineStatus;
  version: string;
  checkedAt: string;
}

const KNOWN: readonly WebEngineStatus[] = ["running", "not_running", "not_configured"];

const WebEngineSchema = z.object({
  status: z.string().default("unknown"),
  version: z.string().default(""),
  checked_at: z.string().default(""),
});

/** The engine status a runtime's metadata holds, or null when no machine has reported one. */
export function parseWebEngine(metadata: unknown): WebEngine | null {
  if (!metadata || typeof metadata !== "object") return null;
  const raw = (metadata as Record<string, unknown>).web_engine;
  if (raw === undefined || raw === null) return null;
  const parsed = parseWithFallback<z.infer<typeof WebEngineSchema> | null>(raw, WebEngineSchema, null, {
    endpoint: "runtime.metadata.web_engine",
  });
  if (!parsed) return null;
  const status = (KNOWN as readonly string[]).includes(parsed.status)
    ? (parsed.status as WebEngineStatus)
    : "unknown";
  return { status, version: parsed.version, checkedAt: parsed.checked_at };
}
