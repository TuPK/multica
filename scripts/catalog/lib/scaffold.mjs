// `catalog new`: the smallest valid template of a kind, so a new one starts green.
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { KIND_DIRS } from "./discover.mjs";
import { catalogDir } from "./paths.mjs";
import { readRegistryConfig } from "./discover.mjs";

const write = (path, text, created) => {
  mkdirSync(join(path, ".."), { recursive: true });
  writeFileSync(path, text);
  created.push(path);
};

export function scaffold({ kind, id, collection, set }) {
  if (!KIND_DIRS[kind]) throw new Error(`unknown kind ${kind}; one of ${Object.keys(KIND_DIRS).join(", ")}`);
  const folder = kind === "skill" ? id.replaceAll(":", "--") : id;
  const dir = kind === "skill" ? join(catalogDir, KIND_DIRS.skill, "custom", folder) : join(catalogDir, KIND_DIRS[kind], folder);
  if (existsSync(dir)) throw new Error(`${dir} already exists`);
  const created = [];
  const manifest = { schemaVersion: 1, kind, id, version: "1.0.0" };
  if (kind === "skill") {
    manifest.sets = [set ?? "sdlc"];
    manifest.source = { type: "authored" };
    write(join(dir, "SKILL.md"), `---\nname: ${id.split(":").pop()}\ndescription: "TODO: say when to use this skill."\n---\n\n# ${id}\n`, created);
  }
  if (kind === "skillset") manifest.target = "quickstart";
  if (kind === "collection") manifest.for = "squad";
  if (kind === "squad") {
    const title = id.split("_").map((w) => w[0].toUpperCase() + w.slice(1)).join(" ");
    manifest.collection = collection ?? "core";
    manifest.squad = {
      name: title,
      description: "TODO: one line on what this squad does.",
      floorName: null,
      leader: `${id}_lead`,
      workflow: null,
      roles: [{ key: `${id}_lead`, agentTemplate: "tech_lead", name: `${title} Lead`, description: "TODO: what the lead does.", role: "Squad leader" }],
      routines: [],
      charter: {
        mission: "TODO: the squad's mission.",
        responsibilities: [{ key: "main", text: "TODO: what the squad is responsible for." }],
        outputs: [{ key: "result", kind: "issue", cadenceHint: "per request" }],
        boundaries: "TODO: what the squad never does.",
        escalation: "TODO: who it asks when blocked.",
        intake: "TODO: how requests reach it.",
      },
      skills: { sets: [] },
    };
    write(join(dir, "instructions.md"), `You lead the ${title} squad. TODO: say how the lead works.\n`, created);
  }
  write(join(dir, "template.json"), `${JSON.stringify(manifest, null, 2)}\n`, created);
  if (["collection", "squad", "pack", "workflow"].includes(kind)) {
    for (const locale of readRegistryConfig().locales) {
      write(join(dir, "i18n", `${locale}.json`), `${JSON.stringify({ title: "TODO", description: "TODO" }, null, 2)}\n`, created);
    }
  }
  return created;
}
