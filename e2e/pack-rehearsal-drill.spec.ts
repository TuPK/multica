import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { expect, test, type Page } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

// P28 of the flow engine plan (spec 12.3, 16.6): the bundle of a shop, as the upstream folder holds it, from the
// import dialog to a published workflow, on a laptop and on a phone. The bundle is
// catalog/_sources/kitviet/upstream/ban-hang-online, read from disk and sent as a zip; its flow is the hand-written
// manifest the Go rehearsal (TestRehearsalPack_KitVietUpstreamRaw) attaches in place of an agent. No agent runs:
// the runtime is a row that only looks online.
//
// Needs a backend with FF_FLOW_AUTHORING, FF_FLOW_GRAPH_V2 and FF_FLOW_PACKS on, a web app pointing at it, and the
// E2E database variables, like the other specs. Set PACK_DRILL_SHOTS to a directory to keep screenshots.

const SHOTS = process.env.PACK_DRILL_SHOTS;
const API_BASE = process.env.NEXT_PUBLIC_API_URL || `http://localhost:${process.env.PORT || "8080"}`;
const BUNDLE_DIR = resolve(process.cwd(), "catalog/_sources/kitviet/upstream/ban-hang-online");
const MANIFEST_FILE = resolve(process.cwd(), "server/internal/handler/testdata/pack_rehearsal/ban_hang_flow.json");

const WRITER_AGENT = "soan-listing-san-pham";
const WRITER_SKILLS = ["mo-ta-san-pham-shop", "tieu-de-seo-san-tmdt", "tinh-gia-ban-san"];
/** The roles of the manifest, and the agent of the bundle each one is given to. */
const ROLE_AGENTS: Record<string, string> = {
  collector: "gom-du-lieu-shop",
  competitor: "tra-cuu-doi-thu-san",
  writer: WRITER_AGENT,
  words_gate: "soat-tu-cam-quang-cao-san",
  margin_gate: "soat-bien-loi-nhuan-shop",
};

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
  const dialogs = await page.getByRole("dialog").evaluateAll((nodes) =>
    nodes.map((n) => ({ scrollWidth: n.scrollWidth, clientWidth: n.clientWidth })),
  );
  for (const d of dialogs) expect(d.scrollWidth, `${where}: a dialog scrolls sideways (${JSON.stringify(d)})`).toBeLessThanOrEqual(d.clientWidth);
}

/** Every control of a dialog sits inside the area that scrolls it: a field that sticks out is cut at that edge. */
async function controlsInsideDialog(page: Page, where: string) {
  const controls = await page.getByRole("dialog").locator("select, input, textarea").evaluateAll((nodes) =>
    nodes.map((n) => {
      let area: Element | null = n.parentElement;
      while (area && !/(auto|scroll)/.test(getComputedStyle(area).overflowY)) area = area.parentElement;
      return { right: n.getBoundingClientRect().right, limit: area?.getBoundingClientRect().right ?? Infinity, name: n.getAttribute("aria-label") || n.tagName };
    }),
  );
  for (const c of controls) expect(c.right, `${where}: ${c.name} sticks out of its scroll area`).toBeLessThanOrEqual(c.limit);
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
function zip(files: Record<string, Buffer>): Buffer {
  const locals: Buffer[] = [];
  const centrals: Buffer[] = [];
  let offset = 0;
  for (const [name, data] of Object.entries(files)) {
    const nameBytes = Buffer.from(name);
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

/** Every file of a folder, by its path inside the folder. */
function folderFiles(dir: string): Record<string, Buffer> {
  const files: Record<string, Buffer> = {};
  const walk = (current: string) => {
    for (const entry of readdirSync(current).sort()) {
      const path = join(current, entry);
      if (statSync(path).isDirectory()) walk(path);
      else files[relative(dir, path).split("\\").join("/")] = readFileSync(path);
    }
  };
  walk(dir);
  return files;
}

interface CommitResult {
  squad_id: string;
  agents: { key: string; agent_id: string; created: boolean }[];
  skills: { name: string; skill_id: string; created: boolean }[];
}

test.describe("Pack rehearsal drill: the upstream bundle, import to published workflow", () => {
  let api: TestApiClient;
  let heartbeat: NodeJS.Timeout | undefined;
  let installed: CommitResult | undefined;

  test.beforeEach(async () => {
    api = await createTestApi();
  });

  test.afterEach(async () => {
    if (heartbeat) clearInterval(heartbeat);
    // The install made a squad, agents and skills the helper does not know: take them out, so a second run starts clean.
    if (installed) {
      await callApi(api, "DELETE", `/api/squads/${installed.squad_id}`).catch(() => undefined);
      for (const a of installed.agents) await callApi(api, "POST", `/api/agents/${a.agent_id}/archive`).catch(() => undefined);
      for (const s of installed.skills) if (s.created) await callApi(api, "DELETE", `/api/skills/${s.skill_id}`).catch(() => undefined);
      installed = undefined;
    }
    await api?.cleanup();
  });

  test("imports the shop bundle as it is, drafts its flow, checks, simulates and publishes", async ({ page }) => {
    test.setTimeout(300_000);
    const runId = Date.now().toString(36);
    const runtimeId = await api.seedRuntime(`Pack rehearsal runtime ${runId}`);
    heartbeat = setInterval(() => {
      void api.queryDb(`UPDATE agent_runtime SET last_seen_at = now() WHERE id = $1`, [runtimeId]).catch(() => undefined);
    }, 15_000);

    // ------------------------------------------------------------------ 1280px, the import
    await page.setViewportSize({ width: 1280, height: 800 });
    const slug = await loginAsDefault(page);
    await page.goto(`/${slug}/squads`, { waitUntil: "domcontentloaded" });
    const importButton = page.getByRole("button", { name: "Import a pack" });
    await expect(importButton).toBeVisible({ timeout: 30_000 });
    await importButton.click();
    const dialog = page.getByRole("dialog");

    await dialog.locator('input[type="file"]').setInputFiles({ name: "ban-hang-online.zip", mimeType: "application/zip", buffer: zip(folderFiles(BUNDLE_DIR)) });
    const previewed = page.waitForResponse((r) => new URL(r.url()).pathname === "/api/squads/packs/preview" && r.request().method() === "POST");
    await dialog.getByRole("button", { name: "Read the pack" }).click();
    const previewResponse = await previewed;
    expect(previewResponse.status(), await previewResponse.text()).toBe(200);
    const preview = (await previewResponse.json()) as { format: string; agents: unknown[]; skills: unknown[] };
    expect(preview.agents).toHaveLength(7);
    expect(preview.skills).toHaveLength(18);

    await expect(dialog.getByRole("heading", { name: "Agents" })).toBeVisible();
    await expect(dialog.getByText(WRITER_AGENT, { exact: true }).first()).toBeVisible();
    await expect(dialog.getByRole("heading", { name: "Skills" })).toBeVisible();
    await shot(page, "36-upstream-preview-1280");
    await noHorizontalScroll(page, "upstream preview at 1280px");

    // The bundle names no leader and no skill of its agents: the person chooses a general leader, and the skills of the writer.
    const install = dialog.getByRole("button", { name: "Install", exact: true });
    await expect(install).toBeDisabled();
    await dialog.getByRole("radio", { name: "Use a general leader" }).check();
    const writerRow = dialog.locator("li", { has: page.getByText(WRITER_AGENT, { exact: true }) }).first();
    for (const skill of WRITER_SKILLS) await writerRow.getByRole("checkbox", { name: skill }).check();
    await dialog.getByRole("textbox", { name: "Squad name" }).fill(`Ban hang online ${runId}`);
    await expect(install).toBeEnabled();
    await shot(page, "37-upstream-ready-1280");

    // ------------------------------------------------------------------- 390px, the same dialog narrowed
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(dialog.getByRole("heading", { name: "Agents" })).toBeVisible();
    await shot(page, "38-upstream-preview-390");
    await noHorizontalScroll(page, "upstream preview at 390px");
    await install.scrollIntoViewIfNeeded();
    await shot(page, "39-upstream-install-390");
    await noHorizontalScroll(page, "upstream install at 390px");

    const committed = page.waitForResponse((r) => new URL(r.url()).pathname === "/api/squads/packs/commit" && r.request().method() === "POST");
    await install.click();
    const commitResponse = await committed;
    expect(commitResponse.status(), await commitResponse.text()).toBe(201);
    const done = (await commitResponse.json()) as CommitResult;
    installed = done;
    expect(done.squad_id).toBeTruthy();
    // Seven agents of the bundle and the general leader the install makes.
    expect(done.agents.filter((a) => a.created)).toHaveLength(8);
    expect(done.skills.filter((s) => s.created)).toHaveLength(18);

    await expect(dialog.getByText("Installed", { exact: true })).toBeVisible();
    await shot(page, "40-upstream-result-390");
    await noHorizontalScroll(page, "upstream result at 390px");

    // ------------------------------------------------------------------ 1280px, the authoring
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(`/${slug}/squads/${done.squad_id}`, { waitUntil: "domcontentloaded" });
    const newWorkflow = page.getByRole("button", { name: "New workflow" });
    await expect(newWorkflow).toBeVisible({ timeout: 30_000 });
    await shot(page, "41-squad-no-workflow-1280");
    await noHorizontalScroll(page, "squad without a workflow at 1280px");
    await newWorkflow.click();
    await page.getByRole("button", { name: /import a file/i }).click();
    await page.getByPlaceholder(/multica\.flow-manifest\/v1/).fill(readFileSync(MANIFEST_FILE, "utf8"));
    await page.getByRole("button", { name: /^import$/i }).click();
    await page.waitForURL(new RegExp(`/squads/${done.squad_id}/workflows/[0-9a-f-]{36}`), { timeout: 30_000 });
    const draftId = page.url().split("/workflows/")[1]!.split(/[?#]/)[0]!;
    await expect(page.getByPlaceholder("Workflow name")).toHaveValue(/Dây chuyền lên sản phẩm mới/, { timeout: 30_000 });
    await shot(page, "42-draft-editor-1280");
    await noHorizontalScroll(page, "draft editor at 1280px");

    await page.getByRole("button", { name: "Graph", exact: true }).click();
    await shot(page, "43-draft-graph-1280");
    await noHorizontalScroll(page, "draft graph at 1280px");

    // The roles are given to the agents of the bundle.
    await page.getByRole("button", { name: "Roles", exact: true }).click();
    for (const [role, agent] of Object.entries(ROLE_AGENTS)) {
      const row = page.locator("tr", { has: page.getByText(role, { exact: true }) }).first();
      await row.getByRole("checkbox", { name: agent }).check();
    }
    await shot(page, "44-roles-bound-1280");
    await noHorizontalScroll(page, "roles at 1280px");

    const validated = page.waitForResponse((r) => r.url().includes(`/flow-drafts/${draftId}/validate`) && r.request().method() === "POST");
    await page.getByRole("button", { name: "Check", exact: true }).click();
    const validation = await (await validated).json();
    expect(validation.issues.filter((i: { severity: string }) => i.severity === "error"), JSON.stringify(validation.issues)).toEqual([]);
    await shot(page, "45-check-1280");

    await page.getByRole("button", { name: "Simulate", exact: true }).click();
    await expect(page.getByRole("dialog")).toHaveCSS("opacity", "1");
    await controlsInsideDialog(page, "simulate dialog at 1280px");
    const simulated = page.waitForResponse((r) => r.url().includes(`/flow-drafts/${draftId}/simulate`) && r.request().method() === "POST");
    await page.getByRole("dialog").getByRole("button", { name: "Run", exact: true }).click();
    const trace = await (await simulated).json();
    expect(trace.hops.length).toBeGreaterThan(3);
    await expect(page.getByRole("dialog").getByText(trace.end.code ? `${trace.end.kind}: ${trace.end.code}` : trace.end.kind, { exact: true })).toBeVisible();
    await shot(page, "46-simulate-1280");
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).toBeHidden();

    await page.getByRole("button", { name: "Publish", exact: true }).click();
    const publish = page.getByRole("dialog");
    await expect(publish.getByText("Checking…")).toBeHidden({ timeout: 30_000 });
    for (const box of await publish.getByRole("checkbox").all()) {
      if (!(await box.isChecked()) && (await box.isEnabled())) await box.check();
    }
    await shot(page, "47-publish-dialog-1280");
    const published = page.waitForResponse((r) => r.url().endsWith(`/flow-drafts/${draftId}/publish`) && r.request().method() === "POST");
    await publish.getByRole("button", { name: "Publish", exact: true }).click();
    const publishResponse = await published;
    expect(publishResponse.status(), await publishResponse.text()).toBe(201);
    await page.waitForURL(new RegExp(`/squads/${done.squad_id}$`), { timeout: 30_000 });
    await expect(page.getByText(/^v1$/)).toBeVisible({ timeout: 30_000 });
    await shot(page, "48-published-1280");
    await noHorizontalScroll(page, "squad page at 1280px");

    // ------------------------------------------------------------------- 390px, the authoring
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`/${slug}/squads/${done.squad_id}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByText(/^v1$/)).toBeVisible({ timeout: 30_000 });
    await shot(page, "49-published-390");
    await noHorizontalScroll(page, "squad page at 390px");

    await page.goto(`/${slug}/squads/${done.squad_id}/workflows/${draftId}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByPlaceholder("Workflow name")).toBeVisible({ timeout: 30_000 });
    await shot(page, "50-draft-editor-390");
    await noHorizontalScroll(page, "draft editor at 390px");

    await page.getByRole("button", { name: "Graph", exact: true }).click();
    await shot(page, "51-draft-graph-390");
    await noHorizontalScroll(page, "draft graph at 390px");

    // The version can be asked for the trial run.
    await page.goto(`/${slug}/squads/${done.squad_id}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: "Trial run" }).first()).toBeVisible({ timeout: 30_000 });
    await page.getByRole("button", { name: "Trial run" }).first().click();
    await expect(page.getByRole("dialog").getByRole("button", { name: "Start", exact: true })).toBeVisible();
    // The dialog slides in; the picture is taken once it has stopped.
    await expect(page.getByRole("dialog")).toHaveCSS("opacity", "1");
    await page.waitForTimeout(600);
    await shot(page, "52-trial-dialog-390");
    await noHorizontalScroll(page, "trial dialog at 390px");
  });
});
