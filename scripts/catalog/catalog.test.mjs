// Tests for the catalog CLI: discovery, validation (every layer that has a rule in the spec) and the
// generated skill bundle. Run: node --test scripts/catalog/*.test.mjs
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { after, describe, it } from "node:test";
import { discover, readRegistryConfig } from "./lib/discover.mjs";
import { renderSkillsBuiltin, renderSkillsChunk, renderSkillsets, skillsetMembers } from "./lib/build-skills.mjs";
import { flowRev, flowsById, renderFlows } from "./lib/build-workflows.mjs";
import { buildAll } from "./lib/build.mjs";
import { load } from "./cli.mjs";
import { buildMap } from "./lib/map.mjs";
import { buildPackBundle, catalogSkillDirsOf } from "./lib/build-packs.mjs";
import { withBuildLock } from "./lib/lock.mjs";
import { supportedLocales, validateCatalog } from "./lib/validate.mjs";

const roots = [];
after(() => {
  for (const root of roots) rmSync(root, { recursive: true, force: true });
});

function write(root, rel, value) {
  const path = join(root, rel);
  mkdirSync(join(path, ".."), { recursive: true });
  writeFileSync(path, typeof value === "string" ? value : `${JSON.stringify(value, null, 2)}\n`);
}

const LOCALES = supportedLocales();

// A catalog with one skillset, one skill, one collection and one squad, all valid.
function fixture(overrides = {}) {
  const root = mkdtempSync(join(tmpdir(), "catalog-test-"));
  roots.push(root);
  write(root, "registry.config.json", { schemaVersion: 1, locales: LOCALES, removed: [] });
  write(root, "skillsets/demo/template.json", { schemaVersion: 1, kind: "skillset", id: "demo", version: "1.0.0", target: "quickstart" });
  write(root, "skills/g/demo-skill/SKILL.md", '---\nname: demo-skill\ndescription: "Use when testing."\n---\n\n# Demo\n');
  write(root, "skills/g/demo-skill/references/notes.md", "notes\n");
  write(root, "skills/g/demo-skill/template.json", {
    schemaVersion: 1, kind: "skill", id: "demo-skill", version: "1.0.0", sets: ["demo"], source: { type: "authored" },
  });
  write(root, "collections/main/template.json", { schemaVersion: 1, kind: "collection", id: "main", version: "1.0.0", for: "squad" });
  for (const locale of LOCALES) write(root, `collections/main/i18n/${locale}.json`, { title: `Main ${locale}` });
  for (const [rel, value] of Object.entries(overrides)) {
    if (value === null) rmSync(join(root, rel), { recursive: true, force: true });
    else write(root, rel, value);
  }
  return root;
}

function check(root) {
  const { entries, errors } = discover(root);
  return { entries, ...validateCatalog({ entries, discoverErrors: errors, config: readRegistryConfig(root) }) };
}

describe("discovery", () => {
  it("finds templates and stops at SKILL.md", () => {
    write(fixture(), "x", "x");
    const root = fixture({ "skills/g/demo-skill/references/example/SKILL.md": "# a sample inside references\n" });
    const { entries } = discover(root);
    assert.deepEqual(entries.filter((e) => e.kind === "skill").map((e) => e.manifest.id), ["demo-skill"]);
  });

  it("reports a skill folder with no template.json", () => {
    const root = fixture({ "skills/g/demo-skill/template.json": null });
    assert.match(check(root).errors.join("\n"), /has SKILL\.md but no template\.json/);
  });
});

describe("validation", () => {
  it("accepts a valid catalog", () => {
    assert.deepEqual(check(fixture()).errors, []);
  });

  it("requires the id to equal the folder name, with ':' stored as '--'", () => {
    const bad = fixture({
      "skills/g/other-name/SKILL.md": '---\nname: other-name\ndescription: "x"\n---\n',
      "skills/g/other-name/template.json": { schemaVersion: 1, kind: "skill", id: "wrong", version: "1.0.0", sets: ["demo"] },
    });
    assert.match(check(bad).errors.join("\n"), /id "wrong" must equal the folder name "other-name"/);
    const colon = fixture({
      "skills/g/plug--tool/SKILL.md": '---\nname: tool\ndescription: "x"\n---\n',
      "skills/g/plug--tool/template.json": { schemaVersion: 1, kind: "skill", id: "plug:tool", version: "1.0.0", sets: ["demo"] },
    });
    assert.deepEqual(check(colon).errors, []);
  });

  it("rejects a bad id, a bad version, an unknown schemaVersion and a wrong kind", () => {
    const root = fixture({
      "skillsets/Bad_Id/template.json": { schemaVersion: 2, kind: "squad", id: "Bad_Id", version: "one", target: "quickstart" },
    });
    const text = check(root).errors.join("\n");
    assert.match(text, /schemaVersion must be 1/);
    assert.match(text, /kind must be "skillset"/);
    assert.match(text, /does not match/);
    assert.match(text, /version must be semver/);
  });

  it("rejects a duplicate id and an unknown skillset", () => {
    const dup = fixture({
      "skills/h/demo-skill/SKILL.md": '---\nname: demo-skill\ndescription: "x"\n---\n',
      "skills/h/demo-skill/template.json": { schemaVersion: 1, kind: "skill", id: "demo-skill", version: "1.0.0", sets: ["missing"] },
    });
    const text = check(dup).errors.join("\n");
    assert.match(text, /duplicate skill id "demo-skill"/);
    assert.match(text, /unknown skillset "missing"/);
  });

  it("checks the SKILL.md frontmatter name", () => {
    const root = fixture({ "skills/g/demo-skill/SKILL.md": '---\nname: another\ndescription: "Use when testing."\n---\n' });
    assert.match(check(root).errors.join("\n"), /frontmatter name "another"/);
  });

  it("requires vendored sources to name repo, ref and license, and refuses TODO", () => {
    const vendored = (source) => ({ "skills/g/demo-skill/template.json": { schemaVersion: 1, kind: "skill", id: "demo-skill", version: "1.0.0", sets: ["demo"], source } });
    assert.match(check(fixture(vendored({ type: "vendored" }))).errors.join("\n"), /vendored source needs repo/);
    assert.match(check(fixture(vendored({ type: "vendored", repo: "a/b", ref: "TODO", license: "MIT" }))).errors.join("\n"), /ref is still TODO/);
    assert.deepEqual(check(fixture(vendored({ type: "vendored", repo: "a/b", ref: "1.2.3", license: "MIT" }))).errors, []);
  });

  it("warns, without failing, about an unpinned ref, a missing upstream licence and a licence with limits", () => {
    const root = fixture({
      "skills/g/demo-skill/template.json": {
        schemaVersion: 1, kind: "skill", id: "demo-skill", version: "1.0.0", sets: ["demo"],
        source: { type: "vendored", repo: "a/b", ref: "unpinned", license: "none-detected" },
      },
    });
    const result = check(root);
    assert.deepEqual(result.errors, []);
    const text = result.warnings.join("\n");
    assert.match(text, /name no commit or version/);
    assert.match(text, /no licence file \(check before shipping\): a\/b/);
  });

  it("refuses a secret-looking value in a skill file", () => {
    const root = fixture({ "skills/g/demo-skill/references/notes.md": "key AKIAABCDEFGHIJKLMNOP\n" });
    assert.match(check(root).errors.join("\n"), /contains an AWS access key id/);
  });

  it("requires every supported locale for a template with UI text, with no fallback", () => {
    const root = fixture({ "collections/main/i18n/ko.json": null, "collections/main/i18n/ja.json": { title: "x", description: "extra" } });
    const text = check(root).errors.join("\n");
    assert.match(text, /i18n\/ko\.json is missing \(no fallback/);
    assert.match(text, /i18n\/ja\.json must hold exactly "title", has: description,title/);
  });

  it("requires registry.config.json locales to equal SUPPORTED_LOCALES", () => {
    const root = fixture({ "registry.config.json": { schemaVersion: 1, locales: ["en"], removed: [] } });
    assert.match(check(root).errors.join("\n"), /must equal SUPPORTED_LOCALES/);
  });

  it("does not let a removed id come back", () => {
    const root = fixture({ "registry.config.json": { schemaVersion: 1, locales: LOCALES, removed: ["skill:demo-skill"] } });
    assert.match(check(root).errors.join("\n"), /id was removed/);
  });

  it("rejects a contract that is malformed or belongs to another kind", () => {
    const manifest = (contracts) => ({ schemaVersion: 1, kind: "skillset", id: "demo", version: "1.0.0", target: "quickstart", contracts });
    const errors = check(fixture({ "skillsets/demo/template.json": manifest(["squad.sdlc", "Bad Id"]) })).errors.join("\n");
    assert.match(errors, /contract "squad\.sdlc" can only be provided by a squad, not a skillset/);
    assert.match(errors, /contract "Bad Id" must look like/);
  });

  it("reports an unknown requires target", () => {
    const root = fixture({
      "skillsets/demo/template.json": { schemaVersion: 1, kind: "skillset", id: "demo", version: "1.0.0", target: "quickstart", requires: [{ kind: "skillset", id: "nope" }] },
    });
    assert.match(check(root).errors.join("\n"), /requires unknown skillset "nope"/);
  });
});

function squadManifest(overrides = {}) {
  return {
    schemaVersion: 1, kind: "squad", id: "demo_squad", version: "1.0.0", collection: "main",
    squad: {
      name: "Demo", description: "A demo squad.", floorName: null, leader: "demo_lead", workflow: null,
      roles: [{ key: "demo_lead", agentTemplate: "tech_lead", name: "Demo Lead", description: "Leads.", role: "Squad leader" }],
      routines: [], charter: { mission: "m" }, skills: { sets: ["demo"] },
      ...overrides,
    },
  };
}

function squadFixture(manifest, { instructions = true } = {}) {
  const files = { "squads/demo_squad/template.json": manifest };
  for (const locale of LOCALES) files[`squads/demo_squad/i18n/${locale}.json`] = { title: `Demo ${locale}`, description: `A demo ${locale}` };
  if (instructions) files["squads/demo_squad/instructions.md"] = "You lead the demo squad.\n";
  return fixture(files);
}

describe("squad validation", () => {
  it("accepts a complete squad", () => {
    assert.deepEqual(check(squadFixture(squadManifest())).errors, []);
  });

  it("requires a known collection, a leader among the roles, and a known skillset", () => {
    const text = check(squadFixture({ ...squadManifest({ leader: "someone", skills: { sets: ["nope"] } }), collection: "nowhere" })).errors.join("\n");
    assert.match(text, /collection "nowhere" does not exist/);
    assert.match(text, /squad\.leader "someone" is not one of its roles/);
    assert.match(text, /unknown skillset "nope"/);
  });

  it("requires instructions.md for a squad that is not a Kit Viet squad", () => {
    assert.match(check(squadFixture(squadManifest(), { instructions: false })).errors.join("\n"), /instructions\.md is missing/);
  });

  it("takes a Kit Viet role's name from the pack, not the manifest", () => {
    const manifest = { ...squadManifest({ skills: undefined }), adapter: { type: "kitviet" } };
    const text = check(squadFixture(manifest, { instructions: false })).errors.join("\n");
    assert.match(text, /role demo_lead: agentTemplate comes from the Kit Viet pack/);
    assert.match(text, /role demo_lead: name comes from the Kit Viet pack/);
  });

  it("accepts skill names in extra, leader and byRole that are quickstart skills", () => {
    const skills = { sets: [], extra: ["demo-skill"], leader: ["demo-skill"], byRole: { demo_lead: ["demo-skill"] } };
    assert.deepEqual(check(squadFixture(squadManifest({ skills }))).errors, []);
  });

  it("rejects a skill name in extra, leader or byRole that is not a quickstart skill, and a byRole key that is not a role", () => {
    const skills = { sets: [], extra: ["no-such-skill"], leader: ["no-such-skill"], byRole: { ghost: ["demo-skill"], demo_lead: ["no-such-skill"] } };
    const text = check(squadFixture(squadManifest({ skills }))).errors.join("\n");
    assert.match(text, /squad\.skills\.extra names "no-such-skill", which is not a skill of a quickstart skillset/);
    assert.match(text, /squad\.skills\.leader names "no-such-skill"/);
    assert.match(text, /squad\.skills\.byRole\.demo_lead names "no-such-skill"/);
    assert.match(text, /squad\.skills\.byRole names role "ghost", which is not a role of this squad/);
  });

  it("requires a role several squads share to be defined the same way", () => {
    const other = squadManifest();
    other.id = "other_squad";
    other.squad.roles = [{ ...other.squad.roles[0], description: "Differently." }];
    const root = squadFixture(squadManifest());
    write(root, "squads/other_squad/template.json", other);
    write(root, "squads/other_squad/instructions.md", "x\n");
    for (const locale of LOCALES) write(root, `squads/other_squad/i18n/${locale}.json`, { title: "t", description: "d" });
    assert.match(check(root).errors.join("\n"), /role demo_lead is defined differently in/);
  });
});

// A domain with a README, one primary and one expansion market, and a skill that belongs to it.
function domainFixture(domainManifest, skillManifest, overrides = {}) {
  return fixture({
    "domains/demo/template.json": domainManifest,
    "domains/demo/README.md": "# Demo\n",
    "skills/g/demo-skill/template.json": skillManifest,
    ...overrides,
  });
}
const DOMAIN = {
  schemaVersion: 1, kind: "domain", id: "demo", version: "1.0.0",
  markets: [{ id: "vn", role: "primary" }, { id: "intl", role: "expansion" }],
  related: [],
};
const SKILL_IN_DOMAIN = {
  schemaVersion: 1, kind: "skill", id: "demo-skill", version: "1.0.0", sets: ["demo"], source: { type: "authored" },
  domain: "demo", markets: ["vn"],
};

describe("domain validation", () => {
  it("accepts a domain with a README, and members that name it and its markets", () => {
    assert.deepEqual(check(domainFixture(DOMAIN, SKILL_IN_DOMAIN)).errors, []);
  });

  it("requires a README.md, exactly one primary market, and related targets that exist", () => {
    const noReadme = check(domainFixture(DOMAIN, SKILL_IN_DOMAIN, { "domains/demo/README.md": null })).errors.join("\n");
    assert.match(noReadme, /README\.md is missing/);
    const twoPrimary = check(domainFixture({ ...DOMAIN, markets: [{ id: "vn", role: "primary" }, { id: "intl", role: "primary" }] }, SKILL_IN_DOMAIN)).errors.join("\n");
    assert.match(twoPrimary, /exactly one market is primary, got 2/);
    const badRelated = check(domainFixture({ ...DOMAIN, related: [{ kind: "squad", id: "nowhere" }] }, SKILL_IN_DOMAIN)).errors.join("\n");
    assert.match(badRelated, /related names squad "nowhere", which does not exist/);
  });

  it("refuses a member whose domain does not exist, or a market the domain does not list", () => {
    const unknown = check(domainFixture(DOMAIN, { ...SKILL_IN_DOMAIN, domain: "ghost" })).errors.join("\n");
    assert.match(unknown, /domain "ghost" does not exist/);
    const market = check(domainFixture(DOMAIN, { ...SKILL_IN_DOMAIN, markets: ["zz"] })).errors.join("\n");
    assert.match(market, /market "zz" is not a market of domain "demo"/);
    const orphan = check(domainFixture(DOMAIN, { ...SKILL_IN_DOMAIN, domain: undefined, markets: ["vn"] })).errors.join("\n");
    assert.match(orphan, /markets needs a domain/);
  });

  it("requires a squad of a domain to sit in the collection of the same id", () => {
    const manifest = squadManifest();
    manifest.domain = "demo";
    manifest.markets = ["vn"];
    const root = squadFixture(manifest);
    write(root, "domains/demo/template.json", DOMAIN);
    write(root, "domains/demo/README.md", "# Demo\n");
    assert.match(check(root).errors.join("\n"), /must sit in collection "demo", not "main"/);
  });
});

// A pack that names one skill of the catalog and holds no copy of it (E5).
function packFixture(skills, overrides = {}) {
  const files = {
    "packs/demo-pack/template.json": { schemaVersion: 1, kind: "pack", id: "demo-pack", version: "1.0.0", source: { type: "authored" } },
    "packs/demo-pack/pack.json": { agent: { name: "Demo Pack", description: "d", instructions_file: "instructions.md" }, skills },
    "packs/demo-pack/instructions.md": "x\n",
    ...overrides,
  };
  for (const locale of LOCALES) files[`packs/demo-pack/i18n/${locale}.json`] = { title: "Demo Pack", description: "d" };
  return fixture(files);
}

describe("pack skills", () => {
  it("resolves a skill the pack names to the catalog copy, and builds the same shape", () => {
    const root = packFixture(["demo-skill"]);
    assert.deepEqual(check(root).errors, []);
    const { entries } = discover(root);
    const bundle = buildPackBundle(join(root, "packs/demo-pack"), root, catalogSkillDirsOf(entries));
    assert.equal(bundle.skills[0].name, "demo-skill");
    const paths = bundle.skills[0].files.map((f) => f.path);
    assert.deepEqual(paths, ["references/notes.md", "SKILL.md"]);
  });

  it("refuses a skill that is in both the pack and the catalog, and one that is in neither", () => {
    const both = packFixture(["demo-skill"], { "packs/demo-pack/skills/demo-skill/SKILL.md": "---\nname: demo-skill\n---\n" });
    assert.match(check(both).errors.join("\n"), /skill demo-skill is in both packs\/demo-pack\/skills and the catalog/);
    const neither = packFixture(["ghost-skill"]);
    assert.match(check(neither).errors.join("\n"), /skill ghost-skill is in neither/);
  });
});

describe("map", () => {
  // A domain with a normal squad, a Kit Viet squad (its skills come from _sources), a skill and a pack.
  function mapFixture() {
    return domainFixture(
      { ...DOMAIN, related: [{ kind: "squad", id: "demo_squad", note: "outside" }] },
      SKILL_IN_DOMAIN,
      {
        "squads/demo_squad/template.json": { ...squadManifest({ skills: { sets: ["demo"], extra: ["demo-skill"] } }), collection: "demo", domain: "demo", markets: ["vn"] },
        "squads/demo_squad/instructions.md": "x\n",
        "squads/demo_squad/i18n/en.json": { title: "t", description: "d" },
        "_sources/kitviet/squads/kv.json": { templateId: "kv_demo", builtinSkills: ["shared-one"], leaderSkills: ["lead-one"], roleSkills: { kv_role: ["role-one"] } },
        "packs/demo-pack/template.json": { schemaVersion: 1, kind: "pack", id: "demo-pack", version: "1.0.0", domain: "demo", source: { type: "authored" } },
        "packs/demo-pack/pack.json": { agent: { name: "Demo Pack", description: "d", instructions_file: "instructions.md" }, skills: ["demo-skill"] },
        "packs/demo-pack/instructions.md": "x\n",
      },
    );
  }

  it("lists the squads, skills, packs and related templates of one domain, as data", () => {
    const root = mapFixture();
    const { entries } = discover(root);
    const map = buildMap(entries, "demo", root);
    assert.deepEqual(map.domain.markets, DOMAIN.markets);
    assert.deepEqual(map.squads.map((s) => [s.id, s.collection, s.markets, s.skills, s.kitviet]), [["demo_squad", "demo", ["vn"], 1, false]]);
    assert.deepEqual(map.skills.groups, { g: [{ id: "demo-skill", markets: ["vn"], source: "authored" }] });
    assert.deepEqual(map.packs, [{ id: "demo-pack", name: "Demo Pack", skills: ["demo-skill"], markets: [] }]);
    assert.deepEqual(map.related, [{ kind: "squad", id: "demo_squad", note: "outside" }]);
    assert.deepEqual(map.warnings, []);
  });

  it("names the Kit Viet skills of a Kit Viet squad with the kv: prefix, read from its source", () => {
    const root = mapFixture();
    const kv = { ...squadManifest(), id: "kv_demo", collection: "kitviet", domain: "demo", markets: ["vn"], adapter: { type: "kitviet" } };
    kv.squad = { ...kv.squad, name: "KV Demo", leader: "kv_role", roles: [{ key: "kv_role", description: "d", role: "r" }] };
    delete kv.squad.skills;
    write(root, "squads/kv_demo/template.json", kv);
    for (const locale of LOCALES) write(root, `squads/kv_demo/i18n/${locale}.json`, { title: "t", description: "d" });
    const { entries } = discover(root);
    const row = buildMap(entries, "demo", root).squads.find((s) => s.id === "kv_demo");
    assert.equal(row.kitviet, true);
    assert.equal(row.skills, 3);
  });

  it("refuses an unknown domain and warns about a skill under the domain's folder that names no domain", () => {
    const root = mapFixture();
    const { entries } = discover(root);
    assert.throws(() => buildMap(entries, "ghost", root), /no domain "ghost"/);
    const orphaned = fixture({ "domains/demo/template.json": DOMAIN, "domains/demo/README.md": "# Demo\n" });
    write(orphaned, "skills/demo/loose-skill/SKILL.md", '---\nname: loose-skill\ndescription: "Use when testing."\n---\n\n# Loose\n');
    write(orphaned, "skills/demo/loose-skill/template.json", { schemaVersion: 1, kind: "skill", id: "loose-skill", version: "1.0.0", sets: ["demo"], source: { type: "authored" } });
    const found = discover(orphaned).entries;
    assert.match(buildMap(found, "demo", orphaned).warnings.join("\n"), /skill loose-skill sits under skills\/demo\/ but has no "domain"/);
  });
});

describe("squad includes", () => {
  // The demo squad (in domain demo, collection demo) and one other squad of the domain, both valid.
  function setFixture(includes, partOverrides = {}) {
    const root = squadFixture({ ...squadManifest({ skills: { sets: [] } }), collection: "demo", domain: "demo", markets: ["vn"], includes });
    write(root, "domains/demo/template.json", DOMAIN);
    write(root, "domains/demo/README.md", "# Demo\n");
    write(root, "collections/demo/template.json", { schemaVersion: 1, kind: "collection", id: "demo", version: "1.0.0", for: "squad" });
    for (const locale of LOCALES) write(root, `collections/demo/i18n/${locale}.json`, { title: "Demo" });
    const part = {
      ...squadManifest({ leader: "part_lead", roles: [{ key: "part_lead", agentTemplate: "tech_lead", name: "Part Lead", description: "Leads.", role: "Squad leader" }], skills: { sets: [] } }),
      id: "part_squad", collection: "demo", domain: "demo", markets: ["vn"], ...partOverrides,
    };
    write(root, "squads/part_squad/template.json", part);
    write(root, "squads/part_squad/instructions.md", "x\n");
    for (const locale of LOCALES) write(root, `squads/part_squad/i18n/${locale}.json`, { title: "Part", description: "d" });
    return root;
  }

  it("accepts a set of squads in one domain, one level deep", () => {
    assert.deepEqual(check(setFixture(["part_squad"])).errors, []);
  });

  it("refuses an id that does not exist, a squad of another domain, a repeat and the squad itself", () => {
    assert.match(check(setFixture(["ghost"])).errors.join("\n"), /includes names squad "ghost", which does not exist/);
    assert.match(check(setFixture(["part_squad"], { domain: "other" })).errors.join("\n"), /includes "part_squad", which is not in domain "demo"/);
    assert.match(check(setFixture(["part_squad", "part_squad"])).errors.join("\n"), /includes lists a squad twice/);
    assert.match(check(setFixture(["demo_squad"])).errors.join("\n"), /includes names the squad itself/);
  });

  it("refuses a squad that is itself a set: a set is one level deep", () => {
    const root = setFixture(["part_squad"]);
    const part = JSON.parse(readFileSync(join(root, "squads/part_squad/template.json"), "utf8"));
    write(root, "squads/part_squad/template.json", { ...part, includes: ["demo_squad"] });
    assert.match(check(root).errors.join("\n"), /includes "part_squad", which includes squads of its own; a set is one level deep/);
  });

  it("needs a domain to include anything", () => {
    const root = setFixture(["part_squad"]);
    const demo = JSON.parse(readFileSync(join(root, "squads/demo_squad/template.json"), "utf8"));
    delete demo.domain;
    write(root, "squads/demo_squad/template.json", demo);
    assert.match(check(root).errors.join("\n"), /includes needs a domain/);
  });
});

describe("designer workflow validation", () => {
  const designer = (steps, overrides = {}) => ({
    "workflows/designer_demo/template.json": { schemaVersion: 1, kind: "workflow", id: "designer_demo", version: "1.0.0", ui: false, designer: { templateId: "demo", steps }, ...overrides },
  });

  it("accepts a template whose transitions name its own columns", () => {
    const steps = [{ statusKey: "todo", transitions: [{ to: "qa" }] }, { new: { name: "QA", category: "started" }, transitions: [{ on: "rejected", to: "todo" }, { to: "done" }] }, { statusKey: "done" }];
    assert.deepEqual(check(fixture(designer(steps))).errors, []);
  });

  it("requires the id to be designer_ plus the template id", () => {
    const errors = check(fixture({ "workflows/other/template.json": { schemaVersion: 1, kind: "workflow", id: "other", version: "1.0.0", ui: false, designer: { templateId: "demo", steps: [{ statusKey: "todo" }] } } })).errors.join("\n");
    assert.match(errors, /id must be "designer_demo"/);
  });

  it("rejects an unknown target, a bad outcome and a duplicate column", () => {
    const steps = [{ statusKey: "todo", transitions: [{ to: "nowhere" }, { on: "maybe", to: "todo" }] }, { statusKey: "todo" }];
    const errors = check(fixture(designer(steps))).errors.join("\n");
    assert.match(errors, /to "nowhere" is not a column/);
    assert.match(errors, /on must be "rejected" or "blocked"/);
    assert.match(errors, /status todo is listed twice/);
  });

  it("is not a workflow a squad can run", () => {
    const root = squadFixture(squadManifest({ workflow: "designer_demo" }));
    write(root, "workflows/designer_demo/template.json", designer([{ statusKey: "todo" }])["workflows/designer_demo/template.json"]);
    assert.match(check(root).errors.join("\n"), /is not an executable workflow/);
  });
});

describe("flow workflow validation", () => {
  const roles = [
    { key: "demo_lead", agentTemplate: "tech_lead", name: "Demo Lead", description: "Leads.", role: "Squad leader" },
    { key: "demo_reader", agentTemplate: "tech_lead", name: "Demo Reader", description: "Reads.", role: "Reads the input" },
    { key: "demo_gate", agentTemplate: "tech_lead", name: "Demo Gate", description: "Checks.", role: "Checks the draft" },
  ];
  const t = (id, label, when, to, extra = {}) => ({ id, label, when, to, ...extra });
  const tasksOutcome = (value) => ({ field: "tasks_outcome", op: "eq", value });
  const validFlow = () => ({
    squad: "demo_squad",
    config: {
      version: 1,
      switches: [{ key: "lookup", type: "bool", default: false }],
      messages: { stop1: "BLOCKED · stop 1\nMissing input.", stop3: "BLOCKED · stop 3\nNo more rounds." },
    },
    steps: [
      { status: { key: "todo" }, statusKey: "todo", transitions: [t("plan_blocked", "Blocked", { field: "outcome", op: "eq", value: "blocked" }, "blocked"), t("plan_ok", "Plan done", null, "collect")] },
      {
        status: { name: "Collect", category: "started" }, statusKey: "collect",
        tasks: [
          { key: "read", title: "Read the input", kind: "work", role: "demo_reader", instructions: "Read it." },
          { key: "extra", title: "Look it up", kind: "work", role: "demo_reader", instructions: "Look.", when: { field: "switch.lookup", op: "eq", value: true } },
        ],
        transitions: [t("collect_blocked", "Blocked", tasksOutcome("any_blocked"), "blocked"), t("collect_next", "Collected", null, "gate")],
      },
      {
        status: { name: "Gate", category: "started" }, statusKey: "gate",
        tasks: [{ key: "check", title: "Check the draft", kind: "gate", role: "demo_gate", instructions: "Check it.", requires: ["outcome"] }],
        transitions: [
          t("gate_blocked", "Blocked", tasksOutcome("any_blocked"), "blocked"),
          t("gate_ok", "Approved", tasksOutcome("all_approved"), "in_review"),
          t("gate_rework", "Back for a fix", { all: [tasksOutcome("any_rejected"), { field: "rework_count", op: "lt", value: 2 }] }, "collect", { rework: true }),
          t("gate_stop", "Out of rounds", null, "blocked"),
        ],
      },
      { status: { key: "in_review" }, statusKey: "in_review", humanOnly: true },
      { status: { key: "blocked" }, statusKey: "blocked" },
    ],
  });
  const manifest = (flow, overrides = {}) => ({ schemaVersion: 1, kind: "workflow", id: "flow_demo_squad", version: "1.0.0", ui: false, flow, ...overrides });
  const withFlow = (flow, overrides) => {
    const root = squadFixture(squadManifest({ roles }));
    write(root, "workflows/flow_demo_squad/template.json", manifest(flow, overrides));
    return root;
  };
  const errorsOf = (flow, overrides) => check(withFlow(flow, overrides)).errors.join("\n");

  it("accepts a complete flow", () => {
    assert.deepEqual(check(withFlow(validFlow())).errors, []);
  });

  it("names the squad it belongs to, and the id says so", () => {
    assert.match(errorsOf({ ...validFlow(), squad: "nowhere" }), /flow\.squad "nowhere" is not a squad/);
    assert.match(errorsOf(validFlow(), { id: "flow_other" }), /id must be "flow_demo_squad"/);
    assert.match(errorsOf(validFlow(), { ui: undefined }), /flow workflows set ui to false/);
  });

  it("takes task roles from the squad, never the leader, and uses every other role", () => {
    const stranger = validFlow();
    stranger.steps[1].tasks[0].role = "ghost";
    assert.match(errorsOf(stranger), /task read: role "ghost" is not a role of squad demo_squad/);
    const lead = validFlow();
    lead.steps[1].tasks.forEach((task) => { task.role = "demo_lead"; });
    const leadErrors = errorsOf(lead);
    assert.match(leadErrors, /task read: role "demo_lead" is the squad leader/);
    assert.match(leadErrors, /role demo_reader is never used by a task/);
  });

  it("allows one flow per squad", () => {
    const root = withFlow(validFlow());
    write(root, "workflows/flow_other/template.json", manifest(validFlow(), { id: "flow_other" }));
    assert.match(check(root).errors.join("\n"), /squad demo_squad already has a flow/);
  });

  it("is a flow, a workflow or a designer template, not two of them", () => {
    const both = errorsOf(validFlow(), { workflow: { name: "x", description: "y", steps: [{ status: { key: "todo" }, statusKey: "todo", role: null, instructions: "" }] } });
    assert.match(both, /either a workflow, a designer or a flow block/);
    const designer = errorsOf(validFlow(), { designer: { templateId: "demo", steps: [{ statusKey: "todo" }] } });
    assert.match(designer, /either a workflow, a designer or a flow block/);
  });

  it("rejects a cycle that has no limited rework transition", () => {
    const loop = validFlow();
    loop.steps[2].transitions[2] = t("gate_rework", "Back for a fix", tasksOutcome("any_rejected"), "collect");
    assert.match(errorsOf(loop), /form a cycle without a rework transition/);
    const unlimited = validFlow();
    unlimited.steps[2].transitions[2] = t("gate_rework", "Back for a fix", tasksOutcome("any_rejected"), "collect", { rework: true });
    assert.match(errorsOf(unlimited), /rework transition gate_rework needs a condition rework_count lt N/);
    const forward = validFlow();
    forward.steps[1].transitions[0] = t("collect_blocked", "Blocked", { field: "rework_count", op: "lt", value: 2 }, "gate", { rework: true });
    assert.match(errorsOf(forward), /rework transition collect_blocked must go back to an earlier step/);
  });

  it("requires a gate to ask for an outcome and to be routed", () => {
    const noOutcome = validFlow();
    noOutcome.steps[2].tasks[0].requires = [];
    assert.match(errorsOf(noOutcome), /gate task check must require an outcome/);
    const unrouted = validFlow();
    unrouted.steps[2].transitions = [t("gate_stop", "Out", null, "blocked")];
    assert.match(errorsOf(unrouted), /step gate has gates and needs a transition for tasks_outcome all_approved/);
  });

  it("requires every switch a condition reads to be declared", () => {
    const flow = validFlow();
    flow.steps[1].tasks[1].when = { field: "switch.unknown", op: "eq", value: true };
    assert.match(errorsOf(flow), /switch "unknown" is not declared in flow\.config/);
    const wrongType = validFlow();
    wrongType.steps[1].tasks[1].when = { field: "switch.lookup", op: "eq", value: "yes" };
    assert.match(errorsOf(wrongType), /switch "lookup" is a bool/);
  });

  it("checks steps, tasks and transitions the way the server does", () => {
    const flow = validFlow();
    flow.steps[0].tasks = [{ key: "x", title: "x", kind: "work", role: "demo_reader", instructions: "" }];
    flow.steps[1].transitions[1].to = "nowhere";
    flow.steps[1].tasks[1].key = "read";
    flow.steps[4].statusKey = "stuck";
    const text = errorsOf(flow);
    assert.match(text, /the plan step todo cannot have tasks/);
    assert.match(text, /to "nowhere" is not a status of the flow/);
    assert.match(text, /task read is listed twice/);
    assert.match(text, /statusKey "stuck" must be "blocked"/);
  });

  it("is not a workflow a squad can run", () => {
    const root = squadFixture(squadManifest({ roles, workflow: "flow_demo_squad" }));
    write(root, "workflows/flow_demo_squad/template.json", manifest(validFlow()));
    assert.match(check(root).errors.join("\n"), /is not an executable workflow/);
  });

  it("gives a revision that ignores key order and follows the content (A18)", () => {
    const flow = validFlow();
    const reordered = JSON.parse(JSON.stringify(flow, (_, v) => (v && typeof v === "object" && !Array.isArray(v) ? Object.fromEntries(Object.entries(v).reverse()) : v)));
    const changed = validFlow();
    changed.steps[1].tasks[0].instructions = "Read it twice.";
    assert.match(flowRev(flow), /^[0-9a-f]{8}$/);
    assert.equal(flowRev(reordered), flowRev(flow));
    assert.notEqual(flowRev(changed), flowRev(flow));
  });

  it("builds the flow keyed by its manifest id, with its revision", () => {
    const { entries } = check(withFlow(validFlow()));
    const flows = flowsById(entries);
    assert.deepEqual(Object.keys(flows), ["flow_demo_squad"]);
    assert.equal(flows.flow_demo_squad.squad, "demo_squad");
    assert.equal(flows.flow_demo_squad.rev, flowRev(validFlow()));
    assert.equal(flows.flow_demo_squad.steps.length, 5);
    assert.match(renderFlows(entries), /export const CATALOG_FLOWS: Readonly<Record<string, CatalogFlow>> = /);
    assert.deepEqual(flowsById(check(fixture()).entries), {});
  });
});

describe("skill bundle", () => {
  it("bundles the skills of a quickstart skillset without the catalog's own files", () => {
    const root = fixture({ "skills/g/demo-skill/i18n/en.json": { x: 1 } });
    const { entries } = check(root);
    const text = renderSkillsBuiltin(entries);
    assert.match(text, /"name": "demo-skill"/);
    assert.match(text, /"path": "references\/notes.md"/);
    assert.doesNotMatch(text, /"path": "template\.json"/);
    assert.doesNotMatch(text, /"path": "i18n\/en\.json"/);
    assert.deepEqual(skillsetMembers(entries), { demo: ["demo-skill"] });
  });
});

describe("chunked skillset", () => {
  // The demo skillset becomes a chunk: its skill leaves the SDLC bundle and gets a module of its own.
  const chunked = () => fixture({ "skillsets/demo/template.json": { schemaVersion: 1, kind: "skillset", id: "demo", version: "1.0.0", target: "quickstart", chunk: true } });

  it("keeps a chunk skill out of the main bundle and writes it into its own module", () => {
    const { entries } = discover(chunked());
    assert.doesNotMatch(renderSkillsBuiltin(entries), /"name": "demo-skill"/);
    const module = renderSkillsChunk(entries, "demo");
    assert.match(module, /export const DEMO_BUILTIN_SKILLS: readonly BuiltinSkill\[\] = \[/);
    assert.match(module, /"name": "demo-skill"/);
    assert.match(renderSkillsets(entries), /"demo": async \(\) => \(await import\("\.\/skills-builtin-demo"\)\)\.DEMO_BUILTIN_SKILLS,/);
  });

  it("keeps a skill of the main bundle in the bundle when no skillset is a chunk", () => {
    const { entries } = discover(fixture());
    assert.match(renderSkillsBuiltin(entries), /"name": "demo-skill"/);
    assert.match(renderSkillsets(entries), /SKILLSET_CHUNK_LOADERS: Readonly<Record<string, \(\) => Promise<readonly BuiltinSkill\[\]>>> = \{\};/);
  });

  it("requires chunk to be a boolean on a quickstart skillset, and refuses a chunk on a starter skillset", () => {
    const notBool = check(fixture({ "skillsets/demo/template.json": { schemaVersion: 1, kind: "skillset", id: "demo", version: "1.0.0", target: "quickstart", chunk: "yes" } })).errors.join("\n");
    assert.match(notBool, /chunk must be a boolean/);
    const starter = check(fixture({ "skillsets/demo/template.json": { schemaVersion: 1, kind: "skillset", id: "demo", version: "1.0.0", target: "starter", chunk: true } })).errors.join("\n");
    assert.match(starter, /a chunk skillset must have target "quickstart"/);
  });

  it("refuses a skill of a chunk skillset that also names another quickstart skillset", () => {
    const root = chunked();
    write(root, "skillsets/other/template.json", { schemaVersion: 1, kind: "skillset", id: "other", version: "1.0.0", target: "quickstart" });
    write(root, "skills/g/demo-skill/template.json", { schemaVersion: 1, kind: "skill", id: "demo-skill", version: "1.0.0", sets: ["demo", "other"], source: { type: "authored" } });
    assert.match(check(root).errors.join("\n"), /must name no other quickstart skillset/);
  });

  it("allows a skill of a chunk skillset to also belong to a starter skillset", () => {
    const root = chunked();
    write(root, "skillsets/starter_demo/template.json", { schemaVersion: 1, kind: "skillset", id: "starter_demo", version: "1.0.0", target: "starter" });
    write(root, "skills/g/demo-skill/template.json", { schemaVersion: 1, kind: "skill", id: "demo-skill", version: "1.0.0", sets: ["demo", "starter_demo"], source: { type: "authored" } });
    assert.deepEqual(check(root).errors, []);
  });
});

describe("build lock", () => {
  const lockModule = new URL("./lib/lock.mjs", import.meta.url).href;

  it("lets only one process into the build at a time", async () => {
    const root = mkdtempSync(join(tmpdir(), "catalog-lock-"));
    roots.push(root);
    const lockDir = join(root, "build.lock");
    const marker = join(root, "inside");
    // Each child fails when it finds another child's marker inside the critical section.
    const script = `
      import { existsSync, rmSync, writeFileSync } from "node:fs";
      import { withBuildLock } from ${JSON.stringify(lockModule)};
      withBuildLock(${JSON.stringify(lockDir)}, () => {
        if (existsSync(${JSON.stringify(marker)})) throw new Error("two builds ran at once");
        writeFileSync(${JSON.stringify(marker)}, String(process.pid));
        Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 80);
        rmSync(${JSON.stringify(marker)});
      });
    `;
    const codes = await Promise.all(
      Array.from({ length: 8 }, () => new Promise((resolve) => {
        const child = spawn(process.execPath, ["--input-type=module", "-e", script], { stdio: "ignore" });
        child.on("exit", resolve);
      })),
    );
    assert.deepEqual(codes, Array(8).fill(0));
    assert.equal(existsSync(lockDir), false, "the lock is released");
  });

  it("takes over a lock whose owner is gone and releases it when fn throws", () => {
    const root = mkdtempSync(join(tmpdir(), "catalog-lock-"));
    roots.push(root);
    const lockDir = join(root, "build.lock");
    mkdirSync(lockDir);
    writeFileSync(join(lockDir, "pid"), "2147483646");
    assert.equal(withBuildLock(lockDir, () => "ran"), "ran");
    assert.throws(() => withBuildLock(lockDir, () => { throw new Error("boom"); }), /boom/);
    assert.equal(existsSync(lockDir), false);
  });

  it("gives up with a clear error when a live build holds the lock", () => {
    const root = mkdtempSync(join(tmpdir(), "catalog-lock-"));
    roots.push(root);
    const lockDir = join(root, "build.lock");
    mkdirSync(lockDir);
    writeFileSync(join(lockDir, "pid"), String(process.pid));
    assert.throws(() => withBuildLock(lockDir, () => {}, { timeoutMs: 150 }), /held by another build/);
    assert.equal(existsSync(lockDir), true, "a live owner keeps its lock");
  });
});

describe("knowledge work plugins (KWP) skills", () => {
  it("enforces Delegora runtime rules and forbids interactive or external steps in authored KWP skills", () => {
    const { entries } = load();
    const kwpAuthored = entries.filter((e) => e.kind === "skill" && e.manifest.id.startsWith("kwp-") && e.manifest.source?.type === "authored");
    assert.equal(kwpAuthored.length, 15, "all 15 authored KWP skills must be present");

    const forbidden = [
      "~~",
      "$ARGUMENTS",
      "@$1",
      "argument-hint",
      "Ask the user",
      "ask the user",
      "send for signature",
      "DocuSign",
      "AskUserQuestion",
    ];

    for (const entry of kwpAuthored) {
      const content = readFileSync(join(entry.dir, "SKILL.md"), "utf8");
      assert.match(
        content,
        /^source:\s*"derived from .+ at e605d8dd \(Apache-2\.0\)"/m,
        `${entry.manifest.id} must declare derived source in frontmatter`,
      );
      assert.match(
        content,
        /^description:\s*"Use when you are /m,
        `${entry.manifest.id} description must start with "Use when you are "`,
      );
      assert.ok(
        content.includes("## Delegora runtime rules"),
        `${entry.manifest.id} must include '## Delegora runtime rules'`,
      );
      for (const phrase of forbidden) {
        assert.ok(
          !content.includes(phrase),
          `${entry.manifest.id} must not contain forbidden phrase "${phrase}"`,
        );
      }
    }
  });

  it("checks KWP skillset memberships: all 26 in kwp and exact 14 starter skills in kwp_company", () => {
    const { entries } = load();
    const skills = entries.filter((e) => e.kind === "skill");
    const kwpMembers = skills.filter((e) => e.manifest.sets?.includes("kwp")).map((e) => e.manifest.id).sort();
    assert.equal(kwpMembers.length, 26, "kwp skillset must hold all 26 KWP skills");

    const expectedCompanySkills = [
      "kwp-absent-is-not-zero",
      "kwp-change-request",
      "kwp-customer-escalation",
      "kwp-incident-response",
      "kwp-kb-article",
      "kwp-knowledge-synthesis",
      "kwp-runbook",
      "kwp-search-strategy",
      "kwp-synthesize-research",
      "kwp-ticket-triage",
      "kwp-untrusted-content",
      "risk-assessment",
      "user-research",
      "validate-data",
    ];
    const companyMembers = skills
      .filter((e) => e.manifest.sets?.includes("kwp_company"))
      .map((e) => e.manifest.id)
      .sort();
    assert.deepEqual(companyMembers, expectedCompanySkills);

    const company = entries.find((e) => e.kind === "company" && e.manifest.id === "starter");
    assert.ok(company, "starter company exists");
    const roleAssigned = [
      ...new Set(
        company.manifest.company.squads.flatMap((squad) =>
          squad.roles.flatMap((role) => role.skills ?? []),
        ),
      ),
    ].sort();
    assert.deepEqual(roleAssigned, expectedCompanySkills);
  });

  it("checks group A vendored KWP skills have no forbidden interactive or external steps", () => {
    const { entries } = load();
    const groupAIds = [
      "analyze",
      "explore-data",
      "sql-queries",
      "statistical-analysis",
      "validate-data",
      "create-viz",
      "build-dashboard",
      "tech-debt",
      "risk-assessment",
      "variance-analysis",
      "user-research",
    ];
    const groupA = entries.filter((e) => e.kind === "skill" && groupAIds.includes(e.manifest.id) && e.manifest.sets?.includes("kwp"));
    assert.equal(groupA.length, 11, "all 11 group A KWP skills must be present");

    const forbidden = [
      "~~",
      "$ARGUMENTS",
      "@$1",
      "Ask the user",
      "ask the user",
      "send for signature",
      "DocuSign",
      "AskUserQuestion",
    ];

    for (const entry of groupA) {
      const content = readFileSync(join(entry.dir, "SKILL.md"), "utf8");
      for (const phrase of forbidden) {
        assert.ok(
          !content.includes(phrase),
          `group A skill ${entry.manifest.id} must not contain forbidden phrase "${phrase}"`,
        );
      }
    }
  });

  it("ensures every skill named on a starter company role exists in the catalog", () => {
    const { entries } = load();
    const skillIds = new Set(entries.filter((e) => e.kind === "skill").map((e) => e.manifest.id));
    const company = entries.find((e) => e.kind === "company" && e.manifest.id === "starter");
    assert.ok(company, "starter company exists");

    for (const squad of company.manifest.company.squads) {
      for (const role of squad.roles) {
        if (role.skills) {
          for (const skill of role.skills) {
            assert.ok(skillIds.has(skill), `role ${squad.key}/${role.key} names unknown skill "${skill}"`);
          }
        }
      }
    }
  });
});

describe("build output", () => {
  const mtimes = (dir) => {
    const out = {};
    const walk = (d) => {
      for (const entry of readdirSync(d, { withFileTypes: true })) {
        const path = join(d, entry.name);
        if (entry.isDirectory()) walk(path);
        else out[path] = statSync(path).mtimeMs;
      }
    };
    walk(dir);
    return out;
  };

  it("rewrites nothing on a second build and removes only the files no template produces", () => {
    const root = mkdtempSync(join(tmpdir(), "catalog-out-"));
    roots.push(root);
    const tsOutDir = join(root, "ts");
    const goOutDir = join(root, "go");
    const { config, entries } = load();
    const first = buildAll({ entries, config, target: "all", tsOutDir, goOutDir });
    assert.ok(first > 0);

    // A watcher or compiler reads these folders while a build runs: an unchanged file must not be touched.
    const before = { ...mtimes(tsOutDir), ...mtimes(goOutDir) };
    mkdirSync(join(goOutDir), { recursive: true });
    writeFileSync(join(goOutDir, ".gitkeep"), "");
    writeFileSync(join(goOutDir, "gone.json"), "{}");
    writeFileSync(join(tsOutDir, "agent-packs", "gone.json"), "{}");
    writeFileSync(join(tsOutDir, "kitviet", "gone.ts"), "export {};\n");
    writeFileSync(join(tsOutDir, "gone.ts"), "export {};\n");
    const second = buildAll({ entries, config, target: "all", tsOutDir, goOutDir });

    assert.equal(second, 0, "a build with no catalog change writes no file");
    const after = { ...mtimes(tsOutDir), ...mtimes(goOutDir) };
    for (const [path, time] of Object.entries(before)) assert.equal(after[path], time, `${path} was rewritten`);
    for (const stale of [join(goOutDir, "gone.json"), join(tsOutDir, "agent-packs", "gone.json"), join(tsOutDir, "kitviet", "gone.ts"), join(tsOutDir, "gone.ts")]) {
      assert.equal(existsSync(stale), false, `${stale} should be removed`);
    }
    assert.equal(existsSync(join(goOutDir, ".gitkeep")), true, ".gitkeep stays");
    assert.deepEqual(Object.keys(after).filter((p) => p.endsWith(".tmp")), [], "no temp file is left behind");
  });
});
