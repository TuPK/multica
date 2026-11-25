import { z } from "zod";
import { count, enumOr, listOfItems, nullableStr, str, bool } from "../schema-helpers";
import { parseWithFallback } from "../../api/schema";
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

// The autonomy envelope (durable spec 3.5, GET/PUT /api/squads/{id}/autonomy/envelope). A level is
// 0 to 3; anything else reads as 0, the safest floor, so a bad value never widens the range.
const level = z.coerce.number().int().min(0).max(3).catch(0).default(0);

export const SquadEnvelopeSchema = z
  .object({
    squad_id: str,
    floor_level: level,
    ceiling_level: level,
    reason: str,
    set_by: str,
    last_auto_change_at: nullableStr,
    updated_at: str,
  })
  .loose();

// The reputation the envelope reads, over 90 days. Rates are null while the squad has no
// acceptance decisions yet, which is different from a rate of zero.
export const SquadReputationSchema = z
  .object({
    n: count,
    score_lower: z.coerce.number().catch(0).default(0),
    window_days: z.coerce.number().catch(90).default(90),
    high_fail_30: bool,
    high_fail_90: bool,
    acceptance_rate: z.coerce.number().nullable().catch(null).default(null),
  })
  .loose();

export const SquadEnvelopeViewSchema = z
  .object({
    envelope: SquadEnvelopeSchema.nullable().catch(null).default(null),
    reputation: SquadReputationSchema.catch({
      n: 0,
      score_lower: 0,
      window_days: 90,
      high_fail_30: false,
      high_fail_90: false,
      acceptance_rate: null,
    }).default({
      n: 0,
      score_lower: 0,
      window_days: 90,
      high_fail_30: false,
      high_fail_90: false,
      acceptance_rate: null,
    }),
  })
  .loose();

export type SquadEnvelope = z.infer<typeof SquadEnvelopeSchema>;
export type SquadReputation = z.infer<typeof SquadReputationSchema>;
export type SquadEnvelopeView = z.infer<typeof SquadEnvelopeViewSchema>;

/** A squad's envelope and reputation. A body of the wrong shape reads as no envelope. */
export function parseSquadEnvelope(raw: unknown, endpoint: string): SquadEnvelopeView {
  return parseWithFallback<SquadEnvelopeView>(
    raw,
    SquadEnvelopeViewSchema,
    { envelope: null, reputation: SquadReputationSchema.parse({}) },
    { endpoint },
  );
}

/** One track record, as the pin route returns it. A body of the wrong shape is null, not a fake row. */
export function parseTrackRecord(raw: unknown, endpoint: string): TrackRecord | null {
  return parseWithFallback<TrackRecord | null>(raw, TrackRecordSchema, null, { endpoint });
}
