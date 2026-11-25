import { DURABLE_FLAGS } from "@multica/core/durable";
import { useFlag } from "@multica/core/feature-flags";

/**
 * Whether the acceptance block, the Outcomes tab, the track record and the envelope are on
 * (durable spec 14.1). A flag the server does not send reads as off, so none of them show.
 */
export function useOutcomesEnabled(): boolean {
  return useFlag(DURABLE_FLAGS.outcomes, false);
}
