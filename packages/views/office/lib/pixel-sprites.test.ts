// @vitest-environment node
import { describe, expect, it } from "vitest";
import { hashString, lookForActor, noteColors, spriteSheetUri } from "./pixel-sprites";

describe("lookForActor", () => {
  it("gives one actor one look, every time", () => {
    expect(lookForActor("agent", "a-1")).toEqual(lookForActor("agent", "a-1"));
  });

  it("tells people, agents and squads apart", () => {
    expect(lookForActor("member", "x").kind).toBe("human");
    expect(lookForActor("agent", "x").kind).toBe("agent");
    expect(lookForActor("squad", "x").kind).toBe("squad");
  });

  it("does not share a look between the same id of different types", () => {
    expect(hashString("member:x")).not.toBe(hashString("agent:x"));
  });
});

describe("spriteSheetUri", () => {
  it("packs two frames side by side", () => {
    const uri = decodeURIComponent(spriteSheetUri(lookForActor("member", "x"), "stand"));
    expect(uri).toContain('viewBox="0 0 18 14"');
  });

  it("crops a seated worker to the upper body", () => {
    const uri = decodeURIComponent(spriteSheetUri(lookForActor("agent", "x"), "sit"));
    expect(uri).toContain('viewBox="0 0 18 10"');
  });

  it("reuses the encoded sheet for the same look and pose", () => {
    const look = lookForActor("member", "y");
    expect(spriteSheetUri(look, "sit")).toBe(spriteSheetUri(look, "sit"));
  });
});

describe("noteColors", () => {
  it("has a note for every priority", () => {
    for (const priority of ["urgent", "high", "medium", "low", "none"] as const) {
      expect(noteColors(priority).paper).toMatch(/^#/);
    }
  });
});
