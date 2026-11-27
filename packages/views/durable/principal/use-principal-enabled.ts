import { useFeatureEnabled } from "@multica/core/config";
import { DURABLE_FLAGS } from "@multica/core/durable";

/** Whether the principal profile is on. A flag the server does not send reads as off. */
export function usePrincipalEnabled(): boolean {
  return useFeatureEnabled(DURABLE_FLAGS.principal, false);
}
