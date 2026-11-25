import { DURABLE_FLAGS } from "@multica/core/durable";
import { useFlag } from "@multica/core/feature-flags";

/** Whether the effect log, the policy grid and the intents are on. A flag the server does not send reads as off. */
export function useEffectsEnabled(): boolean {
  return useFlag(DURABLE_FLAGS.effects, false);
}
