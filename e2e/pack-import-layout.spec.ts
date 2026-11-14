import { resolve } from "node:path";
import { expect, test, type Page } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

// P27 of the flow engine plan (spec 11.9, 16.1 to 16.4, 16.6): the pack import dialog, the proposal banner, the
// "Ask an agent to draft" entry and the badges of the numbers of a version, on a laptop and on a phone. No agent
// runs: the runtime is a row that only looks online. The numbers of the version are the one answer the test
// makes up (an intercepted response), because a version with real runs would need agents to do the work.
//
// Needs a backend with FF_FLOW_AUTHORING, FF_FLOW_GRAPH_V2 and FF_FLOW_PACKS on, a web app pointing at it, and the
// E2E database variables, like the other specs. Set PACK_IMPORT_SHOTS to a directory to keep screenshots.

const SHOTS = process.env.PACK_IMPORT_SHOTS;
const API_BASE = process.env.NEXT_PUBLIC_API_URL || `http://localhost:${process.env.PORT || "8080"}`;

async function shot(page: Page, name: string) {
  if (SHOTS) await page.screenshot({ path: resolve(SHOTS, `${name}.png`), fullPage: false });
}

async function noHorizontalScroll(page: Page, where: string) {
  const sizes = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
    bodyScrollWidth: document.body.scrollWidth,
  }));
  expect(sizes.scrollWidth, `${where}: the page scrolls sideways (${JSON.stringify(sizes)})`).toBeLessThanOrEqual(sizes.clientWidth);
  expect(sizes.bodyScrollWidth, `${where}: the body scrolls sideways (${JSON.stringify(sizes)})`).toBeLessThanOrEqual(sizes.clientWidth);
  // A dialog that holds more than it shows scrolls inside itself, never sideways.
  const dialogs = await page.getByRole("dialog").evaluateAll((nodes) =>
    nodes.map((n) => ({ scrollWidth: n.scrollWidth, clientWidth: n.clientWidth })),
  );
  for (const d of dialogs) expect(d.scrollWidth, `${where}: a dialog scrolls sideways (${JSON.stringify(d)})`).toBeLessThanOrEqual(d.clientWidth);
}

/** The element with this text that the page shows: the graph draws a list for phones and a canvas for laptops. */
function shown(page: Page, text: string) {
  return page.locator(`text="${text}" >> visible=true`).first();
}

function workspaceSlug(api: TestApiClient): string {
  return (api as unknown as { workspaceSlug: string }).workspaceSlug;
}

/** A call to the API as the test user, for the setup and the checks the browser does not make. */
async function callApi<T = unknown>(api: TestApiClient, method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${api.getToken()}`, "X-Workspace-Slug": workspaceSlug(api) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${method} ${path} failed: ${res.status} ${await res.text()}`);
  return (res.status === 204 ? undefined : await res.json()) as T;
}

async function addSquadAgent(api: TestApiClient, squadId: string, agentId: string) {
  await callApi(api, "POST", `/api/squads/${squadId}/members`, { member_type: "agent", member_id: agentId });
}

// ----------------------------------------------------------------------------------------------------- a zip

const CRC_TABLE = Array.from({ length: 256 }, (_, n) => {
  let c = n;
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});

function crc32(data: Buffer): number {
  let crc = 0xffffffff;
  for (const byte of data) crc = CRC_TABLE[(crc ^ byte) & 0xff]! ^ (crc >>> 8);
  return (crc ^ 0xffffffff) >>> 0;
}

/** A zip of stored (uncompressed) files, enough for the server to read a pack from. */
function zip(files: Record<string, string>): Buffer {
  const locals: Buffer[] = [];
  const centrals: Buffer[] = [];
  let offset = 0;
  for (const [name, text] of Object.entries(files)) {
    const nameBytes = Buffer.from(name);
    const data = Buffer.from(text);
    const crc = crc32(data);
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4);
    local.writeUInt32LE(crc, 14);
    local.writeUInt32LE(data.length, 18);
    local.writeUInt32LE(data.length, 22);
    local.writeUInt16LE(nameBytes.length, 26);
    locals.push(local, nameBytes, data);
    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0);
    central.writeUInt16LE(20, 4);
    central.writeUInt16LE(20, 6);
    central.writeUInt32LE(crc, 16);
    central.writeUInt32LE(data.length, 20);
    central.writeUInt32LE(data.length, 24);
    central.writeUInt16LE(nameBytes.length, 28);
    central.writeUInt32LE(offset, 42);
    centrals.push(central, nameBytes);
    offset += local.length + nameBytes.length + data.length;
  }
  const centralSize = centrals.reduce((n, b) => n + b.length, 0);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(Object.keys(files).length, 8);
  end.writeUInt16LE(Object.keys(files).length, 10);
  end.writeUInt32LE(centralSize, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, ...centrals, end]);
}

/** A pack of the agents-folder kind: two agents with settings the install ignores, and two skills. */
function writingPack(runId: string): Buffer {
  return zip({
    "agents/writer.md": `---\nname: Pack writer ${runId}\ndescription: Writes the first draft\nmodel: sonnet\ntools: Read, Write\nskills: style-${runId}\n---\nWrite the draft of the article in the house style.\n`,
    "agents/editor.md": `---\nname: Pack editor ${runId}\ndescription: Edits the draft\nmodel: opus\n---\nEdit the draft for length and tone.\n`,
    "skills/style/SKILL.md": `---\nname: style-${runId}\ndescription: The house style\n---\nUse short sentences.\n`,
    "skills/tone/SKILL.md": `---\nname: tone-${runId}\ndescription: The tone of voice\n---\nBe plain and kind.\n`,
  });
}

/** The newsletter flow of the layout spec: two roles, a gate and a stop. */
function newsletterManifest() {
  return {
    format: "multica.flow-manifest/v1",
    name: "Newsletter flow",
    doc: {
      version: 2,
      start: "plan",
      roles: [
        { key: "writer", kind: "agent" },
        { key: "checker", kind: "agent" },
      ],
      terminals: [{ key: "done", status: "done" }],
      policy: {},
      steps: [
        { key: "plan", title: "Plan", kind: "lead", status: { key: "todo" }, lead: { sets: [] }, edges: [{ id: "plan_ok", label: "Planned", to: "write" }] },
        {
          key: "write", title: "Write", kind: "tasks", status: { key: "in_progress" },
          tasks: [{ key: "write_it", title: "Write the issue", kind: "work", role: "writer" }],
          edges: [{ id: "write_next", label: "Next", to: "check" }],
        },
        {
          key: "check", title: "Check", kind: "tasks", status: { key: "in_review" },
          tasks: [{ key: "check_it", title: "Check the issue", kind: "gate", role: "checker" }],
          edges: [
            { id: "check_pass", label: "Passed", to: "done", when: { field: "join.result", op: "eq", value: "pass" } },
            { id: "check_stop", label: "Stopped", stop: "stalled" },
          ],
        },
      ],
    },
  };
}

test.describe("Pack import, proposals and insights, laptop and phone", () => {
  let api: TestApiClient;
  let heartbeat: NodeJS.Timeout | undefined;

  test.beforeEach(async () => {
    api = await createTestApi();
  });

  test.afterEach(async () => {
    if (heartbeat) clearInterval(heartbeat);
    await api?.cleanup();
  });

  test("imports a pack: source, preview, leader, install and result, without sideways scroll", async ({ page }) => {
    test.setTimeout(240_000);
    const runId = Date.now().toString(36);
    const runtimeId = await api.seedRuntime(`Pack drill runtime ${runId}`);
    heartbeat = setInterval(() => {
      void api.queryDb(`UPDATE agent_runtime SET last_seen_at = now() WHERE id = $1`, [runtimeId]).catch(() => undefined);
    }, 15_000);

    const requests: string[] = [];
    page.on("request", (request) => {
      const url = new URL(request.url());
      if (url.pathname.startsWith("/api/")) requests.push(`${request.method()} ${url.pathname}`);
    });

    // ------------------------------------------------------------------ 1280px
    await page.setViewportSize({ width: 1280, height: 800 });
    const slug = await loginAsDefault(page);
    await page.goto(`/${slug}/squads`, { waitUntil: "domcontentloaded" });

    const importButton = page.getByRole("button", { name: "Import a pack" });
    await expect(importButton).toBeVisible({ timeout: 30_000 });
    await importButton.click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("button", { name: "Read the pack" })).toBeVisible();
    await shot(page, "23-pack-import-source-1280");
    await noHorizontalScroll(page, "pack import source at 1280px");

    await dialog.locator('input[type="file"]').setInputFiles({ name: "writing-pack.zip", mimeType: "application/zip", buffer: writingPack(runId) });
    const previewed = page.waitForResponse((r) => new URL(r.url()).pathname === "/api/squads/packs/preview" && r.request().method() === "POST");
    await dialog.getByRole("button", { name: "Read the pack" }).click();
    const previewResponse = await previewed;
    expect(previewResponse.status(), await previewResponse.text()).toBe(200);

    // The preview: the agents, the skills, the ignored settings, and an install that says why it waits.
    await expect(dialog.getByRole("heading", { name: "Agents" })).toBeVisible();
    await expect(dialog.getByText(`Pack writer ${runId}`).first()).toBeVisible();
    await expect(dialog.getByRole("heading", { name: "Skills" })).toBeVisible();
    await expect(dialog.getByText(`style-${runId}`).first()).toBeVisible();
    await expect(dialog.getByRole("heading", { name: "Ignored settings" })).toBeVisible();
    const install = dialog.getByRole("button", { name: "Install", exact: true });
    await expect(install).toBeDisabled();
    await expect(dialog.getByText("Choose a leader to make the squad")).toBeVisible();
    await shot(page, "24-pack-preview-no-leader-1280");
    await noHorizontalScroll(page, "pack preview at 1280px");

    // A leader, a name for the squad, and the install is ready.
    await dialog.getByRole("radio", { name: `Pack writer ${runId}` }).check();
    await dialog.getByRole("textbox", { name: "Squad name" }).fill(`Writers ${runId}`);
    await expect(install).toBeEnabled();
    await shot(page, "25-pack-preview-ready-1280");

    // ------------------------------------------------------------------- 390px
    // The same dialog, narrowed: the rows stack and nothing scrolls sideways.
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(dialog.getByRole("heading", { name: "Ignored settings" })).toBeVisible();
    await shot(page, "26-pack-preview-390");
    await noHorizontalScroll(page, "pack preview at 390px");
    await install.scrollIntoViewIfNeeded();
    await shot(page, "27-pack-preview-install-390");
    await noHorizontalScroll(page, "pack preview install at 390px");

    const committed = page.waitForResponse((r) => new URL(r.url()).pathname === "/api/squads/packs/commit" && r.request().method() === "POST");
    await install.click();
    const commitResponse = await committed;
    expect(commitResponse.status(), await commitResponse.text()).toBe(201);
    const done = (await commitResponse.json()) as { squad_id: string; agents: { created: boolean }[]; skills: { created: boolean }[] };
    expect(done.squad_id).toBeTruthy();
    expect(done.agents.filter((a) => a.created)).toHaveLength(2);
    expect(done.skills.filter((s) => s.created)).toHaveLength(2);

    await expect(dialog.getByText("Installed", { exact: true })).toBeVisible();
    const design = dialog.getByRole("link", { name: "Design the workflow" });
    await expect(design).toBeVisible();
    await expect(design).toHaveAttribute("href", new RegExp(`/squads/${done.squad_id}$`));
    await shot(page, "28-pack-result-390");
    await noHorizontalScroll(page, "pack result at 390px");

    // The squad exists and the pack is a squad of the workspace.
    const squads = await callApi<{ id: string; name: string }[]>(api, "GET", "/api/squads");
    expect(squads.some((s) => s.id === done.squad_id && s.name === `Writers ${runId}`)).toBe(true);
    expect(requests.some((r) => r === "POST /api/squads/packs/preview")).toBe(true);
    expect(requests.some((r) => r === "POST /api/squads/packs/commit")).toBe(true);
  });

  test("asks an agent to draft, waits for it, offers the import, and shows the numbers of a version", async ({ page }) => {
    test.setTimeout(240_000);
    const runId = Date.now().toString(36);
    const runtimeId = await api.seedRuntime(`Proposal drill runtime ${runId}`);
    heartbeat = setInterval(() => {
      void api.queryDb(`UPDATE agent_runtime SET last_seen_at = now() WHERE id = $1`, [runtimeId]).catch(() => undefined);
    }, 15_000);
    const lead = await api.createAgent(`Draft lead ${runId}`, runtimeId);
    const squad = await api.createSquad(`Draft squad ${runId}`, lead.id, "Proposal drill");
    const writer = await api.createAgent(`Draft writer ${runId}`, runtimeId);
    const checker = await api.createAgent(`Draft checker ${runId}`, runtimeId);
    await addSquadAgent(api, squad.id, writer.id);
    await addSquadAgent(api, squad.id, checker.id);
    const designer = await api.createAgent(`Flow designer ${runId}`, runtimeId);

    // A published version to read numbers of: bind the roles of the manifest, publish, and fork a draft from it.
    const first = await callApi<{ id: string; revision: number; doc: { roles: { key: string }[] } }>(api, "POST", `/api/squads/${squad.id}/flow-drafts`, {
      from: { kind: "manifest", manifest: newsletterManifest() },
    });
    const binds: Record<string, string> = { writer: writer.id, checker: checker.id };
    const bound = await callApi<{ revision: number }>(api, "PATCH", `/api/squads/${squad.id}/flow-drafts/${first.id}`, {
      revision: first.revision,
      doc: { ...first.doc, roles: first.doc.roles.map((r) => ({ ...r, bind: { agent_ids: [binds[r.key]!] } })) },
    });
    const version = await callApi<{ id: string }>(api, "POST", `/api/squads/${squad.id}/flow-drafts/${first.id}/publish`, { revision: bound.revision, ack: [] });
    const fork = await callApi<{ id: string; base_workflow_id: string | null }>(api, "POST", `/api/squads/${squad.id}/flow-drafts`, {
      from: { kind: "workflow", workflow_id: version.id },
    });
    expect(fork.base_workflow_id).toBe(version.id);

    const requests: string[] = [];
    page.on("request", (request) => {
      const url = new URL(request.url());
      if (url.pathname.startsWith("/api/")) requests.push(`${request.method()} ${url.pathname}${url.search}`);
    });

    // ------------------------------------------------------------------ 1280px
    await page.setViewportSize({ width: 1280, height: 800 });
    const slug = await loginAsDefault(page);
    await page.goto(`/${slug}/squads/${squad.id}`, { waitUntil: "domcontentloaded" });
    const newWorkflow = page.getByRole("button", { name: "New workflow" });
    await expect(newWorkflow).toBeVisible({ timeout: 30_000 });
    await newWorkflow.click();
    await page.getByRole("button", { name: /ask an agent to draft/i }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("combobox", { name: "Agent" }).selectOption({ label: `Flow designer ${runId}` });
    await dialog.getByRole("textbox", { name: "What the workflow should do" }).fill("Write the weekly newsletter, then have it checked by a second agent.");
    await shot(page, "29-ask-agent-dialog-1280");
    await noHorizontalScroll(page, "ask an agent at 1280px");
    const created = page.waitForResponse((r) => new URL(r.url()).pathname === `/api/squads/${squad.id}/flow-drafts` && r.request().method() === "POST");
    await dialog.getByRole("button", { name: "Ask the agent" }).click();
    const createdResponse = await created;
    expect(createdResponse.status(), await createdResponse.text()).toBe(201);
    const proposal = (await createdResponse.json()) as { id: string; origin: string; source_ref: { issue_id: string; agent_id: string } };
    expect(proposal.origin).toBe("proposal");
    expect(proposal.source_ref.agent_id).toBe(designer.id);

    // The draft opens with the banner that waits for the agent, and a link to the issue it was given.
    await page.waitForURL(new RegExp(`/squads/${squad.id}/workflows/${proposal.id}`), { timeout: 30_000 });
    await expect(page.getByText("Waiting for the agent's proposal")).toBeVisible({ timeout: 30_000 });
    await expect(page.getByRole("link", { name: "Open the issue" })).toHaveAttribute("href", new RegExp(`/issues/${proposal.source_ref.issue_id}$`));
    await expect(page.getByRole("button", { name: "Import proposal" })).toBeHidden();
    await shot(page, "30-proposal-waiting-1280");
    await noHorizontalScroll(page, "proposal waiting at 1280px");

    // The agent attaches a flow.json to the issue it was given: the banner offers the import.
    await api.queryDb(
      `INSERT INTO attachment (workspace_id, issue_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
       SELECT workspace_id, id, 'agent', $2, 'flow.json', 'https://files.example.test/flow.json', 'application/json', 10 FROM issue WHERE id = $1`,
      [proposal.source_ref.issue_id, designer.id],
    );
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: "Import proposal" })).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText("Waiting for the agent's proposal")).toBeHidden();
    await shot(page, "31-proposal-import-1280");
    await noHorizontalScroll(page, "proposal import at 1280px");

    // The reference, folded beside the editor, opens and reads the instructions of the squad.
    await page.getByRole("button", { name: "Show reference" }).click();
    await expect(page.getByRole("heading", { name: "Agent instructions" })).toBeVisible();
    await shot(page, "32-reference-open-1280");
    await noHorizontalScroll(page, "reference at 1280px");

    // ------------------------------------------------------------------- 390px
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`/${slug}/squads/${squad.id}/workflows/${proposal.id}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: "Import proposal" })).toBeVisible({ timeout: 30_000 });
    await shot(page, "33-proposal-import-390");
    await noHorizontalScroll(page, "proposal import at 390px");

    // ------------------------------------------------------- the numbers of the version the draft comes from
    // The server's answer for a version nobody has run is empty, and the badges stay away.
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(`/${slug}/squads/${squad.id}/workflows/${fork.id}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByPlaceholder("Workflow name")).toBeVisible({ timeout: 30_000 });
    await page.getByRole("button", { name: "Graph", exact: true }).click();
    await expect.poll(() => requests.some((r) => r.startsWith(`GET /api/issue-workflows/${version.id}/flow-insights?include_trial=false`))).toBe(true);
    await expect(page.getByText(/^Entered/)).toHaveCount(0);

    // With runs behind the version the steps and edges carry their numbers. The page asks again after a reload.
    await page.route(`**/api/issue-workflows/${version.id}/flow-insights*`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          requests: { total: 42, open: 5, by_end: { "terminal:done": 30, stalled: 7 } },
          steps: [
            { key: "plan", entered: 42, median_minutes: null, p90_minutes: null },
            { key: "write", entered: 51, median_minutes: 38, p90_minutes: 190 },
            { key: "check", entered: 51, median_minutes: 12, p90_minutes: 45 },
          ],
          edges: [
            { step: "check", edge: "check_pass", taken: 37 },
            { step: "check", edge: "check_stop", taken: 14 },
          ],
          loops: [],
          stalls: [{ reason: "flow_agent_unavailable", count: 7 }],
          manual_closes: 0,
          truncated: false,
        }),
      }),
    );
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.getByRole("button", { name: "Graph", exact: true }).click();
    await expect(page.getByText("Numbers from 42 requests")).toBeVisible({ timeout: 30_000 });
    await expect(shown(page, "Entered 51 times")).toBeVisible();
    await expect(shown(page, "median 38 min, p90 190 min")).toBeVisible();
    await expect(shown(page, "Taken 14 times")).toBeVisible();
    await shot(page, "34-insights-graph-1280");
    await noHorizontalScroll(page, "insights graph at 1280px");

    await page.setViewportSize({ width: 390, height: 844 });
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.getByRole("button", { name: "Graph", exact: true }).click();
    await expect(shown(page, "Entered 51 times")).toBeVisible({ timeout: 30_000 });
    await shot(page, "35-insights-graph-390");
    await noHorizontalScroll(page, "insights graph at 390px");
  });
});
