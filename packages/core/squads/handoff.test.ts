// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  FALLBACK_ISSUE_HANDOFF,
  FALLBACK_SQUAD_HANDOFF_RULE,
  parseIssueHandoff,
  parseSquadHandoffRule,
} from "./handoff";

describe("parseSquadHandoffRule", () => {
  it("reads a rule with its target and mode", () => {
    expect(
      parseSquadHandoffRule({ squad_id: "s1", target_squad_id: "s2", target_squad_name: "SDLC", mode: "auto" }, "x"),
    ).toMatchObject({ squad_id: "s1", target_squad_id: "s2", target_squad_name: "SDLC", mode: "auto" });
  });

  it("shows a squad with no rule as off with no target", () => {
    const rule = parseSquadHandoffRule({ squad_id: "s1", target_squad_id: null, mode: "off" }, "x");
    expect(rule.target_squad_id).toBeNull();
    expect(rule.mode).toBe("off");
  });

  it("reads a mode it does not know as off, and a malformed body as no rule", () => {
    expect(parseSquadHandoffRule({ squad_id: "s1", target_squad_id: "s2", mode: "always" }, "x").mode).toBe("off");
    expect(parseSquadHandoffRule("nope", "x")).toEqual(FALLBACK_SQUAD_HANDOFF_RULE);
  });
});

describe("parseIssueHandoff", () => {
  it("reads a handed-off request with the other squad's request", () => {
    const view = parseIssueHandoff(
      {
        status: "handed_off",
        rule: { squad_id: "s1", target_squad_id: "s2", target_squad_name: "SDLC", mode: "auto" },
        handoff: {
          id: "h1",
          source_issue_id: "i1",
          target_issue_id: "i2",
          target_squad_id: "s2",
          target_squad_name: "SDLC",
          mode: "auto",
          status: "handed_off",
          created_at: "2026-11-10T01:00:00Z",
          decided_at: "2026-11-10T01:00:01Z",
        },
      },
      "x",
    );
    expect(view.status).toBe("handed_off");
    expect(view.handoff?.target_issue_id).toBe("i2");
    expect(view.rule?.mode).toBe("auto");
  });

  it("shows a status from a newer server as none, and a handoff status it does not know as pending", () => {
    expect(parseIssueHandoff({ status: "parked" }, "x").status).toBe("none");
    expect(
      parseIssueHandoff({ status: "pending", handoff: { id: "h1", status: "parked", target_squad_id: "s2" } }, "x").handoff
        ?.status,
    ).toBe("pending");
  });

  it("falls back to no handoff when the body is malformed", () => {
    expect(parseIssueHandoff(42, "x")).toEqual(FALLBACK_ISSUE_HANDOFF);
  });
});
