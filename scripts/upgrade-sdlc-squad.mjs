#!/usr/bin/env node
// Upgrades an SDLC squad the quickstart made earlier to the current template: the built-in
// skills, the agents' and the squad's instructions, and the SDLC Auto workflow (the Environment
// step and the new briefs). A text is replaced only when it still equals an earlier text of ours;
// a text the boss edited is kept and named in the report. See docs/engineering/sdlc-specialist-routing.md.
//
//   node scripts/upgrade-sdlc-squad.mjs [--template sdlc_greenfield|sdlc_maintenance] [--apply]
//
// Without --apply it writes nothing: it reports what would change and asks the server to check the
// workflow write (a dry run). It reads the server, workspace and token of `multica login` from
// ~/.multica/config.json (override with MULTICA_SERVER_URL, MULTICA_WORKSPACE_ID, MULTICA_TOKEN).
// It bundles packages/core/squads/upgrade.ts with the repo's esbuild.

import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir, homedir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const args = process.argv.slice(2);
const apply = args.includes("--apply");
const templateId = args.includes("--template") ? args[args.indexOf("--template") + 1] : "sdlc_greenfield";

const cfgPath = join(homedir(), ".multica", "config.json");
const cfg = existsSync(cfgPath) ? JSON.parse(readFileSync(cfgPath, "utf8")) : {};
const server = (process.env.MULTICA_SERVER_URL ?? cfg.server_url ?? "").replace(/\/$/, "");
const workspaceId = process.env.MULTICA_WORKSPACE_ID ?? cfg.workspace_id;
const token = process.env.MULTICA_TOKEN ?? cfg.token;
if (!server || !workspaceId || !token) {
  console.error("Missing server, workspace or token: run `multica login`, or set MULTICA_SERVER_URL, MULTICA_WORKSPACE_ID and MULTICA_TOKEN.");
  process.exit(1);
}

const esbuild = join(root, "node_modules/.bin/esbuild");
if (!existsSync(esbuild)) {
  console.error("esbuild not found: run `pnpm install`.");
  process.exit(1);
}
const tmp = mkdtempSync(join(tmpdir(), "upgrade-sdlc-"));
const entry = join(tmp, "entry.ts");
const bundle = join(tmp, "bundle.cjs");
try {
  const squads = join(root, "packages/core/squads");
  await import("node:fs").then((fs) =>
    fs.writeFileSync(
      entry,
      `import { SQUAD_TEMPLATES } from ${JSON.stringify(join(squads, "templates.ts"))};
import { upgradeSdlcSquad } from ${JSON.stringify(join(squads, "upgrade.ts"))};
export { SQUAD_TEMPLATES, upgradeSdlcSquad };`,
    ),
  );
  execFileSync(esbuild, [entry, "--bundle", "--platform=node", "--format=cjs", `--outfile=${bundle}`, "--log-level=error"], { stdio: "inherit" });
  const { SQUAD_TEMPLATES, upgradeSdlcSquad } = await import(bundle);
  const template = SQUAD_TEMPLATES.find((t) => t.id === templateId);
  if (!template) throw new Error(`unknown template ${templateId}`);

  const call = async (path, method = "GET", body) => {
    const res = await fetch(server + path, {
      method,
      headers: { Authorization: `Bearer ${token}`, "X-Workspace-ID": workspaceId, "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await res.text();
    const data = text ? JSON.parse(text) : null;
    if (!res.ok) throw Object.assign(new Error(data?.error ?? `${method} ${path}: ${res.status}`), { status: res.status, body: data });
    return data;
  };
  const unused = async () => {
    throw new Error("not used by the upgrade");
  };
  const client = {
    listAgents: () => call("/api/agents?include_archived=false"),
    updateAgent: (id, data) => call(`/api/agents/${id}`, "PUT", data),
    listSquads: () => call("/api/squads"),
    updateSquad: (id, data) => call(`/api/squads/${id}`, "PUT", data),
    listIssueWorkflows: () => call("/api/issue-workflows"),
    updateIssueWorkflow: (id, data) => call(`/api/issue-workflows/${id}`, "PATCH", { ...data, dry_run: false }),
    dryRunIssueWorkflowUpdate: (id, data) => call(`/api/issue-workflows/${id}`, "PATCH", { ...data, dry_run: true }),
    listSkills: () => call("/api/skills"),
    getSkill: (id) => call(`/api/skills/${id}`),
    createSkill: (data) => call("/api/skills", "POST", data),
    updateSkill: (id, data) => call(`/api/skills/${id}`, "PUT", data),
    listSquadSkills: (id) => call(`/api/squads/${id}/skills`),
    setSquadSkills: (id, skillIds) => call(`/api/squads/${id}/skills`, "PUT", { skill_ids: skillIds }),
    createAgent: unused,
    createSquad: unused,
    listCompanyFloors: unused,
    createCompanyFloor: unused,
    setCompanySeats: unused,
    updateSquadCharter: unused,
    createSquadRoutine: unused,
    addSquadMember: unused,
    createIssueWorkflow: unused,
    listAutopilots: unused,
    createAutopilot: unused,
    createAutopilotTrigger: unused,
  };

  const result = await upgradeSdlcSquad(client, template, { dryRun: !apply });
  console.log(apply ? "UPGRADE APPLIED" : "DRY RUN (nothing written; pass --apply to write)");
  console.log(JSON.stringify(result, null, 2));
  const failed = result.workflow.status === "failed" || result.squad.status === "failed" || result.agents.some((a) => a.status === "failed");
  process.exitCode = failed ? 1 : 0;
} finally {
  rmSync(tmp, { recursive: true, force: true });
}
