// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { setSchemaLogger } from "../../api/schema";
import { noopLogger, type Logger } from "../../logger";
import {
  CriterionSchema,
  VerificationSchema,
  parseCriteriaList,
  parseOutcome,
  parseOutcomeList,
  parseOutcomeSummary,
} from "./schemas";

const outcomeRow = {
  id: "o-1",
  issue_id: "i-1",
  agent_id: "a-1",
  member_id: null,
  squad_id: "s-1",
  work_class: "flow:flow_kv_pr:review",
  verification_mode: "full",
  sampled: true,
  verdict: "partial",
  severity: "low",
  fail_cause: "",
  settle_at: "2026-11-20T10:00:00Z",
  settled_at: null,
  superseded_by: null,
  created_at: "2026-11-18T10:00:00Z",
};

const criterionRow = {
  id: "c-1",
  issue_id: "i-1",
  position: 0,
  text: "Unit tests pass",
  check_kind: "command",
  check_spec: { run: "pnpm test", expect_exit: 0 },
  source: "agent",
  created_by_type: "agent",
  created_by_id: "a-1",
  archived_at: null,
  created_at: "2026-11-18T09:00:00Z",
};

const verificationRow = {
  id: "v-1",
  outcome_id: "o-1",
  criterion_id: "c-1",
  verifier_type: "agent",
  verifier_id: "a-2",
  task_id: "t-1",
  result: "pass",
  severity: "none",
  note: "",
  evidence: [],
  created_at: "2026-11-19T09:00:00Z",
};

describe("outcome schemas", () => {
  let warn: Mock<(msg: string, ...data: unknown[]) => void>;

  beforeEach(() => {
    warn = vi.fn<(msg: string, ...data: unknown[]) => void>();
    const logger: Logger = { debug: vi.fn(), info: vi.fn(), warn, error: vi.fn() };
    setSchemaLogger(logger);
  });

  afterEach(() => {
    setSchemaLogger(noopLogger);
  });

  it("accepts an outcome exactly as the server sends it", () => {
    expect(parseOutcome(outcomeRow, "GET /api/issues/:id/outcome")).toMatchObject({
      id: "o-1",
      issue_id: "i-1",
      verdict: "partial",
      severity: "low",
      fail_cause: "",
      verification_mode: "full",
      sampled: true,
      settled_at: null,
      superseded_by: null,
      verifications: [],
    });
    expect(warn).not.toHaveBeenCalled();
  });

  it("maps an unknown verdict to unverified", () => {
    expect(parseOutcome({ ...outcomeRow, verdict: "exploded" }, "e")?.verdict).toBe("unverified");
  });

  it("keeps the pending verdict the server filters on", () => {
    expect(parseOutcome({ ...outcomeRow, verdict: "pending" }, "e")?.verdict).toBe("pending");
  });

  it("degrades the other enums to their safe values", () => {
    const out = parseOutcome(
      { ...outcomeRow, severity: "catastrophic", fail_cause: "gremlins", verification_mode: "turbo" },
      "e",
    );
    expect(out?.severity).toBe("none");
    expect(out?.fail_cause).toBe("");
    expect(out?.verification_mode).toBe("full");
  });

  it("gives a missing field its default", () => {
    expect(parseOutcome({ id: "o-2", issue_id: "i-2" }, "e")).toMatchObject({
      id: "o-2",
      agent_id: null,
      work_class: "",
      verdict: "unverified",
      severity: "none",
      fail_cause: "",
      verification_mode: "full",
      sampled: false,
      settled_at: null,
      verifications: [],
    });
  });

  it("sends a body of the wrong shape to the fallback and logs the endpoint", () => {
    expect(parseOutcome("not an outcome", "GET /api/outcomes/:id")).toBeNull();
    expect(parseOutcome(["o-1"], "GET /api/outcomes/:id")).toBeNull();
    expect(parseOutcome(null, "GET /api/outcomes/:id")).toBeNull();
    expect(warn).toHaveBeenCalledTimes(3);
    expect(String(warn.mock.calls[0]?.[0])).toContain("GET /api/outcomes/:id");
  });

  it("drops one malformed check from an outcome and keeps the rest", () => {
    const out = parseOutcome(
      { ...outcomeRow, verifications: [verificationRow, "junk", { ...verificationRow, id: "v-2" }] },
      "e",
    );
    expect(out?.verifications.map((v) => v.id)).toEqual(["v-1", "v-2"]);
  });

  it("maps an unknown check result to unable, which the reputation does not count", () => {
    expect(VerificationSchema.parse({ ...verificationRow, result: "maybe" }).result).toBe("unable");
    expect(VerificationSchema.parse({ ...verificationRow, verifier_type: "oracle" }).verifier_type).toBe("system");
  });

  it("reads criteria and keeps check_spec as an object", () => {
    const list = parseCriteriaList({ criteria: [criterionRow] }, "GET /api/issues/:id/criteria");
    expect(list).toEqual([
      expect.objectContaining({
        id: "c-1",
        check_kind: "command",
        check_spec: { run: "pnpm test", expect_exit: 0 },
        source: "agent",
        archived_at: null,
      }),
    ]);
  });

  it("maps an unknown check kind to manual, so nothing automated is claimed", () => {
    expect(CriterionSchema.parse({ ...criterionRow, check_kind: "telepathy" }).check_kind).toBe("manual");
  });

  it("maps an unknown criterion source to member and a non-object check_spec to empty", () => {
    const c = CriterionSchema.parse({ ...criterionRow, source: "robot", check_spec: "run tests" });
    expect(c.source).toBe("member");
    expect(c.check_spec).toEqual({});
  });

  it("returns no criteria when the list is missing", () => {
    expect(parseCriteriaList({}, "e")).toEqual([]);
    expect(parseCriteriaList("nope", "e")).toEqual([]);
  });

  it("reads a page of outcomes, dropping a bad row alone and defaulting the cursor", () => {
    const page = parseOutcomeList({ outcomes: [outcomeRow, 42, { ...outcomeRow, id: "o-3" }] }, "GET /api/outcomes");
    expect(page.outcomes.map((o) => o.id)).toEqual(["o-1", "o-3"]);
    expect(page.next_cursor).toBe("");
  });

  it("falls back to an empty page for a body of the wrong shape", () => {
    expect(parseOutcomeList("oops", "GET /api/outcomes")).toEqual({ outcomes: [], next_cursor: "" });
    expect(warn).toHaveBeenCalledTimes(1);
  });

  it("reads the summary and treats a non-numeric count as zero", () => {
    const summary = parseOutcomeSummary(
      {
        group_by: "agent",
        groups: [{ key: "a-1", total: 3, pass: 1, partial: 1, fail: "x", unverified: 0, pending: 1 }],
      },
      "GET /api/outcomes/summary",
    );
    expect(summary.group_by).toBe("agent");
    expect(summary.groups).toEqual([
      { key: "a-1", total: 3, pass: 1, partial: 1, fail: 0, unverified: 0, pending: 1 },
    ]);
  });
});
