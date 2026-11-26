import { createHmac } from "node:crypto";
import { expect, test, type Page } from "@playwright/test";
import { API_BASE, TestApiClient } from "../fixtures";

// Durable spec 17.2 scenario 2, the part that is effects and intents (F3, F4): an L1 agent asks to send an
// email through a connected app, the platform holds it for a person, the person edits the text and approves once.
//
// Setup this needs (see docs/specs/durable-platform/plan.md, Task B7):
//   1. the environment's env file has FF_DURABLE_EFFECTS=true, and the API has been restarted with it;
//   2. `node scripts/fake-composio.mjs up` has written COMPOSIO_BASE_URL (loopback) and COMPOSIO_STATE_SECRET
//      to the same env file. The fake stands in for Composio, so no real email service is called.
//
// The agent's run is seeded as a running task, and the gateway token is minted the way the server mints a run's
// token (connectedapps.SignTaskToken with DeriveTokenSecret). No model runs.

const RUN = Date.now().toString(36);
const EMAIL = `effects-e2e-${RUN}@multica.ai`;
const NAME = "Effects E2E";
const WORKSPACE_SLUG = `effects-e2e-${RUN}`;
const RECIPIENT = "customer@example.com";
const ACCOUNT_ID = `ca_effects_${RUN}`;
const COMPOSIO_USER_ID = `cu_effects_${RUN}`;
const GATEWAY_BASE = "multica-connected-apps-gateway-v1";

const SUBJECT_EDITED = `Quote ${RUN} for editing`;
const SUBJECT_REJECTED = `Quote ${RUN} for rejection`;
const BODY_DRAFT = "Hello, the quote is attached.";
const BODY_EDITED = "Hello, the quote is attached and valid for 14 days.";
const REJECT_REASON = `Wrong customer ${RUN}`;

let api: TestApiClient;
let workspaceId = "";
let slug = "";
let agentId = "";
let taskId = "";
let intentEdited = "";
let intentRejected = "";

function requireEnv(name: string): string {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(
      `${name} is not set. Start the fake Composio (node scripts/fake-composio.mjs up) and set FF_DURABLE_EFFECTS=true in the env file, then restart the API.`,
    );
  }
  return value;
}

/** The gateway token a run presents: the server's token format, signed with its derived key. */
function gatewayToken(run: string): string {
  const key = createHmac("sha256", requireEnv("COMPOSIO_STATE_SECRET")).update(GATEWAY_BASE).digest();
  const payload = Buffer.from(JSON.stringify({ t: run, e: Math.floor(Date.now() / 1000) + 3600 })).toString("base64url");
  const signature = createHmac("sha256", key).update(payload).digest("base64url");
  return `mca_${payload}.${signature}`;
}

/** The agent asks for an app tool through the gateway, as a run's CLI does. */
async function askToSend(subject: string, body: string): Promise<{ status: string; intentId: string }> {
  const res = await fetch(`${API_BASE}/api/apps/mcp`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${gatewayToken(taskId)}` },
    body: JSON.stringify({
      jsonrpc: "2.0",
      id: 1,
      method: "tools/call",
      params: {
        name: "apps_run_tool",
        arguments: {
          tool: "GMAIL_SEND_EMAIL",
          arguments: { user_id: "me", to: RECIPIENT, subject, body, _rationale: "the customer asked for the quote" },
        },
      },
    }),
  });
  expect(res.status).toBe(200);
  const answer = JSON.parse((await res.json()).result.content[0].text) as { status: string; intent_id?: string };
  return { status: answer.status, intentId: answer.intent_id ?? "" };
}

/** A call to the API as the signed-in person. Returns the status and the raw body. */
async function apiCall(method: string, path: string, body?: unknown): Promise<{ status: number; text: string }> {
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${api.getToken()}`,
      "X-Workspace-Slug": slug,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  return { status: res.status, text: await res.text() };
}

async function signIn(page: Page) {
  const token = api.getToken();
  if (!token) throw new Error("the e2e person has no token");
  await page.addInitScript((t) => {
    localStorage.setItem("multica_token", t);
    localStorage.setItem("multica:chat:isOpen", "false");
  }, token);
  // The Next.js dev indicator sits over the bottom-left corner and can catch clicks (see e2e/helpers.ts).
  await page.addStyleTag({ content: "nextjs-portal { display: none !important; }" });
}

function intentCard(page: Page, subject: string) {
  return page.getByRole("article", { name: subject, exact: true });
}

test.describe.serial("F3, F4: an email an agent wants to send waits for a person", () => {
  test.beforeAll(async () => {
    test.setTimeout(240000);
    const fake = new URL(requireEnv("COMPOSIO_BASE_URL"));
    if (!["127.0.0.1", "localhost"].includes(fake.hostname)) {
      throw new Error(`COMPOSIO_BASE_URL must point at the local fake, not ${fake.host}`);
    }
    expect(requireEnv("FF_DURABLE_EFFECTS"), "the env file turns durable_effects on").toBe("true");
    const health = await fetch(`${fake.origin}/health`);
    expect(health.ok, "the fake Composio answers").toBe(true);

    api = new TestApiClient();
    await api.login(EMAIL, NAME);
    const workspace = await api.ensureWorkspace(`Effects E2E ${RUN}`, WORKSPACE_SLUG);
    await api.markUserOnboarded();
    workspaceId = workspace.id;
    slug = workspace.slug;

    // The first request to a route compiles it in `next dev`, which can take longer than a test's navigation.
    const frontend = process.env.PLAYWRIGHT_BASE_URL ?? process.env.FRONTEND_ORIGIN ?? "http://localhost:3000";
    const warm = await fetch(new URL(`/${slug}/inbox`, frontend), { signal: AbortSignal.timeout(180000) });
    expect(warm.ok, "the inbox route compiles and answers").toBe(true);

    const runtimeId = await api.seedRuntime(`effects-runtime-${RUN}`);
    const agent = await api.createAgent(`Effects agent ${RUN}`, runtimeId);
    agentId = agent.id;

    // A Gmail account for the person, as a finished connect flow leaves it.
    await api.queryDb(
      `INSERT INTO user_composio_connection (user_id, toolkit_slug, auth_config_id, connected_account_id, composio_user_id)
       SELECT id, 'gmail', 'ac_effects_e2e', $2, $3 FROM "user" WHERE email = $1`,
      [EMAIL, ACCOUNT_ID, COMPOSIO_USER_ID],
    );
    const attached = await apiCall("PUT", `/api/agents/${agentId}/apps`, {
      apps: [{ toolkit_slug: "gmail", account_scope: "personal", level: "write" }],
    });
    expect(attached.status, attached.text).toBe(200);

    taskId = (await api.seedRunningTask(agentId)).id;
  });

  test.afterAll(async () => {
    if (workspaceId) {
      await api.queryDb(`DELETE FROM effect_event WHERE workspace_id = $1`, [workspaceId]);
      await api.queryDb(`DELETE FROM effect_intent WHERE workspace_id = $1`, [workspaceId]);
    }
    if (taskId) {
      await api.queryDb(`DELETE FROM task_token WHERE task_id = $1`, [taskId]);
      await api.queryDb(`DELETE FROM agent_task_queue WHERE id = $1`, [taskId]);
    }
    await api.queryDb(`DELETE FROM user_composio_connection WHERE connected_account_id = $1`, [ACCOUNT_ID]);
    await api.cleanup();
  });

  test("the agent's send is held for a person, with the agent's reason kept apart", async () => {
    const held = await askToSend(SUBJECT_EDITED, BODY_DRAFT);
    expect(held.status).toBe("awaiting_approval");
    expect(held.intentId).not.toBe("");
    intentEdited = held.intentId;

    const [intent] = await api.queryDb(
      `SELECT state, effect_class, reversibility, source, rationale FROM effect_intent WHERE id = $1`,
      [intentEdited],
    );
    expect(intent).toMatchObject({
      state: "pending",
      effect_class: "send",
      reversibility: "irreversible",
      source: "connected_app",
      rationale: "the customer asked for the quote",
    });

    const events = await api.queryDb(`SELECT decision, status FROM effect_event WHERE intent_id = $1`, [intentEdited]);
    expect(events).toEqual([expect.objectContaining({ decision: "rehearse", status: "awaiting" })]);
  });

  test("a person edits the email in the inbox and approves it once", async ({ page }) => {
    await signIn(page);
    await page.goto(`/${slug}/inbox`);

    const card = intentCard(page, SUBJECT_EDITED);
    await expect(card).toBeVisible({ timeout: 30000 });
    await expect(card).toContainText(RECIPIENT);
    await expect(card).toContainText(BODY_DRAFT);

    await card.getByRole("button", { name: "Edit and approve", exact: true }).click();
    await card.getByLabel("Message text").fill(BODY_EDITED);
    await card.getByRole("button", { name: "Approve edited version", exact: true }).click();

    await expect(page.getByText("Approved. The action runs once.")).toBeVisible();
    await expect(intentCard(page, SUBJECT_EDITED)).toHaveCount(0);

    const [row] = await api.queryDb(
      `SELECT state, call->'args'->>'body' AS body FROM effect_intent WHERE id = $1`,
      [intentEdited],
    );
    expect(row).toMatchObject({ state: "executed", body: BODY_EDITED });
  });

  test("a second approval is refused, and the ledger holds one send with no undo", async () => {
    const again = await apiCall("POST", `/api/intents/${intentEdited}/approve`, {});
    expect(again.status).toBe(409);
    expect(again.text).toContain("intent_not_pending");

    const events = await api.queryDb(
      `SELECT status, undo, undo_until FROM effect_event WHERE intent_id = $1`,
      [intentEdited],
    );
    const executed = events.filter((event) => event.status === "executed");
    expect(executed).toHaveLength(1);
    expect(executed[0]).toMatchObject({ undo: null, undo_until: null });
  });

  test("a person rejects a second send with a reason, and nothing is sent", async ({ page }) => {
    const held = await askToSend(SUBJECT_REJECTED, BODY_DRAFT);
    expect(held.status).toBe("awaiting_approval");
    intentRejected = held.intentId;

    await signIn(page);
    await page.goto(`/${slug}/inbox`);

    const card = intentCard(page, SUBJECT_REJECTED);
    await expect(card).toBeVisible({ timeout: 30000 });
    await card.getByRole("button", { name: "Reject", exact: true }).click();
    await card.getByLabel("Reason (the agent reads it)").fill(REJECT_REASON);
    await card.getByRole("button", { name: "Send rejection", exact: true }).click();

    await expect(page.getByText("Rejected. The agent gets the reason.")).toBeVisible();

    const [row] = await api.queryDb(`SELECT state, decision_note FROM effect_intent WHERE id = $1`, [intentRejected]);
    expect(row).toMatchObject({ state: "rejected", decision_note: REJECT_REASON });

    const executed = await api.queryDb(
      `SELECT 1 FROM effect_event WHERE intent_id = $1 AND status = 'executed'`,
      [intentRejected],
    );
    expect(executed).toHaveLength(0);
  });
});
