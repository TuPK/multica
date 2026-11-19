/**
 * Welcome email resend that succeeds: the platform mails the user through the fake SMTP
 * server, answers 200, and records an account.welcome_resent audit event (flow 25).
 * The request is seeded as completed with an account, so the resend is allowed.
 * Flow numbers refer to docs/engineering/platform-management-flows.md.
 */
import { expect, request as playwrightRequest, test, type APIRequestContext } from "@playwright/test";
import { randomUUID } from "node:crypto";
import pg from "pg";
import { CONSOLE_BASE, signInApi, startPlatform, stopPlatform, testDatabaseUrl, type StaffSeed } from "./harness";
import { startFakeSmtp, type FakeSmtp } from "./fake-smtp";

const ORIGIN = { Origin: CONSOLE_BASE, "Content-Type": "application/json" };

let seed: StaffSeed;
let admin: APIRequestContext;
let mail: FakeSmtp;
const requestIds: string[] = [];
const userIds: string[] = [];
const workspaceIds: string[] = [];

test.beforeAll(async () => {
  test.setTimeout(300_000);
  mail = await startFakeSmtp();
  seed = await startPlatform("admin", { smtpPort: mail.port });
  admin = await playwrightRequest.newContext();
  await signInApi(admin, seed);
});

test.afterAll(async () => {
  test.setTimeout(300_000);
  await admin?.dispose();
  await mail?.close();
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    for (const id of requestIds) await pool.query("DELETE FROM platform_account_request WHERE id = $1", [id]);
    for (const id of requestIds) await pool.query("DELETE FROM audit_event WHERE target_id = $1", [id]);
    for (const id of workspaceIds) await pool.query("DELETE FROM workspace WHERE id = $1", [id]);
    for (const id of userIds) await pool.query(`DELETE FROM "user" WHERE id = $1`, [id]);
  } finally {
    await pool.end();
  }
  await stopPlatform(seed ?? null);
});

test("flow 25: a welcome resend that is sent answers 200 and records account.welcome_resent in the audit log", async () => {
  const id = randomUUID();
  const userId = randomUUID();
  const workspaceId = randomUUID();
  const email = `e2e-welcome-${id.slice(0, 8)}@example.test`;
  const pool = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    await pool.query(`INSERT INTO "user" (id, name, email, avatar_url, created_at, updated_at, timezone)
                      VALUES ($1, 'E2E Welcome', $2, '', now(), now(), 'UTC')`, [userId, email]);
    await pool.query("INSERT INTO workspace (id, name, slug) VALUES ($1, 'E2E Welcome Co', $2)", [workspaceId, `e2e-wel-${id.slice(0, 8)}`]);
    await pool.query(
      `INSERT INTO platform_account_request (id, kind, email, display_name, workspace_name, workspace_slug,
                                             plan_key, seats, interval, trial, comp_reason, state, error_code,
                                             attempts, created_by, created_at, updated_at, completed_at,
                                             user_id, workspace_id)
       VALUES ($1, 'individual', $2, 'E2E Welcome', 'E2E Welcome Co', $3, 'pro', 1, 'month', false,
               'e2e seeded request', 'completed', '', 1, $4, now(), now(), now(), $5, $6)`,
      [id, email, `e2e-wel-${id.slice(0, 8)}`, seed.userId, userId, workspaceId],
    );
  } finally {
    await pool.end();
  }
  requestIds.push(id);
  userIds.push(userId);
  workspaceIds.push(workspaceId);

  const before = mail.sent.length;
  const res = await admin.post(`${CONSOLE_BASE}/api/platform/accounts/${id}/resend-welcome`, { headers: ORIGIN });
  expect(res.status(), `resend answered: ${await res.text()}`).toBe(200);

  const sent = mail.sent.slice(before).filter((m) => m.to === email);
  expect(sent.length, "the welcome did not reach the user").toBe(1);

  const audit = new pg.Pool({ connectionString: testDatabaseUrl(), max: 1 });
  try {
    const row = await audit.query("SELECT count(*)::int AS n FROM audit_event WHERE target_id = $1 AND action = 'account.welcome_resent'", [id]);
    expect(row.rows[0].n).toBe(1);
  } finally {
    await audit.end();
  }
});
