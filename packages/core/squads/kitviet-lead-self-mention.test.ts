// @vitest-environment node
// Guards the wording that stops a Kit Viet lead from waking itself (drill defect D1). In the drill, the lead's
// stop comment carried a mention://agent link to the lead's own id, copied from the resume rule. That mention
// enqueued a second run of the lead on the same request in 5 of 5 cases, and the second run only posted
// "no reply". The resume rule is in the generated delivery core that every role loads. This checks the wording
// only; no lead is run here.

import { describe, expect, it } from "vitest";
import { KITVIET_BUILTIN_SKILLS } from "../catalog/generated/kitviet/common";

function skillContent(name: string): string {
  const skill = KITVIET_BUILTIN_SKILLS.find((s) => s.name === name);
  if (!skill) throw new Error(`missing skill ${name}`);
  return skill.content;
}

const DELIVERY = skillContent("kv--delivery-core");

describe("a stop comment never mentions its own author (drill defect D1)", () => {
  it("the resume rule no longer offers the lead's own mention as the text to copy", () => {
    expect(DELIVERY).not.toContain("mentions the lead, `[@<lead name>](mention://agent/<lead id>)`");
  });

  it("the delivery core forbids a mention of the lead in the lead's own stop comment, with the reason", () => {
    expect(DELIVERY).toContain("Your own stop comment never contains a `mention://agent/` link");
    expect(DELIVERY).toContain("enqueues a new run of you on the same request");
  });

  it("the delivery core tells the lead to name itself in plain words in its stop comment", () => {
    expect(DELIVERY).toContain("name the lead in plain words");
  });
});
