// @vitest-environment node
// Guards the text every Kit Viet gate reads about its VERDICT line: post exactly one line that starts with
// "VERDICT: QUA" or "VERDICT: CHẶN", as the first line of the comment. A gate's instruction is composed from the
// gate kind text, so the wording is checked on that composition with a squad header and a role body, the way a
// gate role is composed. This checks the wording only. It does not test what a gate outputs; no gate is run here.

import { describe, expect, it } from "vitest";
import { composeKvAgentInstructions } from "./kitviet-agent-instructions";
import { KITVIET_BUILTIN_SKILLS } from "../catalog/generated/kitviet/common";

function skillContent(name: string): string {
  const skill = KITVIET_BUILTIN_SKILLS.find((s) => s.name === name);
  if (!skill) throw new Error(`missing skill ${name}`);
  return skill.content;
}

const DELIVERY = skillContent("kv--delivery-core");
const GATE_RULE = 'Post exactly one line that starts with "VERDICT: QUA" or "VERDICT: CHẶN", as the first line of your comment.';

// A gate role as the squad composes it: the squad header, the gate kind text, and the role body from the pack.
const GATE_ROLE = composeKvAgentInstructions({
  kind: "kv_gate",
  header: "You are the Acct Numbers Gate in an Accounting & Tax squad, unattended.",
  body: "Check the numbers in the draft against the ledger.",
});

describe("each gate's instruction asks for one VERDICT line first (text only)", () => {
  it("the gate role states the one-line rule with both verdicts", () => {
    expect(GATE_ROLE).toContain(GATE_RULE);
  });

  it("the gate role states that a comment with no such line, or with two, is CHẶN", () => {
    expect(GATE_ROLE).toContain('A comment with no such line, or with two, is read as CHẶN.');
  });

  it("the gate role keeps the role body after the kind text", () => {
    expect(GATE_ROLE).toContain("Check the numbers in the draft against the ledger.");
    expect(GATE_ROLE.indexOf(GATE_RULE)).toBeLessThan(GATE_ROLE.indexOf("Check the numbers in the draft"));
  });

  it("the delivery core, which every stage role loads, states the same rule", () => {
    expect(DELIVERY).toContain(
      "A gate posts exactly one line that starts with `VERDICT: QUA` or `VERDICT: CHẶN`, as the first line of its comment.",
    );
  });
});

// Drill defect D3: a gate given an inline draft (text in a comment or a description) must not pass it, and a
// gate with no attached draft must still post one CHẶN line with the no-draft reason.
const NO_DRAFT_RULE = 'When the draft is missing or not attached, post "VERDICT: CHẶN - không có bản nháp đính kèm" with the reason';

describe("a gate with no attached draft posts one CHẶN line (drill D3, text only)", () => {
  it("the gate role says the draft is an attached file and inline text is not a draft", () => {
    expect(GATE_ROLE).toContain("The draft is a file attached to your sub-issue and present in your working directory.");
    expect(GATE_ROLE).toContain("Text pasted into a comment or a description is not a draft.");
  });

  it("the gate role gives the no-draft CHẶN line with the reason", () => {
    expect(GATE_ROLE).toContain(NO_DRAFT_RULE);
  });

  it("the delivery core says the same: no attached draft is CHẶN, and inline text does not count", () => {
    expect(DELIVERY).toContain(
      "A gate whose draft is missing or not attached posts `VERDICT: CHẶN - không có bản nháp đính kèm`, with the reason",
    );
    expect(DELIVERY).toContain("text pasted into a comment or a description does not count.");
  });
});

// Drill KVM-75: the gate's first line was the VERDICT line, but its conclusion repeated "VERDICT: CHẶN". The rule is
// one VERDICT line, as the first line, so the conclusion must not repeat it.
const NO_REPEAT_RULE = "Do not repeat the VERDICT line in the conclusion or anywhere else in the comment.";

describe("the gate's conclusion does not repeat the VERDICT line (drill KVM-75, text only)", () => {
  it("the gate role forbids repeating the VERDICT line", () => {
    expect(GATE_ROLE).toContain(NO_REPEAT_RULE);
  });

  it("the delivery core says the VERDICT line appears once, as the first line", () => {
    expect(DELIVERY).toContain(
      "The VERDICT line appears once, as the first line of the comment; the conclusion does not repeat it.",
    );
  });
});
