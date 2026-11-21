"use client";

import { useCallback } from "react";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  useModalStore,
  type LimitRecoveryReason,
} from "@multica/core/modals";

export const SUPPORTED_PLAN_LIMIT_GATES = [
  "agents",
  "runtimes",
  "storage_bytes",
] as const;

export type SupportedPlanLimitGate = (typeof SUPPORTED_PLAN_LIMIT_GATES)[number];

export function isSupportedPlanLimitGate(
  gate: unknown,
): gate is SupportedPlanLimitGate {
  return (
    typeof gate === "string" &&
    (SUPPORTED_PLAN_LIMIT_GATES as readonly string[]).includes(gate)
  );
}

export function isSupportedLimitRecoveryReason(
  reason: unknown,
): reason is LimitRecoveryReason {
  if (typeof reason !== "string") return false;
  return (
    reason === "issue_limit" ||
    reason === "autopilot_quota" ||
    reason === "agent_limit" ||
    reason === "runtime_limit" ||
    reason === "storage_limit" ||
    isSupportedPlanLimitGate(reason)
  );
}

export function extractPlanLimitGate(
  error: unknown,
): SupportedPlanLimitGate | null {
  if (!error || typeof error !== "object") return null;
  const e = error as Record<string, unknown>;
  const body = (e.body && typeof e.body === "object" ? e.body : null) as
    | Record<string, unknown>
    | null;

  const code = e.code ?? body?.code;
  if (code !== "plan_limit_reached") return null;

  const rawGate = e.gate ?? body?.gate;
  if (isSupportedPlanLimitGate(rawGate)) {
    return rawGate;
  }
  return null;
}

export function resolveLimitRecoveryReason(
  input: unknown,
  defaultReason: LimitRecoveryReason = "issue_limit",
): LimitRecoveryReason | null {
  if (
    input === undefined ||
    input === null ||
    (typeof input === "object" && ("nativeEvent" in input || "target" in input))
  ) {
    return isSupportedLimitRecoveryReason(defaultReason) ? defaultReason : null;
  }

  if (typeof input === "string") {
    if (isSupportedLimitRecoveryReason(input)) {
      return input;
    }
    return null;
  }

  const gate = extractPlanLimitGate(input);
  if (gate) {
    return gate;
  }

  if (typeof input === "object") {
    const e = input as Record<string, unknown>;
    const body = (e.body && typeof e.body === "object" ? e.body : null) as
      | Record<string, unknown>
      | null;
    const code = e.code ?? body?.code;
    if (code === "issue_limit_reached") {
      return "issue_limit";
    }
  }

  return null;
}

/** Opens the shared limit-recovery dialog without closing the current surface. */
export function useLimitRecoveryPrompt(
  reason: LimitRecoveryReason = "issue_limit",
): (errorOrReason?: unknown) => boolean {
  const wsId = useWorkspaceId();

  return useCallback(
    (errorOrReason?: unknown) => {
      const targetReason = resolveLimitRecoveryReason(errorOrReason, reason);
      if (!targetReason) {
        return false;
      }
      useModalStore.getState().showLimitRecovery(wsId, targetReason);
      return true;
    },
    [reason, wsId],
  );
}
