import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CreateSkillRequest, Skill, Squad, UpdateSkillRequest } from "../types";
import { KITVIET_EXTENSION_BUILTIN_SKILLS } from "./kitviet-extensions";
import { kitvietPackOf, loadKitvietSkillPacks } from "./kitviet-packs";
import {
  attachKitvietExtensions,
  kitvietExtensionTargetOf,
  type KitvietAttachClient,
} from "./kitviet-extension-attach";

// The attach loads a pack only when a squad of its target exists, so the spy is read in the first
// test, before any other test has loaded a pack (the quickstart keeps its loads in a module cache).
vi.mock("./kitviet-packs", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./kitviet-packs")>();
  return { ...actual, loadKitvietSkillPacks: vi.fn(actual.loadKitvietSkillPacks) };
});

const WS = "ws-1";

function squad(id: string, name: string, extra: Partial<Squad> = {}): Squad {
  return {
    id,
    workspace_id: WS,
    name,
    description: "",
    instructions: "",
    avatar_url: null,
    leader_id: `leader-${id}`,
    creator_id: "user-1",
    created_at: "",
    updated_at: "",
    archived_at: null,
    archived_by: null,
    template_key: null,
    ...extra,
  };
}

function makeSkill(
  id: string,
  name: string,
  config: Record<string, unknown>,
  content = "boss text",
): Skill {
  return {
    id,
    workspace_id: WS,
    name,
    description: "",
    content,
    config,
    created_by: null,
    created_at: "",
    updated_at: "",
    files: [],
  };
}

/** An in-memory stand-in for the skill endpoints. It keeps the squads' skill sets and counts the writes. */
function fakeClient(seed: Skill[] = [], squadSkillIds: Record<string, string[]> = {}, failListFor?: string) {
  const skills = new Map(seed.map((s) => [s.id, s] as const));
  const bySquad = new Map(Object.entries(squadSkillIds));
  let next = 0;
  const writes = { createSkill: [] as string[], updateSkill: [] as string[], setSquadSkills: [] as string[][] };
  const client: KitvietAttachClient = {
    async listSkills() {
      return [...skills.values()].map(({ content: _content, files: _files, ...summary }) => summary);
    },
    async getSkill(id) {
      const skill = skills.get(id);
      if (!skill) throw new Error(`no skill ${id}`);
      return skill;
    },
    async createSkill(data: CreateSkillRequest) {
      const id = `skill-${++next}`;
      const skill = makeSkill(id, data.name, data.config ?? {}, data.content ?? "");
      skill.description = data.description ?? "";
      skill.files = (data.files ?? []).map((f, i) => ({
        id: `${id}-f${i}`,
        skill_id: id,
        path: f.path,
        content: f.content,
        created_at: "",
        updated_at: "",
      }));
      skills.set(id, skill);
      writes.createSkill.push(data.name);
      return skill;
    },
    async updateSkill(id, data: UpdateSkillRequest) {
      writes.updateSkill.push(id);
      const skill = skills.get(id)!;
      skills.set(id, { ...skill, description: data.description ?? skill.description, content: data.content ?? skill.content });
      return skills.get(id)!;
    },
    async listSquadSkills(squadId) {
      if (squadId === failListFor) throw new Error("boom");
      return (bySquad.get(squadId) ?? []).map((id) => {
        const s = skills.get(id)!;
        return { id, name: s.name, description: s.description };
      });
    },
    async setSquadSkills(squadId, ids) {
      writes.setSquadSkills.push(ids);
      bySquad.set(squadId, ids);
      return ids.map((id) => ({ id, name: skills.get(id)!.name, description: "" }));
    },
  };
  return {
    client,
    writes,
    /** The names of the skills a squad has now. */
    namesOf: (squadId: string) => (bySquad.get(squadId) ?? []).map((id) => skills.get(id)!.name),
    idsOf: (squadId: string) => bySquad.get(squadId) ?? [],
    skillNamed: (name: string) => [...skills.values()].find((s) => s.name === name),
  };
}

describe("attachKitvietExtensions (spec 7)", () => {
  beforeEach(() => {
    vi.mocked(loadKitvietSkillPacks).mockClear();
  });

  // First on purpose: the lazy-load check needs the packs unloaded (see the vi.mock above).
  it("loads only the pack of the target it attaches to", async () => {
    const fake = fakeClient();
    await attachKitvietExtensions(fake.client, [squad("build-sq", "Build"), squad("sales", "Sales Loop", { template_key: "sales_loop" })]);
    const packs = vi.mocked(loadKitvietSkillPacks).mock.calls.map(([pack]) => pack);
    expect(new Set(packs)).toEqual(new Set(["it"]));
  });

  it("attaches the extension's skills to a company squad and keeps the squad's own skills", async () => {
    const user = makeSkill("user-1", "my-own-skill", {});
    const fake = fakeClient([user], { "support-sq": ["user-1"] });

    const outcomes = await attachKitvietExtensions(fake.client, [squad("support-sq", "Support")]);

    const wanted = KITVIET_EXTENSION_BUILTIN_SKILLS.support!;
    expect(outcomes).toEqual([{ squadId: "support-sq", target: "support", status: "ready" }]);
    expect(new Set(fake.namesOf("support-sq"))).toEqual(new Set(["my-own-skill", ...wanted]));
    expect(fake.namesOf("support-sq")).toHaveLength(1 + wanted.length);
    // The imported skills are marked as Kit Viet skills, so a rerun may update them.
    for (const name of wanted) expect(fake.skillNamed(name)?.config).toEqual({ source: "kitviet" });
  });

  it("is idempotent: a second run writes nothing", async () => {
    const fake = fakeClient();
    await attachKitvietExtensions(fake.client, [squad("people-sq", "People & Performance")]);
    const createdAfterFirst = fake.writes.createSkill.length;
    const setAfterFirst = fake.writes.setSquadSkills.length;

    const outcomes = await attachKitvietExtensions(fake.client, [squad("people-sq", "People & Performance")]);

    expect(outcomes).toEqual([{ squadId: "people-sq", target: "people_performance", status: "ready" }]);
    expect(fake.writes.createSkill).toHaveLength(createdAfterFirst);
    expect(fake.writes.updateSkill).toEqual([]);
    expect(fake.writes.setSquadSkills).toHaveLength(setAfterFirst);
  });

  it("gives the same skills to each CNTT squad, creating each skill once", async () => {
    const fake = fakeClient();
    const squads = [squad("pa", "Product Analysis"), squad("pl", "Planning"), squad("bd", "Build")];

    const outcomes = await attachKitvietExtensions(fake.client, squads);

    expect(outcomes.map((o) => [o.target, o.status])).toEqual([
      ["product_analysis", "ready"],
      ["planning", "ready"],
      ["build", "ready"],
    ]);
    const wanted = KITVIET_EXTENSION_BUILTIN_SKILLS.build!;
    for (const id of ["pa", "pl", "bd"]) expect(new Set(fake.namesOf(id))).toEqual(new Set(wanted));
    expect(fake.writes.createSkill).toHaveLength(new Set(fake.writes.createSkill).size);
  });

  it("skips squads that are not extension targets", async () => {
    const fake = fakeClient();
    const outcomes = await attachKitvietExtensions(fake.client, [
      squad("sales", "Sales Loop", { template_key: "sales_loop" }),
      squad("admin", "Admin", { template_key: "kv_admin" }),
      // A template_key decides the match: this squad was made from sales_loop, not as a company Support squad.
      squad("sup-sales", "Support", { template_key: "sales_loop" }),
      squad("hand", "Customer Care"),
      squad("old", "Support", { archived_at: "2026-01-01T00:00:00Z" }),
    ]);

    expect(outcomes).toEqual([]);
    expect(fake.writes.setSquadSkills).toEqual([]);
    expect(fake.writes.createSkill).toEqual([]);
  });

  it("names the boss's own skill under an extension name, leaves it alone, and attaches the rest", async () => {
    const bossCopy = makeSkill("boss-1", "kv--dac-ta-yeu-cau-srs", {}, "the boss's own text");
    const fake = fakeClient([bossCopy], { "build-sq": ["boss-1"] });

    const [outcome] = await attachKitvietExtensions(fake.client, [squad("build-sq", "Build")]);

    expect(outcome?.status).toBe("partial");
    expect(outcome?.error).toContain("kv--dac-ta-yeu-cau-srs");
    expect(fake.writes.updateSkill).not.toContain("boss-1");
    expect(fake.idsOf("build-sq")).toContain("boss-1");
    expect(fake.namesOf("build-sq").length).toBeGreaterThan(1);
  });

  it("reports a failed squad and still attaches the others", async () => {
    const fake = fakeClient([], {}, "bad-sq");
    const outcomes = await attachKitvietExtensions(fake.client, [squad("bad-sq", "Support"), squad("ok-sq", "Planning")]);

    // The skills are imported before the squad's list fails, so the squad is partial, not failed.
    expect(outcomes.find((o) => o.squadId === "bad-sq")).toMatchObject({ status: "partial", error: "attach: boom" });
    expect(outcomes.find((o) => o.squadId === "ok-sq")?.status).toBe("ready");
  });
});

describe("kitvietExtensionTargetOf", () => {
  it("matches a company squad by its card name when it has no template_key", () => {
    expect(kitvietExtensionTargetOf(squad("x", "Product Analysis"))).toBe("product_analysis");
    expect(kitvietExtensionTargetOf(squad("x", "People & Performance"))).toBe("people_performance");
  });

  it("matches a template-made squad by its template_key, not its name", () => {
    expect(kitvietExtensionTargetOf(squad("x", "Support", { template_key: "kv_b2b" }))).toBeUndefined();
  });
});

describe("extension skill guard (K8-style)", () => {
  it("every skill each target attaches exists in the pack the target loads", async () => {
    for (const [target, names] of Object.entries(KITVIET_EXTENSION_BUILTIN_SKILLS)) {
      const packs = await loadKitvietSkillPacks(kitvietPackOf(target));
      const exported = new Set(packs.flatMap((pack) => pack.KITVIET_BUILTIN_SKILLS.map((skill) => skill.name)));
      expect(names.filter((name) => !exported.has(name)), target).toEqual([]);
    }
  });
});
