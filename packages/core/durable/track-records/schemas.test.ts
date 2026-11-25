// @vitest-environment node
import { describe, expect, it } from "vitest";
import { TrackRecordSchema, parseTrackRecordList } from "./schemas";

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
