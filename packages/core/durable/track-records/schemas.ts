import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import { count, enumOr, listOfItems, nullableStr, str, bool } from "../schema-helpers";
import { VERIFICATION_MODES } from "../outcomes/schemas";

// One row of the track record (durable spec 3.2). The route GET /api/track-records is not on
// the server yet (plan A6); this shape follows the table, and the list wrapper key is the
// assumption to check when A6 lands. Pin and envelope routes are not in this file.
export const TrackRecordSchema = z
  .object({
    agent_id: str,
    work_class: str,
    window_days: z.coerce.number().catch(30).default(30),
    model: str,
    n_pass: count,
    n_partial: count,
    n_fail: count,
    n_unverified: count,
    n_high_fail: count,
    // Postgres NUMERIC can arrive as a string, so it is read as a number here.
    score_lower: z.coerce.number().catch(0).default(0),
    last_high_fail_at: nullableStr,
    verification_mode: enumOr(VERIFICATION_MODES, "full"),
    mode_pinned: bool,
    computed_at: str,
  })
  .loose();

const TrackRecordListSchema = z.object({ track_records: listOfItems(TrackRecordSchema) }).loose();

export type TrackRecord = z.infer<typeof TrackRecordSchema>;

export function parseTrackRecordList(raw: unknown, endpoint: string): TrackRecord[] {
  return parseWithFallback<{ track_records: TrackRecord[] }>(raw, TrackRecordListSchema, { track_records: [] }, { endpoint })
    .track_records;
}
