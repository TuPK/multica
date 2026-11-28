import { useFeatureEnabled } from "@multica/core/config";
import { DURABLE_FLAGS } from "@multica/core/durable";

/**
 * Whether the Reach Map, OCF history export, and attestation UI are on (durable spec 14.1).
 * A flag the server does not send reads as off.
 */
export function useReachEnabled(): boolean {
  return useFeatureEnabled(DURABLE_FLAGS.open, false);
}
