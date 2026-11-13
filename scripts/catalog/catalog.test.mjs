// Tests for the catalog CLI: discovery, validation (every layer that has a rule in the spec) and the
// generated skill bundle. Run: node --test scripts/catalog/*.test.mjs
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { after, describe, it } from "node:test";
import { discover, readRegistryConfig } from "./lib/discover.mjs";
import { renderSkillsBuiltin, skillsetMembers } from "./lib/build-skills.mjs";
import { flowRev, flowsById, renderFlows } from "./lib/build-workflows.mjs";
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
