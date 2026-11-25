// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { setSchemaLogger } from "../../api/schema";
import { noopLogger, type Logger } from "../../logger";
import { parseEffectList, parsePolicyGrid, parsePolicyCell, parseUndoResult } from "./schemas";

const effectRow = {
  id: "e-1",
  created_at: "2026-11-25T08:00:00Z",
  source: "connected_app",
  target: "gmail:send",
  effect_class: "send",
  reversibility: "irreversible",
  decision: "rehearse",
  status: "awaiting",
  summary: "Gửi báo giá cho khách",
  task_id: "t-1",
  agent_id: "a-1",
  member_id: null,
  undo_available: false,
  undo_until: null,
};

const cellRow = {
  autonomy_level: 1,
  effect_class: "send",
  reversibility: "irreversible",
  default: "rehearse",
  override: null,
  options: [
    { decision: "allow", allowed: false, reason: "loosen_needs_l3" },
    { decision: "confirm", allowed: true, reason: "" },
    { decision: "rehearse", allowed: true, reason: "" },
    { decision: "block", allowed: true, reason: "" },
  ],
};

describe("effect schemas", () => {
  let warn: Mock<(msg: string, ...data: unknown[]) => void>;

  beforeEach(() => {
    warn = vi.fn<(msg: string, ...data: unknown[]) => void>();
    const logger: Logger = { debug: vi.fn(), info: vi.fn(), warn, error: vi.fn() };
    setSchemaLogger(logger);
  });

  afterEach(() => {
    setSchemaLogger(noopLogger);
  });

  it("reads an effect exactly as the server sends it", () => {
    const [effect] = parseEffectList({ effects: [effectRow] }, "GET /api/effects");
    expect(effect).toMatchObject({
      id: "e-1",
      source: "connected_app",
      effect_class: "send",
      decision: "rehearse",
      status: "awaiting",
      task_id: "t-1",
      member_id: null,
      undo_available: false,
      undo_until: null,
    });
    expect(warn).not.toHaveBeenCalled();
  });

  it("degrades an unknown enum to a safe value and keeps the row", () => {
    const [effect] = parseEffectList(
      { effects: [{ ...effectRow, decision: "approve_everything", status: "mystery", source: "new_source" }] },
      "GET /api/effects",
    );
    expect(effect).toMatchObject({ decision: "block", status: "awaiting", source: "connected_app" });
  });

  it("drops a malformed effect and keeps the rest of the list", () => {
    const effects = parseEffectList({ effects: [null, 42, effectRow] }, "GET /api/effects");
    expect(effects.map((e) => e.id)).toEqual(["e-1"]);
  });

  it("answers an empty list for a body that is not a list", () => {
    expect(parseEffectList({ effects: "oops" }, "GET /api/effects")).toEqual([]);
    expect(parseEffectList(null, "GET /api/effects")).toEqual([]);
  });

  it("reads an undo answer", () => {
    expect(
      parseUndoResult({ effect_id: "e-1", status: "undone", undo_event_id: "e-2" }, "POST /api/effects/:id/undo"),
    ).toEqual({ effect_id: "e-1", status: "undone", undo_event_id: "e-2" });
  });

  it("reads a policy grid cell with its lock reasons", () => {
    const cell = parsePolicyCell(cellRow, "GET /api/effects/policy");
    expect(cell).toMatchObject({ autonomy_level: 1, default: "rehearse", override: null });
    expect(cell?.options[0]).toEqual({ decision: "allow", allowed: false, reason: "loosen_needs_l3" });
  });

  it("drops a policy cell whose class it does not know, rather than showing it under another", () => {
    const cells = parsePolicyGrid({ cells: [cellRow, { ...cellRow, effect_class: "teleport" }] }, "GET /api/effects/policy");
    expect(cells).toHaveLength(1);
  });

  it("treats an option whose lock flag is not a boolean as locked", () => {
    const cell = parsePolicyCell(
      { ...cellRow, options: [{ decision: "allow", allowed: "yes", reason: 3 }] },
      "GET /api/effects/policy",
    );
    expect(cell?.options).toEqual([{ decision: "allow", allowed: false, reason: "" }]);
  });
});
