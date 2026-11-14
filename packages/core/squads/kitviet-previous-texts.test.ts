// @vitest-environment node
// Guards the upgrade registry (kitviet-previous-texts.ts). A previous text must not be the current text of its role, and
// every gate role must keep the text it had before the rule that forbids repeating the VERDICT line (drill KVM-75). An
// agent created with that earlier text is upgraded only while the earlier text is listed.

import { describe, expect, it } from "vitest";
import { loadKitvietRoleInstructions } from "./kitviet-packs";
import { KITVIET_PREVIOUS_AGENT_INSTRUCTIONS } from "./kitviet-previous-texts";
import { KITVIET_SQUADS } from "../catalog/generated/kitviet/squads";
import { findKitvietRole } from "./kitviet-squad-roles";

const NO_REPEAT_RULE = "Do not repeat the VERDICT line in the conclusion or anywhere else in the comment.";
// The gate rule as the f27661e8a text stated it, before the no-repeat rule was added.
const FIRST_LINE_RULE = 'Post exactly one line that starts with "VERDICT: QUA" or "VERDICT: CHẶN", as the first line of your comment.';
describe("the upgrade registry lists earlier texts of Kit Viet roles", () => {
  it("every registry key is a Kit Viet role", () => {
    for (const key of Object.keys(KITVIET_PREVIOUS_AGENT_INSTRUCTIONS)) {
      expect(findKitvietRole(key), key).toBeDefined();
    }
  });

  it("no previous text equals the current text of its role", async () => {
    for (const [key, previous] of Object.entries(KITVIET_PREVIOUS_AGENT_INSTRUCTIONS)) {
      const current = await loadKitvietRoleInstructions(key);
      expect(previous, key).not.toContain(current);
    }
  }, 30_000); // loads every role in turn; it runs near the 5s default when the whole monorepo tests in parallel
});

describe("gate texts from before the no-repeat VERDICT rule stay listed (drill KVM-75)", () => {
  it("every gate role has the text that stated the rule before the no-repeat rule", () => {
    const gateKeys = KITVIET_SQUADS.flatMap((squad) => squad.roles.filter((role) => role.kind === "kv_gate").map((role) => role.key));
    expect(gateKeys.length).toBeGreaterThan(0);
    for (const key of gateKeys) {
      const previous = KITVIET_PREVIOUS_AGENT_INSTRUCTIONS[key] ?? [];
      expect(
        previous.some((text) => text.includes(FIRST_LINE_RULE) && !text.includes(NO_REPEAT_RULE)),
        key,
      ).toBe(true);
    }
  });
});
