/**
 * Browser-side helpers for the UI specs: sign in through the console once, keep
 * the session in a storage-state file, and seed the rows a screen shows.
 * The sign-in itself is a user action, so specs that test sign-in use the
 * form directly; the others reuse the saved session.
 */
import { resolve } from "node:path";
import { mkdirSync, readFileSync } from "node:fs";
import { expect, request as playwrightRequest, type Browser, type BrowserContext, type Page } from "@playwright/test";

type Cookies = Parameters<BrowserContext["addCookies"]>[0];
import { randomUUID } from "node:crypto";
import pg from "pg";
import {
  CONSOLE_BASE,
  devVerificationCode,
  freshTotp,
  sepaySignature,
  testDatabaseUrl,
  WEBHOOK_URL,
  type StaffSeed,
} from "../harness";

const STATE_DIR = resolve(process.cwd(), "test-results", "platform");

/** The storage-state file for a named signed-in session. Created by saveSignedInState. */
export function stateFile(name: string): string {
  mkdirSync(STATE_DIR, { recursive: true });
  return resolve(STATE_DIR, `ui-state-${name}.json`);
}

/** Signs in through the console form: email, email code, then the authenticator code. */
export async function signInThroughForm(page: Page, seed: StaffSeed): Promise<void> {
  await page.goto(CONSOLE_BASE);
  await page.locator("#email-input").fill(seed.email);
  await page.getByRole("button", { name: "Send sign-in code" }).click();
  await page.locator("#code-input").fill(devVerificationCode());
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.locator("#totp-input").fill(await freshTotp(seed));
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.getByText("Platform Overview").first().waitFor();
}

/** Signs in once through the form and saves the session cookies for later tests. */
export async function saveSignedInState(browser: Browser, seed: StaffSeed, name: string): Promise<string> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await signInThroughForm(page, seed);
  const path = stateFile(name);
  await context.storageState({ path });
  await context.close();
  return path;
}

/** Adds the saved session cookies to the page's context. Read in beforeEach, after beforeAll has written the file. */
export async function useSignedIn(page: Page, name: string): Promise<void> {
  const state = JSON.parse(readFileSync(stateFile(name), "utf8")) as { cookies: Cookies };
  await page.context().addCookies(state.cookies);
}

/** Inserts a Multica workspace the Tenants screen lists. The schema has no foreign keys, so none are needed. */
export async function seedWorkspace(name: string): Promise<{ id: string; slug: string }> {
  const id = randomUUID();
  const slug = `e2e-ws-${id.slice(0, 8)}`;
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    await pool.query("INSERT INTO workspace (id, name, slug) VALUES ($1, $2, $3)", [id, name, slug]);
  } finally {
    await pool.end();
  }
  return { id, slug };
}

/** Inserts a Multica account the Users screen lists. Returns its id and email. */
export async function seedAccount(name: string): Promise<{ id: string; email: string }> {
  const id = randomUUID();
  const email = `e2e-ui-${id.slice(0, 8)}@example.test`;
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    await pool.query(
      `INSERT INTO "user" (id, name, email, avatar_url, created_at, updated_at, timezone)
       VALUES ($1, $2, $3, '', now(), now(), 'UTC')`,
      [id, name, email],
    );
  } finally {
    await pool.end();
  }
  return { id, email };
}

/**
 * Posts a signed SePay transfer with no transfer code. The server queues it for manual review
 * (flow 57). The event id is recorded so the spec can delete the notice in afterAll.
 */
export async function queueReviewNotice(amount: number, eventIds: string[]): Promise<void> {
  const eventId = Number(`${Date.now()}${Math.floor(Math.random() * 1000)}`);
  eventIds.push(String(eventId));
  const body = JSON.stringify({
    id: eventId,
    content: `E2E review ${eventId}`,
    transactionDate: "2026-11-18 10:00:00",
    transferType: "in",
    transferAmount: amount,
  });
  const timestamp = String(Math.floor(Date.now() / 1000));
  const webhook = await playwrightRequest.newContext();
  try {
    const res = await webhook.post(WEBHOOK_URL, {
      data: body,
      headers: {
        "Content-Type": "application/json",
        "X-SePay-Timestamp": timestamp,
        "X-SePay-Signature": sepaySignature(body, timestamp),
      },
    });
    expect(res.status()).toBeLessThan(300);
  } finally {
    await webhook.dispose();
  }
}

/** Inserts a pending, unexpired VND subscription order for a workspace. Returns its id. */
export async function seedPendingOrder(workspaceId: string, amount: number): Promise<string> {
  const id = randomUUID();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    await pool.query(
      `INSERT INTO billing_order (id, workspace_id, kind, plan_key, interval, seats, amount, vat_amount, currency,
                                  status, provider, transfer_code, buyer, expires_at, created_at,
                                  period_start, period_end)
       VALUES ($1, $2, 'subscribe', 'pro', 'month', 1, $3, 0, 'VND',
               'pending', 'sepay', $4, '{}', now() + interval '7 days', now(),
               now(), now() + interval '1 month')`,
      [id, workspaceId, amount, `E2E${id.replace(/-/g, "").slice(0, 12).toUpperCase()}`],
    );
  } finally {
    await pool.end();
  }
  return id;
}

/** Removes the rows the UI specs created. Failures are reported, not hidden. */
export async function cleanupRows(workspaceIds: string[], userIds: string[]): Promise<void> {
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    for (const id of workspaceIds) {
      await pool.query("DELETE FROM member WHERE workspace_id = $1", [id]);
      await pool.query("DELETE FROM workspace WHERE id = $1", [id]);
    }
    for (const id of userIds) {
      await pool.query("DELETE FROM platform_staff WHERE user_id = $1", [id]);
      await pool.query("DELETE FROM platform_account WHERE user_id = $1", [id]);
      await pool.query("DELETE FROM user_mfa WHERE user_id = $1", [id]);
      await pool.query(`DELETE FROM "user" WHERE id = $1`, [id]);
    }
  } finally {
    await pool.end();
  }
}
