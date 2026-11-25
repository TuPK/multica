import { useFeatureEnabled } from "@multica/core/config";
import { DURABLE_FLAGS } from "@multica/core/durable";

/**
 * Whether the acceptance block, the Outcomes tab, the track record and the envelope are on
 * (durable spec 14.1). The server publishes its flags in the config store, which this reads. A flag
 * the server does not send reads as off, so none of them show.
 */
export function useOutcomesEnabled(): boolean {
  return useFeatureEnabled(DURABLE_FLAGS.outcomes, false);
}
