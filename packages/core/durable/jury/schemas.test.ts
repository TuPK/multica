// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { setSchemaLogger } from "../../api/schema";
import { noopLogger, type Logger } from "../../logger";
import { parseJuryPanelList, parseWorkspaceJurySettings } from "./schemas";

const voteRow = {
  id: "v-1",
  verifier_id: "a-2",
  result: "pass",
  note: "All checks match the brief.",
  created_at: "2026-11-26T10:00:00Z",
};

const panelRow = {
  id: "jp-1",
  criterion_id: "c-1",
  juror_agent_ids: ["a-2", "a-3"],
  diversity: "provider",
  state: "split",
  verdict: null,
  resolved_by_type: null,
  created_at: "2026-11-26T09:00:00Z",
  closed_at: null,
  votes: [
    voteRow,
    { id: "v-2", verifier_id: "a-3", result: "fail", note: "Missing rollback test.", created_at: "2026-11-26T10:05:00Z" },
  ],
};

describe("jury schemas", () => {
  let warn: Mock<(msg: string, ...data: unknown[]) => void>;

  beforeEach(() => {
    warn = vi.fn<(msg: string, ...data: unknown[]) => void>();
    const logger: Logger = { debug: vi.fn(), info: vi.fn(), warn, error: vi.fn() };
    setSchemaLogger(logger);
  });

  afterEach(() => {
    setSchemaLogger(noopLogger);
  });

  it("parses panels with votes after leaving open state", () => {
    const panels = parseJuryPanelList({ panels: [panelRow] }, "GET /api/outcomes/:id/jury");
    expect(panels).toHaveLength(1);
    expect(panels[0]).toMatchObject({
      id: "jp-1",
      criterion_id: "c-1",
      juror_agent_ids: ["a-2", "a-3"],
      diversity: "provider",
      state: "split",
      verdict: null,
      resolved_by_type: null,
    });
    expect(panels[0]?.votes).toHaveLength(2);
    expect(warn).not.toHaveBeenCalled();
  });

  it("keeps votes empty while the panel is open (blind voting)", () => {
    const panels = parseJuryPanelList(
      { panels: [{ ...panelRow, state: "open", votes: [] }] },
      "GET /api/outcomes/:id/jury",
    );
    expect(panels).toHaveLength(1);
    expect(panels[0]?.state).toBe("open");
    expect(panels[0]?.votes).toEqual([]);
  });

  it("falls back unknown state to open, unknown diversity to none, and unknown vote result to unable", () => {
    const panels = parseJuryPanelList(
      {
        panels: [
          {
            ...panelRow,
            state: "deliberating",
            diversity: "quantum",
            verdict: "maybe",
            resolved_by_type: "committee",
            votes: [{ ...voteRow, result: "abstain" }],
          },
        ],
      },
      "GET /api/outcomes/:id/jury",
    );
    expect(panels[0]).toMatchObject({
      state: "open",
      diversity: "none",
      verdict: null,
      resolved_by_type: null,
    });
    expect(panels[0]?.votes[0]?.result).toBe("unable");
  });

  it("drops malformed panels and malformed votes individually", () => {
    const panels = parseJuryPanelList(
      {
        panels: [
          { state: "split" },
          "bad",
          {
            ...panelRow,
            votes: [voteRow, { result: "pass" }, 42],
          },
        ],
      },
      "GET /api/outcomes/:id/jury",
    );
    expect(panels).toHaveLength(1);
    expect(panels[0]?.votes.map((v) => v.id)).toEqual(["v-1"]);
  });

  it("falls back to an empty list for a body of the wrong shape and logs the endpoint", () => {
    expect(parseJuryPanelList(null, "GET /api/outcomes/:id/jury")).toEqual([]);
    expect(parseJuryPanelList("oops", "GET /api/outcomes/:id/jury")).toEqual([]);
    expect(warn).toHaveBeenCalledTimes(2);
    expect(String(warn.mock.calls[0]?.[0])).toContain("GET /api/outcomes/:id/jury");
  });

  it("parses workspace jury settings and treats missing or malformed entries as empty", () => {
    expect(
      parseWorkspaceJurySettings({
        jury_pool: ["a-1", "a-2", "", 123],
        jury_work_classes: ["flow:pr:", "label:legal", null],
      }),
    ).toEqual({
      jury_pool: ["a-1", "a-2"],
      jury_work_classes: ["flow:pr:", "label:legal"],
    });
    expect(parseWorkspaceJurySettings({})).toEqual({ jury_pool: [], jury_work_classes: [] });
    expect(parseWorkspaceJurySettings({ jury_pool: "not-a-list", jury_work_classes: 99 })).toEqual({
      jury_pool: [],
      jury_work_classes: [],
    });
    expect(parseWorkspaceJurySettings(null)).toEqual({ jury_pool: [], jury_work_classes: [] });
  });
});
