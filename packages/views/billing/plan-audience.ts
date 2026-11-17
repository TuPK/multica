import type { WorkspaceBillingAccountKind } from "@multica/core/types";

/** Keep plans whose audience matches the workspace account kind (or "any"). */
export function plansForAccountKind<T extends { audience?: string | null }>(
  plans: readonly T[],
  accountKind: WorkspaceBillingAccountKind,
): T[] {
  return plans.filter((plan) => {
    const audience = plan.audience || "any";
    return audience === "any" || audience === accountKind;
  });
}
