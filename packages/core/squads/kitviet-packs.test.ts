// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";
import { KITVIET_EXTENSIONS } from "../catalog/generated/kitviet/extensions";
import { KITVIET_PACK_LOADERS } from "../catalog/generated/kitviet/packs";
import { KITVIET_SQUADS } from "../catalog/generated/kitviet/squads";

// Each pack chunk is stubbed, and the stub records its import. A test gets a fresh module registry
// with fresh stubs, so each call below shows exactly which chunks it pulled in.
// Every pack the configs generate (common, the squads, the extensions), so a new config is covered too.
const CHUNKS = Object.keys(KITVIET_PACK_LOADERS);
const loaded = vi.hoisted(() => ({ chunks: [] as string[] }));

// The legal squad's first role with a body, so the role loader finds an agent in its stub.
const legalRole = KITVIET_SQUADS.find((squad) => squad.pack === "legal")!.roles.find((r) => r.hasBody)!.key;

let kitvietPacks: typeof import("./kitviet-packs");

beforeEach(async () => {
  vi.resetModules();
  loaded.chunks.length = 0;
  for (const chunk of CHUNKS) {
    vi.doMock(`../catalog/generated/kitviet/${chunk}`, () => {
      loaded.chunks.push(chunk);
      return {
        KITVIET_BUILTIN_SKILLS: [],
        KITVIET_AGENTS: chunk === "legal" ? [{ name: legalRole, kind: "kv_drafter", instructions: "legal body" }] : [],
      };
    });
  }
  kitvietPacks = await import("./kitviet-packs");
});

describe("loadKitvietSkillPacks", () => {
  it("imports only common and the squad's own pack", async () => {
    const packs = await kitvietPacks.loadKitvietSkillPacks("legal");

    expect(packs).toHaveLength(2);
    expect(loaded.chunks.sort()).toEqual(["common", "legal"]);
  });

  it("imports common alone for a squad with no pack", async () => {
    await kitvietPacks.loadKitvietSkillPacks(undefined);

    expect(loaded.chunks).toEqual(["common"]);
  });

  it("does not import common twice when the squad's pack is common", async () => {
    await kitvietPacks.loadKitvietSkillPacks("common");

    expect(loaded.chunks).toEqual(["common"]);
  });

  it("rejects a pack with no loader, naming it", async () => {
    await expect(kitvietPacks.loadKitvietSkillPacks("nope")).rejects.toThrow(/no pack loader for nope/);
  });
});

describe("kitvietPackOf", () => {
  it("maps each Kit Viet squad template to its pack, and any other template to none", () => {
    for (const squad of KITVIET_SQUADS) {
      expect(kitvietPacks.kitvietPackOf(squad.templateId), squad.templateId).toBe(squad.pack);
    }
    expect(kitvietPacks.kitvietPackOf("sdlc_greenfield")).toBeUndefined();
  });

  it("maps each extension target to its extension pack (spec 7)", () => {
    for (const extension of KITVIET_EXTENSIONS) {
      for (const target of extension.targets) {
        expect(kitvietPacks.kitvietPackOf(target), target).toBe(extension.pack);
      }
    }
  });

  it("loads an extension pack with common, and nothing else", async () => {
    const extension = KITVIET_EXTENSIONS[0]!;
    await kitvietPacks.loadKitvietSkillPacks(extension.pack);
    expect(loaded.chunks.sort()).toEqual(["common", extension.pack].sort());
  });
});

describe("loadKitvietRoleInstructions", () => {
  it("loads only the role's own pack for its body", async () => {
    const text = await kitvietPacks.loadKitvietRoleInstructions(legalRole);

    expect(text).toContain("legal body");
    expect(loaded.chunks).toEqual(["legal"]);
  });

  it("returns undefined for a role that is not a Kit Viet role, and loads nothing", async () => {
    expect(await kitvietPacks.loadKitvietRoleInstructions("sdlc_developer")).toBeUndefined();
    expect(loaded.chunks).toEqual([]);
  });
});
