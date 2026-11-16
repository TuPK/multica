// @vitest-environment node
// Checks of the marketing domain (docs/specs/marketing-domain/spec.md, section 7): what the vendored skills
// are, that the front-door squad routes to squads that exist, and that the squads' texts hold the rules.
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { CATALOG_SQUADS } from "./generated/squads";
import { skillsetMembers } from "./skillsets";
import { SQUAD_ROLES, SQUAD_TEMPLATES } from "../squads/templates";

const CATALOG = join(__dirname, "..", "..", "..", "catalog");
const SKILLS = join(CATALOG, "skills", "marketing");
const GROUPS = ["foundation", "strategy", "research", "search", "conversion", "paid", "lifecycle", "measurement"];
const UPSTREAM_PIN = "1f837447b20bb6f0e72d87b80ece1c221c362c7d";

// Every regular file under dir, relative to it, with forward slashes.
function filesUnder(dir: string, prefix = ""): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    const rel = prefix ? `${prefix}/${name}` : name;
    return statSync(path).isDirectory() ? filesUnder(path, rel) : [rel];
  });
}

// Every skill folder (the one with template.json and SKILL.md), by id.
function marketingSkills(): { id: string; group: string; dir: string; manifest: Record<string, unknown> }[] {
  return GROUPS.flatMap((group) =>
    readdirSync(join(SKILLS, group)).map((id) => ({
      id,
      group,
      dir: join(SKILLS, group, id),
      manifest: JSON.parse(readFileSync(join(SKILLS, group, id, "template.json"), "utf8")) as Record<string, unknown>,
    })),
  );
}

const read = (rel: string) => readFileSync(join(CATALOG, rel), "utf8");
const UPSTREAM_IDS = [
  "marketing-plan", "marketing-ideas", "marketing-psychology", "marketing-loops", "launch", "pricing", "offers",
  "product-marketing", "customer-research", "competitor-profiling", "seo-audit", "ai-seo", "programmatic-seo", "schema",
  "site-architecture", "content-strategy", "link-building", "competitors", "cro", "signup", "popups", "copywriting",
  "copy-editing", "ads", "ad-creative", "emails", "onboarding", "churn-prevention", "referrals", "lead-magnets",
  "analytics", "attribution", "ab-testing",
];

describe("marketing skills (vendored)", () => {
  const skills = marketingSkills();

  it("holds 51 skills in the eight groups, each with its own folder and template", () => {
    expect(skills).toHaveLength(51);
    expect(new Set(skills.map((s) => s.group))).toEqual(new Set(GROUPS));
  });

  it("pins every vendored skill to a 40-character commit, and gives it a licence file", () => {
    for (const { id, manifest } of skills) {
      const source = manifest.source as { type: string; ref?: string; license?: string };
      if (source.type !== "vendored") continue;
      expect(source.ref, id).toMatch(/^[0-9a-f]{40}$/);
      expect(source.license, id).toBeTruthy();
    }
    const upstream = skills.filter((s) => (s.manifest.source as { repo?: string }).repo?.endsWith("marketingskills"));
    expect(upstream).toHaveLength(33);
    for (const { manifest } of upstream) expect((manifest.source as { ref: string }).ref).toBe(UPSTREAM_PIN);
    for (const name of ["marketingskills", "pm-skills", "wondelai-skills", "idea-validation-agents"]) {
      expect(existsSync(join(SKILLS, "licenses", `${name}.LICENSE`)), name).toBe(true);
    }
  });

  it("ships no evals, scripts or tools folder, and no advertising for the maintainer's product or a partner", () => {
    for (const { id, dir } of skills) {
      const files = filesUnder(dir);
      expect(files.filter((f) => f.startsWith("evals/") || f.startsWith("tools/")), id).toEqual([]);
      expect(files.filter((f) => /\.(py|mjs|js|sh)$/.test(f)), id).toEqual([]);
      for (const file of files.filter((f) => f.endsWith(".md"))) {
        const text = readFileSync(join(dir, file), "utf8");
        expect(text, `${id}/${file}`).not.toMatch(/magister|\?ref=marketingskills|◆|Verified Partner/i);
        expect(text, `${id}/${file}`).not.toMatch(/\]\((?:\.\.\/)+tools\//);
      }
    }
  });

  it("names every skill and its id the same way, and keeps the upstream list", () => {
    for (const { id, dir } of skills) {
      const head = readFileSync(join(dir, "SKILL.md"), "utf8").split("\n").slice(0, 3).join("\n");
      expect(head, id).toContain(`name: ${id}`);
    }
    expect(UPSTREAM_IDS.every((id) => skills.some((s) => s.id === id))).toBe(true);
  });

  it("gives every skill to the marketing chunk, which is the only skillset that names them", () => {
    const members = skillsetMembers("marketing");
    expect([...members].sort()).toEqual(skills.map((s) => s.id).sort());
  });

  it("is documented with its sources, and each upstream skill is listed there", () => {
    const sources = read("skills/marketing/SOURCES.md");
    expect(sources).toContain(UPSTREAM_PIN);
    for (const id of UPSTREAM_IDS) expect(sources, id).toContain(`\`${id}\``);
  });
});

describe("growth lab squad", () => {
  const growth = CATALOG_SQUADS.find((s) => s.id === "growth_lab")!;
  const roleKeys = growth.squad.roles.map((r) => r.key);

  it("has seven roles led by the lead, in the marketing collection and domain", () => {
    expect(roleKeys).toEqual(["gl_lead", "gl_search", "gl_conversion", "gl_paid", "gl_lifecycle", "gl_measure", "gl_checker"]);
    expect(growth.squad.leader).toBe("gl_lead");
    expect(growth.collection).toBe("marketing");
  });

  it("gives every one of the 33 imported skills to at least one role", () => {
    const given = new Set([
      ...(growth.squad.skills?.extra ?? []),
      ...(growth.squad.skills?.leader ?? []),
      ...Object.values(growth.squad.skills?.byRole ?? {}).flat(),
    ]);
    for (const id of UPSTREAM_IDS) expect(given.has(id), id).toBe(true);
  });

  it("has a role file for every role, each one naming the market skill to load", () => {
    for (const key of roleKeys) {
      const file = join(CATALOG, "squads", "growth_lab", "roles", `${key}.md`);
      expect(existsSync(file), key).toBe(true);
      expect(readFileSync(file, "utf8"), key).toMatch(/mkt-market-vn or mkt-market-intl/);
    }
  });

  it("defaults the market to vn in its lead's instructions, and never tells an agent to send, publish or spend", () => {
    const instructions = read("squads/growth_lab/instructions.md");
    expect(instructions).toMatch(/the market is vn/);
    for (const file of [join("squads", "growth_lab", "instructions.md"), ...roleKeys.map((k) => join("squads", "growth_lab", "roles", `${k}.md`))]) {
      expect(read(file), file).not.toMatch(/\b(send it now|publish it now|post it now|spend the budget|charge the card)\b/i);
    }
  });
});

describe("marketing front-door squad", () => {
  const front = CATALOG_SQUADS.find((s) => s.id === "marketing")!;
  const instructions = read("squads/marketing/instructions.md");
  const names = (id: string) => CATALOG_SQUADS.find((s) => s.id === id)!.squad.name;

  it("includes the four specialist squads, all in its domain, and nothing deeper", () => {
    expect(front.includes).toEqual(["kv_marketing", "content_studio", "growth_lab", "kv_pr"]);
    for (const id of front.includes ?? []) {
      const squad = CATALOG_SQUADS.find((s) => s.id === id)!;
      expect(squad.includes, id).toBeUndefined();
    }
  });

  it("names every squad of its routing table with the exact name the catalog gives it", () => {
    for (const id of ["kv_marketing", "content_studio", "growth_lab", "kv_pr"]) {
      expect(instructions, id).toContain(names(id));
    }
    // The routing table has five rows: four squads and the lead itself.
    expect(instructions.split("\n").filter((l) => l.startsWith("| ") && !l.startsWith("| Request") && !l.startsWith("| ---"))).toHaveLength(5);
  });

  it("hands work over with --assignee-squad, sets the market first, and defaults it to vn", () => {
    expect(instructions).toContain("--assignee-squad");
    expect(instructions).toMatch(/Market: vn/);
    expect(instructions).toMatch(/the market is vn/);
  });

  it("does not write the finished piece, replace a gate, or send, publish or spend", () => {
    expect(instructions).toMatch(/Does not|never|Never|do not write/i);
    expect(instructions).toMatch(/Nothing is sent, published, posted, paid for or signed/);
  });

  it("is the one squad a person creates: the others are listed as included", () => {
    expect(front.squad.description).toMatch(/The one marketing squad to create/);
  });

  it("gives every role name in the marketing domain one role key, so an agent is never shared by mistake", () => {
    const byName = new Map<string, Set<string>>();
    for (const tpl of SQUAD_TEMPLATES) {
      for (const key of tpl.roles) {
        const name = SQUAD_ROLES[key].name;
        byName.set(name, (byName.get(name) ?? new Set()).add(key));
      }
    }
    const shared = [...byName.entries()].filter(([, keys]) => keys.size > 1).map(([name]) => name);
    expect(shared).toEqual([]);
  });
});
