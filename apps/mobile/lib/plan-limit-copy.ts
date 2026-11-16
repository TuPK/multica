import { ApiError } from "./api-error";
import { i18n } from "./i18n/singleton";

const RESOURCE_GATE_KEYS = new Set([
  "issue_count",
  "autopilot_runs",
  "agents",
  "runtimes",
  "member_groups",
]);

const FEATURE_KEYS = new Set([
  "sso",
  "workspace_policies",
  "viewer_role",
  "audit_export",
  "member_groups",
]);

function readRecord(body: unknown): Record<string, unknown> | null {
  if (!body || typeof body !== "object") return null;
  return body as Record<string, unknown>;
}

/**
 * Maps 402 plan quota / feature gating ApiErrors into short localized copy.
 * Returns null for any non-plan error so callers can fall back to their own
 * error handling.
 */
export function formatPlanLimitError(err: unknown): string | null {
  if (!(err instanceof ApiError)) return null;
  const body = readRecord(err.body);
  const code = err.code;

  if (code === "plan_limit_reached") {
    const gate = typeof body?.gate === "string" ? body.gate : undefined;
    if (gate === "storage_bytes") {
      return i18n.t("workspace:plan.limit_reached_storage_bytes");
    }

    const limit = typeof body?.limit === "number" ? body.limit : null;
    const used = typeof body?.used === "number" ? body.used : null;
    if (limit !== null && used !== null) {
      if (gate && RESOURCE_GATE_KEYS.has(gate)) {
        return i18n.t(`workspace:plan.limit_reached_${gate}`, { limit, used });
      }
      return i18n.t("workspace:plan.limit_reached_generic", { limit, used });
    }
    return i18n.t("workspace:plan.limit_reached_generic_no_counts");
  }

  if (code === "plan_feature_unavailable") {
    // Spec 10 names `feature`, while the server's policy/SSO handlers emit `gate`.
    const rawFeature =
      typeof body?.feature === "string" && body.feature.length > 0
        ? body.feature
        : typeof body?.gate === "string" && body.gate.length > 0
          ? body.gate
          : undefined;

    if (rawFeature && FEATURE_KEYS.has(rawFeature)) {
      const featureLabel = i18n.t(`workspace:plan.features.${rawFeature}`);
      return i18n.t("workspace:plan.feature_unavailable_named", {
        feature: featureLabel,
      });
    }
    return i18n.t("workspace:plan.feature_unavailable_generic");
  }

  return null;
}

/**
 * Formats plan 402 errors or viewer_read_only 403 refusals, returning the
 * caller's fallback copy otherwise. Never surfaces raw server English text.
 */
export function formatWorkspaceWriteError(err: unknown, fallback: string): string {
  const planMessage = formatPlanLimitError(err);
  if (planMessage) return planMessage;
  if (err instanceof ApiError && err.code === "viewer_read_only") {
    return i18n.t("workspace:access.viewer_write_blocked");
  }
  return fallback;
}
