import { useFeatureEnabled } from "@multica/core/config";
import { DURABLE_FLAGS } from "@multica/core/durable";

/** Whether the effect log, the policy grid and the intents are on. A flag the server does not send reads as off. */
export function useEffectsEnabled(): boolean {
  return useFeatureEnabled(DURABLE_FLAGS.effects, false);
}
