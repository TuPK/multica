import { useFeatureEnabled } from "@multica/core/config";
import { DURABLE_FLAGS } from "@multica/core/durable";

/**
 * Whether the seat dossier and shadow trial screens are on (durable spec 14.1).
 * The server publishes its flags in the config store; a missing flag reads as off.
 */
export function useSeatsEnabled(): boolean {
  return useFeatureEnabled(DURABLE_FLAGS.seats, false);
}
