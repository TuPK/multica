/**
 * Accounts screen: the status label of each account request state, cancelling a
 * pending request, and the resend-welcome action on a completed request. The
 * requests are seeded in SQL (setup); every action goes through the screen.
 * Flow numbers refer to docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import pg from "pg";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, testDatabaseUrl, type StaffSeed } from "../harness";
import { cleanupRows, saveSignedInState, seedAccount, seedWorkspace, useSignedIn } from "./session";

type RequestState = "pending" | "provisioned" | "completed" | "canceled" | "failed";

let seed: StaffSeed;
let admin: APIRequestContext;
const requestIds: string[] = [];
const accountUserIds: string[] = [];
const workspaceIds: string[] = [];

test.beforeAll(async ({ browser }) => {
  test.setTimeout(300_000);
  seed = await startPlatform("admin");
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
  await saveSignedInState(browser, seed, "accounts");
});

test.afterAll(async () => {
  test.setTimeout(300_000);
  await admin?.dispose();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    if (requestIds.length > 0) {
      await pool.query("DELETE FROM platform_account_request WHERE id = ANY($1)", [requestIds]);
    }
  } finally {
    await pool.end();
  }
  await cleanupRows(workspaceIds, accountUserIds);
  await stopPlatform(seed ?? null);
});

test.beforeEach(async ({ page }) => {
  await useSignedIn(page, "accounts");
});

/** Inserts one account request in the given state. The email is unique per call, so the test finds its own row. */
async function seedRequest(
  state: RequestState,
  extra: { error_code?: string; welcome_sent_at?: Date | null; account?: { userId: string; workspaceId: string } } = {},
): Promise<{ id: string; email: string }> {
  const id = randomUUID();
  const email = `e2e-acct-${id.slice(0, 8)}@example.test`;
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    await pool.query(
      `INSERT INTO platform_account_request (id, kind, email, display_name, workspace_name, workspace_slug,
                                             plan_key, seats, interval, trial, comp_reason, state, error_code,
                                             attempts, created_by, created_at, updated_at, completed_at, welcome_sent_at,
                                             user_id, workspace_id)
       VALUES ($1, 'individual', $2, 'E2E Request Person', 'E2E Request Co', $3,
               'pro', 1, 'month', false, 'e2e seeded request', $4, $5,
               1, $6, now(), now(), $7, $8, $9, $10)`,
      [
        id,
        email,
        `e2e-req-${id.slice(0, 8)}`,
        state,
        extra.error_code ?? "",
        seed.userId,
        state === "completed" ? new Date() : null,
        extra.welcome_sent_at ?? null,
        extra.account?.userId ?? null,
        extra.account?.workspaceId ?? null,
      ],
    );
  } finally {
    await pool.end();
  }
  requestIds.push(id);
  return { id, email };
}

async function openAccounts(page: Page): Promise<void> {
  await page.goto(CONSOLE_BASE);
  await page.getByRole("navigation", { name: "Main Navigation" }).getByRole("button", { name: "Accounts" }).click();
  await expect(page.getByRole("button", { name: "Create account" })).toBeVisible();
}

function rowFor(page: Page, email: string) {
  return page.getByRole("row", { name: new RegExp(email) });
}

test.describe("account request states", () => {
  test("flow 24: each request state shows its own label in the accounts list", async ({ page }) => {
    const pending = await seedRequest("pending");
    const provisioned = await seedRequest("provisioned");
    const completed = await seedRequest("completed");
    const canceled = await seedRequest("canceled");
    const finalizeFailed = await seedRequest("failed", { error_code: "finalize_failed" });

    await openAccounts(page);
    await expect(rowFor(page, pending.email)).toContainText("Pending");
    await expect(rowFor(page, provisioned.email)).toContainText("Provisioned");
    await expect(rowFor(page, completed.email)).toContainText("Completed");
    await expect(rowFor(page, canceled.email)).toContainText("Canceled");
    await expect(rowFor(page, finalizeFailed.email)).toContainText("Finalize failed");
  });
});

test.describe("cancel a pending request", () => {
  test("flow 247: staff keeps a pending request, then cancels it and the row shows Canceled", async ({ page }) => {
    const req = await seedRequest("pending");
    await openAccounts(page);

    await rowFor(page, req.email).getByRole("button", { name: "Cancel", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Cancel account request" });
    await expect(dialog).toContainText(req.email);
    await dialog.getByRole("button", { name: "Keep request" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(rowFor(page, req.email)).toContainText("Pending");

    await rowFor(page, req.email).getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Confirm cancel" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(rowFor(page, req.email)).toContainText("Canceled");
  });

  test("flow 247: a request that is not pending has no Cancel action", async ({ page }) => {
    const req = await seedRequest("completed");
    await openAccounts(page);
    await expect(rowFor(page, req.email)).toContainText("Completed");
    await expect(rowFor(page, req.email).getByRole("button", { name: "Cancel", exact: true })).toHaveCount(0);
  });
});

test.describe("resend welcome, daily limit", () => {
  test("flow 250: a fourth welcome resend in one day is refused by the daily limit", async ({ page }) => {
    const account = await seedAccount("E2E Limit Person");
    const workspace = await seedWorkspace("E2E Limit Co");
    accountUserIds.push(account.id);
    workspaceIds.push(workspace.id);
    const req = await seedRequest("completed", { account: { userId: account.id, workspaceId: workspace.id } });
    await openAccounts(page);
    for (let i = 0; i < 3; i++) {
      await rowFor(page, req.email).getByRole("button", { name: "Resend welcome" }).click();
      await expect(page.getByRole("alert").filter({ hasText: "Failed to send welcome email. Please try again later." })).toBeVisible();
    }
    await rowFor(page, req.email).getByRole("button", { name: "Resend welcome" }).click();
    await expect(page.getByRole("alert").filter({ hasText: /rate limit reached/i })).toBeVisible();
  });
});

test.describe("create account limit", () => {
  test("flow 248: a create past the hourly cap is refused in the create dialog", async ({ page }) => {
    // Fill the actor's hourly budget through the API (setup), then submit one more request in the dialog.
    const origin = { Origin: CONSOLE_BASE, "Content-Type": "application/json" };
    const createBody = (email: string) => ({
      kind: "individual",
      email,
      display_name: "E2E Cap Person",
      workspace_name: "E2E Cap Co",
      workspace_slug: "",
      plan_key: "free",
      seats: 1,
      interval: "month",
      trial: false,
      comp_reason: "",
    });
    let limited = false;
    for (let i = 0; i < 60 && !limited; i++) {
      const email = `e2e-cap-${randomUUID().slice(0, 8)}@example.test`;
      const res = await admin.post(`${CONSOLE_BASE}/api/platform/accounts`, {
        headers: { ...origin, "Idempotency-Key": randomUUID() },
        data: createBody(email),
      });
      if (res.status() === 429) {
        limited = true;
      } else {
        expect(res.status()).toBeLessThan(300);
        const created = (await res.json()) as { id?: string };
        if (created.id) requestIds.push(created.id);
      }
    }
    expect(limited, "the hourly cap was never reached").toBe(true);

    await openAccounts(page);
    await page.getByRole("button", { name: "Create account" }).click();
    const dialog = page.getByRole("dialog", { name: "Create account" });
    await expect(dialog).toBeVisible();
    const email = `e2e-cap-ui-${randomUUID().slice(0, 8)}@example.test`;
    await dialog.locator("#account-plan-key").selectOption("free");
    await dialog.locator("#account-email").fill(email);
    await dialog.locator("#account-confirm-email").fill(email);
    await dialog.locator("#account-display-name").fill("E2E Cap Person");
    await dialog.locator("#account-workspace-name").fill("E2E Cap Co");
    await dialog.getByRole("button", { name: "Submit request" }).click();
    await expect(dialog.getByText(/rate limit reached/i)).toBeVisible();
    await expect(dialog).toBeVisible();
  });
});

test.describe("resend welcome, send failure", () => {
  test("flow 251: a welcome resend that the mail provider refuses shows the send-failed message", async ({ page }) => {
    // The harness has no working mail provider, so the send fails with a transport error.
    const account = await seedAccount("E2E Welcome Person");
    const workspace = await seedWorkspace("E2E Welcome Co");
    accountUserIds.push(account.id);
    workspaceIds.push(workspace.id);
    const req = await seedRequest("completed", { account: { userId: account.id, workspaceId: workspace.id } });
    await openAccounts(page);
    await rowFor(page, req.email).getByRole("button", { name: "Resend welcome" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "Failed to send welcome email. Please try again later." })).toBeVisible();
  });
});

test.describe("resend welcome", () => {
  test("flow 249: a pending request offers no Resend welcome action", async ({ page }) => {
    const req = await seedRequest("pending");
    await openAccounts(page);
    await expect(rowFor(page, req.email)).toContainText("Pending");
    await expect(rowFor(page, req.email).getByRole("button", { name: "Resend welcome" })).toHaveCount(0);
  });

  test("flow 249: a completed request with no account yet is refused a resend, and the reason is shown", async ({ page }) => {
    const req = await seedRequest("completed");
    await openAccounts(page);
    await rowFor(page, req.email).getByRole("button", { name: "Resend welcome" }).click();
    await expect(page.getByRole("alert").filter({ hasText: /only for a request that has an account/ })).toBeVisible();
  });
});
