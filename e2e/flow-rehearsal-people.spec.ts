import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, test, type Page } from "@playwright/test";
import type { TestApiClient } from "./fixtures";
import { asAgent, callApi, persona, rawApi, removeRun, shot, signIn, workspaceSlugOf } from "./flow-e2e-people";

// P28 of the flow engine plan (spec 12.3, 16, C9, C12) as an automated rehearsal with simulated people.
//
//  * OWNER and MEMBER are people. They act in a real browser, through the real pages: the owner imports a copy of
//    the Trade flow, binds its roles, checks, simulates and publishes it, and takes the last person step; the
//    member closes the task that is theirs.
//  * THE AGENTS ARE SIMULATED BY API CALLS, because no real agent runs in a test. Every call named `agent` below is
//    what an agent's CLI does with its own task token: a task is closed with a status and an outcome, a file is
//    attached (a row, as the Go rehearsals do), a comment is posted, and the lead completes the plan step and
//    advances the request.
//  * The request is created and started through the API as the owner (the web form of a new issue is covered by
//    other specs); its pages are then read and acted on in the browser.
//
// The steps are serial tests: each one is a verdict, and a failed step makes the later ones not run.
// Needs a backend with FF_FLOW_AUTHORING and FF_FLOW_GRAPH_V2 on and a web app pointing at it.

const RUN = Date.now().toString(36);
const OWNER_EMAIL = `p28-owner-${RUN}@multica.ai`;
const MEMBER_EMAIL = `p28-member-${RUN}@multica.ai`;
const WORKSPACE_SLUG = `p28-${RUN}`;

const AGENT_ROLES = ["docs_reader", "customs_research", "drafter", "docs_gate", "customs_gate", "handover_gate"] as const;

/** The Trade sample with one more step: a person (a member of the squad) signs before the release. */
function tradeCopyManifest() {
  const doc = JSON.parse(readFileSync(resolve(process.cwd(), "server/internal/issueworkflow/testdata/flows/trade.json"), "utf8"));
  doc.roles.push({ key: "signer", kind: "member" });
  for (const step of doc.steps) {
    for (const edge of step.edges ?? []) if (edge.id === "handover_ok") edge.to = "signoff";
  }
  const release = doc.steps.findIndex((s: { key: string }) => s.key === "release");
  doc.steps.splice(release, 0, {
    key: "signoff",
    title: "Sign-off",
    kind: "tasks",
    status: { name: "Signoff", category: "started" },
    tasks: [{ key: "sign", title: "Sign the cross-check", kind: "gate", role: "signer", instructions: "Check the cross-check and sign it." }],
    edges: [
      { id: "sign_ok", label: "Signed", to: "release", when: { field: "join.result", op: "eq", value: "pass" } },
      { id: "sign_stop1", label: "Missing grounds", when: { field: "join.result", op: "eq", value: "block" }, stop: "grounds_missing" },
      { id: "sign_stop", label: "Rejected", stop: "loop_exhausted" },
    ],
  });
  // Every step of the copy shows a status of its own; the names are made by this run.
  for (const step of doc.steps) if (step.status?.name) step.status.name = `${step.status.name} ${RUN}`.slice(0, 40);
  return { format: "multica.flow-manifest/v1", name: `Trade copy ${RUN}`, doc };
}

interface FlowTaskView {
  issue_id: string;
  identifier: string;
  task: string;
  round: number;
  status: string;
  outcome: string | null;
  closed_at: string | null;
  assignee_id: string | null;
  kind?: string;
}

interface FlowStateView {
  step: string;
  workflow_id: string;
  workflow_name: string;
  tasks: FlowTaskView[];
  trail?: { step: string; edge?: string; to?: string }[];
  closed?: boolean;
  stop: { code: string } | null;
  loops?: Record<string, number>;
  loop_max?: Record<string, number>;
  viewer?: { can_manage: boolean; closable_task_ids: string[] };
}

test.describe.serial("P28: the Trade copy, from the owner's draft to the release (agents simulated by API calls)", () => {
  let owner: TestApiClient | undefined;
  let member: TestApiClient | undefined;
  let heartbeat: NodeJS.Timeout | undefined;
  let slug = "";
  let squadId = "";
  let memberUserId = "";
  let leadAgent = "";
  const agentIds: Record<string, string> = {};
  const agentNames: Record<string, string> = {};
  /** Task token of each agent, by agent id. */
  const tokens: Record<string, { token: string; taskId: string; agentId: string }> = {};
  let draftId = "";
  let workflowId = "";
  let requestId = "";
  let requestIdentifier = "";
  const memberAttempt: { status?: number; code?: string; task?: string; statusAfter?: string } = {};
  const assumptions: { task: string; text: string }[] = [];

  const agentCall = (agentId: string, method: string, path: string, body?: unknown) => asAgent(owner!, tokens[agentId]!, method, path, body);
  const state = async (): Promise<FlowStateView> => callApi<FlowStateView>(owner!, "GET", `/api/issues/${requestId}/flow`);
  const attach = async (issueId: string, agentId: string) => {
    await owner!.queryDb(
      `INSERT INTO attachment (workspace_id, issue_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
       SELECT workspace_id, id, 'agent', $2, 'draft.md', 'https://files.example.test/draft.md', 'text/markdown', 12 FROM issue WHERE id = $1`,
      [issueId, agentId],
    );
  };
  /** The lead wakes, advances the request and reports where it is. */
  const leadAdvances = async (): Promise<FlowStateView> => {
    const r = await agentCall(leadAgent, "POST", `/api/issues/${requestId}/flow/advance`, {});
    expect(r.status, `lead advance: ${JSON.stringify(r.json)}`).toBeLessThan(300);
    return state();
  };
  /** An agent closes the open task of this key with a verdict (and a file, when the task asks for one). */
  const agentCloses = async (taskKey: string, outcome: string | undefined, opts: { file?: boolean; comment?: string } = {}) => {
    const st = await state();
    const task = st.tasks.find((t) => t.task === taskKey && t.closed_at === null);
    expect(task, `no open task ${taskKey}; tasks: ${JSON.stringify(st.tasks.map((t) => [t.task, t.round, t.status]))}`).toBeTruthy();
    const agentId = task!.assignee_id!;
    expect(tokens[agentId], `task ${taskKey} is not given to an agent of this run (${agentId})`).toBeTruthy();
    if (opts.file) await attach(task!.issue_id, agentId);
    if (opts.comment) {
      const c = await agentCall(agentId, "POST", `/api/issues/${task!.issue_id}/comments`, { content: opts.comment });
      expect(c.status, `agent comment: ${JSON.stringify(c.json)}`).toBeLessThan(300);
      assumptions.push({ task: task!.issue_id, text: opts.comment });
    }
    const body: Record<string, unknown> = { status: "done" };
    if (outcome) body.outcome = outcome;
    const r = await agentCall(agentId, "PUT", `/api/issues/${task!.issue_id}`, body);
    expect(r.status, `agent closes ${taskKey}: ${JSON.stringify(r.json)}`).toBe(200);
    return task!;
  };

  async function openRequestPage(page: Page, api: TestApiClient) {
    await signIn(page, api);
    await page.goto(`/${slug}/issues/${requestId}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByText(`Trade copy ${RUN}`).first()).toBeVisible({ timeout: 90_000 });
  }

  test.beforeAll(async () => {
    owner = await persona(OWNER_EMAIL, `P28 Owner ${RUN}`);
    slug = (await owner.ensureWorkspace(`P28 ${RUN}`, WORKSPACE_SLUG)).slug;
    member = await persona(MEMBER_EMAIL, `P28 Member ${RUN}`);
    await owner.addMemberByEmail(MEMBER_EMAIL, "member");
    await member.ensureWorkspace(`P28 ${RUN}`, WORKSPACE_SLUG);
    expect(workspaceSlugOf(member), "the member does not see the workspace").toBe(slug);
    memberUserId = (await callApi<{ id: string }>(member, "GET", "/api/me")).id;

    const runtimeId = await owner.seedRuntime(`P28 runtime ${RUN}`);
    heartbeat = setInterval(() => {
      void owner!.queryDb(`UPDATE agent_runtime SET last_seen_at = now() WHERE id = $1`, [runtimeId]).catch(() => undefined);
    }, 15_000);
    const lead = await owner.createAgent(`p28 lead ${RUN}`, runtimeId);
    leadAgent = lead.id;
    const squad = await owner.createSquad(`P28 squad ${RUN}`, lead.id, "P28 rehearsal with simulated people");
    squadId = squad.id;
    tokens[lead.id] = { ...(await ownerToken(owner, lead.id)), agentId: lead.id };
    for (const role of AGENT_ROLES) {
      const agent = await owner.createAgent(`p28 ${role} ${RUN}`, runtimeId);
      agentIds[role] = agent.id;
      agentNames[role] = agent.name;
      await callApi(owner, "POST", `/api/squads/${squadId}/members`, { member_type: "agent", member_id: agent.id });
      tokens[agent.id] = { ...(await ownerToken(owner, agent.id)), agentId: agent.id };
    }
    await callApi(owner, "POST", `/api/squads/${squadId}/members`, { member_type: "member", member_id: memberUserId });
  });

  test.afterAll(async () => {
    if (heartbeat) clearInterval(heartbeat);
    await removeRun(owner, [OWNER_EMAIL, MEMBER_EMAIL]);
  });

  test("step 1 (owner, browser): imports the Trade copy, binds the roles, Check shows no blocking problem", async ({ page }) => {
    test.setTimeout(300_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await signIn(page, owner!);
    await page.goto(`/${slug}/squads/${squadId}`, { waitUntil: "domcontentloaded" });
    const newWorkflow = page.getByRole("button", { name: "New workflow" });
    await expect(newWorkflow).toBeVisible({ timeout: 90_000 });
    await newWorkflow.click();
    await page.getByRole("button", { name: /import a file/i }).click();
    await page.getByPlaceholder(/multica\.flow-manifest\/v1/).fill(JSON.stringify(tradeCopyManifest()));
    await page.getByRole("button", { name: /^import$/i }).click();
    await page.waitForURL(new RegExp(`/squads/${squadId}/workflows/[0-9a-f-]{36}`), { timeout: 60_000 });
    draftId = page.url().split("/workflows/")[1]!.split(/[?#]/)[0]!;
    await expect(page.getByPlaceholder("Workflow name")).toHaveValue(`Trade copy ${RUN}`, { timeout: 60_000 });

    await page.getByRole("button", { name: "Roles", exact: true }).click();
    // The editor saves the draft by itself a moment after the last change: the bindings must have reached the server.
    const saved = page.waitForResponse((r) => r.url().endsWith(`/flow-drafts/${draftId}`) && r.request().method() === "PATCH" && (r.request().postData() ?? "").includes("member_ids"));
    for (const role of AGENT_ROLES) {
      const row = page.locator("tr", { has: page.getByText(role, { exact: true }) }).first();
      await row.getByRole("checkbox", { name: agentNames[role]!, exact: true }).check();
    }
    const signerRow = page.locator("tr", { has: page.getByText("signer", { exact: true }) }).first();
    await signerRow.getByRole("checkbox", { name: `P28 Member ${RUN}`, exact: true }).check();
    expect((await saved).status(), "the autosave of the bindings").toBe(200);
    await expect(page.getByText("Saved", { exact: true })).toBeVisible();
    await shot(page, "p28-1-roles-bound");

    const validated = page.waitForResponse((r) => r.url().includes(`/flow-drafts/${draftId}/validate`) && r.request().method() === "POST");
    await page.getByRole("button", { name: "Check", exact: true }).click();
    const response = await validated;
    expect(response.status()).toBe(200);
    const body = (await response.json()) as { ok: boolean; issues: { severity: string; code: string; path: string }[] };
    const errors = body.issues.filter((i) => i.severity === "error");
    expect(errors, `blocking problems after Check: ${JSON.stringify(errors)}`).toEqual([]);
    expect(body.ok, "Check says the draft is not ok").toBe(true);
    await shot(page, "p28-1-check");
  });

  test("step 2 (owner, browser): Simulate ends in loop_exhausted when a Trade gate rejects twice", async ({ page }) => {
    test.setTimeout(240_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await signIn(page, owner!);
    await page.goto(`/${slug}/squads/${squadId}/workflows/${draftId}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByPlaceholder("Workflow name")).toHaveValue(`Trade copy ${RUN}`, { timeout: 90_000 });
    await page.getByRole("button", { name: "Simulate", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toHaveCSS("opacity", "1");
    await dialog.locator("label").filter({ hasText: /Customs gate/ }).locator("select").selectOption("rejected");
    const simulated = page.waitForResponse((r) => r.url().includes(`/flow-drafts/${draftId}/simulate`) && r.request().method() === "POST");
    await dialog.getByRole("button", { name: "Run", exact: true }).click();
    const response = await simulated;
    expect(response.status()).toBe(200);
    const trace = (await response.json()) as { hops: { step: string; loops?: Record<string, number> }[]; end: { kind: string; code?: string } };
    expect(trace.end.kind).toBe("stop");
    expect(trace.end.code).toBe("loop_exhausted");
    expect(trace.hops.filter((h) => h.step === "gate").length, "the gate is entered three times: the first and two reworks").toBe(3);
    expect(trace.hops[trace.hops.length - 1]!.loops?.fix, "the loop `fix` is used twice").toBe(2);
    await expect(dialog.getByText("stop: loop_exhausted", { exact: true })).toBeVisible();
    await expect(dialog.getByText("fix: 2", { exact: true })).toBeVisible();
    await shot(page, "p28-2-simulate");
  });

  test("step 3 (owner, browser): Publish makes version 1 and the version is locked", async ({ page }) => {
    test.setTimeout(240_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await signIn(page, owner!);
    await page.goto(`/${slug}/squads/${squadId}/workflows/${draftId}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByPlaceholder("Workflow name")).toHaveValue(`Trade copy ${RUN}`, { timeout: 90_000 });
    await page.getByRole("button", { name: "Publish", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("Checking…")).toBeHidden({ timeout: 60_000 });
    for (const box of await dialog.getByRole("checkbox").all()) {
      if (!(await box.isChecked()) && (await box.isEnabled())) await box.check();
    }
    await shot(page, "p28-3-publish-dialog");
    const published = page.waitForResponse((r) => r.url().endsWith(`/flow-drafts/${draftId}/publish`) && r.request().method() === "POST");
    await dialog.getByRole("button", { name: "Publish", exact: true }).click();
    const response = await published;
    expect(response.status(), await response.text()).toBe(201);
    const version = (await response.json()) as { id: string; version: number };
    expect(version.version).toBe(1);
    workflowId = version.id;
    await page.waitForURL(new RegExp(`/squads/${squadId}$`), { timeout: 60_000 });
    await expect(page.getByText(/^v1$/)).toBeVisible({ timeout: 60_000 });

    // The published version does not change any more.
    const patch = await rawApi(owner!, "PATCH", `/api/issue-workflows/${workflowId}`, { name: `Renamed ${RUN}` });
    expect(patch.status, JSON.stringify(patch.json)).toBe(409);
    expect(patch.json?.code).toBe("workflow_locked");
  });

  test("step 4 (agents simulated by API): the request runs from the lead step to the member's task, with a rework", async ({ page }) => {
    test.setTimeout(300_000);
    const created = await callApi<{ id: string; identifier: string }>(owner!, "POST", "/api/issues", {
      title: `Cross-check ${RUN}`,
      description: "A request for the Trade copy.",
      assignee_type: "squad",
      assignee_id: squadId,
    });
    requestId = created.id;
    requestIdentifier = created.identifier;
    const started = await rawApi(owner!, "POST", `/api/issues/${requestId}/flow/start`, {});
    expect(started.status, JSON.stringify(started.json)).toBeLessThan(300);
    let st = await state();
    expect(st.workflow_id, "the request runs the published version 1").toBe(workflowId);
    expect(st.step).toBe("plan");

    // The lead completes the plan step: no customs lookup, no export.
    const planned = await agentCall(leadAgent, "POST", `/api/issues/${requestId}/flow/steps/plan/complete`, {
      verdict: "planned", vars: { route: "shipment_dossier", customs_lookup: false, export: false },
    });
    expect(planned.status, JSON.stringify(planned.json)).toBeLessThan(300);
    st = await leadAdvances();
    expect(st.step).toBe("collect");

    await agentCloses("docs", undefined, { comment: "Assumption: the invoice total is read from the line items, not the cover page." });
    st = await leadAdvances();
    expect(st.step).toBe("draft");

    await agentCloses("crosscheck", undefined, { file: true });
    st = await leadAdvances();
    expect(st.step).toBe("gate");

    // A member who is not the assignee cannot close a gate task of an agent (checked in step 5).
    const foreign = st.tasks.find((t) => t.task === "docs_gate" && t.closed_at === null)!;
    const attempt = await rawApi(member!, "PUT", `/api/issues/${foreign.issue_id}`, { status: "done", outcome: "approved" });
    memberAttempt.status = attempt.status;
    memberAttempt.code = attempt.json?.code;
    memberAttempt.task = foreign.issue_id;
    memberAttempt.statusAfter = String((await owner!.queryDb(`SELECT status FROM issue WHERE id = $1`, [foreign.issue_id]))[0]?.status);

    // Round one: the customs gate rejects, the documents gate approves: the draft is reworked.
    await agentCloses("docs_gate", "approved", { comment: "Assumption: the stamp date is read as the arrival date." });
    await agentCloses("customs_gate", "rejected");
    st = await leadAdvances();
    expect(st.step, "a rejection sends the draft back").toBe("draft");
    expect(st.loops?.fix).toBe(1);

    // Round two: everything approves.
    await agentCloses("crosscheck", undefined, { file: true });
    st = await leadAdvances();
    expect(st.step).toBe("gate");
    await agentCloses("docs_gate", "approved");
    await agentCloses("customs_gate", "approved");
    st = await leadAdvances();
    expect(st.step).toBe("handover");
    await agentCloses("handover", "approved");
    st = await leadAdvances();
    expect(st.step, "the request waits for the member's sign-off").toBe("signoff");
    const sign = st.tasks.find((t) => t.task === "sign" && t.closed_at === null);
    expect(sign, "the member's task is open").toBeTruthy();
    expect(sign!.assignee_id, "the sign-off task is given to the member").toBe(memberUserId);

    // The owner sees it on the request page.
    await page.setViewportSize({ width: 1280, height: 800 });
    await openRequestPage(page, owner!);
    await expect(page.getByText("signoff", { exact: true }).first()).toBeVisible({ timeout: 60_000 });
    await shot(page, "p28-4-request-at-signoff");
  });

  test("step 5 (member, browser): cannot close a task that is not theirs, closes their own, the request moves on", async ({ page }) => {
    test.setTimeout(300_000);
    expect(memberAttempt.status, `the member's attempt on an agent task answered ${JSON.stringify(memberAttempt)}`).toBe(403);
    expect(memberAttempt.code).toBe("flow_task_not_assignee");
    expect(memberAttempt.statusAfter, "the refused attempt closed the agent's task all the same").not.toBe("done");

    await page.setViewportSize({ width: 1280, height: 800 });
    await openRequestPage(page, member!);
    await expect(page.getByRole("button", { name: "Approve", exact: true })).toBeVisible({ timeout: 60_000 });
    // Only the member's own task offers a verdict; the closed tasks of the agents do not.
    await expect(page.getByRole("button", { name: "Approve", exact: true })).toHaveCount(1);
    await expect(page.getByRole("button", { name: "Reject", exact: true })).toHaveCount(1);
    await shot(page, "p28-5-member-request");
    const closed = page.waitForResponse((r) => new URL(r.url()).pathname.startsWith("/api/issues/") && r.request().method() === "PUT");
    await page.getByRole("button", { name: "Approve", exact: true }).click();
    expect((await closed).status()).toBe(200);

    const st = await state();
    const sign = st.tasks.find((t) => t.task === "sign")!;
    expect(sign.closed_at, "the member's task is closed").not.toBeNull();
    expect(sign.outcome).toBe("approved");
    // The lead wakes on the closed task and advances; the request is at the manual release.
    const after = await leadAdvances();
    expect(after.step, "the request moved on to the release").toBe("release");
    expect(after.closed ?? false, "the flow is still open at the release").toBe(false);
  });

  test("step 6 (owner, browser): the release exit moves the request to done; status and trail", async ({ page }) => {
    test.setTimeout(240_000);
    await page.setViewportSize({ width: 1280, height: 800 });
    await openRequestPage(page, owner!);
    const release = page.getByRole("button", { name: "Release", exact: true });
    await expect(release, "the owner is offered the Release exit").toBeVisible({ timeout: 60_000 });
    // The loop counter is read against the bound of its loop: the gate sent the draft back once, of at most two.
    const open = await state();
    expect(open.loops?.fix, "the loop counter").toBe(1);
    expect(open.loop_max?.fix, "the bound of the loop fix").toBe(2);
    await expect(page.getByText("↺ 1/2"), "the request panel shows the counter against its bound").toBeVisible();
    await shot(page, "p28-6-before-release");
    const exited = page.waitForResponse((r) => r.url().includes(`/flow/exits/`) && r.request().method() === "POST");
    await release.click();
    expect((await exited).status()).toBe(200);

    const issue = await callApi<{ status: string }>(owner!, "GET", `/api/issues/${requestId}`);
    expect(issue.status, "the request is done").toBe("done");
    const st = await state();
    expect(st.closed, "the flow is closed").toBe(true);
    const steps = (st.trail ?? []).map((h) => h.step);
    expect(steps, `trail ${JSON.stringify(st.trail)}`).toEqual(["plan", "collect", "draft", "gate", "draft", "gate", "handover", "signoff", "release"]);
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByText("Closed", { exact: true }).first()).toBeVisible({ timeout: 60_000 });
    await shot(page, "p28-6-closed");
  });

  test("step 7 (agents simulated): the assumptions the agents recorded are in the comments", async ({ page }) => {
    test.setTimeout(180_000);
    expect(assumptions.length, "the run recorded no assumption").toBe(2);
    for (const a of assumptions) {
      const comments = await callApi<{ content: string }[] | { comments: { content: string }[] }>(owner!, "GET", `/api/issues/${a.task}/comments`);
      const list = Array.isArray(comments) ? comments : comments.comments;
      expect(list.some((c) => c.content === a.text), `comment "${a.text}" not found on ${a.task}`).toBe(true);
    }
    // And a person can read one on the task's page.
    await page.setViewportSize({ width: 1280, height: 800 });
    await signIn(page, owner!);
    await page.goto(`/${slug}/issues/${assumptions[0]!.task}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByText(assumptions[0]!.text).first()).toBeVisible({ timeout: 90_000 });
    await shot(page, "p28-7-assumption");
    expect(requestIdentifier).not.toBe("");
  });
});

/** A running task of an agent and its token, as the daemon would give it. */
async function ownerToken(owner: TestApiClient, agentId: string): Promise<{ token: string; taskId: string }> {
  const t = await owner.seedRunningTask(agentId);
  return { token: t.token, taskId: t.id };
}
