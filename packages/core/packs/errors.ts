import { flowErrorBody, flowErrorCode } from "../flow-drafts/errors";
import type { PackPreviousInstall } from "../types/pack";

/** The `code` of a pack route's refusal, such as `pack_conflict`, `pack_name_conflict` or `pack_scan_blocked`. */
export const packErrorCode = flowErrorCode;

/** The install a `pack_conflict` answer names, or null when the body does not carry one. */
export function packConflictPrevious(err: unknown): PackPreviousInstall | null {
  const previous = flowErrorBody(err).previous;
  if (!previous || typeof previous !== "object" || Array.isArray(previous)) return null;
  const p = previous as Record<string, unknown>;
  if (typeof p.install_id !== "string") return null;
  return {
    install_id: p.install_id,
    version: typeof p.version === "string" ? p.version : "",
    squad_id: typeof p.squad_id === "string" ? p.squad_id : null,
    installed_at: typeof p.installed_at === "string" ? p.installed_at : "",
    ...(typeof p.digest === "string" ? { digest: p.digest } : {}),
  };
}

function names(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((n): n is string => typeof n === "string") : [];
}

/** The names a `pack_name_conflict` answer says are taken. */
export function packNameConflicts(err: unknown): { skills: string[]; agents: string[] } {
  const body = flowErrorBody(err);
  return { skills: names(body.skills), agents: names(body.agents) };
}
