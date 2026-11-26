// @vitest-environment node
import { describe, expect, it } from "vitest";
import { SquadEnvelopeViewSchema, TrackRecordSchema, parseSquadEnvelope, parseTrackRecord, parseTrackRecordList } from "./schemas";

const recordRow = {
  agent_id: "a-1",
  work_class: "flow:flow_kv_pr:review",
  window_days: 30,
  model: "",
  n_pass: 8,
  n_partial: 1,
  n_fail: 1,
  n_unverified: 4,
  n_high_fail: 0,
  score_lower: 0.71,
  last_high_fail_at: null,
  verification_mode: "sampled",
  mode_pinned: false,
  computed_at: "2026-11-24T02:00:00Z",
};

describe("track record schemas", () => {
  it("accepts a record as the track_record table describes it", () => {
    expect(TrackRecordSchema.parse(recordRow)).toMatchObject({
      agent_id: "a-1",
      window_days: 30,
      n_pass: 8,
      score_lower: 0.71,
      last_high_fail_at: null,
      verification_mode: "sampled",
      mode_pinned: false,
    });
  });

  it("reads a numeric score sent as a string", () => {
    expect(TrackRecordSchema.parse({ ...recordRow, score_lower: "0.83" }).score_lower).toBe(0.83);
  });

  it("maps an unknown verification mode to full", () => {
    expect(TrackRecordSchema.parse({ ...recordRow, verification_mode: "lenient" }).verification_mode).toBe("full");
  });

  it("gives missing counts and flags their defaults", () => {
    const r = TrackRecordSchema.parse({ agent_id: "a-2" });
    expect(r).toMatchObject({ n_pass: 0, n_fail: 0, score_lower: 0, mode_pinned: false, verification_mode: "full" });
  });

  it("drops one malformed record from the list and keeps the rest", () => {
    const list = parseTrackRecordList({ track_records: [recordRow, "bad", { ...recordRow, agent_id: "a-3" }] }, "GET /api/track-records");
    expect(list.map((r) => r.agent_id)).toEqual(["a-1", "a-3"]);
  });

  it("falls back to an empty list for a body of the wrong shape", () => {
    expect(parseTrackRecordList(null, "GET /api/track-records")).toEqual([]);
    expect(parseTrackRecordList({ track_records: "x" }, "GET /api/track-records")).toEqual([]);
  });
});

const envelopeView = {
  envelope: {
    squad_id: "s-1",
    floor_level: 1,
    ceiling_level: 2,
    reason: "Trial squad",
    set_by: "u-1",
    last_auto_change_at: null,
    updated_at: "2026-11-24T02:00:00Z",
  },
  reputation: {
    n: 14,
    score_lower: 0.6,
    window_days: 90,
    high_fail_30: false,
    high_fail_90: true,
    acceptance_rate: 0.92,
  },
};

describe("autonomy envelope schemas", () => {
  it("accepts the envelope view as the server sends it", () => {
    expect(SquadEnvelopeViewSchema.parse(envelopeView)).toMatchObject({
      envelope: { floor_level: 1, ceiling_level: 2, reason: "Trial squad", last_auto_change_at: null },
      reputation: { n: 14, score_lower: 0.6, high_fail_90: true, acceptance_rate: 0.92 },
    });
  });

  it("reads a squad without an envelope as null, with the reputation kept", () => {
    const view = parseSquadEnvelope({ envelope: null, reputation: envelopeView.reputation }, "GET .../envelope");
    expect(view.envelope).toBeNull();
    expect(view.reputation.n).toBe(14);
  });

  it("gives a missing reputation its defaults, with no acceptance rate", () => {
    const view = parseSquadEnvelope({ envelope: null }, "GET .../envelope");
    expect(view.reputation).toMatchObject({ n: 0, score_lower: 0, window_days: 90, high_fail_30: false, acceptance_rate: null });
  });

  it("keeps levels inside the 0-3 range", () => {
    const view = parseSquadEnvelope(
      { envelope: { ...envelopeView.envelope, floor_level: 9, ceiling_level: "2" } },
      "GET .../envelope",
    );
    expect(view.envelope).toMatchObject({ floor_level: 0, ceiling_level: 2 });
  });

  it("reads one track record returned by the pin route", () => {
    expect(parseTrackRecord({ ...recordRow, verification_mode: "spot", mode_pinned: true }, "PUT /api/track-records/pin")).toMatchObject({
      verification_mode: "spot",
      mode_pinned: true,
    });
    expect(parseTrackRecord("oops", "PUT /api/track-records/pin")).toBeNull();
  });
});
