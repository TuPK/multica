// @vitest-environment node

import { describe, expect, it } from "vitest";
import type { FlowDocRole } from "@multica/core/types";
import { skillsOfRole } from "./role-skills";

const agents = [
  { id: "a1", skills: [{ name: "tone-check" }, { name: "outline-first" }, { name: "off", enabled: false }] },
  { id: "a2", skills: [{ name: "tone-check" }] },
];
const role = (ids: string[], kind = "agent") => ({ key: "r", kind, bind: { agent_ids: ids } }) as unknown as FlowDocRole;

describe("skillsOfRole", () => {
  it("is the squad's skills when no agent is bound", () => {
    expect(skillsOfRole(role([]), agents, ["shared"])).toEqual(["shared"]);
    expect(skillsOfRole(undefined, agents, ["shared"])).toEqual(["shared"]);
  });

  it("adds the enabled skills of the bound agent", () => {
    expect(skillsOfRole(role(["a1"]), agents, ["shared"])).toEqual(["outline-first", "shared", "tone-check"]);
  });

  it("keeps only the skills that every bound agent has", () => {
    expect(skillsOfRole(role(["a1", "a2"]), agents, [])).toEqual(["tone-check"]);
  });

  it("does not read the agents of a role of people", () => {
    expect(skillsOfRole(role(["a1"], "member"), agents, [])).toEqual([]);
  });
});
