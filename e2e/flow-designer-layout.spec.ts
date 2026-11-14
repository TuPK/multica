import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { createTestApi, loginAsDefault } from "./helpers";
import type { TestApiClient } from "./fixtures";

// P21 of the flow engine plan (spec 11.4, 12.2 C15, 12.4 K13): the draft editor on a laptop and on a phone.
// It builds the research sample flow in the editor, checks it, simulates it, publishes it and makes it the
// squad's workflow, and watches that each step reaches the real API. No agent runs: the runtime is a row that
// only looks online, and the draft has nothing to run.
//
// Needs a backend with FF_FLOW_AUTHORING and FF_FLOW_GRAPH_V2 on, a web app pointing at it, and the E2E
// database variables, like the other specs. Set FLOW_DESIGNER_SHOTS to a directory to keep screenshots.

const SHOTS = process.env.FLOW_DESIGNER_SHOTS;
const API_BASE = process.env.NEXT_PUBLIC_API_URL || `http://localhost:${process.env.PORT || "8080"}`;

const ROLES = ["scout", "screener", "decider", "synthesizer", "auditor", "reviewer_a", "reviewer_b"];

async function shot(page: Page, name: string) {
  if (SHOTS) await page.screenshot({ path: resolve(SHOTS, `${name}.png`), fullPage: false });
}

/** The research sample of the flow engine, with six plain steps added so the document has fourteen steps. */
function researchManifest(runId: string) {
  const doc = JSON.parse(
    readFileSync(resolve(process.cwd(), "server/internal/issueworkflow/testdata/flows/research.json"), "utf8"),
  );
  const extras = Array.from({ length: 6 }, (_, i) => ({
    key: `extra_${i + 1}`,
    title: `Extra ${i + 1}`,
    kind: "tasks",
    // A flow may show at most 12 statuses (S06): the six extra steps share one that every workspace has.
    status: { key: "in_progress" },
    tasks: [{ key: `extra_${i + 1}_work`, title: `Extra work ${i + 1}`, kind: "work", role: "scout", instructions: "Do the extra work." }],
    edges: [{ id: `extra_${i + 1}_next`, label: "Next", to: i === 5 ? "decide" : `extra_${i + 2}` }],
  }));
  // The extras run between screening and the decision.
  for (const step of doc.steps) {
    if (step.key === "screen") {
      for (const edge of step.edges) if (edge.to === "decide") edge.to = "extra_1";
    }
  }
  doc.steps.splice(3, 0, ...extras);
  expect(doc.steps).toHaveLength(14);
  // Every step shows a status of its own, so the board of the workspace has to know them: a name makes it.
  for (const step of doc.steps) {
    if (step.status?.name) step.status.name = `${step.status.name} ${runId}`.slice(0, 40);
  }
  return { format: "multica.flow-manifest/v1", name: "Research flow", doc };
}

async function noHorizontalScroll(page: Page, where: string) {
  const sizes = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
    bodyScrollWidth: document.body.scrollWidth,
  }));
  expect(sizes.scrollWidth, `${where}: the page scrolls sideways (${JSON.stringify(sizes)})`).toBeLessThanOrEqual(sizes.clientWidth);
  expect(sizes.bodyScrollWidth, `${where}: the body scrolls sideways (${JSON.stringify(sizes)})`).toBeLessThanOrEqual(sizes.clientWidth);
}

async function addSquadAgent(api: TestApiClient, squadId: string, agentId: string) {
  const res = await fetch(`${API_BASE}/api/squads/${squadId}/members`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${api.getToken()}`, "X-Workspace-Slug": (api as unknown as { workspaceSlug: string }).workspaceSlug },
    body: JSON.stringify({ member_type: "agent", member_id: agentId }),
  });
  if (!res.ok) throw new Error(`add squad member failed: ${res.status} ${await res.text()}`);
}

/** A call to the API as the test user, for the setup and the checks the browser does not make. */
async function callApi<T = unknown>(api: TestApiClient, method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${api.getToken()}`, "X-Workspace-Slug": (api as unknown as { workspaceSlug: string }).workspaceSlug },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${method} ${path} failed: ${res.status} ${await res.text()}`);
  return (res.status === 204 ? undefined : await res.json()) as T;
}

/** A second flow of the squad, a lineage of its own, so that the create form has a workflow to choose. */
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

test.describe("Flow draft editor, laptop and phone", () => {
  let api: TestApiClient;
  let heartbeat: NodeJS.Timeout | undefined;

  test.beforeEach(async () => {
    api = await createTestApi();
  });

  test.afterEach(async () => {
    if (heartbeat) clearInterval(heartbeat);
    await api?.cleanup();
  });

  test("builds, checks, simulates, publishes and activates the research flow without sideways scroll", async ({ page }) => {
    test.setTimeout(300_000);
    const runId = Date.now().toString(36);

    // A runtime that only looks online, and the squad with the agents of the research roles.
    const runtimeId = await api.seedRuntime(`Flow drill runtime ${runId}`);
    heartbeat = setInterval(() => {
      void api.queryDb(`UPDATE agent_runtime SET last_seen_at = now() WHERE id = $1`, [runtimeId]).catch(() => undefined);
    }, 15_000);
    const lead = await api.createAgent(`Research lead ${runId}`, runtimeId);
    const squad = await api.createSquad(`Research squad ${runId}`, lead.id, "Flow editor drill");
    const agents: { id: string; name: string }[] = [];
    for (const role of ROLES) {
      const agent = await api.createAgent(`${role} ${runId}`, runtimeId);
      await addSquadAgent(api, squad.id, agent.id);
      agents.push(agent);
    }

    const requests: string[] = [];
    page.on("request", (request) => {
      const url = new URL(request.url());
      if (url.pathname.startsWith("/api/")) requests.push(`${request.method()} ${url.pathname}`);
    });
    const called = (method: string, path: RegExp) => requests.some((r) => r.startsWith(`${method} `) && path.test(r.slice(method.length + 1)));

    // ------------------------------------------------------------------ 1280px
    await page.setViewportSize({ width: 1280, height: 800 });
    const slug = await loginAsDefault(page);
    await page.goto(`/${slug}/squads/${squad.id}`, { waitUntil: "domcontentloaded" });

    const newWorkflow = page.getByRole("button", { name: "New workflow" });
    await expect(newWorkflow).toBeVisible({ timeout: 30_000 });
    await shot(page, "01-squad-workflows-1280");
    await newWorkflow.click();
    await expect(page.getByRole("button", { name: /import a file/i })).toBeVisible();
    await shot(page, "02-start-dialog-1280");
    await page.getByRole("button", { name: /import a file/i }).click();

    const manifest = researchManifest(runId);
    await page.getByPlaceholder(/multica\.flow-manifest\/v1/).fill(JSON.stringify(manifest));
    await shot(page, "03-import-dialog-1280");
    await page.getByRole("button", { name: /^import$/i }).click();
    await page.waitForURL(new RegExp(`/squads/${squad.id}/workflows/[0-9a-f-]{36}`), { timeout: 30_000 });
    const draftId = page.url().split("/workflows/")[1]!.split(/[?#]/)[0]!;
    expect(called("POST", new RegExp(`^/api/squads/${squad.id}/flow-drafts$`))).toBe(true);

    // The editor loads the draft of this squad from the server.
    await expect(page.getByPlaceholder("Workflow name")).toHaveValue("Research flow", { timeout: 30_000 });
    expect(called("GET", new RegExp(`^/api/squads/${squad.id}/flow-drafts/${draftId}$`))).toBe(true);
    expect(called("GET", /^\/api\/flow\/capabilities$/)).toBe(true);
    await expect(page.getByRole("button", { name: /^Extra 6\b/ })).toBeVisible();
    await shot(page, "04-editor-14-steps-1280");
    await noHorizontalScroll(page, "editor at 1280px");

    // The graph.
    await page.getByRole("button", { name: "Graph", exact: true }).click();
    await shot(page, "05-graph-1280");
    await noHorizontalScroll(page, "graph at 1280px");

    // Every role of the research flow is given to an agent of the squad.
    await page.getByRole("button", { name: "Roles", exact: true }).click();
    for (const [index, role] of ROLES.entries()) {
      const row = page.locator("tr", { has: page.getByText(role, { exact: true }) }).first();
      await row.getByRole("checkbox", { name: agents[index]!.name }).check();
    }
    await shot(page, "06-roles-bound-1280");
    await noHorizontalScroll(page, "roles at 1280px");

    // Check: the document in the editor goes to the server.
    const validated = page.waitForResponse((r) => r.url().includes(`/flow-drafts/${draftId}/validate`) && r.request().method() === "POST");
    await page.getByRole("button", { name: "Check", exact: true }).click();
    const validation = await (await validated).json();
    expect(validation.issues.filter((i: { severity: string }) => i.severity === "error"), JSON.stringify(validation.issues)).toEqual([]);
    await shot(page, "07-check-1280");

    // Simulate: the document, and the results chosen, go to the server.
    await page.getByRole("button", { name: "Simulate", exact: true }).click();
    const simulated = page.waitForResponse((r) => r.url().includes(`/flow-drafts/${draftId}/simulate`) && r.request().method() === "POST");
    await page.getByRole("dialog").getByRole("button", { name: "Run", exact: true }).click();
    const trace = await (await simulated).json();
    expect(trace.hops.length).toBeGreaterThan(5);
    await expect(page.getByRole("dialog").getByText(trace.end.code ? `${trace.end.kind}: ${trace.end.code}` : trace.end.kind, { exact: true })).toBeVisible();
    await shot(page, "08-simulate-1280");
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).toBeHidden();

    // Publish, acknowledge what the server asks, and make the version the squad's.
    await page.getByRole("button", { name: "Publish", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("Checking…")).toBeHidden({ timeout: 30_000 });
    for (const box of await dialog.getByRole("checkbox").all()) {
      if (!(await box.isChecked()) && (await box.isEnabled())) await box.check();
    }
    await shot(page, "09-publish-dialog-1280");
    const published = page.waitForResponse((r) => r.url().endsWith(`/flow-drafts/${draftId}/publish`) && r.request().method() === "POST");
    const activated = page.waitForResponse((r) => r.url().endsWith(`/api/squads/${squad.id}/workflow`) && r.request().method() === "POST");
    await dialog.getByRole("button", { name: "Publish", exact: true }).click();
    const publishResponse = await published;
    expect(publishResponse.status(), await publishResponse.text()).toBe(201);
    const version = await publishResponse.json();
    expect(version.version).toBe(1);
    expect((await activated).status()).toBe(200);

    await page.waitForURL(new RegExp(`/squads/${squad.id}$`), { timeout: 30_000 });
    await expect(page.getByText(/^v1$/)).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText("Active", { exact: true })).toBeVisible();
    await shot(page, "10-published-active-1280");
    await noHorizontalScroll(page, "squad page at 1280px");

    // ------------------------------------------------------------------- 390px
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`/${slug}/squads/${squad.id}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByText(/^v1$/)).toBeVisible({ timeout: 30_000 });
    await shot(page, "11-squad-workflows-390");
    await noHorizontalScroll(page, "squad page at 390px");

    await page.goto(`/${slug}/squads/${squad.id}/workflows/${draftId}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /^Extra 6\b/ })).toBeVisible({ timeout: 30_000 });
    await shot(page, "12-editor-390");
    await noHorizontalScroll(page, "editor at 390px");

    // The inspector of a step opens as a sheet, never beside the outline.
    await page.getByRole("button", { name: /^Extra 3\b/ }).click();
    const sheet: Locator = page.getByRole("dialog");
    await expect(sheet).toBeVisible();
    await expect(sheet.getByPlaceholder("Step title")).toHaveValue("Extra 3");
    // The sheet slides in; the picture is taken once it has stopped.
    await expect(sheet).toHaveCSS("opacity", "1");
    await page.waitForTimeout(600);
    await shot(page, "13-inspector-sheet-390");
    await noHorizontalScroll(page, "inspector sheet at 390px");
    await page.keyboard.press("Escape");
    await expect(sheet).toBeHidden();

    for (const [tab, file] of [
      ["Graph", "14-graph-390"],
      ["Roles", "15-roles-390"],
      ["Variables", "16-vars-390"],
      ["Loops", "17-loops-390"],
      ["Stop reasons", "18-stops-390"],
      ["Terminals", "19-terminals-390"],
    ] as const) {
      await page.getByRole("button", { name: tab, exact: true }).click();
      await shot(page, file);
      await noHorizontalScroll(page, `${tab} at 390px`);
    }

    // ----------------------------------------------- trial runs and the workflow choice (spec 16.5)
    // A second flow makes the squad a place with a workflow to choose; it is published through the API.
    // A manifest carries no bindings, so the roles are bound on the draft before it is published.
    const second = await callApi<{ id: string }>(api, "POST", `/api/squads/${squad.id}/flow-drafts`, {
      from: { kind: "manifest", manifest: newsletterManifest() },
    });
    const secondDraft = await callApi<{ id: string; revision: number; doc: { roles: { key: string; bind?: unknown }[] } }>(
      api, "GET", `/api/squads/${squad.id}/flow-drafts/${second.id}`,
    );
    const binds: Record<string, string> = { writer: agents[0]!.id, checker: agents[1]!.id };
    const bound = await callApi<{ revision: number }>(api, "PATCH", `/api/squads/${squad.id}/flow-drafts/${second.id}`, {
      revision: secondDraft.revision,
      doc: { ...secondDraft.doc, roles: secondDraft.doc.roles.map((r) => ({ ...r, bind: { agent_ids: [binds[r.key]!] } })) },
    });
    const newsletter = await callApi<{ id: string; name: string }>(api, "POST", `/api/squads/${squad.id}/flow-drafts/${second.id}/publish`, {
      revision: bound.revision,
      ack: [],
    });

    // The workflow chooser on the create form: it shows for this squad, and the choice goes out with the create.
    await page.goto(`/${slug}/issues`, { waitUntil: "domcontentloaded" });
    // The create form opens in manual mode, the one with a title and an assignee. The helper of the other specs
    // waits for the sidebar, which a phone does not show, so the preference is set here.
    await page.evaluate(() => localStorage.setItem("multica_create_mode", JSON.stringify({ state: { lastMode: "manual" }, version: 0 })));
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.getByRole("button", { name: "New issue" }).first().click();
    const createDialog = page.getByRole("dialog");
    await expect(createDialog.getByRole("textbox", { name: "Issue title" })).toBeVisible({ timeout: 30_000 });
    await createDialog.getByRole("textbox", { name: "Issue title" }).fill(`Newsletter ${runId}`);
    await createDialog.getByRole("button", { name: /^assignee$|unassigned|assignee/i }).first().click();
    await page.getByRole("option", { name: new RegExp(`Research squad ${runId}`) }).or(page.getByText(`Research squad ${runId}`)).first().click();
    const chooser = createDialog.getByRole("button", { name: "Workflow", exact: true });
    await expect(chooser).toBeVisible();
    // The assignee list closes before the chooser can be used.
    await expect(createDialog.getByPlaceholder("Assign to...")).toBeHidden();
    await page.waitForTimeout(400);
    await chooser.click();
    await expect(page.getByRole("menuitemradio", { name: newsletter.name })).toBeVisible();
    await page.waitForTimeout(400);
    await shot(page, "20-create-form-workflow-chooser-390");
    await page.getByRole("menuitemradio", { name: newsletter.name }).click();
    await expect(chooser).toContainText(newsletter.name);
    await noHorizontalScroll(page, "create form at 390px");
    const created = page.waitForResponse((r) => new URL(r.url()).pathname === "/api/issues" && r.request().method() === "POST");
    await createDialog.getByRole("button", { name: /^create issue$/i }).click();
    const createdResponse = await created;
    expect(createdResponse.status(), await createdResponse.text()).toBe(201);
    expect(JSON.parse(createdResponse.request().postData() ?? "{}")).toMatchObject({ flow_workflow_id: newsletter.id });
    const chosenIssue = (await createdResponse.json()) as { id: string };
    const chosenState = await callApi<{ workflow_id: string; trial: boolean }>(api, "GET", `/api/issues/${chosenIssue.id}/flow`);
    expect(chosenState).toMatchObject({ workflow_id: newsletter.id, trial: false });

    // A trial run of version 1, which is the version the squad runs, and of the newsletter flow, which is not.
    await page.goto(`/${slug}/squads/${squad.id}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByText(/^v1$/).first()).toBeVisible({ timeout: 30_000 });
    const trialButtons = page.getByRole("button", { name: "Trial run" });
    await expect(trialButtons).toHaveCount(2);
    await noHorizontalScroll(page, "squad page with trial buttons at 390px");
    await trialButtons.first().click();
    const trialDialog = page.getByRole("dialog");
    await trialDialog.getByLabel("Title of the sample request").fill(`Trial ${runId}`);
    const trialCreated = page.waitForResponse((r) => new URL(r.url()).pathname === "/api/issues" && r.request().method() === "POST");
    await trialDialog.getByRole("button", { name: "Start", exact: true }).click();
    const trialResponse = await trialCreated;
    expect(trialResponse.status(), await trialResponse.text()).toBe(201);
    expect(JSON.parse(trialResponse.request().postData() ?? "{}")).toMatchObject({ flow_trial: true, assignee_id: squad.id });
    const trialIssue = (await trialResponse.json()) as { id: string };
    await page.waitForURL(new RegExp(`/issues/${trialIssue.id}`), { timeout: 30_000 });
    const trialState = await callApi<{ workflow_id: string; trial: boolean }>(api, "GET", `/api/issues/${trialIssue.id}/flow`);
    expect(trialState.trial).toBe(true);
    // On a phone the properties, and the flow panel in them, are behind the right sidebar button. A sidebar opened
    // while the request still loads closes again, so the button is pressed until the panel stays.
    const badge = page.getByText("Trial", { exact: true }).first();
    await expect(page.getByText("Leave a comment...")).toBeVisible({ timeout: 30_000 });
    await expect(async () => {
      if (!(await badge.isVisible())) await page.getByRole("button", { name: "Toggle right sidebar" }).click();
      await expect(badge).toBeVisible({ timeout: 2_000 });
      await page.waitForTimeout(1_000);
      await expect(badge).toBeVisible({ timeout: 1_000 });
    }).toPass({ timeout: 60_000 });
    await expect(badge).toBeInViewport();
    await shot(page, "21-trial-badge-390");
    await noHorizontalScroll(page, "trial request at 390px");

    // ------------------------------------------- a step template, checked by the real validator (1280px)
    await page.setViewportSize({ width: 1280, height: 800 });
    const templateDraft = await callApi<{ id: string }>(api, "POST", `/api/squads/${squad.id}/flow-drafts`, {
      from: { kind: "manifest", manifest: newsletterManifest() },
      name: `Template drill ${runId}`.slice(0, 56),
    });
    await page.goto(`/${slug}/squads/${squad.id}/workflows/${templateDraft.id}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: "Add step" })).toBeVisible({ timeout: 60_000 });
    await page.getByRole("button", { name: "Add step" }).click();
    await page.getByRole("menuitem", { name: "Gate", exact: true }).click();
    await expect(page.getByPlaceholder("Step title")).toHaveValue("Gate");
    const checked = page.waitForResponse((r) => r.url().includes(`/flow-drafts/${templateDraft.id}/validate`) && r.request().method() === "POST");
    await page.getByRole("button", { name: "Check", exact: true }).click();
    const checkedBody = (await (await checked).json()) as { issues: { code: string; path: string; severity: string }[] };
    await shot(page, "22-gate-template-checked-1280");
    // The step is not linked to anything yet, which is the only error it has. Sharing the review status with the
    // step that already shows it is a warning (S07).
    const ofTheGate = checkedBody.issues.filter((i) => i.path.includes("gate_1"));
    const errors = ofTheGate.filter((i) => i.severity === "error").map((i) => i.code);
    expect(errors, JSON.stringify(ofTheGate)).toEqual(["step_unreachable"]);

    // The calls that carried the work.
    for (const [method, path] of [
      ["PATCH", new RegExp(`^/api/squads/${squad.id}/flow-drafts/${draftId}$`)],
      ["POST", new RegExp(`^/api/squads/${squad.id}/flow-drafts/${draftId}/validate$`)],
      ["POST", new RegExp(`^/api/squads/${squad.id}/flow-drafts/${draftId}/simulate$`)],
      ["POST", new RegExp(`^/api/squads/${squad.id}/flow-drafts/${draftId}/publish$`)],
      ["POST", new RegExp(`^/api/squads/${squad.id}/workflow$`)],
    ] as const) {
      expect(called(method, path), `${method} ${path} was never called; calls: ${requests.join(", ")}`).toBe(true);
    }
  });
});
